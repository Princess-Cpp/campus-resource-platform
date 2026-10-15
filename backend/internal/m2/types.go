package m2

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"
)

type Actor struct {
	ID          string   `json:"id"`
	Department  string   `json:"department"`
	Permissions []string `json:"permissions"`
	Areas       []string `json:"areas"`
	ContactRefs []string `json:"contactRefs"`
}

func (a Actor) Can(p string) bool {
	for _, v := range a.Permissions {
		if v == p {
			return true
		}
	}
	return false
}
func (a Actor) Area(area string) bool {
	for _, v := range a.Areas {
		if v == area || v == "*" {
			return true
		}
	}
	return false
}

type Room struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Area       string `json:"area"`
	Department string `json:"department"`
	Capacity   int    `json:"capacity"`
	Active     bool   `json:"active"`
	OpenHour   int    `json:"openHour"`
	CloseHour  int    `json:"closeHour"`
}
type Directory interface {
	Authenticate(context.Context, string) (Actor, error)
	Rooms(context.Context) ([]Room, error)
	Actors(context.Context) ([]Actor, error)
}
type Policy struct {
	Version       string `json:"version"`
	MaxHours      int    `json:"maxHours"`
	AdvanceDays   int    `json:"advanceDays"`
	ApprovalHours int    `json:"approvalHours"`
}
type CreateRequest struct {
	RoomID     string    `json:"roomId"`
	StartAt    time.Time `json:"startAt"`
	EndAt      time.Time `json:"endAt"`
	Purpose    string    `json:"purpose"`
	Attendees  int       `json:"attendees"`
	ContactRef string    `json:"contactRef"`
}
type Snapshot struct {
	Purpose             string   `json:"purpose"`
	Attendees           int      `json:"attendees"`
	Capacity            int      `json:"capacity"`
	ContactRef          string   `json:"contactRef,omitempty"`
	DepartmentApprovers []string `json:"departmentApprovers"`
	FinalApprovers      []string `json:"finalApprovers"`
	DepartmentActor     string   `json:"departmentActor"`
	Policy              Policy   `json:"policy"`
}
type Application struct {
	ID            string    `json:"id"`
	UserID        string    `json:"userId"`
	Department    string    `json:"department"`
	RoomID        string    `json:"roomId"`
	RoomName      string    `json:"roomName"`
	Area          string    `json:"area"`
	StartAt       time.Time `json:"startAt"`
	EndAt         time.Time `json:"endAt"`
	CreatedAt     time.Time `json:"createdAt"`
	DeadlineAt    time.Time `json:"deadlineAt"`
	Status        string    `json:"status"`
	Version       int64     `json:"version"`
	SyncStatus    string    `json:"syncStatus"`
	OperationID   string    `json:"operationId,omitempty"`
	SourceVersion int64     `json:"sourceVersion"`
	LastError     string    `json:"lastError,omitempty"`
	Snapshot      Snapshot  `json:"details"`
	Events        []Event   `json:"events,omitempty"`
}
type Event struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actorId"`
	Action    string    `json:"action"`
	From      string    `json:"fromStatus"`
	To        string    `json:"toStatus"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"createdAt"`
}
type ActionRequest struct {
	Action          string `json:"action"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Comment         string `json:"comment"`
}
type Operation struct {
	OperationID      string     `json:"operationId"`
	Action           string     `json:"action"`
	SourceModule     string     `json:"sourceModule"`
	SourceBusinessID string     `json:"sourceBusinessId"`
	SourceVersion    int64      `json:"sourceVersion"`
	RoomID           string     `json:"roomId"`
	StartAt          *time.Time `json:"startAt,omitempty"`
	EndAt            *time.Time `json:"endAt,omitempty"`
	DeadlineAt       *time.Time `json:"deadlineAt,omitempty"`
	Reason           string     `json:"reason"`
}
type Receipt struct {
	OperationID      string    `json:"operationId"`
	OperationStatus  string    `json:"operationStatus"`
	SourceModule     string    `json:"sourceModule"`
	SourceBusinessID string    `json:"sourceBusinessId"`
	SourceVersion    int64     `json:"sourceVersion"`
	EffectiveAt      time.Time `json:"effectiveAt"`
	ErrorCode        string    `json:"errorCode"`
}
type Resources interface {
	Available(context.Context, string, time.Time, time.Time) (map[string]bool, error)
	Lookup(context.Context, string) (Receipt, bool, error)
	Execute(context.Context, Operation, string) (Receipt, error)
}
type Fault struct {
	Code   string
	Status int
}

func (e *Fault) Error() string            { return e.Code }
func fault(code string, status int) error { return &Fault{code, status} }
func id() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}

type Service struct {
	DB        *sql.DB
	Directory Directory
	Resources Resources
	Now       func() time.Time
	Policy    Policy
	Zone      *time.Location
}

func New(db *sql.DB, d Directory, r Resources) *Service {
	z, _ := time.LoadLocation("Asia/Shanghai")
	return &Service{db, d, r, time.Now, Policy{"proposal-v1", 4, 14, 24}, z}
}

const cols = "id,user_id,department,room_id,room_name,area,start_at,end_at,created_at,deadline_at,status,version,sync_status,operation_id,source_version,last_error,payload"

func scan(row interface{ Scan(...any) error }) (a Application, e error) {
	var raw []byte
	e = row.Scan(&a.ID, &a.UserID, &a.Department, &a.RoomID, &a.RoomName, &a.Area, &a.StartAt, &a.EndAt, &a.CreatedAt, &a.DeadlineAt, &a.Status, &a.Version, &a.SyncStatus, &a.OperationID, &a.SourceVersion, &a.LastError, &raw)
	if e == nil {
		e = json.Unmarshal(raw, &a.Snapshot)
	}
	return
}
func (s *Service) begin(ctx context.Context) (*sql.Tx, error) {
	return s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}
