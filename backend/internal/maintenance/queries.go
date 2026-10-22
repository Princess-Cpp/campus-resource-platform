package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

func (s *Service) List(ctx context.Context, a domain.Actor, f Filters) (Page, error) {
	out := Page{Items: []Order{}, Page: f.Page, PageSize: f.PageSize}
	if !a.Enabled || a.ID == "" {
		return out, fault("UNAUTHENTICATED", 401)
	}
	if f.Page < 1 || f.Page > 100000 || f.PageSize < 1 || f.PageSize > 100 {
		return out, fault("INVALID_INPUT", 400)
	}
	where := []string{"reporter_id=?"}
	args := []any{a.ID}
	if f.State != "" {
		if !slices.Contains([]string{"REPORTED", "ASSIGNED", "IN_PROGRESS", "AWAITING_ACCEPTANCE", "COMPLETED", "CANCELLED"}, f.State) {
			return out, fault("INVALID_INPUT", 400)
		}
		where = append(where, "state=?")
		args = append(args, f.State)
	}
	if f.Category != "" {
		if !slices.Contains([]string{"LIGHTING", "POWER", "FURNITURE", "DOOR_WINDOW", "OTHER"}, f.Category) {
			return out, fault("INVALID_INPUT", 400)
		}
		where = append(where, "category=?")
		args = append(args, f.Category)
	}
	var from, to time.Time
	var e error
	if f.From != "" {
		from, e = time.Parse(time.RFC3339Nano, f.From)
		if e != nil {
			return out, fault("INVALID_INPUT", 400)
		}
		where = append(where, "created_at>=?")
		args = append(args, from.UTC())
	}
	if f.To != "" {
		to, e = time.Parse(time.RFC3339Nano, f.To)
		if e != nil {
			return out, fault("INVALID_INPUT", 400)
		}
		where = append(where, "created_at<?")
		args = append(args, to.UTC())
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return out, fault("INVALID_INPUT", 400)
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	condition := strings.Join(where, " AND ")
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM m3_workorders WHERE "+condition, args...).Scan(&out.Total); e != nil {
		return out, e
	}
	params := append(append([]any{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, e := tx.QueryContext(ctx, "SELECT snapshot,seat_label FROM m3_workorders WHERE "+condition+" ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?", params...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var raw []byte
		var label string
		var o domain.Order
		if e = rows.Scan(&raw, &label); e != nil {
			rows.Close()
			return out, e
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, dto(o, label))
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
func (s *Service) Get(ctx context.Context, a domain.Actor, id string) (Detail, error) {
	out := Detail{Timeline: []Timeline{}}
	if !a.Enabled || a.ID == "" {
		return out, fault("UNAUTHENTICATED", 401)
	}
	if len(id) == 0 || len(id) > 64 {
		return out, fault("NOT_FOUND", 404)
	}
	var raw []byte
	var label string
	e := s.DB.QueryRowContext(ctx, "SELECT snapshot,seat_label FROM m3_workorders WHERE id=? AND reporter_id=?", id, a.ID).Scan(&raw, &label)
	if errors.Is(e, sql.ErrNoRows) {
		return out, fault("NOT_FOUND", 404)
	}
	if e != nil {
		return out, e
	}
	var order domain.Order
	if e = json.Unmarshal(raw, &order); e != nil {
		return out, e
	}
	out.Order = dto(order, label)
	rows, e := s.DB.QueryContext(ctx, "SELECT event_id,action,occurred_at FROM m3_events WHERE workorder_id=? ORDER BY business_version,event_id", id)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var entry Timeline
		if e = rows.Scan(&entry.ID, &entry.Action, &entry.At); e != nil {
			return out, e
		}
		entry.Text = "故障已上报，等待派单"
		out.Timeline = append(out.Timeline, entry)
	}
	return out, rows.Err()
}
