package booking

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MaintenanceImpact v1 identifies an exact M2 source held when M1 blocked a room.
// M1 owns this snapshot; consumers never query or modify another module's tables.
type MaintenanceImpact struct {
	EventID                  string    `json:"eventId"`
	Type                     string    `json:"type"`
	RoomID                   string    `json:"roomId"`
	SourceModule             string    `json:"sourceModule"`
	SourceBusinessID         string    `json:"sourceBusinessId"`
	SourceVersion            int64     `json:"sourceVersion"`
	OccurredAt               time.Time `json:"occurredAt"`
	ApplicationID            string    `json:"applicationId"`
	ApplicationSourceVersion int64     `json:"applicationSourceVersion"`
	CorrelationID            string    `json:"correlationId"`
}

func (s *Service) queueMaintenanceImpacts(ctx context.Context, tx *sql.Tx, q Operation, at time.Time, trace string) error {
	rows, err := tx.QueryContext(ctx, "SELECT business_id,version FROM m1_sources WHERE module='M2' AND room_id=? AND active=1 AND kind='ROOM' AND end_at>?", q.RoomID, at)
	if err != nil {
		return err
	}
	var events []MaintenanceImpact
	for rows.Next() {
		event := MaintenanceImpact{EventID: ID(), Type: "ROOM_MAINTENANCE_STARTED", RoomID: q.RoomID, SourceModule: "M3", SourceBusinessID: q.SourceBusinessID, SourceVersion: q.SourceVersion, OccurredAt: at, CorrelationID: trace}
		if err = rows.Scan(&event.ApplicationID, &event.ApplicationSourceVersion); err != nil {
			rows.Close()
			return err
		}
		events = append(events, event)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO m1_maintenance_impacts(id,source_business_id,payload,next_attempt,created_at) VALUES (?,?,?,?,?)", event.EventID, event.SourceBusinessID, raw, at, at); err != nil {
			return err
		}
	}
	return nil
}

type ImpactSender interface {
	Send(context.Context, MaintenanceImpact) error
}
type HTTPImpactSender struct {
	endpoint, token string
	client          *http.Client
}

func NewHTTPImpactSender(base, token string) (*HTTPImpactSender, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return nil, fmt.Errorf("invalid maintenance impact endpoint or token")
	}
	return &HTTPImpactSender{strings.TrimRight(base, "/") + "/api/v1/internal/m2/maintenance-impacts", token, &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (h *HTTPImpactSender) Send(ctx context.Context, event MaintenanceImpact) error {
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", h.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("impact transport unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 202 {
		return fmt.Errorf("impact HTTP %d", res.StatusCode)
	}
	var envelope struct {
		Data struct {
			EventID string `json:"eventId"`
			Status  string `json:"status"`
		}
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 16<<10))
	if err = decoder.Decode(&envelope); err != nil {
		return err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("invalid impact receipt suffix")
	}
	if envelope.Data.EventID != event.EventID || envelope.Data.Status != "ACCEPTED" {
		return fmt.Errorf("impact receipt mismatch")
	}
	return nil
}
func (s *Service) DeliverMaintenanceImpacts(ctx context.Context) error {
	if s.ImpactSender == nil {
		return nil
	} // Persist until configured; never claim delivery.
	rows, err := s.DB.QueryContext(ctx, "SELECT id FROM m1_maintenance_impacts WHERE state='PENDING' AND next_attempt<=? ORDER BY next_attempt,id LIMIT 20", s.Now().UTC())
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.deliverImpact(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) deliverImpact(ctx context.Context, id string) error {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked int
	lock := "m1-impact:" + id
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", lock).Scan(&locked); err != nil {
		return err
	}
	if locked != 1 {
		return nil
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn.ExecContext(cleanup, "DO RELEASE_LOCK(?)", lock)
	}()
	var raw []byte
	var state string
	var attempts int
	var due time.Time
	if err = conn.QueryRowContext(ctx, "SELECT payload,state,attempts,next_attempt FROM m1_maintenance_impacts WHERE id=?", id).Scan(&raw, &state, &attempts, &due); err != nil {
		return err
	}
	if state != "PENDING" || due.After(s.Now()) {
		return nil
	}
	var event MaintenanceImpact
	if err = json.Unmarshal(raw, &event); err != nil {
		return err
	}
	// Persist attempts before I/O so crashes do not reset the retry budget.
	if attempts >= 8 {
		_, err = conn.ExecContext(ctx, "UPDATE m1_maintenance_impacts SET state='FAILED',last_error='OUTCOME_UNKNOWN' WHERE id=?", id)
		return err
	}
	if _, err = conn.ExecContext(ctx, "UPDATE m1_maintenance_impacts SET attempts=attempts+1 WHERE id=?", id); err != nil {
		return err
	}
	deliveryErr := s.ImpactSender.Send(ctx, event)
	state = "DELIVERED"
	reason := ""
	if deliveryErr != nil {
		state = "PENDING"
		reason = "DELIVERY_UNAVAILABLE"
		if attempts+1 >= 8 {
			state = "FAILED"
		}
	}
	_, err = conn.ExecContext(ctx, "UPDATE m1_maintenance_impacts SET state=?,last_error=?,next_attempt=? WHERE id=?", state, reason, s.Now().UTC().Add(time.Second*time.Duration(1<<min(attempts+1, 8))), id)
	return err
}
func (s *Service) RetryMaintenanceImpact(ctx context.Context, a Actor, id string) error {
	if a.Service != "M3" {
		return bad("FORBIDDEN", 403)
	}
	tx, err := s.tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state, business string
	if err = tx.QueryRowContext(ctx, "SELECT state,source_business_id FROM m1_maintenance_impacts WHERE id=? FOR UPDATE", id).Scan(&state, &business); err == sql.ErrNoRows {
		return bad("NOT_FOUND", 404)
	} else if err != nil {
		return err
	}
	if state != "FAILED" {
		return bad("INVALID_STATE", 409)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE m1_maintenance_impacts SET state='PENDING',attempts=0,next_attempt=?,last_error='' WHERE id=?", s.Now().UTC(), id); err != nil {
		return err
	}
	if err = s.audit(ctx, tx, "M3", "RETRY_MAINTENANCE_IMPACT", business, "M3", id); err != nil {
		return err
	}
	return tx.Commit()
}
