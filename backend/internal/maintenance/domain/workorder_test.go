package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var epoch = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)

func actor(id string, permissions ...string) Actor {
	return Actor{ID: id, Enabled: true, Department: "CS", Permissions: permissions, Areas: []string{"A"}}
}

func reporter() Actor   { return actor("reporter", "workorder.report") }
func dispatcher() Actor { return actor("dispatcher", "workorder.dispatch") }
func repairer() Actor   { return actor("repairer", "workorder.repair") }
func inspector() Actor  { return actor("inspector", "workorder.accept") }

func room() Room { return Room{"room-1", "A101", "A", "CS", "ACTIVE", []string{"seat-1"}} }
func report() Report {
	return Report{RoomID: "room-1", Category: "LIGHTING", Description: "教室前排照明灯持续闪烁无法正常使用"}
}

func reported(t *testing.T) Order {
	t.Helper()
	c, err := New("order-1", reporter(), room(), report(), ProposedPolicy(), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return c.Order
}

func apply(t *testing.T, o Order, a, target Actor, action, comment string, now time.Time) Order {
	t.Helper()
	c, err := o.Apply(a, target, Command{Action: action, ExpectedVersion: o.Version, Comment: comment, OperationID: "operation-" + action}, now)
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	return c.Order
}

func assigned(t *testing.T) Order {
	return apply(t, reported(t), dispatcher(), repairer(), "ASSIGN", "检查照明线路", epoch.Add(time.Hour))
}

func prepared(t *testing.T) Order {
	o := assigned(t)
	o = apply(t, o, repairer(), Actor{}, "ACCEPT_ASSIGNMENT", "", epoch.Add(2*time.Hour))
	return apply(t, o, repairer(), Actor{}, "START", "", epoch.Add(3*time.Hour))
}

func receipt(o Order, at time.Time) Receipt {
	return Receipt{OperationID: o.Operation.ID, SourceModule: "M3", SourceBusinessID: o.ID, SourceVersion: o.SourceVersion, Status: "CONFIRMED", EffectiveAt: at}
}

func confirm(t *testing.T, o Order, at time.Time) Order {
	t.Helper()
	c, err := o.ConfirmResource(receipt(o, at), at)
	if err != nil {
		t.Fatal(err)
	}
	return c.Order
}

func repairing(t *testing.T) Order { return confirm(t, prepared(t), epoch.Add(3*time.Hour)) }
func awaiting(t *testing.T) Order {
	return apply(t, repairing(t), repairer(), Actor{}, "FINISH", "更换灯管并检查供电正常", epoch.Add(5*time.Hour))
}
func completed(t *testing.T) Order {
	o := apply(t, awaiting(t), inspector(), Actor{}, "APPROVE_ACCEPTANCE", "照明测试正常", epoch.Add(6*time.Hour))
	return confirm(t, o, epoch.Add(7*time.Hour))
}

func denied(t *testing.T, o Order, a, target Actor, q Command, now time.Time, want error) {
	t.Helper()
	before := o.clone()
	c, err := o.Apply(a, target, q, now)
	if !errors.Is(err, want) {
		t.Fatalf("%s: want %v got %v", q.Action, want, err)
	}
	if !reflect.DeepEqual(o, before) || !reflect.DeepEqual(c, Change{}) {
		t.Fatal("rejected action changed data or returned a commit intent")
	}
}

func TestReportValidationAndSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Actor, *Room, *Report, *Policy)
		want   error
	}{
		{"disabled account", func(a *Actor, _ *Room, _ *Report, _ *Policy) { a.Enabled = false }, ErrForbidden},
		{"missing permission", func(a *Actor, _ *Room, _ *Report, _ *Policy) { a.Permissions = nil }, ErrForbidden},
		{"other department", func(a *Actor, _ *Room, _ *Report, _ *Policy) { a.Department = "EE"; a.Areas = nil }, ErrForbidden},
		{"wrong room", func(_ *Actor, _ *Room, r *Report, _ *Policy) { r.RoomID = "another" }, ErrInput},
		{"wrong seat", func(_ *Actor, _ *Room, r *Report, _ *Policy) { r.SeatID = "other-seat" }, ErrInput},
		{"retired", func(_ *Actor, r *Room, _ *Report, _ *Policy) { r.Status = "RETIRED" }, ErrInput},
		{"unknown category", func(_ *Actor, _ *Room, r *Report, _ *Policy) { r.Category = "UNKNOWN" }, ErrInput},
		{"short", func(_ *Actor, _ *Room, r *Report, _ *Policy) { r.Description = "太短" }, ErrInput},
		{"long", func(_ *Actor, _ *Room, r *Report, _ *Policy) { r.Description = strings.Repeat("灯", 1001) }, ErrInput},
		{"invalid policy", func(_ *Actor, _ *Room, _ *Report, p *Policy) { p.RepairAfter = 0 }, ErrInput},
		{"disabled space allowed", func(_ *Actor, r *Room, _ *Report, _ *Policy) { r.Status = "DISABLED" }, nil},
		{"urgent allowed", func(_ *Actor, _ *Room, r *Report, _ *Policy) { r.Severity = "URGENT" }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, r, q, p := reporter(), room(), report(), ProposedPolicy()
			tt.mutate(&a, &r, &q, &p)
			c, err := New("id", a, r, q, p, epoch)
			if !errors.Is(err, tt.want) {
				t.Fatalf("want %v got %v", tt.want, err)
			}
			if err == nil && (c.Order.RoomName != r.Name || c.Order.Policy != p || c.Order.State != Reported) {
				t.Fatal("missing snapshot")
			}
		})
	}
}

