package maintenance_test

import (
	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
	"context"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func completedRepair(t *testing.T) (*repairHarness, maintenance.Order, identity.Session) {
	t.Helper()
	r := setupRepair(t)
	o, _, w, a := r.ready()
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	o = r.command(o, w, "FINISH")
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	r.run(o.Operation.ID)
	return r, r.current(o.ID, a).Order, a
}
func TestEvaluationOnlyReporterAndSameKeyReplays(t *testing.T) {
	r, o, acceptor := completedRepair(t)
	q := map[string]any{"action": "EVALUATE", "expectedVersion": o.Version, "score": 5, "comment": "维修后照明已恢复正常"}
	code, _ := r.act(o.ID, "foreign-evaluation", acceptor, q)
	if code != 403 {
		t.Fatal("staff rated another reporter's order", code)
	}
	code, raw := r.act(o.ID, "evaluation", r.a, q)
	if code != 200 {
		t.Fatalf("reporter evaluation %d %s", code, raw)
	}
	code, raw = r.act(o.ID, "evaluation", r.a, q)
	if code != 200 {
		t.Fatalf("same evaluation replay %d %s", code, raw)
	}
	if r.count("m3_resource_operations") != 2 || r.current(o.ID, r.a).State != "COMPLETED" {
		t.Fatal("evaluation changed resources or business state")
	}
	value := r.current(o.ID, r.a)
	if value.Evaluation == nil || value.Evaluation.Score != 5 || value.Evaluation.Comment != "维修后照明已恢复正常" || slices.Contains(value.AllowedActions, "EVALUATE") {
		t.Fatal("rating not persisted or still editable", value)
	}
	q["expectedVersion"] = value.Version
	code, _ = r.act(o.ID, "second-evaluation", r.a, q)
	if code != 409 {
		t.Fatal("second evaluation allowed", code)
	}
}

func clockFor(h *harness) *atomic.Int64 {
	v := &atomic.Int64{}
	v.Store(h.service.Now().UnixNano())
	h.service.Now = func() time.Time { return time.Unix(0, v.Load()).UTC() }
	return v
}
func TestEvaluationInputWindowAndHistoricalReplay(t *testing.T) {
	r, o, _ := completedRepair(t)
	clock := clockFor(r.harness)
	for _, score := range []any{nil, 0, 6, 1.5, "5", true} {
		code, _ := r.act(o.ID, maintenance.ID(), r.a, map[string]any{"action": "EVALUATE", "expectedVersion": o.Version, "score": score})
		if code != 400 {
			t.Fatal("invalid score", score, code)
		}
	}
	code, _ := r.act(o.ID, maintenance.ID(), r.a, map[string]any{"action": "EVALUATE", "expectedVersion": o.Version, "score": 5, "comment": strings.Repeat("字", 501)})
	if code != 400 {
		t.Fatal("oversized evaluation", code)
	}
	code, _ = r.act(o.ID, maintenance.ID(), r.a, map[string]any{"action": "CANCEL", "expectedVersion": o.Version, "score": 5})
	if code != 400 {
		t.Fatal("score accepted on another action", code)
	}
	q := map[string]any{"action": "EVALUATE", "expectedVersion": o.Version, "score": 1, "comment": strings.Repeat("字", 500)}
	clock.Add(int64(7 * 24 * time.Hour))
	code, _ = r.act(o.ID, "at-deadline", r.a, q)
	if code != 409 {
		t.Fatal("deadline must be exclusive", code)
	}
	clock.Add(-int64(time.Microsecond))
	code, raw := r.act(o.ID, "just-before-deadline", r.a, q)
	if code != 200 {
		t.Fatal(code, string(raw))
	}
	clock.Add(int64(time.Hour))
	code, raw = r.act(o.ID, "just-before-deadline", r.a, q)
	if code != 200 {
		t.Fatal("historical replay expired", code, string(raw))
	}
	uncompleted := r.report()
	code, _ = r.act(uncompleted.ID, maintenance.ID(), r.a, map[string]any{"action": "EVALUATE", "expectedVersion": uncompleted.Version, "score": 5})
	if code != 409 {
		t.Fatal("uncompleted order rated", code)
	}
}
func TestEvaluationConcurrentAndAuditRollback(t *testing.T) {
	r, o, _ := completedRepair(t)
	before := r.count("m3_events")
	if _, e := r.db.Exec("CREATE TRIGGER m3_round4_outbox_failure BEFORE INSERT ON m3_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test failure'"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.db.Exec("DROP TRIGGER IF EXISTS m3_round4_outbox_failure") })
	q := map[string]any{"action": "EVALUATE", "expectedVersion": o.Version, "score": 4}
	code, _ := r.act(o.ID, "failed-rating", r.a, q)
	if code != 500 || r.current(o.ID, r.a).Evaluation != nil || r.count("m3_events") != before {
		t.Fatal("failed rating leaked facts", code)
	}
	r.db.Exec("DROP TRIGGER m3_round4_outbox_failure")
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, _ := r.act(o.ID, maintenance.ID(), r.a, q); codes <- c }()
	}
	wg.Wait()
	close(codes)
	passed, conflicts := 0, 0
	for c := range codes {
		if c == 200 {
			passed++
		} else if c == 409 {
			conflicts++
		} else {
			t.Fatal("rating status", c)
		}
	}
	if passed != 1 || conflicts != 1 || r.count("m3_events") != before+1 {
		t.Fatal("duplicate rating")
	}
}
func TestEscalationBoundaryConcurrentAndRestart(t *testing.T) {
	h := setup(t)
	supervisor := h.staff("workorder.supervise", "A楼")
	h.staff("workorder.supervise", "B楼")
	o := h.report()
	clock := clockFor(h)
	clock.Add(int64(4*time.Hour - time.Microsecond))
	if e := h.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	if h.count("m3_escalations") != 0 {
		t.Fatal("premature escalation")
	}
	clock.Add(int64(time.Microsecond))
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := h.service.EscalateOverdue(context.Background()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if h.count("m3_escalations") != 1 || h.count("m3_events") != 2 {
		t.Fatal("stage escalated more than once")
	}
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	var recipients int
	if e := h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE kind='WORKORDER_ESCALATED' AND recipient_id=?", supervisor.Principal.ID).Scan(&recipients); e != nil || recipients != 1 {
		t.Fatal("supervisor intent missing", e, recipients)
	}
	now := h.service.Now
	h.server.Close()
	h.start()
	h.service.Now = now
	if e := h.service.Sweep(context.Background()); e != nil {
		t.Fatal(e)
	}
	if h.count("m3_escalations") != 1 {
		t.Fatal("restart duplicated escalation")
	}
	code, raw := h.request("GET", "/workorders/"+o.ID, "", h.a, nil)
	d := decode[maintenance.Detail](t, raw)
	if code != 200 || !d.Escalated || d.State != "REPORTED" {
		t.Fatal("escalation changed business state", code, string(raw))
	}
}
func TestEscalationMissingDirectoryRoutesSameEventAfterRecovery(t *testing.T) {
	h := setup(t)
	h.report()
	clock := clockFor(h)
	clock.Add(int64(4 * time.Hour))
	h.service.Supervisors = nil
	if e := h.service.Sweep(context.Background()); e != nil {
		t.Fatal(e)
	}
	var routing string
	if e := h.db.QueryRow("SELECT routing_state FROM m3_escalations").Scan(&routing); e != nil || routing != "DIRECTORY_UNAVAILABLE" {
		t.Fatal("lost directory failure", e, routing)
	}
	// Restore the real public-directory adapter, still with no supervisor configured.
	now := h.service.Now
	h.server.Close()
	h.start()
	h.service.Now = now
	clock.Add(int64(30 * time.Second))
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	h.db.QueryRow("SELECT routing_state FROM m3_escalations").Scan(&routing)
	if routing != "NO_SUPERVISOR" {
		t.Fatal("missing supervisor not retained", routing)
	}
	h.staff("workorder.supervise", "A楼")
	clock.Add(int64(30 * time.Second))
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	h.db.QueryRow("SELECT routing_state FROM m3_escalations").Scan(&routing)
	if routing != "QUEUED" || h.count("m3_escalations") != 1 || h.count("m3_events") != 2 {
		t.Fatal("routing repeated business escalation", routing)
	}
}
func TestEscalationRoutingObsoleteAfterStageChange(t *testing.T) {
	h := setup(t)
	o := h.report()
	clock := clockFor(h)
	clock.Add(int64(4 * time.Hour))
	if e := h.service.Sweep(context.Background()); e != nil {
		t.Fatal(e)
	}
	code, raw := h.request("GET", "/workorders/"+o.ID, "", h.a, nil)
	if code != 200 {
		t.Fatal(code)
	}
	o = decode[maintenance.Detail](t, raw).Order
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	h.assigned(o, d, w)
	h.staff("workorder.supervise", "A楼")
	clock.Add(int64(30 * time.Second))
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	var state string
	h.db.QueryRow("SELECT routing_state FROM m3_escalations").Scan(&state)
	if state != "OBSOLETE" {
		t.Fatal("old stage notified", state)
	}
	var n int
	h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE kind='WORKORDER_ESCALATED'").Scan(&n)
	if n != 0 {
		t.Fatal("old stage received new notification")
	}
}
func TestEscalationReassignUnknownAndReworkStages(t *testing.T) {
	r := setupRepair(t)
	o, d, w, a := r.ready()
	clock := clockFor(r.harness)
	clock.Add(int64(4 * time.Hour))
	o = r.command(o, w, "START")
	if e := r.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	if r.count("m3_escalations") != 0 {
		t.Fatal("pending start escalated old stage")
	}
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	clock.Add(int64(24 * time.Hour))
	if e := r.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	o = r.current(o.ID, w).Order
	firstDeadline := *o.DeadlineAt
	next := r.staff("workorder.repair", "A楼")
	o = r.assigned(o, d, next)
	if !o.Escalated || !o.DeadlineAt.Equal(firstDeadline) {
		t.Fatal("handover reset overdue stage")
	}
	o = r.command(o, next, "ACCEPT_ASSIGNMENT")
	o = r.command(o, next, "FINISH")
	clock.Add(int64(24 * time.Hour))
	if e := r.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	o = r.current(o.ID, a).Order
	o = r.command(o, a, "REJECT_ACCEPTANCE")
	if e := r.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	if r.count("m3_escalations") != 3 {
		t.Fatal("repair, acceptance, rework must have distinct stages", r.count("m3_escalations"))
	}
	if !r.sourceActive("M3", o.ID) {
		t.Fatal("escalation released safety source")
	}
}
func TestOperationsPermissionScopePrivacyAndStatistics(t *testing.T) {
	r, o, _ := completedRepair(t)
	stats := r.staff("workorder.stats", "A楼")
	other := r.staff("workorder.stats", "B楼")
	similar := r.staff("workorder.stats", "a楼")
	admin := r.staff("workorder.escalation.read", "A楼")
	code, _ := r.request("GET", "/stats", "", r.a, nil)
	if code != 403 {
		t.Fatal("student read aggregate stats", code)
	}
	q := map[string]any{"action": "EVALUATE", "expectedVersion": o.Version, "score": 5}
	code, _ = r.act(o.ID, "stats-rating", r.a, q)
	if code != 200 {
		t.Fatal(code)
	}
	unrated := r.report()
	code, raw := r.request("GET", "/stats", "", stats, nil)
	s := decode[maintenance.Statistics](t, raw)
	if code != 200 || s.Total != 2 || s.RatedCount != 1 || s.AverageScore == nil || *s.AverageScore != 5 || s.Repair.Count != 1 {
		t.Fatal("unrated denominator or incomplete repair mean", code, string(raw))
	}
	for _, who := range []identity.Session{other, similar} {
		code, raw = r.request("GET", "/stats", "", who, nil)
		if code != 200 || decode[maintenance.Statistics](t, raw).Total != 0 {
			t.Fatal("statistics area leaked", code, string(raw))
		}
	}
	clock := clockFor(r.harness)
	clock.Add(int64(4 * time.Hour))
	if e := r.service.Sweep(context.Background()); e != nil {
		t.Fatal(e)
	}
	code, raw = r.request("GET", "/escalations", "", admin, nil)
	page := decode[maintenance.EscalationPage](t, raw)
	if code != 200 || page.Total != 1 || page.Items[0].WorkorderID != unrated.ID || page.Items[0].CanOpen {
		t.Fatal("escalation exception scope", code, string(raw))
	}
	for _, secret := range []string{r.a.Principal.ID, unrated.Description, "reporterId", "password"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("escalation metadata leaked private detail", secret)
		}
	}
	code, _ = r.request("GET", "/workorders/"+unrated.ID, "", admin, nil)
	if code != 404 {
		t.Fatal("exception read granted full order", code)
	}
	code, raw = r.request("GET", "/stats", "", stats, nil)
	s = decode[maintenance.Statistics](t, raw)
	if code != 200 || s.RecordedOverdueCount != 1 || s.RecordedOverdueRate == nil || *s.RecordedOverdueRate != .5 {
		t.Fatal("overdue denominator", code, string(raw))
	}
	for _, path := range []string{"/stats?from=bad", "/stats?area=B楼", "/stats?from=2030-01-02T00:00:00Z&to=2030-01-01T00:00:00Z", "/escalations?page=0", "/escalations?active=no"} {
		who := stats
		if strings.HasPrefix(path, "/escalations") {
			who = admin
		}
		code, _ = r.request("GET", path, "", who, nil)
		if code != 400 {
			t.Fatal("bad filters accepted", path, code)
		}
	}
}

