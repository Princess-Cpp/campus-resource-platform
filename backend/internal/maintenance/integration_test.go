package maintenance_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
	maintenancedev "campus-resource-platform/backend/internal/maintenance/dev"
	"campus-resource-platform/backend/migrations"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

type harness struct {
	t       *testing.T
	db      *sql.DB
	auth    *identity.Store
	catalog *maintenancedev.Catalog
	service *maintenance.Service
	router  *gin.Engine
	server  *httptest.Server
	a, b    identity.Session
}

func setup(t *testing.T) *harness {
	t.Helper()
	dsn := os.Getenv("M3_TEST_DSN")
	if dsn == "" {
		t.Skip("M3_TEST_DSN not set; requires isolated campus_m3_check")
	}
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(24)
	t.Cleanup(func() { db.Close() })
	var database string
	if e = db.QueryRow("SELECT DATABASE()").Scan(&database); e != nil {
		t.Fatal(e)
	}
	if database != "campus_m3_check" {
		t.Fatal("refusing cleanup outside campus_m3_check")
	}
	if e = migrations.Apply(context.Background(), db); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"m3_commands", "m3_outbox", "m3_events", "m3_workorders"} {
		if _, e = db.Exec("DELETE FROM " + table); e != nil {
			t.Fatal(e)
		}
	}
	h := &harness{t: t, db: db, auth: identity.New(db, false), catalog: &maintenancedev.Catalog{Data: []maintenance.Room{
		{ID: "room-a", Name: "A101", Area: "A楼", Department: "CS", Status: "ACTIVE", Seats: []maintenance.Seat{{ID: "seat-a", Label: "A01"}}},
		{ID: "disabled", Name: "停用教室", Area: "A楼", Department: "CS", Status: "DISABLED", Seats: []maintenance.Seat{}},
		{ID: "other-department", Name: "其他院系", Area: "B楼", Department: "EE", Status: "ACTIVE"},
		{ID: "retired", Name: "注销教室", Area: "A楼", Department: "CS", Status: "RETIRED"},
	}}}
	for i, target := range []*identity.Session{&h.a, &h.b} {
		*target, e = h.auth.Register(context.Background(), "M3-"+maintenance.ID(), fmt.Sprintf("虚构报修人%d", i), "test-password-at-least-ten", "127.0.0.1")
		if e != nil {
			t.Fatal(e)
		}
	}
	h.start()
	t.Cleanup(func() { h.server.Close() })
	return h
}
func (h *harness) start() {
	h.service = maintenance.New(h.db, h.catalog, maintenancedev.Events{})
	h.service.Development = true
	h.service.Now = func() time.Time { return time.Date(2030, 1, 1, 2, 0, 0, 0, time.UTC) }
	h.router = gin.New()
	h.router.Use((&identity.HTTP{Store: h.auth}).CSRFMiddleware())
	h.service.Register(h.router, maintenancedev.Authenticator(h.auth, true))
	h.server = httptest.NewServer(h.router)
}
func input() maintenance.ReportInput {
	return maintenance.ReportInput{RoomID: "room-a", SeatID: "seat-a", Category: "LIGHTING", Description: "教室前排照明灯持续闪烁无法正常使用", Severity: "NORMAL"}
}
func (h *harness) request(method, path, key string, session identity.Session, body any) (int, []byte) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	r, _ := http.NewRequest(method, h.server.URL+"/api/v1/m3"+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Correlation-Id", "m3-check")
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	if session.Token != "" {
		r.AddCookie(&http.Cookie{Name: identity.CookieName, Value: session.Token})
		r.Header.Set("X-CSRF-Token", session.CSRFToken)
	}
	response, e := h.server.Client().Do(r)
	if e != nil {
		h.t.Fatal(e)
	}
	defer response.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(response.Body)
	return response.StatusCode, buf.Bytes()
}
func decode[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var body struct {
		Data T `json:"data"`
	}
	if e := json.Unmarshal(raw, &body); e != nil {
		t.Fatal(e)
	}
	return body.Data
}
func (h *harness) count(table string) int {
	h.t.Helper()
	var n int
	if e := h.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); e != nil {
		h.t.Fatal(e)
	}
	return n
}

