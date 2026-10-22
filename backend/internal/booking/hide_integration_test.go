package booking_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/migrations"
	_ "github.com/go-sql-driver/mysql"
)

type hideHarness struct {
	t      *testing.T
	db     *sql.DB
	s      *booking.Service
	server *httptest.Server
	now    time.Time
}

func setupHide(t *testing.T) *hideHarness {
	t.Helper()
	dsn := os.Getenv("M1_TEST_DSN")
	if dsn == "" {
		t.Skip("M1_TEST_DSN not set; requires isolated MySQL database")
	}
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	var name string
	if e = db.QueryRow("SELECT DATABASE()").Scan(&name); e != nil {
		t.Fatal(e)
	}
	if name != "campus_m1_check" {
		t.Fatal("tests only run against campus_m1_check")
	}
	if e = migrations.Apply(context.Background(), db); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"m1_sources", "m1_bookings", "m1_audit", "m1_outbox", "m1_violations"} {
		if _, e = db.Exec("DELETE FROM " + table); e != nil {
			t.Fatal(e)
		}
	}
	f, e := dev.Load("../../dev-fixture.json")
	if e != nil {
		t.Fatal(e)
	}
	f.Tokens["student2"] = booking.Actor{ID: "student-2", Department: "CS", Role: "student"}
	h := &hideHarness{t: t, db: db, now: time.Date(2030, 1, 10, 0, 0, 0, 0, time.UTC)}
	h.s = booking.New(db, f, []byte("test-only-signing-secret-at-least-32"))
	h.s.Now = func() time.Time { return h.now }
	h.server = httptest.NewServer(h.s.Router(f.Auth))
	t.Cleanup(h.server.Close)
	return h
}

func (h *hideHarness) call(method, path, token, key string, body any) (int, []byte) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	req, e := http.NewRequest(method, h.server.URL+"/api/v1"+path, bytes.NewReader(raw))
	if e != nil {
		h.t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		h.t.Fatal(e)
	}
	defer res.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(res.Body)
	return res.StatusCode, b.Bytes()
}

func (h *hideHarness) book(seat string, startHour int, key string) string {
	h.t.Helper()
	start := time.Date(2030, 1, 10, startHour, 0, 0, 0, time.UTC)
	status, raw := h.call("POST", "/m1/bookings", "student-demo", key, booking.BookRequest{RoomID: "room-101", SeatID: seat, StartAt: start, EndAt: start.Add(time.Hour)})
	if status != 200 {
		h.t.Fatalf("book status %d: %s", status, raw)
	}
	var v struct{ Data booking.Booking }
	json.Unmarshal(raw, &v)
	return v.Data.ID
}

func (h *hideHarness) listed(id string) bool {
	h.t.Helper()
	status, raw := h.call("GET", "/m1/bookings", "student-demo", "", nil)
	if status != 200 {
		h.t.Fatalf("list status %d: %s", status, raw)
	}
	var v struct{ Data []booking.Booking }
	json.Unmarshal(raw, &v)
	for _, b := range v.Data {
		if b.ID == id {
			return true
		}
	}
	return false
}

func (h *hideHarness) expect(wantStatus int, wantCode, id, token string) {
	h.t.Helper()
	status, raw := h.call("DELETE", "/m1/bookings/"+id, token, "", nil)
	var v struct{ Error struct{ Code string } }
	json.Unmarshal(raw, &v)
	if status != wantStatus || v.Error.Code != wantCode {
		h.t.Fatalf("delete %s as %s = %d %q, want %d %q", id, token, status, v.Error.Code, wantStatus, wantCode)
	}
}

func TestHideBookingKeepsRecordsAndProtectsActiveOrViolations(t *testing.T) {
	h := setupHide(t)
	cancelled := h.book("seat-101-01", 2, "cancelled")
	if status, raw := h.call("POST", "/m1/bookings/"+cancelled+"/cancel", "student-demo", "", nil); status != 200 {
		t.Fatalf("cancel status %d: %s", status, raw)
	}
	reserved := h.book("seat-101-02", 2, "reserved")
	noShow := h.book("seat-101-01", 4, "no-show")

	h.expect(404, "NOT_FOUND", cancelled, "student2")
	h.expect(409, "INVALID_STATE", reserved, "student-demo")
	if !h.listed(reserved) {
		t.Fatal("active booking must stay listed after rejected delete")
	}

	h.expect(200, "", cancelled, "student-demo")
	h.expect(200, "", cancelled, "student-demo")
	if h.listed(cancelled) {
		t.Fatal("deleted booking still listed")
	}
	var rows, audits int
	h.db.QueryRow("SELECT COUNT(*) FROM m1_bookings WHERE id=?", cancelled).Scan(&rows)
	h.db.QueryRow("SELECT COUNT(*) FROM m1_audit WHERE business_id=? AND action='HIDE'", cancelled).Scan(&audits)
	if rows != 1 || audits != 1 {
		t.Fatalf("booking rows=%d hide audits=%d, want 1 and 1", rows, audits)
	}

	h.now = time.Date(2030, 1, 10, 4, 15, 0, 0, time.UTC)
	if e := h.s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	h.expect(409, "INVALID_STATE", noShow, "student-demo")
	if !h.listed(noShow) {
		t.Fatal("no-show record must stay visible")
	}
}
