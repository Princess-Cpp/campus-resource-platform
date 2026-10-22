// Package domain contains M3 business rules. It does not authenticate callers,
// persist data, or replace the SQL transaction and HTTP resource adapters.
package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type State string

const (
	Reported           State = "REPORTED"
	Assigned           State = "ASSIGNED"
	InProgress         State = "IN_PROGRESS"
	AwaitingAcceptance State = "AWAITING_ACCEPTANCE"
	Completed          State = "COMPLETED"
	Cancelled          State = "CANCELLED"
)

type SyncState string

const (
	Idle           SyncState = "IDLE"
	Pending        SyncState = "PENDING"
	Unknown        SyncState = "UNKNOWN"
	NeedsAttention SyncState = "NEEDS_ATTENTION"
)

var (
	ErrInput     = errors.New("INVALID_INPUT")
	ErrForbidden = errors.New("FORBIDDEN")
	ErrState     = errors.New("INVALID_STATE")
	ErrVersion   = errors.New("VERSION_CONFLICT")
	ErrPending   = errors.New("OPERATION_PENDING")
	ErrReceipt   = errors.New("INVALID_RESOURCE_RECEIPT")
)

// Actor must be supplied by a trusted, current directory lookup, never request JSON.
type Actor struct {
	ID          string
	Enabled     bool
	Department  string
	Permissions []string
	Areas       []string
}

func (a Actor) Can(permission string) bool {
	return a.Enabled && a.ID != "" && slices.Contains(a.Permissions, permission)
}

func (a Actor) InArea(area string) bool {
	return a.Enabled && (slices.Contains(a.Areas, area) || slices.Contains(a.Areas, "*"))
}

// Room is a trusted directory snapshot; disabled spaces can still be reported.
type Room struct {
	ID, Name, Area, Department string
	Status                     string // ACTIVE, DISABLED, RETIRED
	SeatIDs                    []string
}

type Policy struct {
	Version                                                                   string
	DispatchAfter, StartAfter, RepairAfter, AcceptanceAfter, EvaluationWindow time.Duration
}

// ProposedPolicy implements DEC-08/09 proposals, not an approved group decision.
func ProposedPolicy() Policy {
	return Policy{"proposal-v1", 4 * time.Hour, 4 * time.Hour, 24 * time.Hour, 24 * time.Hour, 7 * 24 * time.Hour}
}

func (p Policy) valid() bool {
	return p.Version != "" && p.DispatchAfter > 0 && p.StartAfter > 0 && p.RepairAfter > 0 && p.AcceptanceAfter > 0 && p.EvaluationWindow > 0
}

type Report struct {
	RoomID, SeatID, Category, Description, Severity string
}

type Stage struct {
	Number     int64
	State      State
	StartedAt  time.Time
	DeadlineAt time.Time
	Escalated  bool
}

type Evaluation struct {
	Score   int
	Comment string
	At      time.Time
}

type Operation struct {
	ID, Action, SourceModule, SourceBusinessID, RoomID string
	SourceVersion                                      int64
	RequestedAt                                        time.Time
}

type Receipt struct {
	OperationID, SourceModule, SourceBusinessID, Status, ErrorCode string
	SourceVersion                                                  int64
	EffectiveAt                                                    time.Time
}

type Order struct {
	ID, ReporterID, RoomID, RoomName, Area                   string
	SeatID, Category, Description, Severity                  string
	State                                                    State
	Version                                                  int64
	Sync                                                     SyncState
	Policy                                                   Policy
	CreatedAt, UpdatedAt                                     time.Time
	AssigneeID                                               string
	AcceptedAt, FirstAssignedAt, FirstStartedAt, CompletedAt time.Time
	RepairParticipants                                       []string
	RepairElapsed, AcceptanceElapsed                         time.Duration
	Stage                                                    Stage
	Result                                                   string
	Evaluation                                               *Evaluation
	Operation                                                *Operation
	SourceVersion                                            int64
	LastResourceError                                        string
}

// Change is a transaction intent. The application must atomically save Order,
// command result, audit event and notification outbox before exposing success.
type Change struct {
	Order                    Order
	Action, ActorID, Comment string
	From                     State
	At                       time.Time
}

