package maintenance_test

import (
	"campus-resource-platform/backend/internal/maintenance"
	maintenancedev "campus-resource-platform/backend/internal/maintenance/dev"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type notificationSendFunc func(context.Context, string, string) error

func (f notificationSendFunc) Send(ctx context.Context, user, event string) error {
	return f(ctx, user, event)
}
func notificationDirectory(h *harness) {
	h.service.NotificationRecipients = &maintenancedev.Directory{Store: h.auth, StudentReporting: true}
}

func TestNotificationDeliveryConcurrentClaimAndRestart(t *testing.T) {
	h := setup(t)
	o := h.report()
	notificationDirectory(h)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	h.service.NotificationSender = notificationSendFunc(func(context.Context, string, string) error {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return nil
	})
	done := make(chan error, 1)
	go func() { done <- h.service.DispatchNotifications(context.Background()) }()
	<-entered
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal("concurrent double send", calls.Load())
	}
	var state string
	var attempts int
	h.db.QueryRow("SELECT status,attempts FROM m3_outbox WHERE workorder_id=?", o.ID).Scan(&state, &attempts)
	if state != "DELIVERED" || attempts != 1 {
		t.Fatal(state, attempts)
	}
	h.start()
	notificationDirectory(h)
	h.service.NotificationSender = notificationSendFunc(func(context.Context, string, string) error { calls.Add(1); return nil })
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		t.Fatal("restart repeated delivered event")
	}
	if h.count("m3_events") != 1 || h.count("m3_commands") != 1 || h.count("m3_workorders") != 1 {
		t.Fatal("delivery repeated business")
	}
}

func TestNotificationDeliveryRetryLimitAndUnconfigured(t *testing.T) {
	h := setup(t)
	h.report()
	notificationDirectory(h)
	clock := clockFor(h)
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	var state string
	var attempts int
	inspect := func() {
		t.Helper()
		if e := h.db.QueryRow("SELECT status,attempts FROM m3_outbox").Scan(&state, &attempts); e != nil {
			t.Fatal(e)
		}
	}
	inspect()
	if state != "UNCONFIGURED" || attempts != 0 {
		t.Fatal("missing channel counted as sent", state, attempts)
	}
	var calls atomic.Int64
	h.service.NotificationSender = notificationSendFunc(func(context.Context, string, string) error {
		calls.Add(1)
		return errors.New("SMTP test failure: must not store raw server response")
	})
	clock.Add(int64(time.Minute))
	for attempt := 1; attempt <= 5; attempt++ {
		if e := h.service.DispatchNotifications(context.Background()); e != nil {
			t.Fatal(e)
		}
		inspect()
		want := "PENDING"
		if attempt == 5 {
			want = "FAILED"
		}
		if state != want || attempts != attempt {
			t.Fatal("retry state", state, attempts)
		}
		if attempt < 5 {
			clock.Add(int64(time.Minute*time.Duration(1<<(attempt-1))) - int64(time.Microsecond))
			if e := h.service.DispatchNotifications(context.Background()); e != nil {
				t.Fatal(e)
			}
			if int(calls.Load()) != attempt {
				t.Fatal("retry before boundary")
			}
			clock.Add(int64(time.Microsecond))
		}
	}
	clock.Add(int64(24 * time.Hour))
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 5 {
		t.Fatal("unbounded retries", calls.Load())
	}
	var last string
	h.db.QueryRow("SELECT last_error FROM m3_outbox").Scan(&last)
	if last != "DELIVERY_UNAVAILABLE" {
		t.Fatal("raw or missing error", last)
	}
	code, raw := h.request("GET", "/notifications", "", h.a, nil)
	if code != 200 || decode[notificationPage](t, raw).Items[0].DeliveryState != "FAILED" {
		t.Fatal("failure not visible", code, string(raw))
	}
}