func TestWorkflowAndResourceSuccessBoundary(t *testing.T) {
	o := prepared(t)
	if o.State != Assigned || o.Sync != Pending || o.Operation.Action != "BLOCK_ROOM_MAINTENANCE" || o.Operation.RoomID != "room-1" {
		t.Fatal("start must await whole-room block")
	}
	o = confirm(t, o, epoch.Add(3*time.Hour))
	if o.State != InProgress || o.Sync != Idle {
		t.Fatal("confirmed block did not start repair")
	}
	o = apply(t, o, repairer(), Actor{}, "FINISH", "更换并测试完成", epoch.Add(5*time.Hour))
	if o.State != AwaitingAcceptance || o.SourceVersion != 1 || o.Operation != nil {
		t.Fatal("finish must not release source")
	}
	o = apply(t, o, inspector(), Actor{}, "APPROVE_ACCEPTANCE", "", epoch.Add(6*time.Hour))
	if o.State != AwaitingAcceptance || o.Sync != Pending || o.SourceVersion != 2 || o.Operation.SourceBusinessID != o.ID || o.Operation.Action != "RELEASE_SOURCE" {
		t.Fatal("approval must await own-source release")
	}
	o = confirm(t, o, epoch.Add(7*time.Hour))
	if o.State != Completed || o.RepairElapsed != 2*time.Hour || o.AcceptanceElapsed != time.Hour {
		t.Fatalf("wrong completion/timing: %+v", o)
	}
	if _, err := o.ConfirmResource(Receipt{}, epoch.Add(8*time.Hour)); !errors.Is(err, ErrState) {
		t.Fatal("duplicate confirmation accepted")
	}
}

