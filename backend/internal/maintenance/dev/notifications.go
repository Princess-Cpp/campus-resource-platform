package dev

import (
	"campus-resource-platform/backend/internal/maintenance"
	"campus-resource-platform/backend/internal/notification"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type NotificationMail struct {
	SMTP       *notification.LocalSMTP
	Recipients map[string]string
}

func LoadNotificationMail(address, path string) (*NotificationMail, error) {
	if address == "" && path == "" {
		return nil, nil
	}
	smtp, e := notification.NewLocalSMTP(address, "campus@example.test")
	if e != nil {
		return nil, e
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var recipients map[string]string
	if e = json.Unmarshal(raw, &recipients); e != nil {
		return nil, e
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("empty local notification recipient mapping")
	}
	return &NotificationMail{SMTP: smtp, Recipients: recipients}, nil
}
func (m *NotificationMail) Send(ctx context.Context, recipient, event string) error {
	address := m.Recipients[recipient]
	if address == "" {
		return maintenance.ErrNotificationUnconfigured
	}
	e := m.SMTP.Send(ctx, notification.Message{ID: event, To: address})
	if errors.Is(e, notification.ErrInvalidRecipient) {
		return maintenance.ErrNotificationRecipientInvalid
	}
	return e
}
