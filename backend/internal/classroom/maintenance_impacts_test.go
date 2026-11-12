package classroom_test

import (
	"bytes"
	"campus-resource-platform/backend/internal/booking"
	"campus-resource-platform/backend/internal/classroom"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const impactTestToken = "test-only-maintenance-delivery-token-32bytes"

func attachImpacts(t *testing.T, h *harness, lose *atomic.Bool) *httptest.Server {
	t.Helper()
	router := gin.New()
	if err := h.s.RegisterMaintenanceImpacts(router, impactTestToken); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out := httptest.NewRecorder()
		router.ServeHTTP(out, r)
		for k, v := range out.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(out.Code)
		if lose != nil && lose.Load() && out.Code == 202 {
			w.Write([]byte("lost"))
			return
		}
		w.Write(out.Body.Bytes())
	}))
	t.Cleanup(server.Close)
	sender, err := booking.NewHTTPImpactSender(server.URL, impactTestToken)
	if err != nil {
		t.Fatal(err)
	}
	h.resource.ImpactSender = sender
	return server
}
func approveImpactApplication(h *harness, key string) classroom.Application {
	b := h.create(key)
	b = h.action(b, "department-demo", "APPROVE", key+"-dep")
	b = h.action(b, "venue-demo", "APPROVE", key+"-venue")
	if err := h.s.RunOperation(context.Background(), b.OperationID); err != nil {
		h.t.Fatal(err)
	}
	return h.get(b.ID)
}
func blockForImpact(h *harness, business string) {
	q := booking.Operation{OperationID: "block-" + business, Action: "BLOCK_ROOM_MAINTENANCE", SourceModule: "M3", SourceBusinessID: business, SourceVersion: 1, RoomID: "room-101"}
	code, raw := h.request("POST", "/internal/resources/operations", "m3-service", q.OperationID, q)
	if code != 200 {
		h.t.Fatalf("block %d %s", code, raw)
	}
}
func releaseImpactBlock(h *harness, business string) {
	q := booking.Operation{OperationID: "release-" + business, Action: "RELEASE_SOURCE", SourceModule: "M3", SourceBusinessID: business, SourceVersion: 2, RoomID: "room-101"}
	code, raw := h.request("POST", "/internal/resources/operations", "m3-service", q.OperationID, q)
	if code != 200 {
		h.t.Fatalf("release %d %s", code, raw)
	}
}
func impactCount(t *testing.T, h *harness, query string) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestMaintenanceImpactHTTPRecoveryAndSourceIsolation(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	var lose atomic.Bool
	attachImpacts(t, h, &lose)
	b := approveImpactApplication(h, "affected")
	blockForImpact(h, "repair-a")
	blockForImpact(h, "repair-b")
	if n := impactCount(t, h, "SELECT COUNT(*) FROM m1_maintenance_impacts"); n != 2 {
		t.Fatal(n)
	}
	lose.Store(true)
	if err := h.resource.DeliverMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if n := impactCount(t, h, "SELECT COUNT(*) FROM m2_maintenance_impacts"); n != 2 {
		t.Fatal("receipt-loss dropped intents", n)
	}
	lose.Store(false)
	h.now = h.now.Add(3 * time.Second)
	if err := h.resource.DeliverMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if n := impactCount(t, h, "SELECT COUNT(*) FROM m2_maintenance_impacts"); n != 2 {
		t.Fatal("duplicate inbox", n)
	}
	// Reconstruct the consumer after durable receipt; no in-memory event state survives.
	h.s = classroom.New(h.db, h.directory, &classroom.HTTPResources{BaseURL: h.server.URL, Token: "m2-service"})
	h.s.Now = func() time.Time { return h.now }
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := h.s.ProcessMaintenanceImpacts(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	current := h.get(b.ID)
	if current.Status != "APPROVED" || current.SyncStatus != "PENDING" {
		t.Fatal("premature cancellation", current.Status, current.SyncStatus)
	}
	if n := impactCount(t, h, "SELECT COUNT(*) FROM m2_operations WHERE target_status='CANCELLED'"); n != 1 {
		t.Fatal("duplicate compensation", n)
	}
	if err := h.s.RunOperation(ctx, current.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ProcessMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if h.get(b.ID).Status != "CANCELLED" {
		t.Fatal("not cancelled")
	}
	if n := impactCount(t, h, "SELECT COUNT(*) FROM m1_sources WHERE module='M3' AND active=1"); n != 2 {
		t.Fatal("other sources released", n)
	}
	if n := impactCount(t, h, "SELECT COUNT(*) FROM m2_maintenance_impacts WHERE state='DONE'"); n != 2 {
		t.Fatal("not complete", n)
	}
}
func TestMaintenanceImpactLateDeliveryDoesNotCancelNewActivity(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	attachImpacts(t, h, nil)
	old := approveImpactApplication(h, "old-activity")
	h.now = old.StartAt.Add(time.Minute)
	blockForImpact(h, "repair-late")
	releaseImpactBlock(h, "repair-late")
	// Move to an integer hour for the existing application input helper.
	h.now = h.now.Add(59 * time.Minute)
	newer := approveImpactApplication(h, "new-after-repair")
	if err := h.resource.DeliverMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ProcessMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	pending := h.get(old.ID)
	if pending.SyncStatus != "PENDING" {
		t.Fatal("old activity not interrupted")
	}
	if err := h.s.RunOperation(ctx, pending.OperationID); err != nil {
		t.Fatal(err)
	}
	if h.get(old.ID).Status != "TERMINATED" || h.get(newer.ID).Status != "APPROVED" {
		t.Fatal("late impact affected new application")
	}
}
func TestMaintenanceImpactWaitsForUnknownApprovalAndRollsBackAudit(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	attachImpacts(t, h, nil)
	b := h.create("unknown")
	b = h.action(b, "department-demo", "APPROVE", "dep")
	b = h.action(b, "venue-demo", "APPROVE", "venue")
	var raw []byte
	if err := h.db.QueryRow("SELECT request FROM m2_operations WHERE id=?", b.OperationID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var q classroom.Operation
	if err := json.Unmarshal(raw, &q); err != nil {
		t.Fatal(err)
	}
	code, out := h.request("POST", "/internal/resources/operations", "m2-service", q.OperationID, q)
	if code != 200 {
		t.Fatal(code, string(out))
	}
	blockForImpact(h, "unknown-repair")
	if err := h.resource.DeliverMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ProcessMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if h.get(b.ID).OperationID != b.OperationID {
		t.Fatal("unknown approval overwritten")
	}
	if err := h.s.RunOperation(ctx, b.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec("CREATE TRIGGER impact_audit_failure BEFORE INSERT ON m2_events FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.db.Exec("DROP TRIGGER IF EXISTS impact_audit_failure") })
	if err := h.s.ProcessMaintenanceImpacts(ctx); err == nil {
		t.Fatal("audit error ignored")
	}
	if h.get(b.ID).SyncStatus != "IDLE" || impactCount(t, h, "SELECT COUNT(*) FROM m2_operations") != 1 {
		t.Fatal("partial compensation")
	}
	h.db.Exec("DROP TRIGGER impact_audit_failure")
	if err := h.s.ProcessMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	pending := h.get(b.ID)
	if err := h.s.RunOperation(ctx, pending.OperationID); err != nil {
		t.Fatal(err)
	}
	if h.get(b.ID).Status != "CANCELLED" {
		t.Fatal("unknown approval not compensated")
	}
}

func TestMaintenanceImpactAuthenticationIdempotencyAndOutboxRollback(t *testing.T) {
	h := setup(t)
	server := attachImpacts(t, h, nil)
	b := approveImpactApplication(h, "secure")
	if _, err := h.db.Exec("CREATE TRIGGER impact_outbox_failure BEFORE INSERT ON m1_maintenance_impacts FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.db.Exec("DROP TRIGGER IF EXISTS impact_outbox_failure") })
	q := booking.Operation{OperationID: "atomic-block", Action: "BLOCK_ROOM_MAINTENANCE", SourceModule: "M3", SourceBusinessID: "secure-repair", SourceVersion: 1, RoomID: b.RoomID}
	code, _ := h.request("POST", "/internal/resources/operations", "m3-service", q.OperationID, q)
	if code != 500 || impactCount(t, h, "SELECT COUNT(*) FROM m1_sources WHERE module='M3'") != 0 {
		t.Fatal("outbox failure left block", code)
	}
	h.db.Exec("DROP TRIGGER impact_outbox_failure")
	code, _ = h.request("POST", "/internal/resources/operations", "m3-service", q.OperationID, q)
	if code != 200 {
		t.Fatal(code)
	}
	var raw []byte
	if err := h.db.QueryRow("SELECT payload FROM m1_maintenance_impacts").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event classroom.MaintenanceImpact
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	post := func(token string, cookie bool, value classroom.MaintenanceImpact) int {
		body, _ := json.Marshal(value)
		req, _ := http.NewRequest("POST", server.URL+"/api/v1/internal/m2/maintenance-impacts", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if cookie {
			req.Header.Set("Cookie", "campus_session=fake")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		return res.StatusCode
	}
	for _, token := range []string{"student-demo", "m3-service", "m2-service", ""} {
		if code := post(token, false, event); code != 401 {
			t.Fatal("unauthorized event", code)
		}
	}
	if code := post(impactTestToken, true, event); code != 401 {
		t.Fatal("browser cookie accepted", code)
	}
	for i := 0; i < 2; i++ {
		if code := post(impactTestToken, false, event); code != 202 {
			t.Fatal(code)
		}
	}
	event.RoomID = "different-room"
	if code := post(impactTestToken, false, event); code != 409 {
		t.Fatal("event ID reused with changed payload", code)
	}
	if impactCount(t, h, "SELECT COUNT(*) FROM m2_maintenance_impacts") != 1 {
		t.Fatal("duplicate receiver intent")
	}
}
func TestMaintenanceImpactRetryBudgetAndExplicitRecovery(t *testing.T) {
	h := setup(t)
	var lose atomic.Bool
	lose.Store(true)
	attachImpacts(t, h, &lose)
	b := approveImpactApplication(h, "budget")
	blockForImpact(h, "budget-repair")
	for i := 0; i < 8; i++ {
		if err := h.resource.DeliverMaintenanceImpacts(context.Background()); err != nil {
			t.Fatal(err)
		}
		h.now = h.now.Add(300 * time.Second)
	}
	if impactCount(t, h, "SELECT COUNT(*) FROM m1_maintenance_impacts WHERE state='FAILED' AND attempts=8") != 1 {
		t.Fatal("retry budget ignored")
	}
	var id string
	if err := h.db.QueryRow("SELECT id FROM m1_maintenance_impacts").Scan(&id); err != nil {
		t.Fatal(err)
	}
	code, _ := h.request("POST", "/internal/resources/maintenance-impacts/"+id+"/retry", "m2-service", "", nil)
	if code != 403 {
		t.Fatal("wrong module retry", code)
	}
	code, _ = h.request("POST", "/internal/resources/maintenance-impacts/"+id+"/retry", "m3-service", "", nil)
	if code != 200 {
		t.Fatal("manual retry", code)
	}
	lose.Store(false)
	if err := h.resource.DeliverMaintenanceImpacts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if impactCount(t, h, "SELECT COUNT(*) FROM m1_maintenance_impacts WHERE state='DELIVERED'") != 1 {
		t.Fatal("not recovered")
	}
	if err := h.s.ProcessMaintenanceImpacts(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending := h.get(b.ID)
	if err := h.s.RunOperation(context.Background(), pending.OperationID); err != nil {
		t.Fatal(err)
	}
	if h.get(b.ID).Status != "CANCELLED" {
		t.Fatal("not compensated")
	}
}

func TestMaintenanceImpactUnresolvedBatchDoesNotStarveLaterEvents(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	attachImpacts(t, h, nil)
	b := approveImpactApplication(h, "after-unresolved-batch")
	blockForImpact(h, "repair-after-batch")
	var raw []byte
	if err := h.db.QueryRow("SELECT payload FROM m1_maintenance_impacts LIMIT 1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event classroom.MaintenanceImpact
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		event.EventID = fmt.Sprintf("0.pending-%03d", i)
		event.ApplicationID = "missing-application"
		if err := h.s.ReceiveMaintenanceImpact(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.resource.DeliverMaintenanceImpacts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.s.ProcessMaintenanceImpacts(ctx); err == nil {
		t.Fatal("missing applications should remain visible as failures")
	}
	current := h.get(b.ID)
	if current.SyncStatus != "PENDING" {
		t.Fatal("unresolved first batch starved later event", current.SyncStatus)
	}
	if err := h.s.RunOperation(ctx, current.OperationID); err != nil {
		t.Fatal(err)
	}
	if h.get(b.ID).Status != "CANCELLED" {
		t.Fatal("later event was not completed")
	}
}
