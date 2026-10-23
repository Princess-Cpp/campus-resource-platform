package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

type Notification struct {
	EventID       string     `json:"eventId"`
	WorkorderID   string     `json:"workorderId"`
	Kind          string     `json:"kind"`
	Summary       string     `json:"summary"`
	CreatedAt     time.Time  `json:"createdAt"`
	ReadAt        *time.Time `json:"readAt,omitempty"`
	DeliveryState string     `json:"deliveryState"`
	Attempts      int        `json:"attempts"`
	NextAttempt   *time.Time `json:"nextAttempt,omitempty"`
	LastError     string     `json:"lastError,omitempty"`
	CanOpen       bool       `json:"canOpen"`
}
type NotificationPage struct {
	Items       []Notification `json:"items"`
	Total       int            `json:"total"`
	Page        int            `json:"page"`
	PageSize    int            `json:"pageSize"`
	UnreadCount int            `json:"unreadCount"`
}

// Only fixed event summaries leave the outbox. Names, free text and contact
// details are deliberately absent; the detail endpoint rechecks current access.
func notificationSummary(kind string) string {
	switch kind {
	case "WORKORDER_REPORTED":
		return "报修已登记"
	case "WORKORDER_ASSIGNED":
		return "工单已派单或重派"
	case "WORKORDER_ACCEPTED":
		return "维修人员已接单"
	case "WORKORDER_HANDOVER":
		return "工单已交接给其他维修人员"
	case "WORKORDER_START":
		return "已申请开始维修，等待场地下架确认"
	case "WORKORDER_FINISH":
		return "维修已完工，等待验收"
	case "WORKORDER_REJECT_ACCEPTANCE":
		return "验收未通过，继续维修"
	case "WORKORDER_APPROVE_ACCEPTANCE":
		return "验收通过，等待确认本工单的维修限制解除结果"
	case "WORKORDER_RESOURCE_CONFIRMED":
		return "场地操作已确认，请查看工单当前状态"
	case "WORKORDER_CANCELLED":
		return "工单已取消"
	case "WORKORDER_ESCALATED":
		return "工单阶段超时，请在授权范围内处理"
	case "WORKORDER_EVALUATE":
		return "工单已有评价"
	default:
		return "报修工单有更新"
	}
}

func (s *Service) Notifications(ctx context.Context, a domain.Actor, page, size int, unread bool) (NotificationPage, error) {
	out := NotificationPage{Items: []Notification{}, Page: page, PageSize: size}
	if !a.Enabled || a.ID == "" {
		return out, fault("UNAUTHENTICATED", 401)
	}
	if page < 1 || page > 100000 || size < 1 || size > 100 {
		return out, fault("INVALID_INPUT", 400)
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	where := "recipient_id=?"
	if unread {
		where += " AND read_at IS NULL"
	}
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM m3_outbox WHERE "+where, a.ID).Scan(&out.Total); e != nil {
		return out, e
	}
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM m3_outbox WHERE recipient_id=? AND read_at IS NULL", a.ID).Scan(&out.UnreadCount); e != nil {
		return out, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT n.event_id,n.workorder_id,n.kind,n.created_at,n.read_at,n.status,n.attempts,n.next_attempt,n.last_error,w.snapshot FROM m3_outbox n JOIN m3_workorders w ON w.id=n.workorder_id WHERE n.`+where+` ORDER BY n.created_at DESC,n.event_id DESC,n.kind DESC LIMIT ? OFFSET ?`, a.ID, size, (page-1)*size)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var n Notification
		var read, next sql.NullTime
		var raw []byte
		var o domain.Order
		if e = rows.Scan(&n.EventID, &n.WorkorderID, &n.Kind, &n.CreatedAt, &read, &n.DeliveryState, &n.Attempts, &next, &n.LastError, &raw); e != nil {
			return out, e
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			return out, e
		}
		n.CanOpen = o.CanRead(a)
		n.Summary = notificationSummary(n.Kind)
		if read.Valid {
			n.ReadAt = &read.Time
		}
		if next.Valid {
			n.NextAttempt = &next.Time
		}
		out.Items = append(out.Items, n)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	rows.Close()
	return out, tx.Commit()
}

func (s *Service) ReadNotification(ctx context.Context, a domain.Actor, event, kind string) error {
	if !a.Enabled || a.ID == "" {
		return fault("UNAUTHENTICATED", 401)
	}
	if event == "" || len(event) > 64 || kind == "" || len(kind) > 64 {
		return fault("INVALID_INPUT", 400)
	}
	result, e := s.DB.ExecContext(ctx, "UPDATE m3_outbox SET read_at=COALESCE(read_at,?) WHERE event_id=? AND recipient_id=? AND kind=?", s.Now().UTC(), event, a.ID, kind)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		// MySQL may report zero changed rows on a replay. Ownership still has to match.
		var found int
		e = s.DB.QueryRowContext(ctx, "SELECT 1 FROM m3_outbox WHERE event_id=? AND recipient_id=? AND kind=?", event, a.ID, kind).Scan(&found)
		if e == sql.ErrNoRows {
			return fault("NOT_FOUND", 404)
		}
		return e
	}
	return nil
}