func TestPermissionAndStateGuards(t *testing.T) {
	o := assigned(t)
	denied(t, o, repairer(), Actor{}, Command{Action: "START", ExpectedVersion: o.Version, OperationID: "start"}, epoch.Add(2*time.Hour), ErrState)
	denied(t, o, reporter(), Actor{}, Command{Action: "FINISH", ExpectedVersion: o.Version, Comment: "done"}, epoch.Add(2*time.Hour), ErrForbidden)
	denied(t, o, dispatcher(), repairer(), Command{Action: "ASSIGN", ExpectedVersion: o.Version - 1, Comment: "again"}, epoch.Add(2*time.Hour), ErrVersion)
	other := dispatcher()
	other.Areas = []string{"B"}
	denied(t, o, other, repairer(), Command{Action: "ASSIGN", ExpectedVersion: o.Version, Comment: "again"}, epoch.Add(2*time.Hour), ErrForbidden)
	disabled := repairer()
	disabled.Enabled = false
	denied(t, o, dispatcher(), disabled, Command{Action: "ASSIGN", ExpectedVersion: o.Version, Comment: "again"}, epoch.Add(2*time.Hour), ErrForbidden)
	o = repairing(t)
	denied(t, o, reporter(), Actor{}, Command{Action: "CANCEL", ExpectedVersion: o.Version}, epoch.Add(4*time.Hour), ErrState)
	denied(t, o, repairer(), Actor{}, Command{Action: "FINISH", ExpectedVersion: o.Version}, epoch.Add(4*time.Hour), ErrInput)
	denied(t, o, inspector(), Actor{}, Command{Action: "APPROVE_ACCEPTANCE", ExpectedVersion: o.Version, OperationID: "release"}, epoch.Add(4*time.Hour), ErrState)
	denied(t, o, dispatcher(), Actor{}, Command{Action: "FORCE_CLOSE", ExpectedVersion: o.Version}, epoch.Add(4*time.Hour), ErrInput)
}

func TestCannotCancelWhileResourceOutcomeUnknown(t *testing.T) {
	o := prepared(t)
	for _, exhausted := range []bool{false, true} {
		n, err := o.ResourceUnknown(epoch.Add(4*time.Hour), exhausted)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(n.Operation, o.Operation) || n.State != Assigned {
			t.Fatal("unknown result replaced operation or changed business state")
		}
		denied(t, n, reporter(), Actor{}, Command{Action: "CANCEL", ExpectedVersion: n.Version}, epoch.Add(5*time.Hour), ErrPending)
		denied(t, n, dispatcher(), actor("replacement", "workorder.repair"), Command{Action: "ASSIGN", ExpectedVersion: n.Version, Comment: "handover"}, epoch.Add(5*time.Hour), ErrPending)
		// A persisted receipt observed after restart may predate the last retry.
		c, err := n.ConfirmResource(receipt(n, epoch.Add(3*time.Hour)), epoch.Add(5*time.Hour))
		if err != nil || c.Order.State != InProgress {
			t.Fatalf("lookup recovery: %v", err)
		}
	}
}

func TestCancelBeforeStart(t *testing.T) {
	for _, o := range []Order{reported(t), assigned(t)} {
		n := apply(t, o, reporter(), Actor{}, "CANCEL", "不再需要处理", epoch.Add(2*time.Hour))
		if n.State != Cancelled || n.Operation != nil || n.SourceVersion != 0 {
			t.Fatal("early cancellation must not release resources")
		}
		denied(t, n, dispatcher(), repairer(), Command{Action: "ASSIGN", ExpectedVersion: n.Version, Comment: "again"}, epoch.Add(3*time.Hour), ErrState)
	}
}

func TestHistoricalRepairerCannotAccept(t *testing.T) {
	o := repairing(t)
	second := actor("second", "workorder.repair")
	o = apply(t, o, dispatcher(), second, "ASSIGN", "交接维修", epoch.Add(4*time.Hour))
	o = apply(t, o, second, Actor{}, "ACCEPT_ASSIGNMENT", "", epoch.Add(4*time.Hour))
	o = apply(t, o, second, Actor{}, "FINISH", "维修已完成", epoch.Add(5*time.Hour))
	for _, who := range []string{"repairer", "second"} {
		a := actor(who, "workorder.repair", "workorder.accept")
		for _, action := range []string{"APPROVE_ACCEPTANCE", "REJECT_ACCEPTANCE"} {
			denied(t, o, a, Actor{}, Command{Action: action, ExpectedVersion: o.Version, Comment: "检查", OperationID: "release"}, epoch.Add(6*time.Hour), ErrForbidden)
		}
	}
}

