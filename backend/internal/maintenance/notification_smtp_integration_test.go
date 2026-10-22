package maintenance_test

import (
	"bufio"
	maintenancedev "campus-resource-platform/backend/internal/maintenance/dev"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A real, loopback-only SMTP receiver: accepts MAIL/RCPT/DATA over TCP and
// retains the bytes after DATA. It does not replace the sender or SQL logic.
func smtpReceiver(t *testing.T, reply bool) (string, <-chan string) {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	received := make(chan string, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(3 * time.Second))
				r := textproto.NewReader(bufio.NewReader(c))
				fmt.Fprint(c, "220 local-test ESMTP\r\n")
				for {
					line, e := r.ReadLine()
					if e != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
						fmt.Fprint(c, "250 local-test\r\n")
					case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
						fmt.Fprint(c, "250 OK\r\n")
					case line == "DATA":
						fmt.Fprint(c, "354 End with dot\r\n")
						raw, e := io.ReadAll(r.DotReader())
						if e != nil {
							return
						}
						received <- string(raw)
						if !reply {
							return
						}
						fmt.Fprint(c, "250 Stored\r\n")
					case line == "QUIT":
						fmt.Fprint(c, "221 Bye\r\n")
						return
					default:
						fmt.Fprint(c, "500 Unsupported\r\n")
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { l.Close(); <-done })
	return l.Addr().String(), received
}
func mailFor(h *harness, address string) {
	h.t.Helper()
	path := filepath.Join(h.t.TempDir(), "recipients.json")
	raw, _ := json.Marshal(map[string]string{h.a.Principal.ID: "reporter@example.test"})
	if e := os.WriteFile(path, raw, 0600); e != nil {
		h.t.Fatal(e)
	}
	mail, e := maintenancedev.LoadNotificationMail(address, path)
	if e != nil {
		h.t.Fatal(e)
	}
	h.service.NotificationSender = mail
	notificationDirectory(h)
}
func receiveMail(t *testing.T, c <-chan string) string {
	t.Helper()
	select {
	case raw := <-c:
		return raw
	case <-time.After(3 * time.Second):
		t.Fatal("no real SMTP message received")
		return ""
	}
}
func messageID(t *testing.T, raw string) string {
	t.Helper()
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "Message-ID:") {
			return line
		}
	}
	t.Fatal("missing stable message ID")
	return ""
}

func TestNotificationRealSMTPPipeline(t *testing.T) {
	h := setup(t)
	o := h.report()
	addr, received := smtpReceiver(t, true)
	mailFor(h, addr)
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	raw := receiveMail(t, received)
	if !strings.Contains(raw, "To: reporter@example.test") || !strings.Contains(raw, "物业报修有新的消息") || strings.Contains(raw, o.ID) || strings.Contains(raw, input().Description) || strings.Contains(raw, h.a.Principal.ID) {
		t.Fatal("missing mail or sensitive payload", raw)
	}
	messageID(t, raw)
	var status string
	if e := h.db.QueryRow("SELECT status FROM m3_outbox").Scan(&status); e != nil {
		t.Fatal(e)
	}
	if status != "DELIVERED" {
		t.Fatal(status)
	}
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	select {
	case <-received:
		t.Fatal("successful email resent")
	default:
	}
}

func TestNotificationSMTPUnknownReplySameMessageIdentity(t *testing.T) {
	h := setup(t)
	h.report()
	clock := clockFor(h)
	addr, received := smtpReceiver(t, false)
	mailFor(h, addr)
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	first := receiveMail(t, received)
	var state string
	h.db.QueryRow("SELECT status FROM m3_outbox").Scan(&state)
	if state != "PENDING" {
		t.Fatal("unknown response marked success", state)
	}
	next, accepted := smtpReceiver(t, true)
	mailFor(h, next)
	clock.Add(int64(time.Minute))
	if e := h.service.DispatchNotifications(context.Background()); e != nil {
		t.Fatal(e)
	}
	second := receiveMail(t, accepted)
	if messageID(t, first) != messageID(t, second) {
		t.Fatal("retry changed logical email identity")
	}
	h.db.QueryRow("SELECT status FROM m3_outbox").Scan(&state)
	if state != "DELIVERED" || h.count("m3_workorders") != 1 || h.count("m3_events") != 1 {
		t.Fatal("mail retry replayed business", state)
	}
}