func New(id string, actor Actor, room Room, report Report, policy Policy, now time.Time) (Change, error) {
	if !actor.Can("workorder.report") {
		return Change{}, ErrForbidden
	}
	if room.Department != "" && room.Department != actor.Department && !actor.InArea(room.Area) {
		return Change{}, ErrForbidden
	}
	report.Description = strings.TrimSpace(report.Description)
	if report.Severity == "" {
		report.Severity = "NORMAL"
	}
	if id == "" || now.IsZero() || !policy.valid() || room.ID == "" || room.Name == "" || room.Area == "" || report.RoomID != room.ID || (room.Status != "ACTIVE" && room.Status != "DISABLED") || (report.SeatID != "" && !slices.Contains(room.SeatIDs, report.SeatID)) || !slices.Contains([]string{"LIGHTING", "POWER", "FURNITURE", "DOOR_WINDOW", "OTHER"}, report.Category) || !length(report.Description, 10, 1000) || !slices.Contains([]string{"NORMAL", "URGENT"}, report.Severity) {
		return Change{}, ErrInput
	}
	now = now.UTC()
	o := Order{ID: id, ReporterID: actor.ID, RoomID: room.ID, RoomName: room.Name, Area: room.Area, SeatID: report.SeatID, Category: report.Category, Description: report.Description, Severity: report.Severity, State: Reported, Version: 1, Sync: Idle, Policy: policy, CreatedAt: now, UpdatedAt: now}
	o.enter(Reported, now)
	return Change{Order: o, Action: "REPORT", ActorID: actor.ID, At: now}, nil
}

type Command struct {
	Action          string
	ExpectedVersion int64
	Comment         string
	Score           int
	// OperationID is generated once by the application and persisted in the same
	// transaction as the command. Unknown outcomes MUST NOT generate another ID.
	OperationID string
}

func length(v string, min, max int) bool {
	n := utf8.RuneCountInString(v)
	return utf8.ValidString(v) && n >= min && n <= max
}

func (o Order) clone() Order {
	o.RepairParticipants = slices.Clone(o.RepairParticipants)
	if o.Operation != nil {
		op := *o.Operation
		o.Operation = &op
	}
	if o.Evaluation != nil {
		e := *o.Evaluation
		o.Evaluation = &e
	}
	return o
}

func (o Order) CanRead(a Actor) bool {
	if !a.Enabled || a.ID == "" {
		return false
	}
	return a.ID == o.ReporterID || (a.ID == o.AssigneeID && a.Can("workorder.repair") && a.InArea(o.Area)) || (a.InArea(o.Area) && (a.Can("workorder.dispatch") || a.Can("workorder.accept") || a.Can("workorder.supervise")))
}

func (o Order) canRepair(a Actor) bool {
	return a.Can("workorder.repair") && a.ID == o.AssigneeID && a.InArea(o.Area)
}

