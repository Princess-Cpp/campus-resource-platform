package maintenance

import (
	"campus-resource-platform/backend/internal/maintenance/domain"
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"
)

type Metric struct {
	Count          int      `json:"count"`
	AverageSeconds *float64 `json:"averageSeconds"`
}
type Statistics struct {
	AsOf                    time.Time            `json:"asOf"`
	Total                   int                  `json:"total"`
	ByState                 map[domain.State]int `json:"byState"`
	EligibleCount           int                  `json:"eligibleCount"`
	RecordedOverdueCount    int                  `json:"recordedOverdueCount"`
	RecordedOverdueRate     *float64             `json:"recordedOverdueRate"`
	ResourceUnresolvedCount int                  `json:"resourceUnresolvedCount"`
	RatedCount              int                  `json:"ratedCount"`
	AverageScore            *float64             `json:"averageScore"`
	Dispatch                Metric               `json:"dispatch"`
	FirstRepair             Metric               `json:"firstRepair"`
	Repair                  Metric               `json:"repair"`
	Acceptance              Metric               `json:"acceptance"`
}
type Escalation struct {
	ID            string       `json:"id"`
	WorkorderID   string       `json:"workorderId"`
	RoomName      string       `json:"roomName"`
	Area          string       `json:"area"`
	StageNumber   int64        `json:"stageNumber"`
	Level         int          `json:"level"`
	PolicyVersion string       `json:"policyVersion"`
	StageState    domain.State `json:"stageState"`
	CurrentState  domain.State `json:"currentState"`
	DeadlineAt    time.Time    `json:"deadlineAt"`
	DetectedAt    time.Time    `json:"detectedAt"`
	RoutingState  string       `json:"routingState"`
	Active        bool         `json:"active"`
	CanOpen       bool         `json:"canOpen"`
}
type EscalationPage struct {
	Items    []Escalation `json:"items"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
	Total    int          `json:"total"`
}

func operationsScope(a domain.Actor, permissions ...string) ([]string, []any, error) {
	if !a.Enabled || a.ID == "" {
		return nil, nil, fault("UNAUTHENTICATED", 401)
	}
	allowed := false
	for _, p := range permissions {
		allowed = allowed || a.Can(p)
	}
	if !allowed || len(a.Areas) == 0 {
		return nil, nil, fault("FORBIDDEN", 403)
	}
	if a.InArea("*") {
		return []string{"1=1"}, nil, nil
	}
	marks := []string{}
	args := []any{}
	for _, area := range a.Areas {
		marks = append(marks, "?")
		args = append(args, area)
	}
	return []string{"w.area COLLATE utf8mb4_bin IN (" + strings.Join(marks, ",") + ")"}, args, nil
}
func operationFilters(where []string, args []any, f Filters, dateColumn string) ([]string, []any, error) {
	if f.Category != "" {
		if !slices.Contains([]string{"LIGHTING", "POWER", "FURNITURE", "DOOR_WINDOW", "OTHER"}, f.Category) {
			return nil, nil, fault("INVALID_INPUT", 400)
		}
		where = append(where, "w.category=?")
		args = append(args, f.Category)
	}
	var from, to time.Time
	var e error
	if f.From != "" {
		from, e = time.Parse(time.RFC3339Nano, f.From)
		if e != nil {
			return nil, nil, fault("INVALID_INPUT", 400)
		}
		where = append(where, dateColumn+">=?")
		args = append(args, from.UTC())
	}
	if f.To != "" {
		to, e = time.Parse(time.RFC3339Nano, f.To)
		if e != nil {
			return nil, nil, fault("INVALID_INPUT", 400)
		}
		where = append(where, dateColumn+"<?")
		args = append(args, to.UTC())
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return nil, nil, fault("INVALID_INPUT", 400)
	}
	return where, args, nil
}
func (s *Service) Stats(ctx context.Context, a domain.Actor, f Filters) (Statistics, error) {
	out := Statistics{AsOf: s.Now().UTC(), ByState: map[domain.State]int{}}
	where, args, e := operationsScope(a, "workorder.stats")
	if e != nil {
		return out, e
	}
	where, args, e = operationFilters(where, args, f, "w.created_at")
	if e != nil {
		return out, e
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT w.snapshot,EXISTS(SELECT 1 FROM m3_escalations e WHERE e.workorder_id=w.id) FROM m3_workorders w WHERE `+strings.Join(where, " AND "), args...)
	if e != nil {
		return out, e
	}
	var dispatch, first, repair, acceptance, scores float64
	for rows.Next() {
		var raw []byte
		var overdue bool
		var o domain.Order
		if e = rows.Scan(&raw, &overdue); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			break
		}
		out.Total++
		out.ByState[o.State]++
		if o.State == domain.Cancelled {
			continue
		}
		out.EligibleCount++
		if overdue {
			out.RecordedOverdueCount++
		}
		if o.Sync != domain.Idle {
			out.ResourceUnresolvedCount++
		}
		if !o.FirstAssignedAt.IsZero() {
			out.Dispatch.Count++
			dispatch += o.FirstAssignedAt.Sub(o.CreatedAt).Seconds()
		}
		if !o.FirstStartedAt.IsZero() {
			out.FirstRepair.Count++
			first += o.FirstStartedAt.Sub(o.CreatedAt).Seconds()
		}
		if o.State == domain.Completed {
			out.Repair.Count++
			repair += o.RepairElapsed.Seconds()
			out.Acceptance.Count++
			acceptance += o.AcceptanceElapsed.Seconds()
			if o.Evaluation != nil {
				out.RatedCount++
				scores += float64(o.Evaluation.Score)
			}
		}
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	average := func(total float64, count int) *float64 {
		if count == 0 {
			return nil
		}
		v := total / float64(count)
		return &v
	}
	out.Dispatch.AverageSeconds = average(dispatch, out.Dispatch.Count)
	out.FirstRepair.AverageSeconds = average(first, out.FirstRepair.Count)
	out.Repair.AverageSeconds = average(repair, out.Repair.Count)
	out.Acceptance.AverageSeconds = average(acceptance, out.Acceptance.Count)
	out.AverageScore = average(scores, out.RatedCount)
	out.RecordedOverdueRate = average(float64(out.RecordedOverdueCount), out.EligibleCount)
	return out, tx.Commit()
}
func (s *Service) Escalations(ctx context.Context, a domain.Actor, f Filters, activeOnly bool) (EscalationPage, error) {
	out := EscalationPage{Items: []Escalation{}, Page: f.Page, PageSize: f.PageSize}
	where, args, e := operationsScope(a, "workorder.supervise", "workorder.escalation.read")
	if e != nil {
		return out, e
	}
	if f.Page < 1 || f.Page > 100000 || f.PageSize < 1 || f.PageSize > 100 {
		return out, fault("INVALID_INPUT", 400)
	}
	where, args, e = operationFilters(where, args, f, "e.detected_at")
	if e != nil {
		return out, e
	}
	if activeOnly {
		where = append(where, "e.stage_number=JSON_EXTRACT(w.snapshot,'$.Stage.Number')", "w.state NOT IN ('COMPLETED','CANCELLED')")
	}
	clause := ` FROM m3_escalations e JOIN m3_workorders w ON e.workorder_id=w.id WHERE ` + strings.Join(where, " AND ")
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	if e = tx.QueryRowContext(ctx, "SELECT COUNT(*)"+clause, args...).Scan(&out.Total); e != nil {
		return out, e
	}
	params := append(append([]any{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, e := tx.QueryContext(ctx, `SELECT e.id,e.workorder_id,e.stage_number,e.level,e.policy_version,e.stage_state,e.deadline_at,e.detected_at,e.routing_state,w.snapshot`+clause+" ORDER BY e.detected_at DESC,e.id DESC LIMIT ? OFFSET ?", params...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v Escalation
		var raw []byte
		var o domain.Order
		if e = rows.Scan(&v.ID, &v.WorkorderID, &v.StageNumber, &v.Level, &v.PolicyVersion, &v.StageState, &v.DeadlineAt, &v.DetectedAt, &v.RoutingState, &raw); e != nil {
			break
		}
		if e = json.Unmarshal(raw, &o); e != nil {
			break
		}
		v.RoomName, v.Area, v.CurrentState = o.RoomName, o.Area, o.State
		v.Active = o.Stage.Number == v.StageNumber && o.State != domain.Completed && o.State != domain.Cancelled
		v.CanOpen = o.CanRead(a)
		out.Items = append(out.Items, v)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	return out, tx.Commit()
}