func TestNotificationDeliveryExpiredLeaseRecoversSameIdentity(t *testing.T) {
	h := setup(t)
	h.report()
	notificationDirectory(h)
	clock := clockFor(h)
	// Simulate the persisted boundary after a worker sent but crashed before ack.
	var event string
	h.db.QueryRow("SELECT event_id FROM m3_outbox").Scan(&event)
	if _, e := h.db.Exec("UPDATE m3_outbox SET status='SENDING',attempts=1,lease_token='crashed',lease_until=?", h.service.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	var identities []string
	h.service.NotificationSender = notificationSendFunc(func(_ context.Context, _ string, id string) error { identities = append(identities, id); return nil })
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(identities) != 0 {
		t.Fatal("active lease stolen")
	}
	clock.Add(int64(time.Minute))
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(identities) != 1 || !strings.Contains(identities[0], event) {
		t.Fatal("recovery changed logical event", identities)
	}
	var attempts int
	h.db.QueryRow("SELECT attempts FROM m3_outbox").Scan(&attempts)
	if attempts != 2 {
		t.Fatal("lost prior attempt", attempts)
	}
}

func TestNotificationDeliveryReassignmentRevocationAndPrivacy(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	old := h.staff("workorder.repair", "A楼")
	next := h.staff("workorder.repair", "A楼")
	o := h.assigned(h.report(), d, old)
	h.assigned(o, d, next)
	notificationDirectory(h)
	sent := map[string][]string{}
	h.service.NotificationSender = notificationSendFunc(func(_ context.Context, user, id string) error { sent[user] = append(sent[user], id); return nil })
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	var suppressed, delivered int
	h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE recipient_id=? AND kind='WORKORDER_ASSIGNED' AND status='SUPPRESSED'", old.Principal.ID).Scan(&suppressed)
	h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE recipient_id=? AND kind='WORKORDER_HANDOVER' AND status='DELIVERED'", old.Principal.ID).Scan(&delivered)
	if suppressed != 1 || delivered != 1 || len(sent[old.Principal.ID]) != 1 {
		t.Fatal("old worker received new details", suppressed, delivered, sent)
	}
	code, raw := h.request("GET", "/notifications", "", old, nil)
	if code != 200 {
		t.Fatal(code, string(raw))
	}
	for _, n := range decode[notificationPage](t, raw).Items {
		if n.CanOpen {
			t.Fatal("old worker can open", string(raw))
		}
	}
	h.report()
	if _, e := h.db.Exec("UPDATE d_users SET active=0 WHERE id=?", h.a.Principal.ID); e != nil {
		t.Fatal(e)
	}
	previous := len(sent[h.a.Principal.ID])
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(sent[h.a.Principal.ID]) != previous {
		t.Fatal("disabled recipient sent mail")
	}
}

func TestNotificationRevokedAreaAlsoRemovesBusinessAccess(t *testing.T) {
	h := setup(t)
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.assigned(h.report(), d, w)
	if _, e := h.db.Exec("DELETE FROM d_user_areas WHERE user_id=?", w.Principal.ID); e != nil {
		t.Fatal(e)
	}
	code, raw := h.request("GET", "/workorders?view=assigned", "", w, nil)
	if code != 200 || decode[maintenance.Page](t, raw).Total != 0 {
		t.Fatal("revoked area still listed", code, string(raw))
	}
	code, _ = h.request("GET", "/workorders/"+o.ID, "", w, nil)
	if code != 404 {
		t.Fatal("revoked area can read", code)
	}
	code, _ = h.act(o.ID, "revoked-accept", w, action("ACCEPT_ASSIGNMENT", o.Version, ""))
	if code != 404 {
		t.Fatal("revoked area can act", code)
	}
	code, raw = h.request("GET", "/notifications", "", w, nil)
	if code != 200 {
		t.Fatal(code, string(raw))
	}
	for _, n := range decode[notificationPage](t, raw).Items {
		if n.CanOpen {
			t.Fatal("revoked area can open notification")
		}
	}
	notificationDirectory(h)
	var sent atomic.Int64
	h.service.NotificationSender = notificationSendFunc(func(_ context.Context, id, _ string) error {
		if id == w.Principal.ID {
			sent.Add(1)
		}
		return nil
	})
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if sent.Load() != 0 {
		t.Fatal("revoked area still notified")
	}
}

func TestNotificationLeaseFencesLateAcknowledgement(t *testing.T) {
	h := setup(t)
	h.report()
	notificationDirectory(h)
	clock := clockFor(h)
	firstEntered, secondEntered := make(chan struct{}), make(chan struct{})
	firstRelease, secondRelease := make(chan struct{}), make(chan struct{})
	var releaseFirst, releaseSecond sync.Once
	defer releaseFirst.Do(func() { close(firstRelease) })
	defer releaseSecond.Do(func() { close(secondRelease) })
	var calls atomic.Int64
	h.service.NotificationSender = notificationSendFunc(func(context.Context, string, string) error {
		switch calls.Add(1) {
		case 1:
			close(firstEntered)
			<-firstRelease
			return errors.New("late old result")
		case 2:
			close(secondEntered)
			<-secondRelease
		}
		return nil
	})
	firstDone := make(chan error, 1)
	go func() { firstDone <- h.service.DispatchNotifications(context.Background()) }()
	select {
	case <-firstEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("first claimant did not start")
	}
	clock.Add(int64(30 * time.Second))
	secondDone := make(chan error, 1)
	go func() { secondDone <- h.service.DispatchNotifications(context.Background()) }()
	select {
	case <-secondEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("second claimant did not recover expired lease")
	}
	releaseFirst.Do(func() { close(firstRelease) })
	if e := <-firstDone; e != nil {
		t.Fatal(e)
	}
	var state string
	var attempts int
	if e := h.db.QueryRow("SELECT status,attempts FROM m3_outbox").Scan(&state, &attempts); e != nil {
		t.Fatal(e)
	}
	if state != "SENDING" || attempts != 2 || calls.Load() != 2 {
		t.Fatal("old claimant overwrote active new lease", state, attempts, calls.Load())
	}
	releaseSecond.Do(func() { close(secondRelease) })
	if e := <-secondDone; e != nil {
		t.Fatal(e)
	}
	if e := h.db.QueryRow("SELECT status FROM m3_outbox").Scan(&state); e != nil {
		t.Fatal(e)
	}
	if state != "DELIVERED" {
		t.Fatal(state)
	}
}

func TestNotificationDirectoryFailureAndMalformedRecipient(t *testing.T) {
	h := setup(t)
	h.report()
	clock := clockFor(h)
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	var status, last string
	var attempts int
	if e := h.db.QueryRow("SELECT status,last_error,attempts FROM m3_outbox").Scan(&status, &last, &attempts); e != nil {
		t.Fatal(e)
	}
	if status != "PENDING" || last != "DIRECTORY_UNAVAILABLE" || attempts != 0 {
		t.Fatal("lost unavailable directory", status, last, attempts)
	}
	notificationDirectory(h)
	h.service.NotificationSender = notificationSendFunc(func(context.Context, string, string) error { return maintenance.ErrNotificationRecipientInvalid })
	clock.Add(int64(time.Minute))
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := h.db.QueryRow("SELECT status,last_error,attempts FROM m3_outbox").Scan(&status, &last, &attempts); e != nil {
		t.Fatal(e)
	}
	if status != "FAILED" || last != "INVALID_RECIPIENT" || attempts != 1 {
		t.Fatal("bad recipient not diagnosable", status, last, attempts)
	}
}

func TestNotificationObsoleteEscalationSuppressedBeforeSend(t *testing.T) {
	h := setup(t)
	clock := clockFor(h)
	supervisor := h.staff("workorder.supervise", "A楼")
	d := h.staff("workorder.dispatch", "A楼")
	w := h.staff("workorder.repair", "A楼")
	o := h.report()
	clock.Add(int64(4 * time.Hour))
	if e := h.service.EscalateOverdue(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := h.service.RouteEscalations(context.Background()); e != nil {
		t.Fatal(e)
	}
	code, raw := h.request("GET", "/workorders/"+o.ID, "", d, nil)
	if code != 200 {
		t.Fatal(code, string(raw))
	}
	o = decode[maintenance.Detail](t, raw).Order
	h.assigned(o, d, w)
	notificationDirectory(h)
	var sent atomic.Int64
	h.service.NotificationSender = notificationSendFunc(func(_ context.Context, id, _ string) error {
		if id == supervisor.Principal.ID {
			sent.Add(1)
		}
		return nil
	})
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	if sent.Load() != 0 {
		t.Fatal("obsolete escalation delivered")
	}
	var count int
	if e := h.db.QueryRow("SELECT COUNT(*) FROM m3_outbox WHERE kind='WORKORDER_ESCALATED' AND status='SUPPRESSED'").Scan(&count); e != nil || count != 1 {
		t.Fatal("obsolete event not retained as suppressed", count, e)
	}
}