// Apply validates one command. target is a current trusted directory lookup for
// ASSIGN, not a client-provided role. SQL row locking/version checks remain required.
func (o Order) Apply(a Actor, target Actor, q Command, now time.Time) (Change, error) {
	if !o.CanRead(a) {
		return Change{}, ErrForbidden
	}
	if q.ExpectedVersion != o.Version {
		return Change{}, ErrVersion
	}
	if now.IsZero() || now.Before(o.UpdatedAt) {
		return Change{}, ErrInput
	}
	if o.Sync != Idle {
		return Change{}, ErrPending
	}
	q.Comment = strings.TrimSpace(q.Comment)
	if !length(q.Comment, 0, 1000) {
		return Change{}, ErrInput
	}
	n := o.clone()
	now = now.UTC()
	switch q.Action {
	case "ASSIGN":
		if !a.Can("workorder.dispatch") || !a.InArea(o.Area) || !target.Can("workorder.repair") || !target.InArea(o.Area) {
			return Change{}, ErrForbidden
		}
		if o.State != Reported && o.State != Assigned && o.State != InProgress {
			return Change{}, ErrState
		}
		if q.Comment == "" || target.ID == o.AssigneeID {
			return Change{}, ErrInput
		}
		n.AssigneeID, n.AcceptedAt = target.ID, time.Time{}
		if o.State == Reported {
			n.FirstAssignedAt = now
			n.enter(Assigned, now)
		}
		// Reassignment never changes stage, first assignment, or repair elapsed.
	case "ACCEPT_ASSIGNMENT":
		if !o.canRepair(a) {
			return Change{}, ErrForbidden
		}
		if (o.State != Assigned && o.State != InProgress) || !o.AcceptedAt.IsZero() {
			return Change{}, ErrState
		}
		n.AcceptedAt = now
		if o.State == InProgress {
			n.addParticipant(a.ID)
		}
	case "START":
		if !o.canRepair(a) {
			return Change{}, ErrForbidden
		}
		if o.State != Assigned || o.AcceptedAt.IsZero() {
			return Change{}, ErrState
		}
		if q.OperationID == "" {
			return Change{}, ErrInput
		}
		n.prepare(q.OperationID, "BLOCK_ROOM_MAINTENANCE", now)
		// Register the repair actor before persisting the pending operation.
		n.addParticipant(a.ID)
	case "FINISH":
		if !o.canRepair(a) {
			return Change{}, ErrForbidden
		}
		if o.State != InProgress || o.AcceptedAt.IsZero() {
			return Change{}, ErrState
		}
		if q.Comment == "" {
			return Change{}, ErrInput
		}
		n.Result = q.Comment
		n.RepairElapsed += now.Sub(o.Stage.StartedAt)
		n.enter(AwaitingAcceptance, now)
	case "REJECT_ACCEPTANCE", "APPROVE_ACCEPTANCE":
		if !a.Can("workorder.accept") || !a.InArea(o.Area) || slices.Contains(o.RepairParticipants, a.ID) {
			return Change{}, ErrForbidden
		}
		if o.State != AwaitingAcceptance {
			return Change{}, ErrState
		}
		if q.Action == "REJECT_ACCEPTANCE" {
			if q.Comment == "" {
				return Change{}, ErrInput
			}
			n.AcceptanceElapsed += now.Sub(o.Stage.StartedAt)
			n.enter(InProgress, now)
		} else {
			if q.OperationID == "" {
				return Change{}, ErrInput
			}
			n.prepare(q.OperationID, "RELEASE_SOURCE", now)
		}
	case "CANCEL":
		if a.ID != o.ReporterID {
			return Change{}, ErrForbidden
		}
		if o.State != Reported && o.State != Assigned {
			return Change{}, ErrState
		}
		n.enter(Cancelled, now)
	case "EVALUATE":
		if a.ID != o.ReporterID {
			return Change{}, ErrForbidden
		}
		if o.State != Completed || o.Evaluation != nil || !now.Before(o.CompletedAt.Add(o.Policy.EvaluationWindow)) {
			return Change{}, ErrState
		}
		if q.Score < 1 || q.Score > 5 || !length(q.Comment, 0, 500) {
			return Change{}, ErrInput
		}
		n.Evaluation = &Evaluation{q.Score, q.Comment, now}
	default:
		return Change{}, ErrInput
	}
	n.Version++
	n.UpdatedAt = now
	return Change{n, q.Action, a.ID, q.Comment, o.State, now}, nil
}

func (o *Order) addParticipant(id string) {
	if !slices.Contains(o.RepairParticipants, id) {
		o.RepairParticipants = append(o.RepairParticipants, id)
	}
}

func (o *Order) enter(state State, now time.Time) {
	o.State = state
	o.Stage = Stage{Number: o.Stage.Number + 1, State: state, StartedAt: now}
	switch state {
	case Reported:
		o.Stage.DeadlineAt = now.Add(o.Policy.DispatchAfter)
	case Assigned:
		o.Stage.DeadlineAt = now.Add(o.Policy.StartAfter)
	case InProgress:
		o.Stage.DeadlineAt = now.Add(max(time.Duration(0), o.Policy.RepairAfter-o.RepairElapsed))
	case AwaitingAcceptance:
		o.Stage.DeadlineAt = now.Add(o.Policy.AcceptanceAfter)
	}
}

