package mail_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

func TestRender_IsAPlainTextMessageWithCRLF(t *testing.T) {
	at := time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC)
	got := mail.Render("Bahati <noreply@example.com>", []string{"ops@example.com"}, "An ID is waiting", "Open the back office.\n.hidden", at)
	head, body, ok := strings.Cut(got, "\r\n\r\n")
	if !ok {
		t.Fatal("no blank line between headers and body")
	}
	for _, h := range []string{"From: Bahati <noreply@example.com>", "To: ops@example.com", "Subject: An ID is waiting",
		"Date: Wed, 28 Oct 2026 09:30:00 +0000", "Content-Type: text/plain; charset=utf-8"} {
		if !strings.Contains(head, h+"\r\n") {
			t.Errorf("header %q missing in %q", h, head)
		}
	}
	if body != "Open the back office.\r\n..hidden\r\n" {
		t.Errorf("body = %q", body)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("a bare line feed survived")
	}
}

func TestSMTP_RefusesHeaderInjectionBeforeConnecting(t *testing.T) {
	s := mail.SMTP{Host: "smtp.invalid", Port: 587, From: "noreply@example.com"}
	err := s.Send(context.Background(), mail.Message{To: []string{"ops@example.com"}, Subject: "x\r\nBcc: everyone@example.com", Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "line break") {
		t.Fatalf("err = %v", err)
	}
	err = s.Send(context.Background(), mail.Message{To: []string{"not an address"}, Subject: "x", Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("err = %v", err)
	}
	err = mail.SMTP{Host: "smtp.invalid", Port: 587, From: "nope"}.Send(context.Background(), mail.Message{To: []string{"ops@example.com"}, Subject: "x", Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "SMTP_FROM") {
		t.Fatalf("err = %v", err)
	}
}

func TestLog_SendsNothingAndSucceeds(t *testing.T) {
	if err := (mail.Log{}).Send(context.Background(), mail.Message{To: []string{"ops@example.com"}, Subject: "x", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
}
