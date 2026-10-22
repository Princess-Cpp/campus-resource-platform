package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

// These are M3 caller ports, not the unreviewed M2 shared job/outbox API.
type NotificationDirectory interface {
	Recipient(context.Context, string) (domain.Actor, error)
}
type NotificationSender interface {
	Send(context.Context, string, string) error
}

var ErrNotificationUnconfigured = errors.New("notification channel not configured")
var ErrNotificationRecipientInvalid = errors.New("notification recipient invalid")

type deliveryClaim struct {
	event, order, recipient, kind, token string
	attempts                             int
	payload                              []byte
}

// DispatchNotifications is a bounded callback for a future shared runner. The
// existing development ticker calls it; it does not spawn another scheduler.
func (s *Service) DispatchNotifications(ctx context.Context) error {
	for i := 0; i < 50; i++ {
		if e := ctx.Err(); e != nil {
			return e
		}
		c, e := s.claimNotification(ctx)
		if e != nil {
			return e
		}
		if c == nil {
			return nil
		}
		if e = s.deliverNotification(ctx, c); e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) claimNotification(ctx context.Context) (*deliveryClaim, error) {
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	now := s.Now().UTC()
	c := &deliveryClaim{token: ID()}
	e = tx.QueryRowContext(ctx, `SELECT event_id,workorder_id,recipient_id,kind,attempts,payload FROM m3_outbox
 WHERE (status IN ('PENDING','UNCONFIGURED') AND (next_attempt IS NULL OR next_attempt<=?)) OR (status='SENDING' AND lease_until<=?)
 ORDER BY created_at,event_id,recipient_id,kind LIMIT 1 FOR UPDATE SKIP LOCKED`, now, now).Scan(&c.event, &c.order, &c.recipient, &c.kind, &c.attempts, &c.payload)
	if e == sql.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE m3_outbox SET status='SENDING',lease_token=?,lease_until=? WHERE event_id=? AND recipient_id=? AND kind=?`, c.token, now.Add(30*time.Second), c.event, c.recipient, c.kind)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return c, nil
}

func (s *Service) deliverNotification(ctx context.Context, c *deliveryClaim) error {
	if c.attempts >= 5 {
		return s.finishNotification(ctx, c, "FAILED", "OUTCOME_UNKNOWN", nil)
	}
	if s.NotificationRecipients == nil {
		return s.finishNotification(ctx, c, "PENDING", "DIRECTORY_UNAVAILABLE", durationPtr(time.Minute))
	}
	a, e := s.NotificationRecipients.Recipient(ctx, c.recipient)
	if e != nil {
		return s.finishNotification(ctx, c, "PENDING", "DIRECTORY_UNAVAILABLE", durationPtr(time.Minute))
	}
	valid, e := s.notificationApplicable(ctx, c, a)
	if e != nil {
		return s.finishNotification(ctx, c, "PENDING", "SOURCE_UNAVAILABLE", durationPtr(time.Minute))
	}
	if !valid {
		return s.finishNotification(ctx, c, "SUPPRESSED", "RECIPIENT_OR_STAGE_CHANGED", nil)
	}
	if s.NotificationSender == nil {
		return s.finishNotification(ctx, c, "UNCONFIGURED", "CHANNEL_UNCONFIGURED", durationPtr(time.Minute))
	}
	// Persist the attempt BEFORE network I/O. A crash never erases an attempt.
	r, e := s.DB.ExecContext(ctx, `UPDATE m3_outbox SET attempts=attempts+1 WHERE event_id=? AND recipient_id=? AND kind=? AND status='SENDING' AND lease_token=? AND lease_until>?`, c.event, c.recipient, c.kind, c.token, s.Now().UTC())
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil || n == 0 {
		return e
	}
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	e = s.NotificationSender.Send(sendCtx, c.recipient, c.event+":"+c.kind)
	cancel()
	if errors.Is(e, ErrNotificationUnconfigured) {
		// No SMTP connection was attempted by the adapter in this case.
		_, dbError := s.DB.ExecContext(ctx, `UPDATE m3_outbox SET attempts=attempts-1 WHERE event_id=? AND recipient_id=? AND kind=? AND status='SENDING' AND lease_token=?`, c.event, c.recipient, c.kind, c.token)
		if dbError != nil {
			return dbError
		}
		return s.finishNotification(ctx, c, "UNCONFIGURED", "CHANNEL_UNCONFIGURED", durationPtr(time.Minute))
	}
	if e == nil {
		return s.finishNotification(ctx, c, "DELIVERED", "", nil)
	}
	if errors.Is(e, ErrNotificationRecipientInvalid) {
		return s.finishNotification(ctx, c, "FAILED", "INVALID_RECIPIENT", nil)
	}
	state := "PENDING"
	var retry *time.Duration
	if c.attempts+1 >= 5 {
		state = "FAILED"
	} else {
		retry = durationPtr(time.Minute * time.Duration(1<<c.attempts))
	}
	return s.finishNotification(ctx, c, state, "DELIVERY_UNAVAILABLE", retry)
}
func durationPtr(d time.Duration) *time.Duration { return &d }
func (s *Service) finishNotification(ctx context.Context, c *deliveryClaim, state, reason string, retry *time.Duration) error {
	now := s.Now().UTC()
	var next, delivered any
	if retry != nil {
		next = now.Add(*retry)
	}
	if state == "DELIVERED" {
		delivered = now
	}
	_, e := s.DB.ExecContext(ctx, `UPDATE m3_outbox SET status=?,last_error=?,next_attempt=?,delivered_at=?,lease_until=NULL,lease_token='' WHERE event_id=? AND recipient_id=? AND kind=? AND status='SENDING' AND lease_token=?`, state, reason, next, delivered, c.event, c.recipient, c.kind, c.token)
	return e
}

func (s *Service) notificationApplicable(ctx context.Context, c *deliveryClaim, a domain.Actor) (bool, error) {
	if !a.Enabled || a.ID != c.recipient {
		return false, nil
	}
	var raw []byte
	var o domain.Order
	if e := s.DB.QueryRowContext(ctx, "SELECT snapshot FROM m3_workorders WHERE id=?", c.order).Scan(&raw); e != nil {
		return false, e
	}
	if e := json.Unmarshal(raw, &o); e != nil {
		return false, e
	}
	if c.kind == "WORKORDER_ESCALATED" {
		var p struct {
			StageNumber int64 `json:"stageNumber"`
		}
		if e := json.Unmarshal(c.payload, &p); e != nil {
			return false, e
		}
		return a.Can("workorder.supervise") && a.InArea(o.Area) && p.StageNumber == o.Stage.Number && o.State != domain.Completed && o.State != domain.Cancelled, nil
	}
	if c.kind == "WORKORDER_HANDOVER" {
		var count int
		e := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM m3_assignment_history WHERE id=? AND previous_assignee_id=?", c.event, c.recipient).Scan(&count)
		return count == 1, e
	}
	return c.recipient == o.ReporterID || (c.recipient == o.AssigneeID && a.Can("workorder.repair") && a.InArea(o.Area)), nil
}
