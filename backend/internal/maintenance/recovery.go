package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

// Tick resumes persisted work after restart. Unknown outcomes keep their original
// request and operation ID; exhausting retries never proves a source absent.
func (s *Service) Tick(ctx context.Context) error {
	if s.Resources == nil {
		return nil
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT id FROM m3_resource_operations WHERE state='PENDING' AND next_attempt<=? ORDER BY next_attempt,id LIMIT 20`, s.Now().UTC())
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var first error
	for _, id := range ids {
		if e = s.RunOperation(ctx, id); e != nil && first == nil {
			first = e
		}
	}
	return first
}

func (s *Service) RunOperation(ctx context.Context, id string) error {
	if s.Resources == nil {
		return fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	// Connection lock serializes workers across processes without holding a
	// business transaction or workorder row lock during the HTTP request.
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	lock := "m3-op:" + id
	var acquired int
	if e = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", lock).Scan(&acquired); e != nil {
		return e
	}
	if acquired != 1 {
		return nil
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn.ExecContext(cleanup, "DO RELEASE_LOCK(?)", lock)
	}()
	var request []byte
	var actor, key, trace, state, orderID string
	var attempts int
	e = conn.QueryRowContext(ctx, "SELECT request,actor_id,idem_key,correlation_id,state,workorder_id,attempts FROM m3_resource_operations WHERE id=?", id).Scan(&request, &actor, &key, &trace, &state, &orderID, &attempts)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if state != "PENDING" {
		return nil
	}
	var q ResourceRequest
	if e = json.Unmarshal(request, &q); e != nil {
		return e
	}
	receipt, found, transportErr := s.Resources.Lookup(ctx, id)
	if transportErr == nil && !found {
		receipt, transportErr = s.Resources.Execute(ctx, q, trace)
	}
	// Lock in the same command -> order -> operation order used by Act.
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var saved []byte
	if e = tx.QueryRowContext(ctx, "SELECT response FROM m3_commands WHERE actor_id=? AND idem_key=? FOR UPDATE", actor, key).Scan(&saved); e != nil {
		return e
	}
	var raw []byte
	var seat, name string
	if e = tx.QueryRowContext(ctx, "SELECT snapshot,seat_label,assignee_name FROM m3_workorders WHERE id=? FOR UPDATE", orderID).Scan(&raw, &seat, &name); e != nil {
		return e
	}
	var order domain.Order
	if e = json.Unmarshal(raw, &order); e != nil {
		return e
	}
	if e = tx.QueryRowContext(ctx, "SELECT state FROM m3_resource_operations WHERE id=? FOR UPDATE", id).Scan(&state); e != nil {
		return e
	}
	if state != "PENDING" {
		return nil
	}
	if order.Operation == nil || order.Operation.ID != id || q.OperationID != id || q.SourceBusinessID != orderID || q.SourceVersion != order.Operation.SourceVersion || q.SourceModule != "M3" || q.RoomID != order.RoomID || q.Action != order.Operation.Action {
		return fmt.Errorf("inconsistent M3 persisted resource operation")
	}
	now := s.Now().UTC().Truncate(time.Microsecond)
	var change domain.Change
	if transportErr == nil {
		change, transportErr = order.ConfirmResource(receipt.domainReceipt(), now)
	}
	var receiptJSON any
	lastError := ""
	opState := "CONFIRMED"
	attempts++
	nextAttempt := now.Add(time.Duration(1<<min(attempts, 8)) * time.Second)
	if transportErr != nil {
		next, err := order.ResourceUnknown(now, attempts >= 8)
		if err != nil {
			return err
		}
		change = domain.Change{Order: next, Action: "RESOURCE_UNKNOWN", ActorID: "system", From: order.State, At: now}
		opState = "PENDING"
		if attempts >= 8 {
			opState = "PAUSED"
		}
		lastError = "DEPENDENCY_UNAVAILABLE"
	} else {
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		receiptJSON = encoded
		if receipt.Status == "FAILED" {
			opState = "FAILED"
			lastError = receipt.ErrorCode
		}
	}
	next := change.Order
	raw, e = json.Marshal(next)
	if e != nil {
		return e
	}
	result, e := tx.ExecContext(ctx, `UPDATE m3_workorders SET snapshot=?,state=?,version=?,updated_at=? WHERE id=? AND version=?`, raw, next.State, next.Version, now, orderID, order.Version)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return fault("VERSION_CONFLICT", 409)
	}
	if s.Events == nil {
		return fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	if e = s.Events.Append(ctx, tx, change, ID(), trace); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE m3_resource_operations SET state=?,attempts=?,next_attempt=?,last_error=?,receipt=?,updated_at=? WHERE id=?`, opState, attempts, nextAttempt, lastError, receiptJSON, now, id); e != nil {
		return e
	}
	responseOrder := dto(next, seat, name)
	if responseOrder.Operation != nil {
		responseOrder.Operation.State = opState
		responseOrder.Operation.Attempts = attempts
		responseOrder.Operation.LastError = lastError
		if opState == "PENDING" {
			responseOrder.Operation.NextAttempt = &nextAttempt
		}
	}
	response, e := json.Marshal(responseOrder)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE m3_commands SET response=? WHERE actor_id=? AND idem_key=?", response, actor, key); e != nil {
		return e
	}
	return tx.Commit()
}