func TestReassignmentPreservesClocksAndRevokesOldRepairer(t *testing.T) {
	for _, o := range []Order{assigned(t), repairing(t)} {
		second := actor("second", "workorder.repair")
		n := apply(t, o, dispatcher(), second, "ASSIGN", "交接处理", epoch.Add(4*time.Hour))
		if n.Stage != o.Stage || n.FirstAssignedAt != o.FirstAssignedAt || n.FirstStartedAt != o.FirstStartedAt || n.SourceVersion != o.SourceVersion || n.State != o.State {
			t.Fatal("reassignment reset timing or resource state")
		}
		if n.CanRead(repairer()) {
			t.Fatal("old repairer retained access")
		}
		denied(t, n, repairer(), Actor{}, Command{Action: "FINISH", ExpectedVersion: n.Version, Comment: "done"}, epoch.Add(5*time.Hour), ErrForbidden)
		if o.State == InProgress {
			denied(t, n, second, Actor{}, Command{Action: "FINISH", ExpectedVersion: n.Version, Comment: "done"}, epoch.Add(5*time.Hour), ErrState)
		}
	}
}

func TestRejectAcceptanceKeepsBlockAndAccumulatedRepair(t *testing.T) {
	o := awaiting(t)
	previousStage := o.Stage.Number
	o = apply(t, o, inspector(), Actor{}, "REJECT_ACCEPTANCE", "照明仍有闪烁", epoch.Add(6*time.Hour))
	if o.State != InProgress || o.Operation != nil || o.SourceVersion != 1 || o.RepairElapsed != 2*time.Hour || o.Stage.DeadlineAt != epoch.Add(28*time.Hour) {
		t.Fatal("rejection lost block/timing")
	}
	o = apply(t, o, repairer(), Actor{}, "FINISH", "再次检查完成", epoch.Add(8*time.Hour))
	if o.RepairElapsed != 4*time.Hour || o.AcceptanceElapsed != time.Hour || o.Stage.Number != previousStage+2 {
		t.Fatalf("wrong stage totals: %+v", o)
	}
}

func TestReceiptProvenanceAndFailure(t *testing.T) {
	o := prepared(t)
	for _, mutate := range []func(*Receipt){
		func(r *Receipt) { r.OperationID = "other" }, func(r *Receipt) { r.SourceModule = "M2" },
		func(r *Receipt) { r.SourceBusinessID = "other" }, func(r *Receipt) { r.SourceVersion++ },
		func(r *Receipt) { r.Status = "PENDING" }, func(r *Receipt) { r.ErrorCode = "unexpected" },
		func(r *Receipt) { r.EffectiveAt = epoch }, func(r *Receipt) { r.EffectiveAt = epoch.Add(100 * time.Hour) },
	} {
		r := receipt(o, epoch.Add(3*time.Hour))
		mutate(&r)
		if _, err := o.ConfirmResource(r, epoch.Add(4*time.Hour)); !errors.Is(err, ErrReceipt) {
			t.Fatalf("accepted invalid receipt: %+v %v", r, err)
		}
	}
	r := receipt(o, epoch.Add(3*time.Hour))
	r.Status = "FAILED"
	r.ErrorCode = "RESOURCE_UNAVAILABLE"
	c, err := o.ConfirmResource(r, epoch.Add(4*time.Hour))
	if err != nil || c.Order.State != Assigned || c.Order.Sync != Idle {
		t.Fatal("failed block incorrectly started repair")
	}
	n := apply(t, c.Order, reporter(), Actor{}, "CANCEL", "", epoch.Add(5*time.Hour))
	if n.State != Cancelled {
		t.Fatal("terminal failure should permit early cancellation")
	}
	o = apply(t, awaiting(t), inspector(), Actor{}, "APPROVE_ACCEPTANCE", "", epoch.Add(6*time.Hour))
	r = receipt(o, epoch.Add(6*time.Hour))
	r.Status = "FAILED"
	r.ErrorCode = "VERSION_CONFLICT"
	c, err = o.ConfirmResource(r, epoch.Add(7*time.Hour))
	if err != nil || c.Order.State != AwaitingAcceptance || c.Order.Sync != NeedsAttention || c.Order.Operation == nil {
		t.Fatal("failed release erased safety state")
	}
}

