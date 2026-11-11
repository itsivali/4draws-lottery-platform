package mail_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

func TestRender_IsAPlainTextMessageWithCRLF(t *testing.T) {
	at := time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC)
	got := mail.Render("4draws <noreply@example.com>", []string{"ops@example.com"}, "An ID is waiting", "Open the back office.\n.hidden", at)
	head, body, ok := strings.Cut(got, "\r\n\r\n")
	if !ok {
		t.Fatal("no blank line between headers and body")
	}
	for _, h := range []string{"From: 4draws <noreply@example.com>", "To: ops@example.com", "Subject: An ID is waiting",
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

func TestRenderMessage_AttachesAFileAsBase64AndRefusesABadName(t *testing.T) {
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	data := []byte(strings.Repeat(`{"k":"v"}`, 40))
	got, err := mail.RenderMessage("4draws <noreply@example.com>", []string{"jane@example.com"}, mail.Message{
		Subject: "Your data", Text: "Attached.",
		Attachments: []mail.Attachment{{Name: "4draws-data-2026-11-11.json", ContentType: "application/json", Data: data}},
	}, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Content-Type: multipart/mixed; boundary=", "Content-Type: text/plain; charset=utf-8", "Attached.\r\n",
		`Content-Disposition: attachment; filename="4draws-data-2026-11-11.json"`, "Content-Transfer-Encoding: base64"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	// the file comes back whole from its base64 lines, none longer than 76
	part := got[strings.Index(got, "Content-Transfer-Encoding: base64\r\n\r\n")+len("Content-Transfer-Encoding: base64\r\n\r\n"):]
	part = part[:strings.Index(part, "\r\n--")]
	for _, line := range strings.Split(part, "\r\n") {
		if len(line) > 76 {
			t.Fatalf("a base64 line of %d characters", len(line))
		}
	}
	back, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(part, "\r\n", ""))
	if err != nil || string(back) != string(data) {
		t.Fatalf("the attachment did not survive: %v", err)
	}
	for _, bad := range []string{"../etc/passwd", "a\r\nBcc: x@y.z", "", `q".json`} {
		if _, err := mail.RenderMessage("a@b.c", []string{"d@e.f"}, mail.Message{Attachments: []mail.Attachment{{Name: bad, ContentType: "application/json", Data: []byte("x")}}}, at); err == nil {
			t.Fatalf("attachment name %q accepted", bad)
		}
	}
	// without attachments it is the plain message, as before
	plain, err := mail.RenderMessage("a@b.c", []string{"d@e.f"}, mail.Message{Subject: "S", Text: "T"}, at)
	if err != nil || plain != mail.Render("a@b.c", []string{"d@e.f"}, "S", "T", at) {
		t.Fatalf("plain message changed: %v", err)
	}
}
