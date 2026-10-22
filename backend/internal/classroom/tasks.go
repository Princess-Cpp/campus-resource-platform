package classroom

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// RunOperation uses a connection-scoped lock, not a business transaction, during HTTP.
// Other processes can pick up persisted operations after disconnect/restart.
func (s *Service) RunOperation(ctx context.Context, operation string) error {
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	var locked int
	name := "m2:" + operation
	if e = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", name).Scan(&locked); e != nil {
		return e
	}
	if locked != 1 {
		return nil
	}
	defer conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", name)
	var raw []byte
	var application, target, actor, trace, state string
	var attempts int
	e = conn.QueryRowContext(ctx, "SELECT application_id,request,target_status,actor_id,correlation_id,state,attempts FROM m2_operations WHERE id=?", operation).Scan(&application, &raw, &target, &actor, &trace, &state, &attempts)
	if e != nil {
		return e
	}
	if state != "PENDING" {
		return nil
	}
	var q Operation
	if e = json.Unmarshal(raw, &q); e != nil {
		return e
	}
	receipt, found, e := s.Resources.Lookup(ctx, q.OperationID)
	if e == nil && !found {
		receipt, e = s.Resources.Execute(ctx, q, trace)
	}
	if e == nil && (receipt.OperationID != q.OperationID || receipt.SourceModule != "M2" || receipt.SourceBusinessID != q.SourceBusinessID || receipt.SourceVersion != q.SourceVersion || receipt.EffectiveAt.IsZero() || (receipt.OperationStatus != "CONFIRMED" && receipt.OperationStatus != "FAILED") || (receipt.OperationStatus == "CONFIRMED" && receipt.ErrorCode != "") || (receipt.OperationStatus == "FAILED" && receipt.ErrorCode == "")) {
		e = fmt.Errorf("invalid receipt")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	b, err := scan(tx.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE id=? FOR UPDATE", application))
	if err != nil {
		return err
	}
	if b.OperationID != operation {
		return fmt.Errorf("operation/application mismatch")
	}
	if e != nil {
		attempts++
		state = "PENDING"
		b.SyncStatus = "UNKNOWN"
		if attempts >= 8 {
			state = "PAUSED"
			b.SyncStatus = "FAILED"
		}
		b.LastError = "DEPENDENCY_UNAVAILABLE"
		b.Version++
		delay := time.Duration(1<<min(attempts, 8)) * time.Second
		if _, err = tx.ExecContext(ctx, "UPDATE m2_operations SET attempts=?,state=?,last_error=?,next_attempt=? WHERE id=?", attempts, state, b.LastError, s.Now().UTC().Add(delay), operation); err != nil {
			return err
		}
		if err = s.save(ctx, tx, b); err != nil {
			return err
		}
		return tx.Commit()
	}
	from := b.Status
	b.LastError = receipt.ErrorCode
	b.Version++
	if receipt.OperationStatus == "FAILED" {
		if q.Action == "HOLD_ROOM_INTERVAL" {
			b.SyncStatus = "IDLE"
			if !s.Now().Before(b.DeadlineAt) {
				b.Status = "EXPIRED"
			}
			state = "DONE"
		} else {
			b.SyncStatus = "FAILED"
			state = "PAUSED"
		}
	} else {
		state = "DONE"
		b.SyncStatus = "IDLE"
		b.Status = target
		if q.Action == "HOLD_ROOM_INTERVAL" {
			if !receipt.EffectiveAt.Before(b.DeadlineAt) {
				b.Status = from
				if err = s.queue(ctx, tx, &b, "system", "RELEASE_SOURCE", "EXPIRED", trace); err != nil {
					return err
				}
			} else {
				if _, err = tx.ExecContext(ctx, "UPDATE m2_applications SET decision='APPROVED',decided_at=? WHERE id=?", receipt.EffectiveAt.UTC(), b.ID); err != nil {
					return err
				}
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE m2_operations SET state=?,attempts=attempts+1,last_error=? WHERE id=?", state, receipt.ErrorCode, operation); err != nil {
		return err
	}
	if err = s.save(ctx, tx, b); err != nil {
		return err
	}
	if err = s.event(ctx, tx, b, actor, "RESOURCE_"+receipt.OperationStatus, from, receipt.ErrorCode, trace); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) Retry(ctx context.Context, a Actor, application string, version int64) (Application, error) {
	tx, e := s.begin(ctx)
	if e != nil {
		return Application{}, e
	}
	defer tx.Rollback()
	b, e := scan(tx.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE id=? FOR UPDATE", application))
	if e == sql.ErrNoRows {
		return b, fault("NOT_FOUND", 404)
	}
	if e != nil {
		return b, e
	}
	if !canRead(a, b) {
		return Application{}, fault("NOT_FOUND", 404)
	}
	if b.Version != version {
		return b, fault("VERSION_CONFLICT", 409)
	}
	if b.SyncStatus != "FAILED" && b.SyncStatus != "UNKNOWN" {
		return b, fault("INVALID_STATE", 409)
	}
	// Retry the persisted operation; never mint a new key for an unknown outcome.
	_, e = tx.ExecContext(ctx, "UPDATE m2_operations SET state='PENDING',attempts=0,next_attempt=? WHERE id=?", s.Now().UTC(), b.OperationID)
	if e != nil {
		return b, e
	}
	b.SyncStatus = "PENDING"
	b.Version++
	if e = s.save(ctx, tx, b); e != nil {
		return b, e
	}
	if e = s.event(ctx, tx, b, a.ID, "RETRY", b.Status, "", id()); e != nil {
		return b, e
	}
	return b, tx.Commit()
}
func (s *Service) Tick(ctx context.Context) error {
	rows, e := s.DB.QueryContext(ctx, "SELECT id FROM m2_operations WHERE state='PENDING' AND next_attempt<=? ORDER BY next_attempt,id LIMIT 50", s.Now().UTC())
	if e != nil {
		return e
	}
	operations := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			rows.Close()
			return e
		}
		operations = append(operations, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range operations {
		if e = s.RunOperation(ctx, v); e != nil {
			return e
		}
	}
	rows, e = s.DB.QueryContext(ctx, "SELECT id FROM m2_applications WHERE status IN ('SUBMITTED','DEPARTMENT_APPROVED') AND sync_status='IDLE' AND deadline_at<=? LIMIT 100", s.Now().UTC())
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range ids {
		if e = s.expire(ctx, v); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) expire(ctx context.Context, application string) error {
	tx, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	b, e := scan(tx.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE id=? FOR UPDATE", application))
	if e != nil {
		return e
	}
	if (b.Status != "SUBMITTED" && b.Status != "DEPARTMENT_APPROVED") || b.SyncStatus != "IDLE" || s.Now().Before(b.DeadlineAt) {
		return nil
	}
	from := b.Status
	b.Status = "EXPIRED"
	b.Version++
	if e = s.save(ctx, tx, b); e != nil {
		return e
	}
	if e = s.event(ctx, tx, b, "system", "EXPIRE", from, "审批期限已到", id()); e != nil {
		return e
	}
	return tx.Commit()
}