func TestEvaluationWindowOwnershipAndOneShot(t *testing.T) {
	o := completed(t)
	q := Command{Action: "EVALUATE", ExpectedVersion: o.Version, Score: 5}
	denied(t, o, dispatcher(), Actor{}, q, epoch.Add(8*time.Hour), ErrForbidden)
	denied(t, o, reporter(), Actor{}, q, o.CompletedAt.Add(o.Policy.EvaluationWindow), ErrState)
	for _, score := range []int{0, 6} {
		bad := q
		bad.Score = score
		denied(t, o, reporter(), Actor{}, bad, epoch.Add(8*time.Hour), ErrInput)
	}
	q.Comment = strings.Repeat("好", 501)
	denied(t, o, reporter(), Actor{}, q, epoch.Add(8*time.Hour), ErrInput)
	q.Comment = "维修及时"
	c, err := o.Apply(reporter(), Actor{}, q, o.CompletedAt.Add(o.Policy.EvaluationWindow-time.Nanosecond))
	if err != nil || c.Order.Evaluation.Score != 5 || c.Order.SourceVersion != o.SourceVersion || c.Order.State != Completed {
		t.Fatal("valid evaluation changed resources or failed")
	}
	q.ExpectedVersion = c.Order.Version
	denied(t, c.Order, reporter(), Actor{}, q, c.At, ErrState)
	if o.Evaluation != nil {
		t.Fatal("evaluation mutated input")
	}
	unfinished := reported(t)
	q.ExpectedVersion = unfinished.Version
	denied(t, unfinished, reporter(), Actor{}, q, epoch.Add(time.Hour), ErrState)
}

func TestEscalationBoundariesAndStageInstances(t *testing.T) {
	for _, o := range []Order{reported(t), assigned(t), repairing(t), awaiting(t)} {
		if _, ok := o.Escalate(o.Stage.DeadlineAt.Add(-time.Nanosecond)); ok {
			t.Fatal("early escalation")
		}
		c, ok := o.Escalate(o.Stage.DeadlineAt)
		if !ok || c.Order.State != o.State || c.Order.SourceVersion != o.SourceVersion {
			t.Fatal("threshold did not escalate or changed state")
		}
		if _, ok = c.Order.Escalate(o.Stage.DeadlineAt.Add(time.Hour)); ok {
			t.Fatal("duplicate stage escalation")
		}
	}
	o := repairing(t)
	c, _ := o.Escalate(epoch.Add(27 * time.Hour))
	o = apply(t, c.Order, repairer(), Actor{}, "FINISH", "完成待验收", epoch.Add(28*time.Hour))
	o = apply(t, o, inspector(), Actor{}, "REJECT_ACCEPTANCE", "仍存在故障", epoch.Add(29*time.Hour))
	if _, ok := o.Escalate(epoch.Add(29 * time.Hour)); !ok {
		t.Fatal("new stage with exhausted repair budget must escalate immediately")
	}
	if _, ok := completed(t).Escalate(epoch.Add(1000 * time.Hour)); ok {
		t.Fatal("completed order escalated")
	}
	if _, ok := prepared(t).Escalate(epoch.Add(1000 * time.Hour)); ok {
		t.Fatal("unknown operation must reconcile before escalation")
	}
}

func TestTransitionsDoNotMutateSharedInput(t *testing.T) {
	o := repairing(t)
	// Force spare capacity so append would mutate a shallow copy's backing array.
	o.RepairParticipants = make([]string, 1, 8)
	o.RepairParticipants[0] = "repairer"
	o = apply(t, o, dispatcher(), actor("second", "workorder.repair"), "ASSIGN", "交接", epoch.Add(4*time.Hour))
	before := o.clone()
	n := apply(t, o, actor("second", "workorder.repair"), Actor{}, "ACCEPT_ASSIGNMENT", "", epoch.Add(5*time.Hour))
	n.RepairParticipants[0] = "changed"
	if !reflect.DeepEqual(o, before) {
		t.Fatal("transition mutated input participants")
	}
}
