package maintenance

import (
	"campus-resource-platform/backend/internal/maintenance/domain"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Sweep resumes resource reconciliation first, then scans business stages.
// The existing process ticker is a development scheduler, not a new service.
func (s *Service) Sweep(ctx context.Context) error {
	resourceCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	resourceError := s.Tick(resourceCtx)
	cancel()
	return errors.Join(resourceError, s.EscalateOverdue(ctx), s.RouteEscalations(ctx))
}

func (s *Service) EscalateOverdue(ctx context.Context) error {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,snapshot FROM m3_workorders WHERE state IN ('REPORTED','ASSIGNED','IN_PROGRESS','AWAITING_ACCEPTANCE') AND JSON_UNQUOTE(JSON_EXTRACT(snapshot,'$.Sync'))='IDLE' AND JSON_EXTRACT(snapshot,'$.Stage.Escalated')=false ORDER BY created_at,id`)
	if e != nil {
		return e
	}
	ids := []string{}
	now := s.Now().UTC()
	for rows.Next() {
		var id string
		var raw []byte
		var o domain.Order
		if e = rows.Scan(&id, &raw); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			break
		}
		if _, due := o.Escalate(now); due {
			ids = append(ids, id)
		}
		if len(ids) >= 100 {
			break
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	var result error
	for _, id := range ids {
		result = errors.Join(result, s.escalateOne(ctx, id))
	}
	return result
}
func (s *Service) escalateOne(ctx context.Context, id string) error {
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw []byte
	var o domain.Order
	if e = tx.QueryRowContext(ctx, "SELECT snapshot FROM m3_workorders WHERE id=? FOR UPDATE", id).Scan(&raw); e != nil {
		return e
	}
	if e = json.Unmarshal(raw, &o); e != nil {
		return e
	}
	now := s.Now().UTC().Truncate(time.Microsecond)
	change, due := o.Escalate(now)
	if !due {
		return nil
	}
	event := ID()
	raw, e = json.Marshal(change.Order)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE m3_workorders SET snapshot=?,version=?,updated_at=? WHERE id=?", raw, change.Order.Version, now, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO m3_escalations(id,workorder_id,stage_number,level,policy_version,stage_state,deadline_at,detected_at,routing_state,next_route_at,updated_at) VALUES (?,?,?,1,?,?,?,?,'PENDING',?,?)`, event, id, o.Stage.Number, o.Policy.Version, o.State, o.Stage.DeadlineAt, now, now, now); e != nil {
		return e
	}
	if s.Events == nil {
		return fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	if e = s.Events.Append(ctx, tx, change, event, "m3-escalation-"+event); e != nil {
		return e
	}
	return tx.Commit()
}

// Routing retries the original escalation event without repeating a business
// transition. Missing supervisors remain a scoped, administrator-visible fact.
func (s *Service) RouteEscalations(ctx context.Context) error {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,workorder_id FROM m3_escalations WHERE routing_state IN ('PENDING','NO_SUPERVISOR','DIRECTORY_UNAVAILABLE') AND next_route_at<=? ORDER BY next_route_at,id LIMIT 100`, s.Now().UTC())
	if e != nil {
		return e
	}
	ids := [][2]string{}
	for rows.Next() {
		var v [2]string
		if e = rows.Scan(&v[0], &v[1]); e != nil {
			break
		}
		ids = append(ids, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return e
	}
	var result error
	for _, v := range ids {
		result = errors.Join(result, s.routeOne(ctx, v[0], v[1]))
	}
	return result
}
func (s *Service) routeOne(ctx context.Context, event, id string) error {
	var area string
	if e := s.DB.QueryRowContext(ctx, "SELECT area FROM m3_workorders WHERE id=?", id).Scan(&area); e != nil {
		return e
	}
	recipients := []string{}
	routing := "DIRECTORY_UNAVAILABLE"
	if s.Supervisors != nil {
		people, e := s.Supervisors.Supervisors(ctx, area)
		if e == nil {
			recipients = people
			routing = "NO_SUPERVISOR"
			if len(people) > 0 {
				routing = "QUEUED"
			}
		}
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var raw []byte
	var o domain.Order
	if e = tx.QueryRowContext(ctx, "SELECT snapshot FROM m3_workorders WHERE id=? FOR UPDATE", id).Scan(&raw); e != nil {
		return e
	}
	if e = json.Unmarshal(raw, &o); e != nil {
		return e
	}
	var state string
	var stage int64
	var due time.Time
	if e = tx.QueryRowContext(ctx, "SELECT routing_state,stage_number,next_route_at FROM m3_escalations WHERE id=? FOR UPDATE", event).Scan(&state, &stage, &due); e != nil {
		return e
	}
	now := s.Now().UTC().Truncate(time.Microsecond)
	if state == "QUEUED" || state == "OBSOLETE" || due.After(now) {
		return nil
	}
	if o.Stage.Number != stage || o.State == domain.Completed || o.State == domain.Cancelled {
		routing = "OBSOLETE"
	}
	if routing == "QUEUED" {
		payload, e := json.Marshal(map[string]any{"workorderId": id, "roomName": o.RoomName, "stageNumber": stage, "state": o.State})
		if e != nil {
			return e
		}
		seen := map[string]bool{}
		for _, recipient := range recipients {
			if recipient == "" || seen[recipient] {
				continue
			}
			seen[recipient] = true
			if _, e = tx.ExecContext(ctx, `INSERT INTO m3_outbox(event_id,workorder_id,recipient_id,kind,payload,created_at) VALUES (?,?,?,'WORKORDER_ESCALATED',?,?)`, event, id, recipient, payload, now); e != nil {
				return e
			}
		}
	}
	if _, e = tx.ExecContext(ctx, "UPDATE m3_escalations SET routing_state=?,next_route_at=?,updated_at=? WHERE id=?", routing, now.Add(30*time.Second), now, event); e != nil {
		return e
	}
	return tx.Commit()
}
