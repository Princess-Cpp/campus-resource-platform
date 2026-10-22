// Package dev contains explicit M3 development adapters, not a second identity
// service or a claim that D-02/D-03 integration is complete.
package dev

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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
	return func(ctx context.Context, r *http.Request) (domain.Actor, error) {
		session, e := store.AuthenticateRequest(ctx, r)
		if e != nil {
			return domain.Actor{}, e
		}
		p := session.Principal
		permissions := append([]string{}, p.Permissions...)
		if studentReporting && slices.Contains(p.Roles, "student") && !p.Can("workorder.report") {
			permissions = append(permissions, "workorder.report")
		}
		return domain.Actor{ID: p.ID, Enabled: true, Department: p.Department, Permissions: permissions, Areas: append([]string{}, p.Areas...)}, nil
	}
}

// Events stores audit facts and pending notification intent in the *business*
// transaction. A future shared adapter can replace it without changing Report.
// It deliberately does not send mail or claim the common outbox is delivered.
type Events struct{}

func (Events) Append(ctx context.Context, tx *sql.Tx, c domain.Change, eventID, trace string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO m3_events(event_id,workorder_id,business_version,actor_id,action,old_state,new_state,occurred_at,correlation_id) VALUES (?,?,?,?,?,?,?,?,?)`, eventID, c.Order.ID, c.Order.Version, c.ActorID, c.Action, c.From, c.Order.State, c.At, trace)
	if e != nil {
		return e
	}
	payload, e := json.Marshal(map[string]any{"workorderId": c.Order.ID, "roomName": c.Order.RoomName, "state": c.Order.State})
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO m3_outbox(event_id,workorder_id,recipient_id,kind,payload,created_at) VALUES (?,?,?,?,?,?)`, eventID, c.Order.ID, c.Order.ReporterID, "WORKORDER_REPORTED", payload, c.At)
	return e
}
