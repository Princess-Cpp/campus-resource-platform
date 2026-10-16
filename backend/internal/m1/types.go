package m1

import (
	"context"
	"database/sql"
	"time"
)

type Actor struct {
	ID          string   `json:"id"`
	Department  string   `json:"department"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
	Service     string   `json:"-"`
}

func (a Actor) Can(permission string) bool {
	for _, value := range a.Permissions {
		if value == permission {
			return true
		}
	}
	return false
}

type Seat struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Outlet bool   `json:"outlet"`
	Active bool   `json:"active"`
}
type Room struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Area       string `json:"area"`
	Department string `json:"department"`
	Active     bool   `json:"active"`
	OpenHour   int    `json:"openHour"`
	CloseHour  int    `json:"closeHour"`
	Seats      []Seat `json:"seats"`
}

// Implemented by M2's master-data module. Updates must use WithRoomLock and
// validate existing occupancy in the same transaction before changing a room.
type Catalog interface {
	Rooms(context.Context, *sql.Tx) ([]Room, error)
}
type Booking struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	RoomID    string    `json:"roomId"`
	SeatID    string    `json:"seatId"`
	RoomName  string    `json:"roomName"`
	SeatLabel string    `json:"seatLabel"`
	StartAt   time.Time `json:"startAt"`
	EndAt     time.Time `json:"endAt"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
}
type BookRequest struct {
	SeatID  string    `json:"seatId" binding:"required,max=64"`
	RoomID  string    `json:"roomId" binding:"required,max=64"`
	StartAt time.Time `json:"startAt" binding:"required"`
	EndAt   time.Time `json:"endAt" binding:"required"`
}
type Operation struct {
	OperationID      string     `json:"operationId" binding:"required,max=64"`
	Action           string     `json:"action" binding:"required"`
	SourceModule     string     `json:"sourceModule" binding:"required"`
	SourceBusinessID string     `json:"sourceBusinessId" binding:"required,max=64"`
	SourceVersion    int64      `json:"sourceVersion" binding:"required"`
	RoomID           string     `json:"roomId" binding:"required,max=64"`
	StartAt          *time.Time `json:"startAt"`
	EndAt            *time.Time `json:"endAt"`
	DeadlineAt       *time.Time `json:"deadlineAt"`
	Reason           string     `json:"reason"`
}
type Receipt struct {
	OperationID      string    `json:"operationId"`
	OperationStatus  string    `json:"operationStatus"`
	SourceModule     string    `json:"sourceModule"`
	SourceBusinessID string    `json:"sourceBusinessId"`
	SourceVersion    int64     `json:"sourceVersion"`
	EffectiveAt      time.Time `json:"effectiveAt"`
	ErrorCode        string    `json:"errorCode,omitempty"`
}
type Fault struct {
	Code   string
	Status int
}

func (f *Fault) Error() string          { return f.Code }
func bad(code string, status int) error { return &Fault{code, status} }

type Service struct {
	DB         *sql.DB
	Catalog    Catalog
	Now        func() time.Time
	CodeSecret []byte
	Zone       *time.Location
}

func New(db *sql.DB, c Catalog, secret []byte) *Service {
	zone, _ := time.LoadLocation("Asia/Shanghai")
	return &Service{db, c, time.Now, secret, zone}
}
