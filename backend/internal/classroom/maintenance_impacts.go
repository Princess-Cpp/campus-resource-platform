package classroom

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Consumer representation of resource-maintenance-impact v1.
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

var impactID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)

func (s *Service) RegisterMaintenanceImpacts(r *gin.Engine, token string) error {
	if len(token) < 32 || strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("maintenance impact service token must contain at least 32 bytes")
	}
	r.POST("/api/v1/internal/m2/maintenance-impacts", func(c *gin.Context) {
		// A separate M1 delivery credential, never a browser or M3/M2 resource token.
		if c.GetHeader("Cookie") != "" || subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+token)) != 1 {
			reply(c, nil, fault("UNAUTHENTICATED", 401))
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var event MaintenanceImpact
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&event); err != nil {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			reply(c, nil, fault("INVALID_INPUT", 400))
			return
		}
		if err := s.ReceiveMaintenanceImpact(c.Request.Context(), event); err != nil {
			reply(c, nil, err)
			return
		}
		c.JSON(202, gin.H{"data": gin.H{"eventId": event.EventID, "status": "ACCEPTED"}})
	})
	return nil
}
func (s *Service) ReceiveMaintenanceImpact(ctx context.Context, event MaintenanceImpact) error {
	if !impactID.MatchString(event.EventID) || event.Type != "ROOM_MAINTENANCE_STARTED" || event.SourceModule != "M3" || event.SourceBusinessID == "" || len(event.SourceBusinessID) > 64 || event.RoomID == "" || len(event.RoomID) > 64 || event.ApplicationID == "" || len(event.ApplicationID) > 64 || event.SourceVersion < 1 || event.ApplicationSourceVersion < 1 || event.OccurredAt.IsZero() || len(event.CorrelationID) > 64 {
		return fault("INVALID_INPUT", 400)
	}
	event.OccurredAt = event.OccurredAt.UTC()
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT INTO m2_maintenance_impacts(id,request_hash,application_id,payload,created_at) VALUES (?,?,?,?,?) ON DUPLICATE KEY UPDATE id=id", event.EventID, hash(event), event.ApplicationID, raw, s.Now().UTC()); err != nil {
		return err
	}
	var digest string
	if err = tx.QueryRowContext(ctx, "SELECT request_hash FROM m2_maintenance_impacts WHERE id=? FOR UPDATE", event.EventID).Scan(&digest); err != nil {
		return err
	}
	if digest != hash(event) {
		return fault("IDEMPOTENCY_CONFLICT", 409)
	}
	return tx.Commit()
}
func (s *Service) ProcessMaintenanceImpacts(ctx context.Context) error {
	// Keyset pagination keeps unresolved early events from starving later ones.
	var result error
	after := ""
	for {
		rows, err := s.DB.QueryContext(ctx, "SELECT id FROM m2_maintenance_impacts WHERE state IN ('PENDING','QUEUED') AND id>? ORDER BY id LIMIT 100", after)
		if err != nil {
			return errors.Join(result, err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return errors.Join(result, err)
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return errors.Join(result, err)
		}
		for _, id := range ids {
			result = errors.Join(result, s.processMaintenanceImpact(ctx, id))
		}
		if len(ids) < 100 {
			return result
		}
		after = ids[len(ids)-1]
	}
}

func (s *Service) processMaintenanceImpact(ctx context.Context, eventID string) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw []byte
	var state string
	if err = tx.QueryRowContext(ctx, "SELECT payload,state FROM m2_maintenance_impacts WHERE id=? FOR UPDATE", eventID).Scan(&raw, &state); err != nil {
		return err
	}
	if state == "DONE" {
		return nil
	}
	var event MaintenanceImpact
	if err = json.Unmarshal(raw, &event); err != nil {
		return err
	}
	b, err := scan(tx.QueryRowContext(ctx, "SELECT "+cols+" FROM m2_applications WHERE id=? FOR UPDATE", event.ApplicationID))
	if err != nil {
		return err
	}
	if b.RoomID != event.RoomID {
		return fault("SOURCE_MISMATCH", 409)
	}
	// Never override unknown approval/release outcomes. Normal operation recovery
	// runs first and this durable event is revisited after its receipt is known.
	if b.SyncStatus != "IDLE" {
		return nil
	}
	if b.Status == "APPROVED" {
		if state == "QUEUED" || b.SourceVersion != event.ApplicationSourceVersion || !b.EndAt.After(event.OccurredAt) {
			return fault("SOURCE_MISMATCH", 409)
		}
		target := "CANCELLED"
		if !event.OccurredAt.Before(b.StartAt) {
			target = "TERMINATED"
		}
		from := b.Status
		if err = s.queue(ctx, tx, &b, "resource-maintenance", "RELEASE_SOURCE", target, event.CorrelationID); err != nil {
			return err
		}
		b.Version++
		if err = s.save(ctx, tx, b); err != nil {
			return err
		}
		if err = s.event(ctx, tx, b, "resource-maintenance", "MAINTENANCE_CANCEL", from, "Maintenance "+event.SourceBusinessID+"; event "+event.EventID, event.CorrelationID); err != nil {
			return err
		}
		state = "QUEUED"
	} else if contains([]string{"CANCELLED", "TERMINATED", "EXPIRED", "REJECTED"}, b.Status) {
		state = "DONE"
	} else {
		return fault("SOURCE_MISMATCH", 409)
	}
	if _, err = tx.ExecContext(ctx, "UPDATE m2_maintenance_impacts SET state=? WHERE id=?", state, eventID); err != nil {
		return err
	}
	return tx.Commit()
}
