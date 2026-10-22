package maintenance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
	maintenancedev "campus-resource-platform/backend/internal/maintenance/dev"
	"github.com/gin-gonic/gin"
)

// Administrative SQL below is test-fixture setup, only after setup has verified
// campus_m3_check. Runtime M3 code must use the public identity directory.
func (h *harness) staff(permission, area string) identity.Session {
	h.t.Helper()
	s, e := h.auth.Register(context.Background(), "M3-"+maintenance.ID(), "虚构物业同学", "test-password-at-least-ten", "127.0.0.1")
	if e != nil {
		h.t.Fatal(e)
	}
	role := "m3-test-" + maintenance.ID()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO d_roles(id,name) VALUES (?,?)", []any{role, "M3隔离测试角色"}},
		{"INSERT IGNORE INTO d_permissions(id,name) VALUES (?,?)", []any{permission, permission}},
		{"INSERT INTO d_role_permissions(role_id,permission_id) VALUES (?,?)", []any{role, permission}},
		{"INSERT INTO d_user_roles(user_id,role_id) VALUES (?,?)", []any{s.Principal.ID, role}},
		{"INSERT INTO d_user_areas(user_id,area) VALUES (?,?)", []any{s.Principal.ID, area}},
	} {
		if _, e = h.db.Exec(q.sql, q.args...); e != nil {
			h.t.Fatal(e)
		}
	}
	h.t.Cleanup(func() { h.db.Exec("UPDATE d_users SET active=0 WHERE id=?", s.Principal.ID) })
	return s
}
func (h *harness) report() maintenance.Order {
	h.t.Helper()
	code, raw := h.request("POST", "/workorders", maintenance.ID(), h.a, input())
	if code != 201 {
		h.t.Fatalf("create: %d %s", code, raw)
	}
	return decode[maintenance.Order](h.t, raw)
}
func action(name string, version int64, target string) map[string]any {
	q := map[string]any{"action": name, "expectedVersion": version, "comment": "检查线路并安排安全处理"}
	if target != "" {
		q["assigneeId"] = target
	}
	return q
}
func (h *harness) act(id, key string, s identity.Session, q any) (int, []byte) {
	return h.request("POST", "/workorders/"+id+"/actions", key, s, q)
}
func (h *harness) assigned(o maintenance.Order, d, w identity.Session) maintenance.Order {
	h.t.Helper()
	code, raw := h.act(o.ID, maintenance.ID(), d, action("ASSIGN", o.Version, w.Principal.ID))
	if code != 200 {
		h.t.Fatalf("assign: %d %s", code, raw)
	}
	return decode[maintenance.Order](h.t, raw)
}
func TestDispatchQueriesCandidatesAndAcceptance(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	other := h.staff("workorder.dispatch", "B楼")
	alias := h.staff("workorder.dispatch", "a楼")
	h.staff("workorder.repair", "B楼")
	o := h.report()
	code, raw := h.request("GET", "/workorders?view=dispatch", "", d, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 1 {
		t.Fatalf("dispatch list %d %s", code, raw)
	}
	code, raw = h.request("GET", "/workorders?view=dispatch", "", other, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 0 {
		t.Fatal("area list leaked", code, string(raw))
	}
	code, raw = h.request("GET", "/workorders?view=dispatch", "", alias, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 0 {
		t.Fatal("case-sensitive area leaked", code, string(raw))
	}
	code, raw = h.request("GET", "/workorders/"+o.ID+"/assignees", "", d, nil)
	workers := decode[[]struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}](t, raw)
	if code != 200 || len(workers) != 1 || workers[0].ID != w.Principal.ID || workers[0].Name == "" {
		t.Fatalf("candidate scope %d %s", code, raw)
	}
	for _, s := range []identity.Session{h.a, other} {
		code, _ = h.act(o.ID, maintenance.ID(), s, action("ASSIGN", 1, w.Principal.ID))
		if code != 403 && code != 404 {
			t.Fatal("unauthorized dispatch", code)
		}
	}
	o = h.assigned(o, d, w)
	if o.State != "ASSIGNED" || o.Version != 2 {
		t.Fatalf("bad assigned %+v", o)
	}
	code, raw = h.request("GET", "/workorders?view=assigned", "", w, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 1 {
		t.Fatal("missing assigned", code, string(raw))
	}
	code, raw = h.act(o.ID, "accept", w, action("ACCEPT_ASSIGNMENT", 2, ""))
	var accepted struct {
		Version    int64      `json:"version"`
		State      string     `json:"state"`
		AcceptedAt *time.Time `json:"acceptedAt"`
	}
	accepted = decode[struct {
		Version    int64      `json:"version"`
		State      string     `json:"state"`
		AcceptedAt *time.Time `json:"acceptedAt"`
	}](t, raw)
	if code != 200 || accepted.Version != 3 || accepted.State != "ASSIGNED" || accepted.AcceptedAt == nil {
		t.Fatalf("accept became repair or lost timestamp %d %s", code, raw)
	}
	if h.count("m3_assignment_history") != 1 || h.count("m3_events") != 3 {
		t.Fatal("missing history")
	}
	code, raw = h.request("GET", "/workorders/"+o.ID, "", d, nil)
	if code != 200 || len(decode[maintenance.Detail](t, raw).Timeline) != 3 {
		t.Fatal("timeline", code, string(raw))
	}
}
func TestDispatchRejectsDisabledRevokedAndForeignWorkers(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	foreign := h.staff("workorder.repair", "B楼")
	revoked := h.staff("workorder.repair", "A楼")
	o := h.report()
	h.db.Exec("UPDATE d_users SET active=0 WHERE id=?", w.Principal.ID)
	h.db.Exec("DELETE FROM d_user_roles WHERE user_id=?", revoked.Principal.ID)
	for _, id := range []string{w.Principal.ID, foreign.Principal.ID, revoked.Principal.ID, h.b.Principal.ID, "missing"} {
		code, _ := h.act(o.ID, maintenance.ID(), d, action("ASSIGN", 1, id))
		if code != 403 && code != 404 {
			t.Fatal("invalid worker accepted", id, code)
		}
	}
	code, raw := h.request("GET", "/workorders/"+o.ID+"/assignees", "", d, nil)
	if code != 200 || len(decode[[]any](t, raw)) != 0 {
		t.Fatal("invalid candidates", code, string(raw))
	}
	if h.count("m3_assignment_history") != 0 || h.count("m3_events") != 1 {
		t.Fatal("invalid assignment wrote data")
	}
}
func TestConcurrentDispatchVersionAndCommandIsolation(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	w2 := h.staff("workorder.repair", "A楼")
	o := h.report()
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for i, id := range []string{w.Principal.ID, w2.Principal.ID} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			code, _ := h.act(o.ID, fmt.Sprint("race-", i), d, action("ASSIGN", 1, id))
			codes <- code
		}(i, id)
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for code := range codes {
		seen[code]++
	}
	if seen[200] != 1 || seen[409] != 1 || h.count("m3_assignment_history") != 1 {
		t.Fatal("version race", seen)
	}
	_, raw := h.request("GET", "/workorders/"+o.ID, "", d, nil)
	latest := decode[maintenance.Detail](t, raw)
	if latest.Version != 2 {
		t.Fatal("lost update")
	}
}
func TestActionSameKeyConcurrencyAndCrossOrderConflict(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := h.act(o.ID, "same-action", d, action("ASSIGN", 1, w.Principal.ID))
			codes <- code
		}()
	}
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != 200 {
			t.Fatal("same action did not replay", code)
		}
	}
	if h.count("m3_assignment_history") != 1 || h.count("m3_events") != 2 {
		t.Fatal("duplicate action effects")
	}
	other := h.report()
	code, _ := h.act(other.ID, "same-action", d, action("ASSIGN", 1, w.Principal.ID))
	if code != 409 {
		t.Fatal("key shared across orders", code)
	}
	q := action("ASSIGN", 1, w.Principal.ID)
	q["comment"] = "不同的建议不能覆盖先前命令"
	code, _ = h.act(o.ID, "same-action", d, q)
	if code != 409 {
		t.Fatal("key content changed", code)
	}
}
func TestReassignmentRevokesOldWorkerAndPreservesClock(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	next := h.staff("workorder.repair", "A楼")
	o := h.assigned(h.report(), d, w)
	codeBefore, accepted := h.act(o.ID, "old-accepted", w, action("ACCEPT_ASSIGNMENT", o.Version, ""))
	if codeBefore != 200 {
		t.Fatal("initial accept", codeBefore)
	}
	o = decode[maintenance.Order](t, accepted)
	_, before := h.request("GET", "/workorders/"+o.ID, "", d, nil)
	h.service.Now = func() time.Time { return time.Date(2030, 1, 1, 3, 0, 0, 0, time.UTC) }
	o = h.assigned(o, d, next)
	_, after := h.request("GET", "/workorders/"+o.ID, "", d, nil)
	var clock struct {
		FirstAssignedAt string     `json:"firstAssignedAt"`
		DeadlineAt      string     `json:"deadlineAt"`
		AcceptedAt      *time.Time `json:"acceptedAt"`
	}
	a := decode[struct {
		FirstAssignedAt string     `json:"firstAssignedAt"`
		DeadlineAt      string     `json:"deadlineAt"`
		AcceptedAt      *time.Time `json:"acceptedAt"`
	}](t, before)
	clock = decode[struct {
		FirstAssignedAt string     `json:"firstAssignedAt"`
		DeadlineAt      string     `json:"deadlineAt"`
		AcceptedAt      *time.Time `json:"acceptedAt"`
	}](t, after)
	if a.FirstAssignedAt == "" || clock.FirstAssignedAt != a.FirstAssignedAt || clock.DeadlineAt != a.DeadlineAt || clock.AcceptedAt != nil {
		t.Fatal("reassignment reset clock")
	}
	code, _ := h.request("GET", "/workorders/"+o.ID, "", w, nil)
	if code != 404 {
		t.Fatal("old worker reads", code)
	}
	code, _ = h.act(o.ID, "old-accepted", w, action("ACCEPT_ASSIGNMENT", 2, ""))
	if code != 403 && code != 404 {
		t.Fatal("old worker mutates", code)
	}
	code, _ = h.act(o.ID, "new-accept", next, action("ACCEPT_ASSIGNMENT", o.Version, ""))
	if code != 200 {
		t.Fatal("new worker cannot accept", code)
	}
	var n int
	h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE recipient_id=? AND kind='WORKORDER_HANDOVER'", w.Principal.ID).Scan(&n)
	if n != 1 {
		t.Fatal("old worker missing handover", n)
	}
}
func TestEarlyCancellationTerminalAndForgery(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.assigned(h.report(), d, w)
	code, _ := h.act(o.ID, "other-cancel", h.b, action("CANCEL", 2, ""))
	if code != 403 && code != 404 {
		t.Fatal("other user cancel", code)
	}
	code, raw := h.act(o.ID, "cancel", h.a, action("CANCEL", 2, ""))
	if code != 200 || decode[maintenance.Order](t, raw).State != "CANCELLED" {
		t.Fatal("cancel", code, string(raw))
	}
	code, _ = h.act(o.ID, "cancel", h.a, action("CANCEL", 2, ""))
	if code != 200 {
		t.Fatal("cancel replay", code)
	}
	code, _ = h.act(o.ID, "late-assign", d, action("ASSIGN", 3, w.Principal.ID))
	if code != 409 {
		t.Fatal("terminal dispatch", code)
	}
	for _, q := range []map[string]any{action("START", 3, ""), {"action": "CANCEL", "expectedVersion": 3, "userId": h.a.Principal.ID}, {"action": "CANCEL", "expectedVersion": 3, "assigneeId": w.Principal.ID}} {
		code, _ = h.act(o.ID, maintenance.ID(), h.a, q)
		if code != 400 {
			t.Fatal("unsupported/forged action", code)
		}
	}
	code, raw = h.request("GET", "/workorders?view=assigned", "", w, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 0 {
		t.Fatal("cancelled still pending")
	}
}
func TestDispatchEventFailureRollsBackVersionHistoryAndKey(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	_, e := h.db.Exec("CREATE TRIGGER m3_dispatch_failure BEFORE INSERT ON m3_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test failure'")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { h.db.Exec("DROP TRIGGER IF EXISTS m3_dispatch_failure") })
	code, _ := h.act(o.ID, "failed-assign", d, action("ASSIGN", 1, w.Principal.ID))
	if code != 500 {
		t.Fatal(code)
	}
	_, raw := h.request("GET", "/workorders/"+o.ID, "", h.a, nil)
	if decode[maintenance.Detail](t, raw).Version != 1 || h.count("m3_assignment_history") != 0 || h.count("m3_commands") != 1 || h.count("m3_events") != 1 {
		t.Fatal("partial action commit")
	}
	h.db.Exec("DROP TRIGGER m3_dispatch_failure")
	code, _ = h.act(o.ID, "failed-assign", d, action("ASSIGN", 1, w.Principal.ID))
	if code != 200 {
		t.Fatal("key failed to roll back", code)
	}
}
func TestLostActionReplyAndCurrentAuthorizationOnReplay(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	drop := httptest.NewServer(http.HandlerFunc(func(wr http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.router.ServeHTTP(rec, r)
		conn, _, e := wr.(http.Hijacker).Hijack()
		if e == nil {
			conn.Close()
		}
	}))
	defer drop.Close()
	q := action("ASSIGN", 1, w.Principal.ID)
	body, _ := json.Marshal(q)
	req, _ := http.NewRequest("POST", drop.URL+"/api/v1/m3/workorders/"+o.ID+"/actions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "lost-action")
	req.Header.Set("X-CSRF-Token", d.CSRFToken)
	req.AddCookie(&http.Cookie{Name: identity.CookieName, Value: d.Token})
	response, e := drop.Client().Do(req)
	if response != nil {
		response.Body.Close()
	}
	if e == nil {
		t.Fatal("reply not lost")
	}
	if h.count("m3_assignment_history") != 1 {
		t.Fatal("action not committed")
	}
	h.server.Close()
	h.start()
	code, _ := h.act(o.ID, "lost-action", d, q)
	if code != 200 || h.count("m3_assignment_history") != 1 {
		t.Fatal("restart replay", code)
	}
	h.db.Exec("DELETE FROM d_user_areas WHERE user_id=?", d.Principal.ID)
	code, _ = h.act(o.ID, "lost-action", d, q)
	if code != 403 && code != 404 {
		t.Fatal("replay bypassed current area", code)
	}
}

func TestCandidateInvalidatedAfterListingAndHistoricalAssignmentReplay(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	code, raw := h.request("GET", "/workorders/"+o.ID+"/assignees", "", d, nil)
	if code != 200 || len(decode[[]any](t, raw)) != 1 {
		t.Fatal("candidate load", code)
	}
	h.db.Exec("UPDATE d_users SET active=0 WHERE id=?", w.Principal.ID)
	code, _ = h.act(o.ID, "stale-candidate", d, action("ASSIGN", 1, w.Principal.ID))
	if code != 403 {
		t.Fatal("stale candidate assigned", code)
	}
	h.db.Exec("UPDATE d_users SET active=1 WHERE id=?", w.Principal.ID)
	q := action("ASSIGN", 1, w.Principal.ID)
	code, _ = h.act(o.ID, "committed", d, q)
	if code != 200 {
		t.Fatal(code)
	}
	h.db.Exec("UPDATE d_users SET active=0 WHERE id=?", w.Principal.ID)
	code, _ = h.act(o.ID, "committed", d, q)
	if code != 200 || h.count("m3_assignment_history") != 1 {
		t.Fatal("historical action wrongly rerun", code)
	}
}
func TestCancellationAndDispatchRaceHasOneVersionWinner(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		c, _ := h.act(o.ID, "race-assign", d, action("ASSIGN", 1, w.Principal.ID))
		codes <- c
	}()
	go func() { defer wg.Done(); c, _ := h.act(o.ID, "race-cancel", h.a, action("CANCEL", 1, "")); codes <- c }()
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for c := range codes {
		seen[c]++
	}
	if seen[200] != 1 || seen[409] != 1 {
		t.Fatal("cancellation race", seen)
	}
	_, raw := h.request("GET", "/workorders/"+o.ID, "", h.a, nil)
	latest := decode[maintenance.Detail](t, raw)
	if latest.Version != 2 || h.count("m3_events") != 2 || h.count("m3_commands") != 2 {
		t.Fatal("partial race result")
	}
}
func TestActionSessionCSRFAndBodyValidation(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	q := action("ASSIGN", 1, w.Principal.ID)
	code, _ := h.act(o.ID, "anonymous", identity.Session{}, q)
	if code != 401 {
		t.Fatal("anonymous", code)
	}
	noCSRF := d
	noCSRF.CSRFToken = ""
	code, _ = h.act(o.ID, "no-csrf", noCSRF, q)
	if code != 403 {
		t.Fatal("missing CSRF", code)
	}
	for _, input := range []map[string]any{{"action": "ASSIGN", "expectedVersion": 1, "assigneeId": w.Principal.ID, "permissions": []string{"workorder.dispatch"}}, action("ASSIGN", 0, w.Principal.ID), action("ASSIGN", 1, "")} {
		code, _ = h.act(o.ID, "invalid", d, input)
		if code != 400 {
			t.Fatal("body validation", code)
		}
	}
	code, _ = h.act(o.ID, "", d, q)
	if code != 400 {
		t.Fatal("missing idempotency key", code)
	}
	if h.count("m3_assignment_history") != 0 || h.count("m3_commands") != 1 {
		t.Fatal("rejected write persisted")
	}
}

