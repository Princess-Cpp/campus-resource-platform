// Package dev contains explicit M3 development adapters, not a second identity
// service or a claim that D-02/D-03 integration is complete.
package dev

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"unicode/utf8"

	"campus-resource-platform/backend/internal/identity"
	"campus-resource-platform/backend/internal/maintenance"
	"campus-resource-platform/backend/internal/maintenance/domain"
)

type Catalog struct{ Data []maintenance.Room }

func (c *Catalog) Rooms(context.Context) ([]maintenance.Room, error) {
	out := append([]maintenance.Room{}, c.Data...)
	for i := range out {
		out[i].Seats = append([]maintenance.Seat{}, out[i].Seats...)
	}
	return out, nil
}
func LoadCatalog(path string) (*Catalog, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var input struct {
		Rooms []struct {
			ID         string             `json:"id"`
			Name       string             `json:"name"`
			Area       string             `json:"area"`
			Department string             `json:"department"`
			Status     string             `json:"status"`
			Seats      []maintenance.Seat `json:"seats"`
		} `json:"rooms"`
	}
	if e = json.Unmarshal(raw, &input); e != nil {
		return nil, e
	}
	c := &Catalog{}
	ids := map[string]bool{}
	for _, r := range input.Rooms {
		if r.ID == "" || len(r.ID) > 64 || ids[r.ID] || r.Name == "" || utf8.RuneCountInString(r.Name) > 128 || r.Area == "" || utf8.RuneCountInString(r.Area) > 128 || !slices.Contains([]string{"ACTIVE", "DISABLED", "RETIRED"}, r.Status) {
			return nil, fmt.Errorf("invalid M3 development room")
		}
		ids[r.ID] = true
		seats := map[string]bool{}
		for _, seat := range r.Seats {
			if seat.ID == "" || len(seat.ID) > 64 || seats[seat.ID] || utf8.RuneCountInString(seat.Label) > 128 {
				return nil, fmt.Errorf("invalid M3 development seat")
			}
			seats[seat.ID] = true
		}
		c.Data = append(c.Data, maintenance.Room{ID: r.ID, Name: r.Name, Area: r.Area, Department: r.Department, Status: r.Status, Seats: r.Seats})
	}
	if len(c.Data) == 0 {
		return nil, fmt.Errorf("M3 development catalog is empty")
	}
	return c, nil
}

// Authenticator always uses the real SQL-backed browser session, including
// current disable/revocation checks. The optional student mapping is explicit
// development policy; it never writes D roles or accepts roles from JSON.
func Authenticator(store *identity.Store, studentReporting bool) maintenance.Authenticate {
	return (&Directory{Store: store, StudentReporting: studentReporting}).Authenticate
}

// Events stores audit facts and pending notification intent in the *business*
// transaction. A future shared adapter can replace it without changing Report.
// It deliberately does not send mail or claim the common outbox is delivered.
type Events struct{}

func (Events) Append(ctx context.Context, tx *sql.Tx, c domain.Change, eventID, trace string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO m3_events(event_id,workorder_id,business_version,actor_id,action,old_state,new_state,occurred_at,correlation_id,comment) VALUES (?,?,?,?,?,?,?,?,?,?)`, eventID, c.Order.ID, c.Order.Version, c.ActorID, c.Action, c.From, c.Order.State, c.At, trace, c.Comment)
	if e != nil {
		return e
	}
	payload, e := json.Marshal(map[string]any{"workorderId": c.Order.ID, "roomName": c.Order.RoomName, "state": c.Order.State})
	if e != nil {
		return e
	}
	kind := "WORKORDER_" + c.Action
	switch c.Action {
	case "REPORT":
		kind = "WORKORDER_REPORTED"
	case "ASSIGN":
		kind = "WORKORDER_ASSIGNED"
	case "ACCEPT_ASSIGNMENT":
		kind = "WORKORDER_ACCEPTED"
	case "CANCEL":
		kind = "WORKORDER_CANCELLED"
	}
	recipients := map[string]string{c.Order.ReporterID: kind}
	if c.Action != "REPORT" && c.Order.AssigneeID != "" {
		recipients[c.Order.AssigneeID] = kind
	}
	if c.Action == "ASSIGN" {
		var old string
		if e = tx.QueryRowContext(ctx, "SELECT previous_assignee_id FROM m3_assignment_history WHERE id=?", eventID).Scan(&old); e != nil {
			return e
		}
		if old != "" && old != c.Order.AssigneeID {
			recipients[old] = "WORKORDER_HANDOVER"
		}
	}
	for recipient, notification := range recipients {
		if _, e = tx.ExecContext(ctx, `INSERT INTO m3_outbox(event_id,workorder_id,recipient_id,kind,payload,created_at) VALUES (?,?,?,?,?,?)`, eventID, c.Order.ID, recipient, notification, payload, c.At); e != nil {
			return e
		}
	}
	return nil
}
