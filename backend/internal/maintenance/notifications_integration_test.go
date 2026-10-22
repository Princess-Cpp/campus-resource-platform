package maintenance_test

import (
	"campus-resource-platform/backend/internal/identity"
	"encoding/json"
	"strings"
	"testing"
)

type notificationPage struct {
	Items []struct {
		EventID       string  `json:"eventId"`
		Kind          string  `json:"kind"`
		ReadAt        *string `json:"readAt"`
		CanOpen       bool    `json:"canOpen"`
		DeliveryState string  `json:"deliveryState"`
	} `json:"items"`
	Total       int `json:"total"`
	UnreadCount int `json:"unreadCount"`
}

func TestNotificationInboxOwnerReadIsolation(t *testing.T) {
	h := setup(t)
	h.report()
	code, raw := h.request("GET", "/notifications", "", h.a, nil)
	if code != 200 {
		t.Fatalf("inbox status=%d body=%s", code, raw)
	}
	page := decode[notificationPage](t, raw)
	if page.Total != 1 || page.UnreadCount != 1 || len(page.Items) != 1 || !page.Items[0].CanOpen {
		t.Fatalf("unexpected page %s", raw)
	}
	n := page.Items[0]
	for _, s := range []identity.Session{h.b, h.staff("workorder.dispatch", "A楼")} {
		code, raw = h.request("GET", "/notifications", "", s, nil)
		if code != 200 || decode[notificationPage](t, raw).Total != 0 {
			t.Fatal("other inbox leaked", code, string(raw))
		}
		code, raw = h.request("POST", "/notifications/"+n.EventID+"/read", "", s, map[string]string{"kind": n.Kind})
		if code != 404 {
			t.Fatal("other user marked notification", code, string(raw))
		}
	}
	for i := 0; i < 2; i++ {
		code, raw = h.request("POST", "/notifications/"+n.EventID+"/read", "", h.a, map[string]string{"kind": n.Kind})
		if code != 200 {
			t.Fatal(code, string(raw))
		}
	}
	code, raw = h.request("GET", "/notifications", "", h.a, nil)
	page = decode[notificationPage](t, raw)
	if code != 200 || page.UnreadCount != 0 || page.Items[0].ReadAt == nil {
		t.Fatal("read not idempotently persisted", code, string(raw))
	}
	for _, q := range []string{"?userId=other", "?page=0", "?pageSize=101", "?unread=maybe", "?page=1&page=2"} {
		code, _ = h.request("GET", "/notifications"+q, "", h.a, nil)
		if code != 400 {
			t.Fatal("invalid filter", q, code)
		}
	}
	code, _ = h.request("GET", "/notifications", "", identity.Session{}, nil)
	if code != 401 {
		t.Fatal("anonymous inbox", code)
	}
}

func TestNotificationInboxPaginationAndMinimalPayload(t *testing.T) {
	h := setup(t)
	for i := 0; i < 3; i++ {
		h.report()
	}
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		code, raw := h.request("GET", "/notifications?pageSize=1&page="+string(rune('0'+page)), "", h.a, nil)
		p := decode[notificationPage](t, raw)
		if code != 200 || p.Total != 3 || len(p.Items) != 1 || seen[p.Items[0].EventID] {
			t.Fatal("pagination", code, string(raw))
		}
		seen[p.Items[0].EventID] = true
		var anyBody map[string]any
		json.Unmarshal(raw, &anyBody)
		if strings.Contains(string(raw), input().Description) || strings.Contains(string(raw), h.a.Principal.ID) || strings.Contains(string(raw), "payload") {
			t.Fatal("private raw event exposed", string(raw))
		}
	}
}

func TestNotificationResourceIntentDoesNotClaimSuccess(t *testing.T) {
	r := setupRepair(t)
	o, _, worker, acceptor := r.ready()
	o = r.command(o, worker, "START")
	assertPendingSummary := func(kind string) {
		t.Helper()
		code, raw := r.request("GET", "/notifications", "", r.a, nil)
		if code != 200 {
			t.Fatalf("notification query %d %s", code, raw)
		}
		type summaryPage struct {
			Items []struct {
				Kind    string `json:"kind"`
				Summary string `json:"summary"`
			}
		}
		page := decode[summaryPage](t, raw)
		found := false
		for _, n := range page.Items {
			if n.Kind == kind {
				found = true
				if !strings.Contains(n.Summary, "等待") || !strings.Contains(n.Summary, "确认") {
					t.Fatalf("pending resource intent claims success: %s", n.Summary)
				}
			}
		}
		if !found {
			t.Fatalf("missing notification %s", kind)
		}
	}
	if o.State != "ASSIGNED" || o.SyncState == "IDLE" {
		t.Fatalf("start unexpectedly confirmed: %+v", o)
	}
	assertPendingSummary("WORKORDER_START")
	r.run(o.Operation.ID)
	o = r.current(o.ID, worker).Order
	o = r.command(o, worker, "FINISH")
	o = r.command(o, acceptor, "APPROVE_ACCEPTANCE")
	if o.State != "AWAITING_ACCEPTANCE" || o.SyncState == "IDLE" {
		t.Fatalf("release unexpectedly confirmed: %+v", o)
	}
	assertPendingSummary("WORKORDER_APPROVE_ACCEPTANCE")
	r.run(o.Operation.ID)
	if r.current(o.ID, r.a).State != "COMPLETED" {
		t.Fatal("resource receipt did not complete")
	}
}