func TestReportingPersistsAndMigrationReplays(t *testing.T) {
	h := setup(t)
	code, raw := h.request("POST", "/workorders", "report", h.a, input())
	if code != 201 {
		t.Fatalf("%d %s", code, raw)
	}
	o := decode[maintenance.Order](t, raw)
	if o.ID == "" || o.ReporterID != h.a.Principal.ID || o.SeatLabel != "A01" || o.State != "REPORTED" || o.SyncState != "IDLE" {
		t.Fatalf("bad order %+v", o)
	}
	for _, table := range []string{"m3_workorders", "m3_commands", "m3_events", "m3_outbox"} {
		if h.count(table) != 1 {
			t.Fatal(table)
		}
	}
	if e := migrations.Apply(context.Background(), h.db); e != nil {
		t.Fatal(e)
	}
	h.server.Close()
	h.start()
	code, raw = h.request("GET", "/workorders/"+o.ID, "", h.a, nil)
	d := decode[maintenance.Detail](t, raw)
	if code != 200 || d.ID != o.ID || len(d.Timeline) != 1 || d.Description != input().Description {
		t.Fatalf("not persisted: %d %s", code, raw)
	}
	var version int
	if e := h.db.QueryRow("SELECT COUNT(*) FROM m1_schema_migrations WHERE version='006'").Scan(&version); e != nil || version != 1 {
		t.Fatal("migration replay duplicated or failed", e)
	}
}
func TestAuthenticationCSRFAndBodyCannotSpoofIdentity(t *testing.T) {
	h := setup(t)
	code, _ := h.request("POST", "/workorders", "anonymous", identity.Session{}, input())
	if code != 401 {
		t.Fatal(code)
	}
	noCSRF := h.a
	noCSRF.CSRFToken = ""
	code, _ = h.request("POST", "/workorders", "csrf", noCSRF, input())
	if code != 403 {
		t.Fatal(code)
	}
	code, _ = h.request("POST", "/workorders", "spoof", h.a, map[string]any{"roomId": "room-a", "category": "LIGHTING", "description": input().Description, "reporterId": h.b.Principal.ID})
	if code != 400 {
		t.Fatal(code)
	}
	r, _ := http.NewRequest("POST", h.server.URL+"/api/v1/m3/workorders", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer "+h.a.Token)
	response, e := h.server.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("session bearer accepted")
	}
	if h.count("m3_workorders") != 0 {
		t.Fatal("invalid request wrote data")
	}
}
func TestRoomAccessAndReportValidation(t *testing.T) {
	h := setup(t)
	code, raw := h.request("GET", "/rooms", "", h.a, nil)
	rooms := decode[[]maintenance.Room](t, raw)
	if code != 200 || len(rooms) != 2 {
		t.Fatalf("catalog leaked or omitted rooms %d %s", code, raw)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*maintenance.ReportInput)
		want   int
	}{
		{"other department", func(q *maintenance.ReportInput) { q.RoomID = "other-department"; q.SeatID = "" }, 404},
		{"retired", func(q *maintenance.ReportInput) { q.RoomID = "retired"; q.SeatID = "" }, 404},
		{"seat mismatch", func(q *maintenance.ReportInput) { q.SeatID = "other-seat" }, 400},
		{"short", func(q *maintenance.ReportInput) { q.Description = "太短" }, 400},
		{"category", func(q *maintenance.ReportInput) { q.Category = "INVALID" }, 400},
		{"disabled allowed", func(q *maintenance.ReportInput) { q.RoomID = "disabled"; q.SeatID = "" }, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := input()
			tc.mutate(&q)
			status, body := h.request("POST", "/workorders", maintenance.ID(), h.a, q)
			if status != tc.want {
				t.Fatalf("%d %s", status, body)
			}
		})
	}
	if h.count("m3_workorders") != 1 {
		t.Fatal("invalid reports persisted")
	}
}
func TestIdempotentConcurrentReportsAndChangedContent(t *testing.T) {
	h := setup(t)
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	codes := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, raw := h.request("POST", "/workorders", "concurrent-key", h.a, input())
			codes <- code
			ids <- decode[maintenance.Order](t, raw).ID
		}()
	}
	wg.Wait()
	close(ids)
	close(codes)
	for code := range codes {
		if code != 201 {
			t.Fatal(code)
		}
	}
	seen := map[string]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 || h.count("m3_workorders") != 1 || h.count("m3_events") != 1 || h.count("m3_outbox") != 1 {
		t.Fatal("duplicated concurrent command")
	}
	q := input()
	q.Description = "另一份不同的真实报修不应覆盖原来的工单"
	code, _ := h.request("POST", "/workorders", "concurrent-key", h.a, q)
	if code != 409 {
		t.Fatal(code)
	}
	code, _ = h.request("POST", "/workorders", "new-report", h.a, input())
	if code != 201 || h.count("m3_workorders") != 2 {
		t.Fatal("separate report was merged")
	}
	code, _ = h.request("POST", "/workorders", "concurrent-key", h.b, input())
	if code != 201 || h.count("m3_workorders") != 3 {
		t.Fatal("different actors shared key")
	}
}
func TestOwnerQueriesFiltersAndPagination(t *testing.T) {
	h := setup(t)
	var id string
	for i := 0; i < 3; i++ {
		_, raw := h.request("POST", "/workorders", fmt.Sprint(i), h.a, input())
		id = decode[maintenance.Order](t, raw).ID
	}
	code, _ := h.request("GET", "/workorders/"+id, "", h.b, nil)
	if code != 404 {
		t.Fatal("other user read detail", code)
	}
	_, raw := h.request("GET", "/workorders", "", h.b, nil)
	if decode[maintenance.Page](t, raw).Total != 0 {
		t.Fatal("other user list leaked")
	}
	_, raw = h.request("GET", "/workorders?pageSize=2", "", h.a, nil)
	first := decode[maintenance.Page](t, raw)
	_, raw = h.request("GET", "/workorders?pageSize=2&page=2", "", h.a, nil)
	second := decode[maintenance.Page](t, raw)
	seen := map[string]bool{}
	for _, o := range append(first.Items, second.Items...) {
		seen[o.ID] = true
	}
	if first.Total != 3 || len(first.Items) != 2 || len(second.Items) != 1 || len(seen) != 3 {
		t.Fatal("bad stable pagination")
	}
	for _, path := range []string{"/workorders?category=POWER", "/workorders?state=COMPLETED", "/workorders?to=2030-01-01T02:00:00Z"} {
		_, raw = h.request("GET", path, "", h.a, nil)
		if decode[maintenance.Page](t, raw).Total != 0 {
			t.Fatal(path)
		}
	}
	for _, path := range []string{"/workorders?userId=someone", "/workorders?view=dispatch", "/workorders?page=0", "/workorders?pageSize=101", "/workorders?state=INVALID", "/workorders?from=tomorrow", "/workorders?page=1&page=2"} {
		code, _ = h.request("GET", path, "", h.a, nil)
		if code != 400 {
			t.Fatal(path, code)
		}
	}
}
func TestEventFailureRollsBackEntireReport(t *testing.T) {
	h := setup(t)
	_, e := h.db.Exec("CREATE TRIGGER m3_test_outbox_failure BEFORE INSERT ON m3_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test failure'")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.db.Exec("DROP TRIGGER IF EXISTS m3_test_outbox_failure") })
	code, _ := h.request("POST", "/workorders", "rollback", h.a, input())
	if code != 500 {
		t.Fatal(code)
	}
	for _, table := range []string{"m3_workorders", "m3_commands", "m3_events", "m3_outbox"} {
		if h.count(table) != 0 {
			t.Fatal("partial transaction", table)
		}
	}
	h.db.Exec("DROP TRIGGER m3_test_outbox_failure")
	code, _ = h.request("POST", "/workorders", "rollback", h.a, input())
	if code != 201 || h.count("m3_workorders") != 1 {
		t.Fatal("failed command did not roll back")
	}
}
func TestRevocationAndDisableApplyToReplay(t *testing.T) {
	h := setup(t)
	h.request("POST", "/workorders", "replay", h.a, input())
	if _, e := h.db.Exec("DELETE FROM d_user_roles WHERE user_id=?", h.a.Principal.ID); e != nil {
		t.Fatal(e)
	}
	code, _ := h.request("POST", "/workorders", "replay", h.a, input())
	if code != 403 {
		t.Fatal("revoked permission replayed", code)
	}
	if _, e := h.db.Exec("UPDATE d_users SET active=0 WHERE id=?", h.a.Principal.ID); e != nil {
		t.Fatal(e)
	}
	code, _ = h.request("GET", "/workorders", "", h.a, nil)
	if code != 401 {
		t.Fatal("disabled account read", code)
	}
	if h.count("m3_workorders") != 1 {
		t.Fatal("revocation changed original")
	}
}
func TestLostHTTPReplyReconcilesAfterServiceRecreation(t *testing.T) {
	h := setup(t)
	var lost atomic.Bool
	dropping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.router.ServeHTTP(rec, r)
		if !lost.Swap(true) {
			conn, _, e := w.(http.Hijacker).Hijack()
			if e == nil {
				conn.Close()
			}
			return
		}
		for name, values := range rec.Header() {
			w.Header()[name] = values
		}
		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes())
	}))
	defer dropping.Close()
	raw, _ := json.Marshal(input())
	r, _ := http.NewRequest("POST", dropping.URL+"/api/v1/m3/workorders", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", "lost-reply")
	r.Header.Set("X-CSRF-Token", h.a.CSRFToken)
	r.AddCookie(&http.Cookie{Name: identity.CookieName, Value: h.a.Token})
	response, e := dropping.Client().Do(r)
	if response != nil {
		response.Body.Close()
	}
	if e == nil {
		t.Fatal("reply was not dropped")
	}
	if h.count("m3_workorders") != 1 {
		t.Fatal("write not committed before disconnect")
	}
	h.server.Close()
	h.start()
	code, body := h.request("POST", "/workorders", "lost-reply", h.a, input())
	if code != 201 || decode[maintenance.Order](t, body).ID == "" || h.count("m3_workorders") != 1 || h.count("m3_outbox") != 1 {
		t.Fatal("replay after restart duplicated")
	}
}

func TestReplaySurvivesLaterRoomRetirement(t *testing.T) {
	h := setup(t)
	_, raw := h.request("POST", "/workorders", "historical", h.a, input())
	original := decode[maintenance.Order](t, raw)
	h.catalog.Data[0].Status = "RETIRED"
	code, raw := h.request("POST", "/workorders", "historical", h.a, input())
	if code != 201 || decode[maintenance.Order](t, raw).ID != original.ID {
		t.Fatal("historical command lost after room retirement", code)
	}
	code, _ = h.request("POST", "/workorders", "new-after-retirement", h.a, input())
	if code != 404 || h.count("m3_workorders") != 1 {
		t.Fatal("retired room accepted a new report")
	}
}
