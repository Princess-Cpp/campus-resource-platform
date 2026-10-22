// Package notification provides transport only. Shared job persistence and
// scheduling remain owned by M2; this package never reads business tables.
package notification

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	ID string
	To string
}

var ErrInvalidRecipient = errors.New("invalid test notification recipient")

type LocalSMTP struct{ address, host, from string }

// NewLocalSMTP is explicitly a development channel for a local receiver. It
// must not silently turn into production mail with unverified contact refs.
func NewLocalSMTP(address, from string) (*LocalSMTP, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, fmt.Errorf("invalid local SMTP endpoint")
	}
	ip := net.ParseIP(host)
	n, e := strconv.Atoi(port)
	if ip == nil || !ip.IsLoopback() || e != nil || n < 1 || n > 65535 || !testMailbox(from) {
		return nil, fmt.Errorf("local SMTP requires loopback and test mailbox")
	}
	return &LocalSMTP{address: address, host: host, from: from}, nil
}
func testMailbox(value string) bool {
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	a, e := mail.ParseAddress(value)
	return e == nil && a.Name == "" && a.Address == value && strings.HasSuffix(strings.ToLower(value), ".test")
}
func (s *LocalSMTP) Send(ctx context.Context, m Message) error {
	if !testMailbox(m.To) {
		return ErrInvalidRecipient
	}
	if m.ID == "" || len(m.ID) > 256 {
		return fmt.Errorf("invalid test notification")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", s.address)
	if e != nil {
		return e
	}
	defer conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if e = conn.SetDeadline(deadline); e != nil {
		return e
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	client, e := smtp.NewClient(conn, s.host)
	if e != nil {
		return e
	}
	defer client.Close()
	if e = client.Mail(s.from); e != nil {
		return e
	}
	if e = client.Rcpt(m.To); e != nil {
		return e
	}
	writer, e := client.Data()
	if e != nil {
		return e
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(m.ID+"\x00"+m.To)))
	subject := base64.StdEncoding.EncodeToString([]byte("校园平台：报修消息更新"))
	body := "物业报修有新的消息。请登录校园资源服务平台，在当前授权范围内查看。\r\n此邮件来自本地开发测试渠道。\r\n"
	_, e = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nMessage-ID: <%s@campus.example.test>\r\nSubject: =?UTF-8?B?%s?=\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s", s.from, m.To, id, subject, body)
	if e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	// DATA's final 250 is the acceptance boundary. QUIT losing its reply must
	// not turn an accepted message into a reported send failure.
	_ = client.Quit()
	return nil
}
