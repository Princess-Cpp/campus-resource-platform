package classroom_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/classroom"
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/migrations"
	_ "github.com/go-sql-driver/mysql"
)

// testM2ServiceToken is test-only; runtime tokens must come from M2_SERVICE_TOKEN.
const testM2ServiceToken = "m2-service"

type harness struct {
	t         *testing.T
	db        *sql.DB
	s         *classroom.Service
	resource  *booking.Service
	server    *httptest.Server
	fixture   *dev.Fixture
	directory *dev.ClassroomFixture
	now       time.Time
	students  []classroom.Actor
}

func setup(t *testing.T) *harness {
	t.Helper()
	dsn := os.Getenv("M2_TEST_DSN")
	if dsn == "" {
		t.Skip("M2_TEST_DSN not set; requires isolated MySQL database")
	}
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(20)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	var name string
	if e = db.QueryRow("SELECT DATABASE()").Scan(&name); e != nil {
		t.Fatal(e)
	}
	if name != "campus_m2_check" {
		t.Fatal("tests only run against campus_m2_check")
	}
	if e = migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	// Test-only data cleanup in the isolated test database, never production or preview.
	for _, table := range []string{"m2_commands", "m2_events", "m2_outbox", "m2_operations", "m2_applications", "m1_operations", "m1_sources", "m1_bookings", "m1_audit", "m1_outbox", "m1_violations"} {
		if _, e = db.Exec("DELETE FROM " + table); e != nil {
			t.Fatal(e)
		}
	}
	f, e := dev.Load("../../dev-fixture.json")
	if e != nil {
		t.Fatal(e)
	}
	f.Tokens[testM2ServiceToken] = booking.Actor{ID: testM2ServiceToken, Service: "M2"}
	d, e := dev.LoadClassroom("../../dev-fixture.json")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2030, 1, 10, 0, 0, 0, 0, time.UTC)
	h := &harness{t: t, db: db, fixture: f, directory: d, now: now}
	for _, v := range []struct{ token, id, department string }{{"student2", "student-2", "CS"}, {"outsider", "outsider", "EE"}} {
		f.Tokens[v.token] = booking.Actor{ID: v.id, Department: v.department, Role: "student"}
		d.Tokens[v.token] = classroom.Actor{ID: v.id, Department: v.department, Permissions: []string{"classroom.apply"}, ContactRefs: []string{"campus-email"}}
	}
	d.Tokens["other-dept"] = classroom.Actor{ID: "other-dept", Department: "EE", Permissions: []string{"approval.department"}}
	d.Tokens["other-area"] = classroom.Actor{ID: "other-area", Department: "CS", Permissions: []string{"approval.final", "classroom.stats"}, Areas: []string{"B楼"}}
	h.resource = booking.New(db, f, []byte("test-only-signing-secret-at-least-32"))
	h.resource.Now = func() time.Time { return h.now }
	router := h.resource.Router(f.Auth)
	h.server = httptest.NewServer(router)
	t.Cleanup(h.server.Close)
	h.s = classroom.New(db, d, &classroom.HTTPResources{BaseURL: h.server.URL, Token: testM2ServiceToken})
	h.s.Now = func() time.Time { return h.now }
	h.s.Register(router)
	return h
}
func (h *harness) request(method, path, token, key string, body any) (int, []byte) {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	req, e := http.NewRequest(method, h.server.URL+"/api/v1"+path, bytes.NewReader(raw))
	if e != nil {
		h.t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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
func (h *harness) application(method, path, token, key string, body any) classroom.Application {
	h.t.Helper()
	status, raw := h.request(method, path, token, key, body)
	if status != 200 {
		h.t.Fatalf("%s %s: %d %s", method, path, status, raw)
	}
	var v struct{ Data classroom.Application }
	if e := json.Unmarshal(raw, &v); e != nil {
		h.t.Fatal(e)
	}
	return v.Data
}
func (h *harness) input() classroom.CreateRequest {
	return classroom.CreateRequest{RoomID: "room-101", StartAt: h.now.Add(25 * time.Hour), EndAt: h.now.Add(27 * time.Hour), Purpose: "计算机课程小组讨论与项目汇报", Attendees: 10, ContactRef: "campus-email"}
}
func (h *harness) create(key string) classroom.Application {
	return h.application("POST", "/m2/applications", "student-demo", key, h.input())
}
func (h *harness) action(b classroom.Application, token, action, key string) classroom.Application {
	return h.application("POST", "/m2/applications/"+b.ID+"/actions", token, key, classroom.ActionRequest{Action: action, ExpectedVersion: b.Version, Comment: "符合教室使用要求"})
}
func (h *harness) get(id string) classroom.Application {
	return h.application("GET", "/m2/applications/"+id, "student-demo", "", nil)
}
func (h *harness) code(status int, code, method, path, token, key string, body any) {
	h.t.Helper()
	got, raw := h.request(method, path, token, key, body)
	var v struct{ Error struct{ Code string } }
	json.Unmarshal(raw, &v)
	if got != status || v.Error.Code != code {
		h.t.Fatalf("wanted %d %s got %d %s", status, code, got, raw)
	}
}
func (h *harness) run(b classroom.Application) classroom.Application {
	h.t.Helper()
	if e := h.s.RunOperation(context.Background(), b.OperationID); e != nil {
		h.t.Fatal(e)
	}
	return h.get(b.ID)
}

func TestM2HTTPWorkflowAndScopes(t *testing.T) {
	h := setup(t)
	q := h.input()
	h.code(401, "UNAUTHENTICATED", "POST", "/m2/applications", "", "noauth", q)
	h.code(403, "FORBIDDEN", "POST", "/m2/applications", "operator-demo", "nopermission", q)
	b := h.create("create")
	again := h.create("create")
	if b.ID != again.ID {
		t.Fatal("idempotency created another application")
	}
	q.Purpose = "另一个完全不同的活动申请内容"
	h.code(409, "IDEMPOTENCY_CONFLICT", "POST", "/m2/applications", "student-demo", "create", q)
	h.code(404, "NOT_FOUND", "GET", "/m2/applications/"+b.ID, "student2", "", nil)
	h.code(404, "NOT_FOUND", "GET", "/m2/applications/"+b.ID, "operator-demo", "", nil)
	h.code(404, "NOT_FOUND", "DELETE", "/m2/applications/"+b.ID, "student2", "", nil)
	h.code(409, "INVALID_STATE", "DELETE", "/m2/applications/"+b.ID, "student-demo", "", nil)
	h.code(404, "NOT_FOUND", "POST", "/m2/applications/"+b.ID+"/actions", "other-area", "x", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
	h.code(403, "FORBIDDEN", "POST", "/m2/applications/"+b.ID+"/actions", "student-demo", "self", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
	h.code(403, "FORBIDDEN", "POST", "/m2/applications/"+b.ID+"/actions", "venue-demo", "skip", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
	b = h.action(b, "department-demo", "APPROVE", "department")
	var sources int
	h.db.QueryRow("SELECT COUNT(*) FROM m1_sources WHERE module='M2'").Scan(&sources)
	if sources != 0 {
		t.Fatal("department approval occupied resource")
	}
	b = h.action(b, "venue-demo", "APPROVE", "final")
	if b.Status != "DEPARTMENT_APPROVED" || b.SyncStatus != "PENDING" {
		t.Fatal("approved before receipt")
	}
	h.code(409, "OPERATION_PENDING", "POST", "/m2/applications/"+b.ID+"/actions", "student-demo", "cancel-pending", classroom.ActionRequest{Action: "CANCEL", ExpectedVersion: b.Version})
	b = h.run(b)
	if b.Status != "APPROVED" || b.SyncStatus != "IDLE" {
		t.Fatalf("not approved: %+v", b)
	}
	h.code(409, "RESOURCE_CONFLICT", "POST", "/m1/bookings", "student2", "seat-conflict", map[string]any{"roomId": b.RoomID, "seatId": "seat-101-01", "startAt": b.StartAt, "endAt": b.EndAt})
	// Another source must survive this application's release.
	_, e := h.resource.Operate(context.Background(), booking.Actor{Service: "M3"}, booking.Operation{OperationID: "other-maintenance", Action: "BLOCK_ROOM_MAINTENANCE", SourceModule: "M3", SourceBusinessID: "workorder", SourceVersion: 1, RoomID: b.RoomID}, "other-maintenance", "test")
	if e != nil {
		t.Fatal(e)
	}
	b = h.action(b, "student-demo", "CANCEL", "cancel")
	if b.Status != "APPROVED" || b.SyncStatus != "PENDING" {
		t.Fatal("cancelled before release receipt")
	}
	b = h.run(b)
	if b.Status != "CANCELLED" {
		t.Fatal(b.Status)
	}
	var other int
	h.db.QueryRow("SELECT COUNT(*) FROM m1_sources WHERE module='M3' AND active=1").Scan(&other)
	if other != 1 {
		t.Fatal("released another module's source")
	}
	repeated := h.action(b, "student-demo", "CANCEL", "cancel-again")
	if repeated.Status != "CANCELLED" {
		t.Fatal("cancel retry failed")
	}
	for range 2 {
		status, raw := h.request("DELETE", "/m2/applications/"+b.ID, "student-demo", "", nil)
		if status != 200 {
			t.Fatalf("delete application: %d %s", status, raw)
		}
	}
	status, raw := h.request("GET", "/m2/applications?page=1&pageSize=20", "student-demo", "", nil)
	var page struct{ Data classroom.Page }
	json.Unmarshal(raw, &page)
	if status != 200 || page.Data.Total != 0 || len(page.Data.Items) != 0 {
		t.Fatalf("hidden application remained in owner list: %d %s", status, raw)
	}
	var applicationRows, hideEvents int
	h.db.QueryRow("SELECT COUNT(*) FROM m2_applications WHERE id=? AND hidden_at IS NOT NULL", b.ID).Scan(&applicationRows)
	h.db.QueryRow("SELECT COUNT(*) FROM m2_events WHERE application_id=? AND action='HIDE'", b.ID).Scan(&hideEvents)
	if applicationRows != 1 || hideEvents != 1 {
		t.Fatalf("application rows=%d hide events=%d, want 1 and 1", applicationRows, hideEvents)
	}
	status, raw = h.request("GET", "/m2/stats", "operator-demo", "", nil)
	var stats struct{ Data classroom.Statistics }
	json.Unmarshal(raw, &stats)
	if status != 200 || stats.Data.Approved != 1 || stats.Data.Total != 1 || stats.Data.ApprovalRate == nil || *stats.Data.ApprovalRate != 1 {
		t.Fatalf("statistics %d %s", status, raw)
	}
	h.code(403, "FORBIDDEN", "GET", "/m2/stats", "student-demo", "", nil)
	status, raw = h.request("GET", "/m2/stats", "other-area", "", nil)
	json.Unmarshal(raw, &stats)
	if status != 200 || stats.Data.Total != 0 {
		t.Fatal("statistics scope leaked")
	}
	var events, outbox int
	h.db.QueryRow("SELECT COUNT(*) FROM m2_events WHERE application_id=?", b.ID).Scan(&events)
	h.db.QueryRow("SELECT COUNT(*) FROM m2_outbox WHERE application_id=?", b.ID).Scan(&outbox)
	if events < 6 || outbox < events {
		t.Fatal("missing transactional events")
	}
}

func TestM2ValidationAndConcurrentDecisions(t *testing.T) {
	h := setup(t)
	q := h.input()
	q.Attendees = 31
	h.code(409, "RESOURCE_UNAVAILABLE", "POST", "/m2/applications", "student-demo", "capacity", q)
	q = h.input()
	q.StartAt = q.StartAt.Add(30 * time.Minute)
	h.code(400, "INVALID_INPUT", "POST", "/m2/applications", "student-demo", "nonhour", q)
	q = h.input()
	q.ContactRef = "someone-else"
	h.code(400, "INVALID_INPUT", "POST", "/m2/applications", "student-demo", "contact", q)
	b := h.create("concurrent")
	path := "/m2/applications/" + b.ID + "/actions"
	h.code(400, "INVALID_INPUT", "POST", path, "department-demo", "empty-reject", classroom.ActionRequest{Action: "REJECT", ExpectedVersion: b.Version})
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			status, _ := h.request("POST", path, "department-demo", fmt.Sprintf("concurrent-%d", i), classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
			statuses <- status
		}(i)
	}
	wg.Wait()
	close(statuses)
	ok, conflict := 0, 0
	for status := range statuses {
		if status == 200 {
			ok++
		}
		if status == 409 {
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("race: %d successes, %d conflicts", ok, conflict)
	}
	b = h.get(b.ID)
	b = h.action(b, "venue-demo", "REJECT", "reject")
	if b.Status != "REJECTED" {
		t.Fatal(b.Status)
	}
	h.code(403, "FORBIDDEN", "POST", path, "venue-demo", "resurrect", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
}

func TestM2ConflictExpiryAndRestart(t *testing.T) {
	h := setup(t)
	b := h.create("conflict")
	q := h.input()
	h.application("POST", "/m1/bookings", "student2", "occupied", map[string]any{"roomId": q.RoomID, "seatId": "seat-101-01", "startAt": q.StartAt, "endAt": q.EndAt})
	b = h.action(b, "department-demo", "APPROVE", "dep")
	b = h.action(b, "venue-demo", "APPROVE", "final")
	b = h.run(b)
	if b.Status != "DEPARTMENT_APPROVED" || b.LastError != "RESOURCE_CONFLICT" || b.SyncStatus != "IDLE" {
		t.Fatalf("conflict: %+v", b)
	}
	h.now = b.DeadlineAt
	if e := h.s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	b = h.get(b.ID)
	if b.Status != "EXPIRED" {
		t.Fatal("deadline not enforced")
	}
	var count int
	h.db.QueryRow("SELECT COUNT(*) FROM m2_events WHERE application_id=? AND action='EXPIRE'", b.ID).Scan(&count)
	if e := h.s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	var after int
	h.db.QueryRow("SELECT COUNT(*) FROM m2_events WHERE application_id=? AND action='EXPIRE'", b.ID).Scan(&after)
	if count != 1 || after != 1 {
		t.Fatal("expiry duplicated")
	}
}

// A real HTTP proxy drops only the successful POST response. Resource mutation still runs.
func TestM2LostResponseRecoversAfterRestart(t *testing.T) {
	h := setup(t)
	b := h.create("lost-response")
	b = h.action(b, "department-demo", "APPROVE", "dep")
	b = h.action(b, "venue-demo", "APPROVE", "final")
	dropped := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, _ := http.NewRequestWithContext(r.Context(), r.Method, h.server.URL+r.URL.RequestURI(), r.Body)
		req.Header = r.Header.Clone()
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Error(e)
			w.WriteHeader(503)
			return
		}
		defer res.Body.Close()
		if r.Method == "POST" && !dropped {
			dropped = true
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		var buf bytes.Buffer
		buf.ReadFrom(res.Body)
		w.Write(buf.Bytes())
	}))
	defer proxy.Close()
	h.s.Resources = &classroom.HTTPResources{BaseURL: proxy.URL, Token: testM2ServiceToken}
	b = h.run(b)
	if b.Status == "APPROVED" || b.SyncStatus != "UNKNOWN" {
		t.Fatal("lost response reported success")
	}
	// Advance beyond deadline. Persisted pre-deadline receipt still decides approval.
	h.now = b.DeadlineAt.Add(time.Hour)
	restarted := classroom.New(h.db, h.directory, &classroom.HTTPResources{BaseURL: h.server.URL, Token: testM2ServiceToken})
	restarted.Now = func() time.Time { return h.now }
	if e := restarted.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	b = h.get(b.ID)
	if b.Status != "APPROVED" || b.SyncStatus != "IDLE" {
		t.Fatalf("restart: %+v", b)
	}
	var sources int
	h.db.QueryRow("SELECT COUNT(*) FROM m1_sources WHERE module='M2' AND business_id=?", b.ID).Scan(&sources)
	if sources != 1 {
		t.Fatal("duplicated source")
	}
}

func TestM2ConcurrentCreateAndBookingRace(t *testing.T) {
	h := setup(t)
	var wg sync.WaitGroup
	results := make(chan classroom.Application, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- h.create("same-key") }()
	}
	wg.Wait()
	close(results)
	ids := map[string]bool{}
	for b := range results {
		ids[b.ID] = true
	}
	if len(ids) != 1 {
		t.Fatal("concurrent create duplicated application")
	}
	var b classroom.Application
	for application := range ids {
		b = h.get(application)
	}
	b = h.action(b, "department-demo", "APPROVE", "dep")
	b = h.action(b, "venue-demo", "APPROVE", "final")
	var bookingStatus int
	var resourceErr error
	wg.Add(2)
	go func() { defer wg.Done(); resourceErr = h.s.RunOperation(context.Background(), b.OperationID) }()
	go func() {
		defer wg.Done()
		bookingStatus, _ = h.request("POST", "/m1/bookings", "student2", "race-seat", map[string]any{"roomId": b.RoomID, "seatId": "seat-101-01", "startAt": b.StartAt, "endAt": b.EndAt})
	}()
	wg.Wait()
	if resourceErr != nil {
		t.Fatal(resourceErr)
	}
	b = h.get(b.ID)
	if !((bookingStatus == 200 && b.Status == "DEPARTMENT_APPROVED" && b.LastError == "RESOURCE_CONFLICT") || (bookingStatus == 409 && b.Status == "APPROVED")) {
		t.Fatalf("double occupancy or lost result: booking=%d application=%+v", bookingStatus, b)
	}
	var active int
	if e := h.db.QueryRow("SELECT COUNT(*) FROM m1_sources WHERE active=1").Scan(&active); e != nil {
		t.Fatal(e)
	}
	if active != 1 {
		t.Fatalf("active sources: %d", active)
	}
}

func TestM2SelfApprovalAndScopeAreServerEnforced(t *testing.T) {
	h := setup(t)
	owner := h.directory.Tokens["student-demo"]
	owner.Permissions = append(owner.Permissions, "approval.department", "approval.final")
	owner.Areas = []string{"A楼"}
	h.directory.Tokens["student-demo"] = owner
	b := h.create("self-scope")
	h.code(403, "FORBIDDEN", "POST", "/m2/applications/"+b.ID+"/actions", "student-demo", "self", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
	h.code(404, "NOT_FOUND", "GET", "/m2/applications/"+b.ID, "other-dept", "", nil)
	// A formerly valid approver loses current scope after assignment was frozen.
	actor := h.directory.Tokens["department-demo"]
	actor.Department = "EE"
	h.directory.Tokens["department-demo"] = actor
	h.code(404, "NOT_FOUND", "POST", "/m2/applications/"+b.ID+"/actions", "department-demo", "scope", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
	status, raw := h.request("GET", "/m2/approvals", "department-demo", "", nil)
	var result struct{ Data classroom.Page }
	json.Unmarshal(raw, &result)
	if status != 200 || result.Data.Total != 0 {
		t.Fatalf("out-of-scope inbox: %d %s", status, raw)
	}
}

func TestM2ReleaseUnavailableAndRetryLimit(t *testing.T) {
	h := setup(t)
	b := h.create("release-lost")
	b = h.action(b, "department-demo", "APPROVE", "dep")
	b = h.action(b, "venue-demo", "APPROVE", "final")
	b = h.run(b)
	b = h.action(b, "student-demo", "CANCEL", "cancel")
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer unavailable.Close()
	h.s.Resources = &classroom.HTTPResources{BaseURL: unavailable.URL, Token: testM2ServiceToken}
	for i := 0; i < 8; i++ {
		b = h.run(b)
	}
	if b.Status != "APPROVED" || b.SyncStatus != "FAILED" {
		t.Fatalf("unconfirmed release became terminal: %+v", b)
	}
	var active bool
	if e := h.db.QueryRow("SELECT active FROM m1_sources WHERE module='M2' AND business_id=?", b.ID).Scan(&active); e != nil || !active {
		t.Fatal("source unexpectedly released")
	}
	original := b.OperationID
	b = h.application("POST", "/m2/applications/"+b.ID+"/retry", "student-demo", "", map[string]any{"expectedVersion": b.Version})
	if b.OperationID != original {
		t.Fatal("retry changed operation identity")
	}
	h.s.Resources = &classroom.HTTPResources{BaseURL: h.server.URL, Token: testM2ServiceToken}
	b = h.run(b)
	if b.Status != "CANCELLED" {
		t.Fatal("release did not recover")
	}
}

func TestM2ApprovalDeadlineAndTermination(t *testing.T) {
	h := setup(t)
	b := h.create("deadline")
	b = h.action(b, "department-demo", "APPROVE", "dep")
	h.now = b.DeadlineAt
	h.code(409, "DEADLINE_EXPIRED", "POST", "/m2/applications/"+b.ID+"/actions", "venue-demo", "late", classroom.ActionRequest{Action: "APPROVE", ExpectedVersion: b.Version})
	if e := h.s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	if h.get(b.ID).Status != "EXPIRED" {
		t.Fatal("not expired at deadline")
	}
	b = h.create("terminate")
	b = h.action(b, "department-demo", "APPROVE", "dep2")
	b = h.action(b, "venue-demo", "APPROVE", "final2")
	b = h.run(b)
	h.now = b.StartAt
	h.code(409, "INVALID_STATE", "POST", "/m2/applications/"+b.ID+"/actions", "student-demo", "late-cancel", classroom.ActionRequest{Action: "CANCEL", ExpectedVersion: b.Version})
	h.code(409, "INVALID_STATE", "POST", "/m2/applications/"+b.ID+"/actions", "venue-demo", "empty-terminate", classroom.ActionRequest{Action: "TERMINATE", ExpectedVersion: b.Version})
	b = h.action(b, "venue-demo", "TERMINATE", "terminate")
	b = h.run(b)
	if b.Status != "TERMINATED" {
		t.Fatal("not terminated")
	}
}

func TestM2AuditFailureRollsBackBusinessAndOutbox(t *testing.T) {
	h := setup(t)
	if _, e := h.db.Exec("CREATE TRIGGER m2_test_audit_failure BEFORE INSERT ON m2_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test audit failure'"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.db.Exec("DROP TRIGGER IF EXISTS m2_test_audit_failure") })
	h.code(500, "INTERNAL_ERROR", "POST", "/m2/applications", "student-demo", "audit-fails", h.input())
	for _, table := range []string{"m2_applications", "m2_events", "m2_outbox"} {
		var count int
		if e := h.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); e != nil {
			t.Fatal(e)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after rollback", table, count)
		}
	}
	if _, e := h.db.Exec("DROP TRIGGER m2_test_audit_failure"); e != nil {
		t.Fatal(e)
	}
	if b := h.create("audit-fails"); b.Status != "SUBMITTED" {
		t.Fatal("retry after rollback failed")
	}
}

func TestM2MissingReceiptAfterDeadlineDoesNotApprove(t *testing.T) {
	h := setup(t)
	b := h.create("pending-expired")
	b = h.action(b, "department-demo", "APPROVE", "dep")
	b = h.action(b, "venue-demo", "APPROVE", "final")
	h.now = b.DeadlineAt
	if e := h.s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	b = h.get(b.ID)
	if b.Status != "EXPIRED" || b.SyncStatus != "IDLE" {
		t.Fatalf("expired pending hold: %+v", b)
	}
	var count int
	if e := h.db.QueryRow("SELECT COUNT(*) FROM m1_sources WHERE module='M2' AND active=1").Scan(&count); e != nil {
		t.Fatal(e)
	}
	if count != 0 {
		t.Fatal("late operation occupied room")
	}
}
