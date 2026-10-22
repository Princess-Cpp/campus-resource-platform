package classroom

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/go-sql-driver/mysql"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Service) interval(start, end time.Time) error {
	a, b := start.In(s.Zone), end.In(s.Zone)
	now := s.Now()
	if !start.After(now) || start.After(now.AddDate(0, 0, s.Policy.AdvanceDays)) || !end.After(start) || end.Sub(start) > time.Duration(s.Policy.MaxHours)*time.Hour || a.Minute() != 0 || a.Second() != 0 || a.Nanosecond() != 0 || b.Minute() != 0 || b.Second() != 0 || b.Nanosecond() != 0 || a.Format("2006-01-02") != b.Add(-time.Nanosecond).Format("2006-01-02") {
		return fault("INVALID_INPUT", 400)
	}
	return nil
}
func (s *Service) Search(ctx context.Context, a Actor, token string, start, end time.Time, area string, capacity int) ([]Room, error) {
	if capacity < 1 {
		return nil, fault("INVALID_INPUT", 400)
	}
	if e := s.interval(start, end); e != nil {
		return nil, e
	}
	rooms, e := s.Directory.Rooms(ctx)
	if e != nil {
		return nil, e
	}
	available, e := s.Resources.Available(ctx, token, start, end)
	if e != nil {
		return nil, e
	}
	out := []Room{}
	for _, r := range rooms {
		endHour := end.In(s.Zone).Hour()
		if endHour == 0 {
			endHour = 24
		}
		if r.Active && r.Capacity >= capacity && (area == "" || strings.Contains(r.Area, area)) && (r.Department == "" || r.Department == a.Department) && start.In(s.Zone).Hour() >= r.OpenHour && endHour <= r.CloseHour && available[r.ID] {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *Service) Create(ctx context.Context, a Actor, token string, q CreateRequest, key, trace string) (Application, error) {
	if !a.Can("classroom.apply") {
		return Application{}, fault("FORBIDDEN", 403)
	}
	if key == "" || len(key) > 128 {
		return Application{}, fault("INVALID_IDEMPOTENCY_KEY", 400)
	}
	q.Purpose = strings.TrimSpace(q.Purpose)
	q.StartAt = q.StartAt.UTC()
	q.EndAt = q.EndAt.UTC()
	old, e := s.existing(ctx, a.ID, key, hash(q))
	if e == nil {
		return old, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return old, e
	}
	if utf8.RuneCountInString(q.Purpose) < 10 || utf8.RuneCountInString(q.Purpose) > 500 || !contains(a.ContactRefs, q.ContactRef) {
		return Application{}, fault("INVALID_INPUT", 400)
	}
	rooms, e := s.Search(ctx, a, token, q.StartAt, q.EndAt, "", q.Attendees)
	if e != nil {
		return Application{}, e
	}
	var room *Room
	for i := range rooms {
		if rooms[i].ID == q.RoomID {
			room = &rooms[i]
			break
		}
	}
	if room == nil {
		return Application{}, fault("RESOURCE_UNAVAILABLE", 409)
	}
	actors, e := s.Directory.Actors(ctx)
	if e != nil {
		return Application{}, e
	}
	deps, finals := []string{}, []string{}
	for _, v := range actors {
		if v.ID == a.ID {
			continue
		}
		if v.Can("approval.department") && v.Department == a.Department {
			deps = append(deps, v.ID)
		}
		if v.Can("approval.final") && v.Area(room.Area) {
			finals = append(finals, v.ID)
		}
	}
	pair := false
	for _, d := range deps {
		for _, f := range finals {
			if d != f {
				pair = true
			}
		}
	}
	if !pair {
		return Application{}, fault("APPROVERS_UNAVAILABLE", 409)
	}
	now := s.Now().UTC()
	deadline := now.Add(time.Duration(s.Policy.ApprovalHours) * time.Hour)
	if q.StartAt.Before(deadline) {
		deadline = q.StartAt
	}
	b := Application{ID: id(), UserID: a.ID, Department: a.Department, RoomID: room.ID, RoomName: room.Name, Area: room.Area, StartAt: q.StartAt, EndAt: q.EndAt, CreatedAt: now, DeadlineAt: deadline, Status: "SUBMITTED", Version: 1, SyncStatus: "IDLE", Snapshot: Snapshot{Purpose: q.Purpose, Attendees: q.Attendees, Capacity: room.Capacity, ContactRef: q.ContactRef, DepartmentApprovers: deps, FinalApprovers: finals, Policy: s.Policy}}
	raw, _ := json.Marshal(b.Snapshot)
	tx, e := s.begin(ctx)
	if e != nil {
		return b, e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, "INSERT INTO m2_applications ("+cols+",idem_key,request_hash) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", b.ID, b.UserID, b.Department, b.RoomID, b.RoomName, b.Area, b.StartAt, b.EndAt, b.CreatedAt, b.DeadlineAt, b.Status, b.Version, b.SyncStatus, b.OperationID, b.SourceVersion, b.LastError, raw, key, hash(q))
	if e != nil {
		var m *mysql.MySQLError
		if errors.As(e, &m) && m.Number == 1062 {
			tx.Rollback()
			return s.existing(ctx, a.ID, key, hash(q))
		}
		return b, e
	}
	if e = s.event(ctx, tx, b, a.ID, "SUBMIT", "", q.Purpose, trace); e != nil {
		return b, e
	}
	return b, tx.Commit()
}
func (s *Service) existing(ctx context.Context, user, key, digest string) (Application, error) {
	b, e := scan(s.DB.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE user_id=? AND idem_key=?", user, key))
	if e != nil {
		return b, e
	}
	var old string
	e = s.DB.QueryRowContext(ctx, "SELECT request_hash FROM m2_applications WHERE id=?", b.ID).Scan(&old)
	if e == nil && old != digest {
		e = fault("IDEMPOTENCY_CONFLICT", 409)
	}
	return b, e
}
func (s *Service) save(ctx context.Context, tx *sql.Tx, b Application) error {
	raw, _ := json.Marshal(b.Snapshot)
	_, e := tx.ExecContext(ctx, "UPDATE m2_applications SET status=?,version=?,sync_status=?,operation_id=?,source_version=?,last_error=?,payload=? WHERE id=?", b.Status, b.Version, b.SyncStatus, b.OperationID, b.SourceVersion, b.LastError, raw, b.ID)
	return e
}
func (s *Service) event(ctx context.Context, tx *sql.Tx, b Application, actor, action, from, comment, trace string) error {
	eid := id()
	_, e := tx.ExecContext(ctx, "INSERT INTO m2_events VALUES (?,?,?,?,?,?,?,?,?)", eid, b.ID, actor, action, from, b.Status, comment, trace, s.Now().UTC())
	if e != nil {
		return e
	}
	recipients := map[string]bool{b.UserID: true}
	if b.Status == "SUBMITTED" {
		for _, v := range b.Snapshot.DepartmentApprovers {
			recipients[v] = true
		}
	}
	if b.Status == "DEPARTMENT_APPROVED" {
		for _, v := range b.Snapshot.FinalApprovers {
			recipients[v] = true
		}
	}
	for recipient := range recipients {
		if _, e = tx.ExecContext(ctx, "INSERT INTO m2_outbox(event_id,application_id,recipient_id,kind,created_at) VALUES (?,?,?,?,?)", eid+":"+recipient, b.ID, recipient, action, s.Now().UTC()); e != nil {
			return e
		}
	}
	return nil
}
func canRead(a Actor, b Application) bool {
	return a.ID == b.UserID || (a.Can("approval.department") && a.Department == b.Department && contains(b.Snapshot.DepartmentApprovers, a.ID)) || (a.Can("approval.final") && a.Area(b.Area) && contains(b.Snapshot.FinalApprovers, a.ID))
}
func (s *Service) Get(ctx context.Context, a Actor, application string) (Application, error) {
	b, e := scan(s.DB.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE id=?", application))
	if errors.Is(e, sql.ErrNoRows) {
		return b, fault("NOT_FOUND", 404)
	}
	if e != nil {
		return b, e
	}
	if !canRead(a, b) {
		return Application{}, fault("NOT_FOUND", 404)
	}
	rows, e := s.DB.QueryContext(ctx, "SELECT id,actor_id,action,from_status,to_status,comment,created_at FROM m2_events WHERE application_id=? ORDER BY created_at,id", application)
	if e != nil {
		return b, e
	}
	defer rows.Close()
	b.Events = []Event{}
	for rows.Next() {
		var ev Event
		if e = rows.Scan(&ev.ID, &ev.ActorID, &ev.Action, &ev.From, &ev.To, &ev.Comment, &ev.CreatedAt); e != nil {
			return b, e
		}
		b.Events = append(b.Events, ev)
	}
	if a.ID != b.UserID {
		b.Snapshot.ContactRef = ""
	}
	return b, rows.Err()
}

// Hide removes a completed application from its owner's list while preserving
// the application, approval events and statistics.
func (s *Service) Hide(ctx context.Context, a Actor, application, trace string) error {
	tx, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback()

	var status, syncStatus string
	var endAt time.Time
	var hiddenAt sql.NullTime
	e = tx.QueryRowContext(ctx, "SELECT status,sync_status,end_at,hidden_at FROM m2_applications WHERE id=? AND user_id=? FOR UPDATE", application, a.ID).Scan(&status, &syncStatus, &endAt, &hiddenAt)
	if errors.Is(e, sql.ErrNoRows) {
		return fault("NOT_FOUND", 404)
	}
	if e != nil {
		return e
	}
	if hiddenAt.Valid {
		return nil
	}
	terminal := contains([]string{"REJECTED", "CANCELLED", "EXPIRED", "TERMINATED"}, status)
	finishedApproval := status == "APPROVED" && !s.Now().Before(endAt)
	if syncStatus != "IDLE" || (!terminal && !finishedApproval) {
		return fault("INVALID_STATE", 409)
	}
	now := s.Now().UTC()
	if _, e = tx.ExecContext(ctx, "UPDATE m2_applications SET hidden_at=? WHERE id=?", now, application); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO m2_events VALUES (?,?,?,?,?,?,?,?,?)", id(), application, a.ID, "HIDE", status, status, "", trace, now); e != nil {
		return e
	}
	return tx.Commit()
}

func approvalAllowed(a Actor, b Application) bool {
	if a.ID == b.UserID {
		return false
	}
	if b.Status == "SUBMITTED" {
		return a.Can("approval.department") && a.Department == b.Department && contains(b.Snapshot.DepartmentApprovers, a.ID)
	}
	return b.Status == "DEPARTMENT_APPROVED" && a.Can("approval.final") && a.Area(b.Area) && contains(b.Snapshot.FinalApprovers, a.ID) && a.ID != b.Snapshot.DepartmentActor
}
func (s *Service) Action(ctx context.Context, a Actor, application string, q ActionRequest, key, trace string) (Application, error) {
	if key == "" || len(key) > 128 || utf8.RuneCountInString(q.Comment) > 500 {
		return Application{}, fault("INVALID_INPUT", 400)
	}
	q.Comment = strings.TrimSpace(q.Comment)
	tx, e := s.begin(ctx)
	if e != nil {
		return Application{}, e
	}
	defer tx.Rollback()
	b, e := scan(tx.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE id=? FOR UPDATE", application))
	if errors.Is(e, sql.ErrNoRows) {
		return b, fault("NOT_FOUND", 404)
	}
	if e != nil {
		return b, e
	}
	if !canRead(a, b) {
		return Application{}, fault("NOT_FOUND", 404)
	}
	var digest string
	e = tx.QueryRowContext(ctx, "SELECT request_hash FROM m2_commands WHERE application_id=? AND actor_id=? AND idem_key=?", b.ID, a.ID, key).Scan(&digest)
	if e == nil {
		if digest != hash(q) {
			return b, fault("IDEMPOTENCY_CONFLICT", 409)
		}
		return b, nil
	}
	if e != sql.ErrNoRows {
		return b, e
	}
	e = nil
	if q.ExpectedVersion != b.Version {
		return b, fault("VERSION_CONFLICT", 409)
	}
	if b.SyncStatus == "PENDING" || b.SyncStatus == "UNKNOWN" || b.SyncStatus == "FAILED" {
		return b, fault("OPERATION_PENDING", 409)
	}
	from := b.Status
	now := s.Now().UTC()
	switch q.Action {
	case "APPROVE", "REJECT":
		if !approvalAllowed(a, b) {
			return b, fault("FORBIDDEN", 403)
		}
		if !now.Before(b.DeadlineAt) {
			return b, fault("DEADLINE_EXPIRED", 409)
		}
		if q.Action == "REJECT" {
			if q.Comment == "" {
				return b, fault("INVALID_INPUT", 400)
			}
			b.Status = "REJECTED"
			_, e = tx.ExecContext(ctx, "UPDATE m2_applications SET decision='REJECTED',decided_at=? WHERE id=?", now, b.ID)
		} else if b.Status == "SUBMITTED" {
			b.Status = "DEPARTMENT_APPROVED"
			b.Snapshot.DepartmentActor = a.ID
		} else {
			e = s.queue(ctx, tx, &b, a.ID, "HOLD_ROOM_INTERVAL", "APPROVED", trace)
		}
	case "CANCEL":
		if a.ID != b.UserID {
			return b, fault("FORBIDDEN", 403)
		}
		if b.Status == "CANCELLED" {
			return b, nil
		}
		if !now.Before(b.StartAt) || (b.Status != "SUBMITTED" && b.Status != "DEPARTMENT_APPROVED" && b.Status != "APPROVED") {
			return b, fault("INVALID_STATE", 409)
		}
		if b.Status == "APPROVED" {
			e = s.queue(ctx, tx, &b, a.ID, "RELEASE_SOURCE", "CANCELLED", trace)
		} else {
			b.Status = "CANCELLED"
		}
	case "TERMINATE":
		if !a.Can("classroom.terminate") || !a.Area(b.Area) {
			return b, fault("FORBIDDEN", 403)
		}
		if b.Status != "APPROVED" || now.Before(b.StartAt) || !now.Before(b.EndAt) || q.Comment == "" {
			return b, fault("INVALID_STATE", 409)
		}
		e = s.queue(ctx, tx, &b, a.ID, "RELEASE_SOURCE", "TERMINATED", trace)
	default:
		return b, fault("INVALID_INPUT", 400)
	}
	if e != nil {
		return b, e
	}
	b.Version++
	if e = s.save(ctx, tx, b); e != nil {
		return b, e
	}
	if e = s.event(ctx, tx, b, a.ID, q.Action, from, q.Comment, trace); e != nil {
		return b, e
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO m2_commands VALUES (?,?,?,?)", b.ID, a.ID, key, hash(q)); e != nil {
		return b, e
	}
	return b, tx.Commit()
}
func (s *Service) queue(ctx context.Context, tx *sql.Tx, b *Application, actor, action, target, trace string) error {
	b.SourceVersion++
	b.OperationID = id()
	b.SyncStatus = "PENDING"
	b.LastError = ""
	q := Operation{OperationID: b.OperationID, Action: action, SourceModule: "M2", SourceBusinessID: b.ID, SourceVersion: b.SourceVersion, RoomID: b.RoomID, Reason: target}
	if action == "HOLD_ROOM_INTERVAL" {
		q.StartAt = &b.StartAt
		q.EndAt = &b.EndAt
		q.DeadlineAt = &b.DeadlineAt
	}
	raw, _ := json.Marshal(q)
	_, e := tx.ExecContext(ctx, "INSERT INTO m2_operations(id,application_id,request,target_status,actor_id,correlation_id,state,next_attempt) VALUES (?,?,?,?,?,?,'PENDING',?)", q.OperationID, b.ID, raw, target, actor, trace, s.Now().UTC())
	return e
}