func (o *Order) prepare(id, action string, now time.Time) {
	o.SourceVersion++
	o.Operation = &Operation{id, action, "M3", o.ID, o.RoomID, o.SourceVersion, now}
	o.Sync, o.LastResourceError = Pending, ""
}

// ResourceUnknown preserves the operation for lookup and same-key retry. Exhausted
// retries still mean UNKNOWN; NeedsAttention must never imply absence of a block.
func (o Order) ResourceUnknown(now time.Time, exhausted bool) (Order, error) {
	if o.Operation == nil || o.Sync == Idle || now.Before(o.UpdatedAt) {
		return Order{}, ErrState
	}
	n := o.clone()
	n.Sync = Unknown
	if exhausted {
		n.Sync = NeedsAttention
	}
	n.LastResourceError = "DEPENDENCY_UNAVAILABLE"
	n.Version++
	n.UpdatedAt = now.UTC()
	return n, nil
}

// ConfirmResource is applied under the workorder lock, once per persisted
// operation. Matching provenance and a terminal receipt are mandatory.
func (o Order) ConfirmResource(r Receipt, now time.Time) (Change, error) {
	op := o.Operation
	if op == nil || o.Sync == Idle {
		return Change{}, ErrState
	}
	if now.Before(o.UpdatedAt) || r.EffectiveAt.IsZero() || r.EffectiveAt.Before(op.RequestedAt) || r.EffectiveAt.After(now) || r.OperationID != op.ID || r.SourceModule != "M3" || r.SourceBusinessID != o.ID || r.SourceVersion != op.SourceVersion || (r.Status != "CONFIRMED" && r.Status != "FAILED") || (r.Status == "CONFIRMED" && r.ErrorCode != "") || (r.Status == "FAILED" && r.ErrorCode == "") {
		return Change{}, ErrReceipt
	}
	n := o.clone()
	n.LastResourceError = r.ErrorCode
	if r.Status == "FAILED" {
		if op.Action == "BLOCK_ROOM_MAINTENANCE" {
			// A terminal failed receipt is different from a timeout. No block was
			// confirmed; a new deliberate start may use a new operation/version.
			n.Sync = Idle
			n.Operation = nil
		} else {
			// A failed release cannot justify completing the order or clearing safety.
			n.Sync = NeedsAttention
		}
	} else {
		switch op.Action {
		case "BLOCK_ROOM_MAINTENANCE":
			if o.State != Assigned {
				return Change{}, ErrState
			}
			n.FirstStartedAt = r.EffectiveAt.UTC()
			n.enter(InProgress, n.FirstStartedAt)
		case "RELEASE_SOURCE":
			if o.State != AwaitingAcceptance {
				return Change{}, ErrState
			}
			n.AcceptanceElapsed += op.RequestedAt.Sub(o.Stage.StartedAt)
			n.CompletedAt = r.EffectiveAt.UTC()
			n.enter(Completed, n.CompletedAt)
		default:
			return Change{}, ErrReceipt
		}
		n.Sync = Idle
		n.Operation = nil
	}
	n.Version++
	n.UpdatedAt = now.UTC()
	return Change{n, "RESOURCE_" + r.Status, "system", r.ErrorCode, o.State, now.UTC()}, nil
}

// Escalate emits at most one intent per stage in a serialized SQL transaction.
// A unique persisted (order, stage, level) event is still required for concurrency.
// Pending synchronization is reconciled first to avoid escalating an already
// effective but not yet observed start/completion.
func (o Order) Escalate(now time.Time) (Change, bool) {
	if o.Sync != Idle || o.Stage.DeadlineAt.IsZero() || o.Stage.Escalated || now.Before(o.Stage.DeadlineAt) || now.Before(o.UpdatedAt) {
		return Change{}, false
	}
	n := o.clone()
	n.Stage.Escalated = true
	n.Version++
	n.UpdatedAt = now.UTC()
	return Change{n, "ESCALATE", "system", "", o.State, now.UTC()}, true
}