func TestDevelopmentStaffBindingDoesNotWritePublicRolesAndHonorsRevocation(t *testing.T) {
	h := setup(t)
	w := h.staff("workorder.repair", "A楼")
	directory := &maintenancedev.Directory{Store: h.auth, StudentReporting: true, Staff: []maintenancedev.StaffBinding{{UserID: h.b.Principal.ID, Permissions: []string{"workorder.dispatch"}, Areas: []string{"A楼"}}}}
	before, e := h.auth.Principal(context.Background(), h.b.Principal.ID)
	if e != nil {
		t.Fatal(e)
	}
	if before.Can("workorder.dispatch") {
		t.Fatal("test starts with formal dispatch authority")
	}
	h.server.Close()
	h.router = gin.New()
	h.router.Use((&identity.HTTP{Store: h.auth}).CSRFMiddleware())
	h.service.People = directory
	h.service.Register(h.router, directory.Authenticate)
	h.server = httptest.NewServer(h.router)
	o := h.report()
	q := action("ASSIGN", 1, w.Principal.ID)
	code, _ := h.act(o.ID, "bound-dispatch", h.b, q)
	if code != 200 {
		t.Fatal("configured binding failed", code)
	}
	after, e := h.auth.Principal(context.Background(), h.b.Principal.ID)
	if e != nil {
		t.Fatal(e)
	}
	if after.Can("workorder.dispatch") || len(after.Areas) != len(before.Areas) || len(after.Roles) != len(before.Roles) {
		t.Fatal("development adapter changed D roles or areas")
	}
	h.db.Exec("DELETE FROM d_user_roles WHERE user_id=?", h.b.Principal.ID)
	code, _ = h.act(o.ID, "bound-dispatch", h.b, q)
	if code != 403 && code != 404 {
		t.Fatal("binding bypassed base-role revocation", code)
	}
}
