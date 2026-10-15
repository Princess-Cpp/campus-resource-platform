package m2

import (
	"context"
	"strings"
	"time"
)

type Filters struct {
	Status, From, To, RoomID string
	Page, PageSize           int
	Inbox                    bool
}
type Page struct {
	Items    []Application `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

func (s *Service) filterDates(f Filters) (string, []any, error) {
	where := ""
	args := []any{}
	for _, v := range []struct{ value, op string }{{f.From, ">="}, {f.To, "<"}} {
		if v.value == "" {
			continue
		}
		d, e := time.ParseInLocation("2006-01-02", v.value, s.Zone)
		if e != nil {
			return "", nil, fault("INVALID_INPUT", 400)
		}
		if v.op == "<" {
			d = d.AddDate(0, 0, 1)
		}
		where += " AND start_at" + v.op + "?"
		args = append(args, d.UTC())
	}
	if f.From != "" && f.To != "" && f.From > f.To {
		return "", nil, fault("INVALID_INPUT", 400)
	}
	if f.RoomID != "" {
		where += " AND room_id=?"
		args = append(args, f.RoomID)
	}
	if f.Status != "" {
		if !contains([]string{"SUBMITTED", "DEPARTMENT_APPROVED", "APPROVED", "REJECTED", "CANCELLED", "EXPIRED", "TERMINATED"}, f.Status) {
			return "", nil, fault("INVALID_INPUT", 400)
		}
		where += " AND status=?"
		args = append(args, f.Status)
	}
	return where, args, nil
}
func (s *Service) List(ctx context.Context, a Actor, f Filters) (Page, error) {
	result := Page{Items: []Application{}, Page: f.Page, PageSize: f.PageSize}
	if f.Page < 1 || f.Page > 100000 || f.PageSize < 1 || f.PageSize > 100 {
		return result, fault("INVALID_INPUT", 400)
	}
	where := "user_id=? AND hidden_at IS NULL"
	args := []any{a.ID}
	if f.Inbox {
		scopes := []string{}
		args = []any{}
		if a.Can("approval.department") {
			scopes = append(scopes, "(status='SUBMITTED' AND department=? AND JSON_CONTAINS(payload,JSON_QUOTE(?),'$.departmentApprovers'))")
			args = append(args, a.Department, a.ID)
		}
		if a.Can("approval.final") && len(a.Areas) > 0 {
			area, av := areaClause(a)
			scopes = append(scopes, "(status='DEPARTMENT_APPROVED' AND "+area+" AND JSON_CONTAINS(payload,JSON_QUOTE(?),'$.finalApprovers') AND JSON_UNQUOTE(JSON_EXTRACT(payload,'$.departmentActor'))<>?)")
			args = append(args, av...)
			args = append(args, a.ID, a.ID)
		}
		if len(scopes) == 0 {
			return result, fault("FORBIDDEN", 403)
		}
		where = "(" + strings.Join(scopes, " OR ") + ") AND user_id<>?"
		args = append(args, a.ID)
	}
	extra, values, e := s.filterDates(f)
	if e != nil {
		return result, e
	}
	where += extra
	args = append(args, values...)
	if e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM m2_applications WHERE "+where, args...).Scan(&result.Total); e != nil {
		return result, e
	}
	pageArgs := append(append([]any{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, e := s.DB.QueryContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE "+where+" ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?", pageArgs...)
	if e != nil {
		return result, e
	}
	defer rows.Close()
	for rows.Next() {
		b, e := scan(rows)
		if e != nil {
			return result, e
		}
		b.Snapshot.ContactRef = ""
		result.Items = append(result.Items, b)
	}
	return result, rows.Err()
}
func areaClause(a Actor) (string, []any) {
	if contains(a.Areas, "*") {
		return "1=1", nil
	}
	if len(a.Areas) == 0 {
		return "1=0", nil
	}
	marks := []string{}
	args := []any{}
	for _, area := range a.Areas {
		marks = append(marks, "?")
		args = append(args, area)
	}
	return "area IN (" + strings.Join(marks, ",") + ")", args
}

type Statistics struct {
	Total        int        `json:"total"`
	Approved     int        `json:"approved"`
	Rejected     int        `json:"rejected"`
	Pending      int        `json:"pending"`
	ApprovalRate *float64   `json:"approvalRate"`
	AverageHours *float64   `json:"averageHours"`
	Rooms        []RoomStat `json:"rooms"`
}
type RoomStat struct {
	RoomID   string `json:"roomId"`
	RoomName string `json:"roomName"`
	Total    int    `json:"total"`
	Approved int    `json:"approved"`
	Rejected int    `json:"rejected"`
}

func (s *Service) Stats(ctx context.Context, a Actor, f Filters) (Statistics, error) {
	v := Statistics{Rooms: []RoomStat{}}
	if !a.Can("classroom.stats") {
		return v, fault("FORBIDDEN", 403)
	}
	scope, args := areaClause(a)
	extra, values, e := s.filterDates(f)
	if e != nil {
		return v, e
	}
	args = append(args, values...)
	where := scope + extra
	e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*),COALESCE(SUM(decision='APPROVED'),0),COALESCE(SUM(decision='REJECTED'),0),COALESCE(SUM(status IN ('SUBMITTED','DEPARTMENT_APPROVED')),0),AVG(TIMESTAMPDIFF(SECOND,created_at,decided_at)/3600) FROM m2_applications WHERE "+where, args...).Scan(&v.Total, &v.Approved, &v.Rejected, &v.Pending, &v.AverageHours)
	if e != nil {
		return v, e
	}
	if v.Approved+v.Rejected > 0 {
		rate := float64(v.Approved) / float64(v.Approved+v.Rejected)
		v.ApprovalRate = &rate
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT room_id,MAX(room_name),COUNT(*),COALESCE(SUM(decision='APPROVED'),0),COALESCE(SUM(decision='REJECTED'),0) FROM m2_applications WHERE "+where+" GROUP BY room_id ORDER BY room_id", args...)
	if e != nil {
		return v, e
	}
	defer rows.Close()
	for rows.Next() {
		var r RoomStat
		if e = rows.Scan(&r.RoomID, &r.RoomName, &r.Total, &r.Approved, &r.Rejected); e != nil {
			return v, e
		}
		v.Rooms = append(v.Rooms, r)
	}
	return v, rows.Err()
}
