package maintenance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/dev"
	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
)

type repairHarness struct {
	*harness
	m1        *httptest.Server
	m1catalog *dev.Fixture
	resource  *maintenance.HTTPResources
	mode      atomic.Int32 // fault injection at HTTP boundary, real M1 still executes
	posts     atomic.Int32
}

func setupRepair(t *testing.T) *repairHarness {
	h := setup(t)
	// Fixture cleanup is confined to the campus_m3_check database checked above.
	for _, table := range []string{"m1_operations", "m1_sources", "m1_bookings", "m1_audit", "m1_outbox", "m1_violations"} {
		if _, e := h.db.Exec("DELETE FROM " + table); e != nil {
			t.Fatal(e)
		}
	}
	f, e := dev.Load("../../dev-fixture.json")
	if e != nil {
		t.Fatal(e)
	}
	f.RoomsData = []booking.Room{{ID: "room-a", Name: "A101", Area: "A楼", Active: true, OpenHour: 8, CloseHour: 22, Seats: []booking.Seat{{ID: "seat-a", Label: "A01", Active: true}}}}
	s := booking.New(h.db, f, []byte("test-only-resource-secret-at-least32"))
	s.Now = h.service.Now
	router := s.Router(f.Auth)
	r := &repairHarness{harness: h, m1catalog: f}
	r.m1 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Cookie") != "" {
			t.Error("service request must not forward browser cookies")
		}
		if r.mode.Load() == 3 && req.URL.Path == "/api/v1/internal/resources/operations" || r.mode.Load() == 3 && req.Method == "GET" && len(req.URL.Path) > len("/api/v1/internal/resources/operations/") && req.URL.Path[:len("/api/v1/internal/resources/operations/")] == "/api/v1/internal/resources/operations/" {
			w.WriteHeader(503)
			return
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		if req.Method == "POST" && req.URL.Path == "/api/v1/internal/resources/operations" {
			r.posts.Add(1)
		}
		for k, v := range out.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(out.Code)
		if r.mode.Load() == 1 && req.Method == "POST" && out.Code == 200 {
			io.WriteString(w, "{lost-response")
			return
		}
		if r.mode.Load() == 2 && out.Code == 200 && req.URL.Path != "/api/v1/internal/resources/sources" {
			var v map[string]any
			if json.Unmarshal(out.Body.Bytes(), &v) == nil {
				if data, ok := v["data"].(map[string]any); ok {
					data["sourceBusinessId"] = "another-workorder"
					json.NewEncoder(w).Encode(v)
					return
				}
			}
		}
		w.Write(out.Body.Bytes())
	}))
	t.Cleanup(r.m1.Close)
	r.resource, e = maintenance.NewHTTPResources(r.m1.URL, "m3-service")
	if e != nil {
		t.Fatal(e)
	}
	h.service.Resources = r.resource
	return r
}
func (r *repairHarness) m1call(method, path, token, key string, q any) (int, []byte) {
	r.t.Helper()
	raw, _ := json.Marshal(q)
	req, _ := http.NewRequest(method, r.m1.URL+"/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	res, e := r.m1.Client().Do(req)
	if e != nil {
		r.t.Fatal(e)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, body
}
func (r *repairHarness) current(id string, who identity.Session) maintenance.Detail {
	r.t.Helper()
	code, raw := r.request("GET", "/workorders/"+id, "", who, nil)
	if code != 200 {
		r.t.Fatalf("detail %d %s", code, raw)
	}
	return decode[maintenance.Detail](r.t, raw)
}
func (r *repairHarness) command(o maintenance.Order, who identity.Session, name string) maintenance.Order {
	r.t.Helper()
	code, raw := r.act(o.ID, maintenance.ID(), who, action(name, o.Version, ""))
	want := 200
	if name == "START" || name == "APPROVE_ACCEPTANCE" || name == "RETRY_RESOURCE" {
		want = 202
	}
	if code != want {
		r.t.Fatalf("%s %d %s", name, code, raw)
	}
	return decode[maintenance.Order](r.t, raw)
}
func (r *repairHarness) ready() (maintenance.Order, identity.Session, identity.Session, identity.Session) {
	d := r.staff("workorder.dispatch", "A楼")
	w := r.staff("workorder.repair", "A楼")
	a := r.staff("workorder.accept", "A楼")
	o := r.assigned(r.report(), d, w)
	o = r.command(o, w, "ACCEPT_ASSIGNMENT")
	return o, d, w, a
}
func (r *repairHarness) run(id string) {
	r.t.Helper()
	if e := r.service.RunOperation(context.Background(), id); e != nil {
		r.t.Fatal(e)
	}
}
func (r *repairHarness) available() bool {
	r.t.Helper()
	start := r.service.Now().Add(time.Hour)
	end := start.Add(time.Hour)
	code, raw := r.m1call("GET", "/resources/availability?startAt="+url.QueryEscape(start.Format(time.RFC3339))+"&endAt="+url.QueryEscape(end.Format(time.RFC3339)), "student-demo", "", nil)
	if code != 200 {
		r.t.Fatalf("availability %d %s", code, raw)
	}
	return len(decode[[]any](r.t, raw)) > 0
}
func (r *repairHarness) sourceActive(module, business string) bool {
	r.t.Helper()
	code, raw := r.m1call("GET", "/internal/resources/sources", map[string]string{"M2": "m2-service", "M3": "m3-service"}[module], "", nil)
	if code != 200 {
		r.t.Fatalf("sources %d %s", code, raw)
	}
	rows := decode[[]struct {
		ID     string `json:"sourceBusinessId"`
		Active bool   `json:"active"`
	}](r.t, raw)
	for _, v := range rows {
		if v.ID == business {
			return v.Active
		}
	}
	return false
}
func (r *repairHarness) extraSource(module, id string, version int64) {
	r.t.Helper()
	op := booking.Operation{OperationID: "extra-" + maintenance.ID(), Action: "BLOCK_ROOM_MAINTENANCE", SourceModule: module, SourceBusinessID: id, SourceVersion: version, RoomID: "room-a", Reason: "isolated integration fixture"}
	if module == "M2" {
		start := r.service.Now().Add(time.Hour)
		end := start.Add(time.Hour)
		deadline := end.Add(time.Hour)
		op.Action = "HOLD_ROOM_INTERVAL"
		op.StartAt = &start
		op.EndAt = &end
		op.DeadlineAt = &deadline
	}
	code, raw := r.m1call("POST", "/internal/resources/operations", map[string]string{"M2": "m2-service", "M3": "m3-service"}[module], op.OperationID, op)
	if code != 200 {
		r.t.Fatalf("extra source %d %s", code, raw)
	}
}

func TestRepairHTTPFullLifecycleAndIndependentAcceptance(t *testing.T) {
	r := setupRepair(t)
	o, _, w, a := r.ready()
	if !r.available() {
		t.Fatal("room unavailable before repair")
	}
	start := r.service.Now().Add(time.Hour)
	code, raw := r.m1call("POST", "/m1/bookings", "student-demo", "existing-booking", booking.BookRequest{RoomID: "room-a", SeatID: "seat-a", StartAt: start, EndAt: start.Add(time.Hour)})
	if code != 200 {
		t.Fatalf("initial booking %d %s", code, raw)
	}
	oldBooking := decode[booking.Booking](t, raw)
	o = r.command(o, w, "START")
	if o.State != "ASSIGNED" || o.SyncState != "PENDING" || o.Operation == nil {
		t.Fatalf("premature start %+v", o)
	}
	r.run(o.Operation.ID)
	detail := r.current(o.ID, w)
	o = detail.Order
	if o.State != "IN_PROGRESS" || o.SyncState != "IDLE" || o.FirstStartedAt == nil || !r.sourceActive("M3", o.ID) || r.available() {
		t.Fatalf("missing confirmed block %+v", o)
	}
	code, _ = r.m1call("POST", "/m1/bookings", "student-demo", "blocked-booking", booking.BookRequest{RoomID: "room-a", SeatID: "seat-a", StartAt: start, EndAt: start.Add(time.Hour)})
	if code != 409 {
		t.Fatal("maintenance allowed booking", code)
	}
	o = r.command(o, w, "FINISH")
	if o.State != "AWAITING_ACCEPTANCE" || o.Result == "" || !r.sourceActive("M3", o.ID) {
		t.Fatal("finish released block", o)
	}
	code, raw = r.request("GET", "/workorders?view=acceptance", "", a, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 1 {
		t.Fatalf("acceptance queue %d %s", code, raw)
	}
	o = r.command(o, a, "REJECT_ACCEPTANCE")
	if o.State != "IN_PROGRESS" || r.available() {
		t.Fatal("rejection cleared block")
	}
	o = r.command(o, w, "FINISH")
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	if o.State != "AWAITING_ACCEPTANCE" || o.SyncState != "PENDING" || !r.sourceActive("M3", o.ID) {
		t.Fatal("premature completion")
	}
	r.run(o.Operation.ID)
	o = r.current(o.ID, a).Order
	if o.State != "COMPLETED" || o.CompletedAt == nil || o.SyncState != "IDLE" || r.sourceActive("M3", o.ID) || !r.available() {
		t.Fatalf("completion %+v", o)
	}
	code, raw = r.m1call("GET", "/m1/bookings", "student-demo", "", nil)
	if code != 200 {
		t.Fatal(code)
	}
	for _, b := range decode[[]booking.Booking](t, raw) {
		if b.ID == oldBooking.ID && b.Status != "CANCELLED" {
			t.Fatal("old booking revived", b)
		}
	}
	if r.count("m3_resource_operations") != 2 || r.posts.Load() != 2 {
		t.Fatal("duplicate resource operations")
	}
}

func TestRepairReleasePreservesOtherM3AndM2Sources(t *testing.T) {
	r := setupRepair(t)
	r.extraSource("M2", "activity-fixture", 1)
	r.extraSource("M3", "other-workorder", 1)
	o, _, w, a := r.ready()
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	o = r.command(o, w, "FINISH")
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	r.run(o.Operation.ID)
	if r.current(o.ID, a).State != "COMPLETED" || r.sourceActive("M3", o.ID) || !r.sourceActive("M3", "other-workorder") || !r.sourceActive("M2", "activity-fixture") || r.available() {
		t.Fatal("release crossed source boundary")
	}
}

func TestRepairCurrentAndHistoricalParticipantsCannotSelfAccept(t *testing.T) {
	r := setupRepair(t)
	o, d, w, a := r.ready()
	// Additional permission is an isolated identity fixture, never runtime role mutation.
	_, e := r.db.Exec("INSERT IGNORE INTO d_role_permissions(role_id,permission_id) SELECT role_id,'workorder.accept' FROM d_user_roles WHERE user_id=? AND role_id<>'student'", w.Principal.ID)
	if e != nil {
		t.Fatal(e)
	}
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	next := r.staff("workorder.repair", "A楼")
	if _, e = r.db.Exec("INSERT IGNORE INTO d_role_permissions(role_id,permission_id) SELECT role_id,'workorder.accept' FROM d_user_roles WHERE user_id=? AND role_id<>'student'", next.Principal.ID); e != nil {
		t.Fatal(e)
	}
	o = r.assigned(o, d, next)
	o = r.command(o, next, "ACCEPT_ASSIGNMENT")
	o = r.command(o, next, "FINISH")
	for _, actor := range []identity.Session{w, next} {
		code, _ := r.act(o.ID, maintenance.ID(), actor, action("APPROVE_ACCEPTANCE", o.Version, ""))
		if code != 403 && code != 404 {
			t.Fatal("repair participant accepted own work", code)
		}
	}
	code, raw := r.request("GET", "/workorders?view=acceptance", "", w, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 0 {
		t.Fatalf("participant in queue %d %s", code, raw)
	}
	if !slices.Contains(r.current(o.ID, a).AllowedActions, "APPROVE_ACCEPTANCE") {
		t.Fatal("independent acceptance missing")
	}
}

func TestRepairLostStartReplyRestartsWithSameOperation(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	q := action("START", o.Version, "")
	code, raw := r.act(o.ID, "same-start", w, q)
	if code != 202 {
		t.Fatal(code, string(raw))
	}
	o = decode[maintenance.Order](t, raw)
	id := o.Operation.ID
	r.mode.Store(1)
	r.run(id)
	current := r.current(o.ID, w)
	if current.SyncState != "UNKNOWN" || current.State != "ASSIGNED" || !r.sourceActive("M3", o.ID) {
		t.Fatal("lost reply treated as absence", current)
	}
	r.server.Close()
	r.start()
	r.service.Resources = r.resource // rebuild application from SQL
	r.run(id)
	current = r.current(o.ID, w)
	if current.State != "IN_PROGRESS" || current.SyncState != "IDLE" || r.posts.Load() != 1 {
		t.Fatal("restart resent operation or failed recovery", current)
	}
	code, raw = r.act(o.ID, "same-start", w, q)
	if code != 200 || decode[maintenance.Order](t, raw).State != "IN_PROGRESS" || r.count("m3_resource_operations") != 1 {
		t.Fatal("same command did not reconcile", code, string(raw))
	}
}

func TestRepairLostReleaseReplyDoesNotPrematurelyComplete(t *testing.T) {
	r := setupRepair(t)
	o, _, w, a := r.ready()
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	o = r.command(o, w, "FINISH")
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	id := o.Operation.ID
	r.mode.Store(1)
	r.run(id)
	if r.current(o.ID, a).State != "AWAITING_ACCEPTANCE" || r.current(o.ID, a).SyncState != "UNKNOWN" {
		t.Fatal("unknown release completed")
	}
	r.server.Close()
	r.start()
	r.service.Resources = r.resource
	r.run(id)
	if r.current(o.ID, a).State != "COMPLETED" || r.posts.Load() != 2 {
		t.Fatal("release recovery duplicated or incomplete")
	}
}

func TestRepairWrongReceiptIsUnknownAndCannotFinish(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	o = r.command(o, w, "START")
	id := o.Operation.ID
	r.mode.Store(2)
	r.run(id)
	current := r.current(o.ID, w)
	if current.State != "ASSIGNED" || current.SyncState != "UNKNOWN" || current.Operation.ID != id {
		t.Fatalf("wrong provenance accepted %+v", current)
	}
	code, _ := r.act(o.ID, maintenance.ID(), w, action("FINISH", current.Version, ""))
	if code != 409 {
		t.Fatal("pending resource allowed finish", code)
	}
	r.mode.Store(0)
	r.run(id)
	if r.current(o.ID, w).State != "IN_PROGRESS" || r.posts.Load() != 1 {
		t.Fatal("matching receipt not reconciled")
	}
}

func TestRepairUnavailableExhaustionAndManualSameOperationRetry(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	o = r.command(o, w, "START")
	id := o.Operation.ID
	r.mode.Store(3)
	for i := 0; i < 8; i++ {
		r.run(id)
	}
	current := r.current(o.ID, w)
	if current.SyncState != "NEEDS_ATTENTION" || current.Operation.State != "PAUSED" || current.Operation.Attempts != 8 || current.State != "ASSIGNED" {
		t.Fatalf("retry exhaustion inferred absence %+v", current)
	}
	code, _ := r.act(o.ID, maintenance.ID(), r.a, action("CANCEL", current.Version, ""))
	if code != 409 {
		t.Fatal("unknown start allowed cancellation", code)
	}
	r.mode.Store(0)
	originalNow := r.service.Now
	r.service.Now = func() time.Time { return originalNow().Add(-time.Minute) }
	code, _ = r.act(o.ID, maintenance.ID(), w, action("RETRY_RESOURCE", current.Version, ""))
	if code != 400 {
		t.Fatal("resource retry accepted a backwards clock", code)
	}
	r.service.Now = originalNow
	o = r.command(current.Order, w, "RETRY_RESOURCE")
	if o.Operation.ID != id {
		t.Fatal("retry minted new operation")
	}
	r.run(id)
	if r.current(o.ID, w).State != "IN_PROGRESS" || r.count("m3_resource_operations") != 1 || r.posts.Load() != 1 {
		t.Fatal("manual retry duplicated source")
	}
}

func TestRepairLocalOutboxFailureAfterRemoteCommitRecovers(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	o = r.command(o, w, "START")
	id := o.Operation.ID
	_, e := r.db.Exec("CREATE TRIGGER m3_round3_outbox_failure BEFORE INSERT ON m3_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test failure'")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.db.Exec("DROP TRIGGER IF EXISTS m3_round3_outbox_failure") })
	if e = r.service.RunOperation(context.Background(), id); e == nil {
		t.Fatal("audit failure silently committed")
	}
	if r.current(o.ID, w).State != "ASSIGNED" || !r.sourceActive("M3", o.ID) {
		t.Fatal("incorrect local rollback")
	}
	r.db.Exec("DROP TRIGGER m3_round3_outbox_failure")
	r.run(id)
	if r.current(o.ID, w).State != "IN_PROGRESS" || r.posts.Load() != 1 {
		t.Fatal("receipt recovery after local failure failed")
	}
}

func TestRepairKnownReleaseFailureKeepsSafetyAndRejectsRetry(t *testing.T) {
	r := setupRepair(t)
	o, _, w, a := r.ready()
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	o = r.command(o, w, "FINISH")
	r.extraSource("M3", o.ID, 99) // provider's version fence advances through public HTTP
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	r.run(o.Operation.ID)
	current := r.current(o.ID, a)
	if current.State != "AWAITING_ACCEPTANCE" || current.SyncState != "NEEDS_ATTENTION" || current.LastResourceError != "VERSION_CONFLICT" || current.Operation.State != "FAILED" || !r.sourceActive("M3", o.ID) {
		t.Fatalf("release failure unsafe %+v", current)
	}
	if slices.Contains(current.AllowedActions, "RETRY_RESOURCE") {
		t.Fatal("terminal failed receipt offered unknown retry")
	}
	code, _ := r.act(o.ID, maintenance.ID(), a, action("RETRY_RESOURCE", current.Version, ""))
	if code != 409 {
		t.Fatal("terminal failed operation reset", code)
	}
}

func TestRepairConcurrentSameCommandAndWorkersAreIdempotent(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	q := action("START", o.Version, "")
	var wg sync.WaitGroup
	results := make(chan []byte, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, raw := r.act(o.ID, "concurrent-start", w, q)
			if code != 202 {
				t.Error("concurrent start", code, string(raw))
			}
			results <- raw
		}()
	}
	wg.Wait()
	close(results)
	id := ""
	for raw := range results {
		v := decode[maintenance.Order](t, raw)
		if v.Operation == nil {
			t.Fatal("missing op")
		}
		if id != "" && id != v.Operation.ID {
			t.Fatal("duplicate operation IDs")
		}
		id = v.Operation.ID
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := r.service.RunOperation(context.Background(), id); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if r.current(o.ID, w).State != "IN_PROGRESS" || r.posts.Load() != 1 || r.count("m3_resource_operations") != 1 {
		t.Fatal("concurrent processing duplicated source")
	}
}

func TestRepairMissingResourceConfigurationFailsClosed(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	r.service.Resources = nil
	if slices.Contains(r.current(o.ID, w).AllowedActions, "START") {
		t.Fatal("unconfigured start offered")
	}
	code, _ := r.act(o.ID, maintenance.ID(), w, action("START", o.Version, ""))
	if code != 503 || r.count("m3_resource_operations") != 0 {
		t.Fatal("unconfigured resource action committed", code)
	}
}

func TestRepairExplicitBlockFailureCanDeliberatelyStartAgain(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	r.m1catalog.RoomsData[0].Active = false
	o = r.command(o, w, "START")
	first := o.Operation.ID
	r.run(first)
	o = r.current(o.ID, w).Order
	if o.State != "ASSIGNED" || o.SyncState != "IDLE" || o.LastResourceError != "RESOURCE_UNAVAILABLE" || o.Operation != nil || r.sourceActive("M3", o.ID) {
		t.Fatalf("failed block advanced business %+v", o)
	}
	r.m1catalog.RoomsData[0].Active = true
	o = r.command(o, w, "START")
	if o.Operation.ID == first {
		t.Fatal("new deliberate attempt reused terminal failed operation")
	}
	r.run(o.Operation.ID)
	if r.current(o.ID, w).State != "IN_PROGRESS" || r.count("m3_resource_operations") != 2 {
		t.Fatal("deliberate retry failed")
	}
}

func TestRepairReleaseDoesNotEnableDisabledRoom(t *testing.T) {
	r := setupRepair(t)
	o, _, w, a := r.ready()
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	o = r.command(o, w, "FINISH")
	r.m1catalog.RoomsData[0].Active = false
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	r.run(o.Operation.ID)
	if r.current(o.ID, a).State != "COMPLETED" || r.sourceActive("M3", o.ID) || r.available() {
		t.Fatal("release enabled disabled room")
	}
}

func TestRepairDurablePendingOperationResumesThroughTick(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	o = r.command(o, w, "START")
	if r.posts.Load() != 0 {
		t.Fatal("network request occurred before durable command returned")
	}
	r.server.Close()
	r.start()
	r.service.Resources = r.resource
	if e := r.service.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	if r.current(o.ID, w).State != "IN_PROGRESS" || r.posts.Load() != 1 {
		t.Fatal("tick did not resume persisted operation")
	}
}

func TestRepairStartAndCancellationRaceOnlyOneVersionWins(t *testing.T) {
	r := setupRepair(t)
	o, _, w, _ := r.ready()
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for _, task := range []struct {
		who  identity.Session
		name string
	}{{w, "START"}, {r.a, "CANCEL"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := r.act(o.ID, maintenance.ID(), task.who, action(task.name, o.Version, ""))
			codes <- code
		}()
	}
	wg.Wait()
	close(codes)
	success, conflict := 0, 0
	for code := range codes {
		switch code {
		case 200, 202:
			success++
		case 409:
			conflict++
		default:
			t.Fatal("unexpected race status", code)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("contradictory commands both won", success, conflict)
	}
	current := r.current(o.ID, r.a)
	if current.State == "CANCELLED" && r.count("m3_resource_operations") != 0 {
		t.Fatal("cancelled order queued block")
	}
	if current.State == "ASSIGNED" {
		if current.SyncState != "PENDING" {
			t.Fatal("missing pending guard")
		}
		r.run(current.Operation.ID)
		if !r.sourceActive("M3", o.ID) {
			t.Fatal("winning repair not blocked")
		}
	}
}

func TestRepairRequiresAcceptedAssignment(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.assigned(h.report(), d, w)
	code, raw := h.act(o.ID, "start-before-accept", w, action("START", o.Version, ""))
	if code != 409 {
		t.Fatalf("unaccepted repair must be a state conflict: %d %s", code, raw)
	}
}
