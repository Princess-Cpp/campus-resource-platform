package notification_test

import (
	"campus-resource-platform/backend/internal/notification"
	"context"
	"testing"
)

func TestLocalSMTPRejectsUnsafeConfiguration(t *testing.T) {
	for _, address := range []string{"smtp.example.test:25", "192.0.2.1:25", "127.0.0.1:0", "127.0.0.1:bad"} {
		if _, e := notification.NewLocalSMTP(address, "campus@example.test"); e == nil {
			t.Fatal("unsafe endpoint accepted", address)
		}
	}
	smtp, e := notification.NewLocalSMTP("127.0.0.1:1", "campus@example.test")
	if e != nil {
		t.Fatal(e)
	}
	for _, to := range []string{"victim@example.com", "x@example.test\r\nBcc: y@example.test", "Display <x@example.test>"} {
		if e = smtp.Send(context.Background(), notification.Message{ID: "event", To: to}); e == nil {
			t.Fatal("unsafe address accepted", to)
		}
	}
}
