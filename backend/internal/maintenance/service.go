package maintenance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func readableRoom(a domain.Actor, r Room) bool {
	return a.Enabled && a.ID != "" && (r.Department == "" || r.Department == a.Department || a.InArea(r.Area))
}
func (s *Service) Rooms(ctx context.Context, a domain.Actor) ([]Room, error) {
	if !a.Can("workorder.report") {
		return nil, fault("FORBIDDEN", 403)
	}
	if s.Catalog == nil {
		return nil, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	rooms, e := s.Catalog.Rooms(ctx)
	if e != nil {
		return nil, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	out := []Room{}
	for _, r := range rooms {
		if readableRoom(a, r) && (r.Status == "ACTIVE" || r.Status == "DISABLED") {
			if r.Seats == nil {
				r.Seats = []Seat{}
			}
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *Service) Report(ctx context.Context, a domain.Actor, q ReportInput, key, trace string) (Order, error) {
	if !a.Can("workorder.report") {
		return Order{}, fault("FORBIDDEN", 403)
	}
	if !keyPattern.MatchString(key) || len(q.RoomID) > 64 || len(q.SeatID) > 64 {
		return Order{}, fault("INVALID_INPUT", 400)
	}
	q.Description = strings.TrimSpace(q.Description)
	if q.Severity == "" {
		q.Severity = "NORMAL"
	}
	canonical, _ := json.Marshal(struct {
		Action string
		Input  ReportInput
	}{"REPORT", q})
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])
	// A committed command is historical data owned by this authenticated actor.
	// A later room rename/retirement must not turn response-loss recovery into a
	// new report. Current report permission is still checked above on every call.
	var previousHash string
	var previous []byte
	e := s.DB.QueryRowContext(ctx, "SELECT request_hash,response FROM m3_commands WHERE actor_id=? AND idem_key=?", a.ID, key).Scan(&previousHash, &previous)
	if e == nil {
		if previousHash != hash {
			return Order{}, fault("IDEMPOTENCY_CONFLICT", 409)
		}
		if len(previous) == 0 {
			return Order{}, fault("DEPENDENCY_UNAVAILABLE", 503)
		}
		var result Order
		if e = json.Unmarshal(previous, &result); e != nil {
			return Order{}, e
		}
		return result, nil
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return Order{}, e
	}
	rooms, e := s.Rooms(ctx, a)
	if e != nil {
		return Order{}, e
	}
	var room *Room
	for i := range rooms {
		if rooms[i].ID == q.RoomID {
			room = &rooms[i]
			break
		}
	}
	if room == nil {
		return Order{}, fault("NOT_FOUND", 404)
	}
	seatIDs := []string{}
	seatLabel := ""
	for _, seat := range room.Seats {
		seatIDs = append(seatIDs, seat.ID)
		if seat.ID == q.SeatID {
			seatLabel = seat.Label
		}
	}
	now := s.Now().UTC().Truncate(time.Microsecond)
	change, e := domain.New("WO-"+ID(), a, domain.Room{ID: room.ID, Name: room.Name, Area: room.Area, Department: room.Department, Status: room.Status, SeatIDs: seatIDs}, domain.Report{RoomID: q.RoomID, SeatID: q.SeatID, Category: q.Category, Description: q.Description, Severity: q.Severity}, domain.ProposedPolicy(), now)
	if e != nil {
		if errors.Is(e, domain.ErrForbidden) {
			return Order{}, fault("FORBIDDEN", 403)
		}
		return Order{}, fault("INVALID_INPUT", 400)
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return Order{}, e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO m3_commands(actor_id,idem_key,request_hash,created_at) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE idem_key=idem_key`, a.ID, key, hash, now)
	if e != nil {
		return Order{}, e
	}
	var storedHash string
	var saved []byte
	if e = tx.QueryRowContext(ctx, "SELECT request_hash,response FROM m3_commands WHERE actor_id=? AND idem_key=? FOR UPDATE", a.ID, key).Scan(&storedHash, &saved); e != nil {
		return Order{}, e
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
	raw, e := json.Marshal(change.Order)
	if e != nil {
		return Order{}, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO m3_workorders(id,reporter_id,room_id,area,category,state,version,snapshot,seat_label,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, change.Order.ID, a.ID, room.ID, room.Area, q.Category, change.Order.State, change.Order.Version, raw, seatLabel, now, now)
	if e != nil {
		return Order{}, e
	}
	if s.Events == nil {
		return Order{}, fault("DEPENDENCY_UNAVAILABLE", 503)
	}
	if e = s.Events.Append(ctx, tx, change, ID(), trace); e != nil {
		return Order{}, e
	}
	result := dto(change.Order, seatLabel)
	response, e := json.Marshal(result)
	if e != nil {
		return Order{}, e
	}
	if _, e = tx.ExecContext(ctx, "UPDATE m3_commands SET response=? WHERE actor_id=? AND idem_key=?", response, a.ID, key); e != nil {
		return Order{}, e
	}
	return result, tx.Commit()
}
