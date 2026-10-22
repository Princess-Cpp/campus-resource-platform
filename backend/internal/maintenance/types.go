package maintenance

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"time"

	"campus-resource-platform/backend/internal/maintenance/domain"
)

type Seat struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Room struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Area       string `json:"area"`
	Department string `json:"-"`
	Status     string `json:"status"`
	Seats      []Seat `json:"seats"`
}
type Catalog interface {
	Rooms(context.Context) ([]Room, error)
}
type Events interface {
	Append(context.Context, *sql.Tx, domain.Change, string, string) error
}
type Worker struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Actor domain.Actor `json:"-"`
}
type People interface {
	Workers(context.Context) ([]Worker, error)
}
type SupervisorDirectory interface {
	Supervisors(context.Context, string) ([]string, error)
}
type ActionInput struct {
	Score           *int   `json:"score,omitempty"`
	Action          string `json:"action"`
	ExpectedVersion int64  `json:"expectedVersion"`
	AssigneeID      string `json:"assigneeId,omitempty"`
	Comment         string `json:"comment,omitempty"`
}
type Authenticate func(context.Context, *http.Request) (domain.Actor, error)
type ReportInput struct {
	RoomID      string `json:"roomId"`
	SeatID      string `json:"seatId,omitempty"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}
type OperationSummary struct {
	ID          string     `json:"id"`
	Action      string     `json:"action"`
	State       string     `json:"state"`
	Attempts    int        `json:"attempts"`
	NextAttempt *time.Time `json:"nextAttempt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
}
type Evaluation struct {
	Score   int       `json:"score"`
	Comment string    `json:"comment"`
	At      time.Time `json:"at"`
}
type Order struct {
	Evaluation           *Evaluation       `json:"evaluation,omitempty"`
	EvaluationDeadlineAt *time.Time        `json:"evaluationDeadlineAt,omitempty"`
	StageNumber          int64             `json:"stageNumber"`
	Escalated            bool              `json:"escalated"`
	PolicyVersion        string            `json:"policyVersion"`
	RepairSeconds        float64           `json:"repairSeconds"`
	AcceptanceSeconds    float64           `json:"acceptanceSeconds"`
	Result               string            `json:"result,omitempty"`
	FirstStartedAt       *time.Time        `json:"firstStartedAt,omitempty"`
	CompletedAt          *time.Time        `json:"completedAt,omitempty"`
	LastResourceError    string            `json:"lastResourceError,omitempty"`
	Operation            *OperationSummary `json:"operation,omitempty"`
	AssigneeID           string            `json:"assigneeId,omitempty"`
	AssigneeName         string            `json:"assigneeName,omitempty"`
	AcceptedAt           *time.Time        `json:"acceptedAt,omitempty"`
	FirstAssignedAt      *time.Time        `json:"firstAssignedAt,omitempty"`
	DeadlineAt           *time.Time        `json:"deadlineAt,omitempty"`
	ID                   string            `json:"id"`
	ReporterID           string            `json:"reporterId"`
	RoomID               string            `json:"roomId"`
	RoomName             string            `json:"roomName"`
	Area                 string            `json:"area"`
	SeatID               string            `json:"seatId,omitempty"`
	SeatLabel            string            `json:"seatLabel,omitempty"`
	Category             string            `json:"category"`
	Description          string            `json:"description"`
	Severity             string            `json:"severity"`
	State                domain.State      `json:"state"`
	SyncState            domain.SyncState  `json:"syncState"`
	Version              int64             `json:"version"`
	CreatedAt            time.Time         `json:"createdAt"`
	UpdatedAt            time.Time         `json:"updatedAt"`
}
type Timeline struct {
	ActorID string    `json:"actorId,omitempty"`
	Comment string    `json:"comment,omitempty"`
	ID      string    `json:"id"`
	Action  string    `json:"action"`
	Text    string    `json:"text"`
	At      time.Time `json:"at"`
}
type Detail struct {
	AllowedActions []string `json:"allowedActions"`
	Order
	Timeline []Timeline `json:"timeline"`
}
type Filters struct {
	View                      string
	Page, PageSize            int
	State, Category, From, To string
}
type Page struct {
	Items    []Order `json:"items"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
	Total    int     `json:"total"`
}
type Fault struct {
	Code   string
	Status int
}

func (f *Fault) Error() string            { return f.Code }
func fault(code string, status int) error { return &Fault{code, status} }
func ID() string {
	var raw [16]byte
	if _, e := rand.Read(raw[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(raw[:])
}

type Service struct {
	NotificationRecipients NotificationDirectory
	NotificationSender     NotificationSender
	DB                     *sql.DB
	Resources              Resources
	People                 People
	Supervisors            SupervisorDirectory
	Catalog                Catalog
	Events                 Events
	Now                    func() time.Time
	// These flags describe adapters, never an assertion of production readiness.
	Development bool
}

func New(db *sql.DB, catalog Catalog, events Events) *Service {
	return &Service{DB: db, Catalog: catalog, Events: events, Now: time.Now}
}
func dto(o domain.Order, seatLabel string, name ...string) Order {
	result := Order{ID: o.ID, ReporterID: o.ReporterID, RoomID: o.RoomID, RoomName: o.RoomName, Area: o.Area, SeatID: o.SeatID, SeatLabel: seatLabel, Category: o.Category, Description: o.Description, Severity: o.Severity, State: o.State, SyncState: o.Sync, Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt, AssigneeID: o.AssigneeID}
	result.Result, result.LastResourceError = o.Result, o.LastResourceError
	result.StageNumber, result.Escalated, result.PolicyVersion = o.Stage.Number, o.Stage.Escalated, o.Policy.Version
	result.RepairSeconds, result.AcceptanceSeconds = o.RepairElapsed.Seconds(), o.AcceptanceElapsed.Seconds()
	if o.Evaluation != nil {
		result.Evaluation = &Evaluation{Score: o.Evaluation.Score, Comment: o.Evaluation.Comment, At: o.Evaluation.At}
	}
	if !o.CompletedAt.IsZero() {
		deadline := o.CompletedAt.Add(o.Policy.EvaluationWindow)
		result.EvaluationDeadlineAt = &deadline
	}
	if !o.FirstStartedAt.IsZero() {
		t := o.FirstStartedAt
		result.FirstStartedAt = &t
	}
	if !o.CompletedAt.IsZero() {
		t := o.CompletedAt
		result.CompletedAt = &t
	}
	if o.Operation != nil {
		state := "PENDING"
		if o.Sync == domain.NeedsAttention {
			state = "PAUSED"
			if o.LastResourceError != "DEPENDENCY_UNAVAILABLE" {
				state = "FAILED"
			}
		}
		result.Operation = &OperationSummary{ID: o.Operation.ID, Action: o.Operation.Action, State: state}
	}
	if len(name) > 0 {
		result.AssigneeName = name[0]
	}
	if !o.AcceptedAt.IsZero() {
		t := o.AcceptedAt
		result.AcceptedAt = &t
	}
	if !o.FirstAssignedAt.IsZero() {
		t := o.FirstAssignedAt
		result.FirstAssignedAt = &t
	}
	if !o.Stage.DeadlineAt.IsZero() {
		t := o.Stage.DeadlineAt
		result.DeadlineAt = &t
	}
	return result
}