func TestStatisticsRealDurationsReworkCancellationAndEmptySet(t *testing.T) {
	r := setupRepair(t)
	clock := clockFor(r.harness)
	start := r.service.Now()
	d := r.staff("workorder.dispatch", "A楼")
	w := r.staff("workorder.repair", "A楼")
	a := r.staff("workorder.accept", "A楼")
	stats := r.staff("workorder.stats", "A楼")
	empty := r.staff("workorder.stats", "B楼")
	o := r.report()
	clock.Add(int64(10 * time.Minute))
	o = r.assigned(o, d, w)
	o = r.command(o, w, "ACCEPT_ASSIGNMENT")
	clock.Add(int64(10 * time.Minute))
	o = r.command(o, w, "START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, w).Order
	clock.Add(int64(30 * time.Minute))
	o = r.command(o, w, "FINISH")
	clock.Add(int64(10 * time.Minute))
	o = r.command(o, a, "REJECT_ACCEPTANCE")
	clock.Add(int64(15 * time.Minute))
	o = r.command(o, w, "FINISH")
	clock.Add(int64(5 * time.Minute))
	o = r.command(o, a, "APPROVE_ACCEPTANCE")
	clock.Add(int64(2 * time.Minute))
	r.run(o.Operation.ID)
	cancel := r.report()
	r.command(cancel, r.a, "CANCEL")
	code, raw := r.request("GET", "/stats", "", stats, nil)
	s := decode[maintenance.Statistics](t, raw)
	if code != 200 || s.Total != 2 || s.EligibleCount != 1 || s.ByState["CANCELLED"] != 1 || s.RatedCount != 0 || s.AverageScore != nil {
		t.Fatal("stats counts", code, string(raw))
	}
	for name, entry := range map[string]struct {
		metric maintenance.Metric
		want   float64
	}{"dispatch": {s.Dispatch, 600}, "first": {s.FirstRepair, 1200}, "repair": {s.Repair, 2700}, "acceptance": {s.Acceptance, 900}} {
		if entry.metric.Count != 1 || entry.metric.AverageSeconds == nil || *entry.metric.AverageSeconds != entry.want {
			t.Fatal("wrong duration", name, entry)
		}
	}
	code, raw = r.request("GET", "/stats?from="+start.Format(time.RFC3339)+"&to="+start.Add(time.Minute).Format(time.RFC3339), "", stats, nil)
	if code != 200 || decode[maintenance.Statistics](t, raw).Total != 1 {
		t.Fatal("created-at cohort filter", code, string(raw))
	}
	code, raw = r.request("GET", "/stats", "", empty, nil)
	s = decode[maintenance.Statistics](t, raw)
	if code != 200 || s.Total != 0 || s.AverageScore != nil || s.Repair.AverageSeconds != nil || s.RecordedOverdueRate != nil {
		t.Fatal("empty averages must be null", code, string(raw))
	}
}

func TestEscalationAuditRollbackAndOutboxRetry(t *testing.T) {
	h := setup(t)
	h.staff("workorder.supervise", "A楼")
	o := h.report()
	clock := clockFor(h)
	clock.Add(int64(4 * time.Hour))
	if _, e := h.db.Exec("CREATE TRIGGER m3_round4_audit_failure BEFORE INSERT ON m3_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test failure'"); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		h.db.Exec("DROP TRIGGER IF EXISTS m3_round4_audit_failure")
		h.db.Exec("DROP TRIGGER IF EXISTS m3_round4_route_failure")
	})
	if e := h.service.EscalateOverdue(context.Background()); e == nil {
		t.Fatal("audit failure ignored")
	}
	if h.count("m3_escalations") != 0 {
		t.Fatal("failed escalation leaked row")
	}
	code, raw := h.request("GET", "/workorders/"+o.ID, "", h.a, nil)
	if code != 200 || decode[maintenance.Detail](t, raw).Escalated {
		t.Fatal("failed escalation changed order")
	}
	h.db.Exec("DROP TRIGGER m3_round4_audit_failure")
	if e := h.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := h.db.Exec("CREATE TRIGGER m3_round4_route_failure BEFORE INSERT ON m3_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test failure'"); e != nil {
		t.Fatal(e)
	}
	if e := h.service.RouteEscalations(context.Background()); e == nil {
		t.Fatal("outbox failure ignored")
	}
	if h.count("m3_escalations") != 1 || h.count("m3_events") != 2 {
		t.Fatal("notification failure lost business fact")
	}
	h.db.Exec("DROP TRIGGER m3_round4_route_failure")
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	var n int
	h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE kind='WORKORDER_ESCALATED'").Scan(&n)
	if n != 1 {
		t.Fatal("route duplicated notification intent", n)
	}
}
