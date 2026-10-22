package maintenance

import (
	"campus-resource-platform/backend/internal/maintenance/domain"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

func canAct(o domain.Order, a domain.Actor, action string) bool {
	switch action {
	case "ASSIGN":
		return a.Can("workorder.dispatch") && a.InArea(o.Area)
	case "ACCEPT_ASSIGNMENT":
		return a.Can("workorder.repair") && a.ID == o.AssigneeID
	case "CANCEL":
		return a.Enabled && a.ID == o.ReporterID
	}
	return false
}
func allowedActions(o domain.Order, a domain.Actor) []string {
	out := []string{}
	if o.Sync != domain.Idle {
		return out
	}
	if canAct(o, a, "ASSIGN") && (o.State == domain.Reported || o.State == domain.Assigned || o.State == domain.InProgress) {
		out = append(out, "ASSIGN")
	}
	if canAct(o, a, "ACCEPT_ASSIGNMENT") && (o.State == domain.Assigned || o.State == domain.InProgress) && o.AcceptedAt.IsZero() {
		out = append(out, "ACCEPT_ASSIGNMENT")
	}
	if canAct(o, a, "CANCEL") && (o.State == domain.Reported || o.State == domain.Assigned) {
		out = append(out, "CANCEL")
	}
	return out
}
func (s *Service) Assignees(ctx context.Context, a domain.Actor, id string) ([]Worker, error) {
	d, e := s.Get(ctx, a, id)
	if e != nil {
		return nil, e
	}
	if !a.Can("workorder.dispatch") || !a.InArea(d.Area) {
		return nil, fault("FORBIDDEN", 403)
	}
	return s.workersInArea(ctx, d.Area)
}
func (s *Service) workersInArea(ctx context.Context, area string) ([]Worker, error) {
	if s.People == nil {
		return nil, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	people, e := s.People.Workers(ctx)
	if e != nil {
		return nil, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	out := []Worker{}
	for _, p := range people {
		if p.ID != "" && p.ID == p.Actor.ID && p.Actor.Can("workorder.repair") && p.Actor.InArea(area) {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *Service) Act(ctx context.Context, a domain.Actor, id string, q ActionInput, key, trace string) (Order, error) {
	if !a.Enabled || a.ID == "" {
		return Order{}, fault("UNAUTHENTICATED", 401)
	}
	if !slices.Contains([]string{"ASSIGN", "ACCEPT_ASSIGNMENT", "CANCEL"}, q.Action) || q.ExpectedVersion < 1 || len(id) == 0 || len(id) > 64 || !keyPattern.MatchString(key) || len(q.AssigneeID) > 64 || (q.Action == "ASSIGN" && q.AssigneeID == "") || (q.Action != "ASSIGN" && q.AssigneeID != "") {
		return Order{}, fault("INVALID_INPUT", 400)
	}
	q.Comment = strings.TrimSpace(q.Comment)
	canonical, _ := json.Marshal(struct {
		Kind, ID string
		Input    ActionInput
	}{"WORKORDER_ACTION", id, q})
	digest := sha256.Sum256(canonical)
	hash := hex.EncodeToString(digest[:])
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return Order{}, e
	}
	defer tx.Rollback()
	now := s.Now().UTC().Truncate(time.Microsecond)
	if _, e = tx.ExecContext(ctx, `INSERT INTO m3_commands(actor_id,idem_key,request_hash,created_at) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE idem_key=idem_key`, a.ID, key, hash, now); e != nil {
		return Order{}, e
	}
	var storedHash string
	var saved []byte
	if e = tx.QueryRowContext(ctx, "SELECT request_hash,response FROM m3_commands WHERE actor_id=? AND idem_key=? FOR UPDATE", a.ID, key).Scan(&storedHash, &saved); e != nil {
		return Order{}, e
	}
	var raw []byte
	var seat, name string
	var order domain.Order
	e = tx.QueryRowContext(ctx, "SELECT snapshot,seat_label,assignee_name FROM m3_workorders WHERE id=? FOR UPDATE", id).Scan(&raw, &seat, &name)
	if errors.Is(e, sql.ErrNoRows) {
		return Order{}, fault("NOT_FOUND", 404)
	}
	if e != nil {
		return Order{}, e
	}
	if e = json.Unmarshal(raw, &order); e != nil {
		return Order{}, e
	}
	// Current authority is required even when the response is a historical replay.
	if !order.CanRead(a) {
		return Order{}, fault("NOT_FOUND", 404)
	}
	if !canAct(order, a, q.Action) {
		return Order{}, fault("FORBIDDEN", 403)
	}
	if storedHash != hash {
		return Order{}, fault("IDEMPOTENCY_CONFLICT", 409)
	}
	if len(saved) > 0 {
		var result Order
		if e = json.Unmarshal(saved, &result); e != nil {
			return Order{}, e
		}
		return result, tx.Commit()
	}
	if q.ExpectedVersion != order.Version {
		return Order{}, fault("VERSION_CONFLICT", 409)
	}
	var target domain.Actor
	if q.Action == "ASSIGN" {
		people, e := s.workersInArea(ctx, order.Area)
		if e != nil {
			return Order{}, e
		}
		for _, p := range people {
			if p.ID == q.AssigneeID {
				target = p.Actor
				name = p.Name
				break
			}
		}
		if target.ID == "" {
			return Order{}, fault("FORBIDDEN", 403)
		}
	}
	change, e := order.Apply(a, target, domain.Command{Action: q.Action, ExpectedVersion: q.ExpectedVersion, Comment: q.Comment}, now)
	if e != nil {
		status := 400
		switch {
		case errors.Is(e, domain.ErrForbidden):
			status = 403
		case errors.Is(e, domain.ErrVersion), errors.Is(e, domain.ErrState), errors.Is(e, domain.ErrPending):
			status = 409
		}
		return Order{}, fault(e.Error(), status)
	}
	next := change.Order
	raw, e = json.Marshal(next)
	if e != nil {
		return Order{}, e
	}
	result, e := tx.ExecContext(ctx, `UPDATE m3_workorders SET snapshot=?,state=?,version=?,assignee_id=?,assignee_name=?,updated_at=? WHERE id=? AND version=?`, raw, next.State, next.Version, next.AssigneeID, name, now, id, order.Version)
	if e != nil {
		return Order{}, e
	}
	affected, e := result.RowsAffected()
	if e != nil {
		return Order{}, e
	}
	if affected != 1 {
		return Order{}, fault("VERSION_CONFLICT", 409)
	}
	eventID := ID()
	if q.Action == "ASSIGN" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO m3_assignment_history(id,workorder_id,business_version,dispatcher_id,previous_assignee_id,assignee_id,assignee_name,comment,assigned_at) VALUES (?,?,?,?,?,?,?,?,?)`, eventID, id, next.Version, a.ID, order.AssigneeID, next.AssigneeID, name, q.Comment, now); e != nil {
			return Order{}, e
		}
	}
	if s.Events == nil {
		return Order{}, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	if e = s.Events.Append(ctx, tx, change, eventID, trace); e != nil {
		return Order{}, e
	}
	response := dto(next, seat, name)
	saved, e = json.Marshal(response)
	if e != nil {
		return Order{}, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE m3_commands SET response=? WHERE actor_id=? AND idem_key=?", saved, a.ID, key); e != nil {
		return Order{}, e
	}
	return response, tx.Commit()
}
