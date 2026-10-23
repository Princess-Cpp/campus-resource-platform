package maintenance_test

import (
	"testing"

	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
)

// Real Cookie/CSRF HTTP requests and SQL persistence, including a system
// administration permission which must not imply any property business role.
func TestMaintenanceRoleMatrixRejectsUnrelatedActionsWithoutWrites(t *testing.T) {
	r := setupRepair(t)
	o, dispatcher, worker, acceptor := r.ready()
	supervisor := r.staff("workorder.supervise", "A楼")
	operator := r.staff("workorder.stats", "A楼")
	admin := r.staff("account.manage", "A楼")
	foreign := r.staff("workorder.dispatch", "B楼")
	unassigned := r.staff("workorder.repair", "A楼")
	all := []string{"ASSIGN", "ACCEPT_ASSIGNMENT", "START", "FINISH", "APPROVE_ACCEPTANCE", "REJECT_ACCEPTANCE", "RETRY_RESOURCE"}
	type roleCase struct {
		name   string
		who    identity.Session
		read   int
		denied []string
		queues [5]int // dispatch, assigned, acceptance, stats, escalations
	}
	cases := []roleCase{
		{"reporter", r.a, 200, all, [5]int{403, 403, 403, 403, 403}},
		{"other_student", r.b, 404, all, [5]int{403, 403, 403, 403, 403}},
		{"system_admin_only", admin, 404, all, [5]int{403, 403, 403, 403, 403}},
		{"statistics_only", operator, 404, all, [5]int{403, 403, 403, 200, 403}},
		{"supervisor_only", supervisor, 200, all, [5]int{403, 403, 403, 403, 200}},
		{"outside_area", foreign, 404, all, [5]int{200, 403, 403, 403, 403}},
		{"other_repairer", unassigned, 404, all, [5]int{403, 200, 403, 403, 403}},
		{"dispatcher", dispatcher, 200, []string{"START", "FINISH", "ACCEPT_ASSIGNMENT", "APPROVE_ACCEPTANCE", "REJECT_ACCEPTANCE", "RETRY_RESOURCE"}, [5]int{200, 403, 403, 403, 403}},
		{"assigned_repairer", worker, 200, []string{"ASSIGN", "APPROVE_ACCEPTANCE", "REJECT_ACCEPTANCE", "RETRY_RESOURCE"}, [5]int{403, 200, 403, 403, 403}},
		{"independent_acceptor", acceptor, 200, []string{"ASSIGN", "START", "FINISH", "ACCEPT_ASSIGNMENT", "RETRY_RESOURCE"}, [5]int{403, 403, 200, 403, 403}},
	}
	counts := map[string]int{}
	for _, table := range []string{"m3_commands", "m3_events", "m3_assignment_history", "m3_resource_operations"} {
		counts[table] = r.count(table)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, raw := r.request("GET", "/workorders/"+o.ID, "", tc.who, nil)
			if code != tc.read {
				t.Fatalf("detail %d, want %d: %s", code, tc.read, raw)
			}
			wantAction := 403
			if tc.read == 404 {
				wantAction = 404
			}
			for _, name := range tc.denied {
				q := action(name, o.Version, "")
				if name == "ASSIGN" {
					q["assigneeId"] = worker.Principal.ID
				}
				code, raw = r.act(o.ID, maintenance.ID(), tc.who, q)
				if code != wantAction {
					t.Fatalf("%s %d, want %d: %s", name, code, wantAction, raw)
				}
			}
			for i, path := range []string{"/workorders?view=dispatch", "/workorders?view=assigned", "/workorders?view=acceptance", "/stats", "/escalations"} {
				code, raw = r.request("GET", path, "", tc.who, nil)
				if code != tc.queues[i] {
					t.Fatalf("%s %d, want %d: %s", path, code, tc.queues[i], raw)
				}
				if code == 200 && (tc.name == "outside_area" || tc.name == "other_repairer") && decode[maintenance.Page](t, raw).Total != 0 {
					t.Fatalf("another person's order leaked through %s: %s", path, raw)
				}
			}
		})
	}
	for table, before := range counts {
		if r.count(table) != before {
			t.Fatalf("denied requests wrote %s", table)
		}
	}
	current := r.current(o.ID, worker)
	if current.Version != o.Version || current.State != o.State || current.AssigneeID != o.AssigneeID {
		t.Fatal("denied request changed workorder")
	}
}
