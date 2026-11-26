package kyc_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/itsivali/4draws-lottery-platform/internal/kyc"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/mail"
)

type sentMail struct{ msgs []mail.Message }

func (s *sentMail) Send(_ context.Context, m mail.Message) error {
	s.msgs = append(s.msgs, m)
	return nil
}

func TestMailReviewNotifier_LinksTheSubmissionAndNamesThePlayer(t *testing.T) {
	sent := &sentMail{}
	n := kyc.MailReviewNotifier{Sender: sent, To: "ops@example.com", BackOfficeURL: "https://backoffice.example.com/"}
	at := time.Date(2026, 10, 28, 6, 5, 0, 0, time.UTC)
	if err := n.NotifyReview(context.Background(), kyc.ReviewNotice{UserID: "00000000-0000-7000-8000-000000000009", FullName: "Amina Wanjiru Otieno", SubmittedAt: at, Waiting: 1}); err != nil {
		t.Fatal(err)
	}
	if len(sent.msgs) != 1 || sent.msgs[0].To[0] != "ops@example.com" {
		t.Fatalf("sent %+v", sent.msgs)
	}
	m := sent.msgs[0]
	if m.Subject != "4draws: an identity submission is waiting for review" {
		t.Errorf("subject = %q", m.Subject)
	}
	for _, want := range []string{"Amina Wanjiru Otieno submitted their national ID", "https://backoffice.example.com/kyc?user=00000000-0000-7000-8000-000000000009", "Wed 28 Oct 2026 at 09:05 EAT", "only submission waiting"} {
		if !strings.Contains(m.Text, want) {
			t.Errorf("text lacks %q:\n%s", want, m.Text)
		}
	}
	if err := n.NotifyReview(context.Background(), kyc.ReviewNotice{UserID: "u", SubmittedAt: at, Waiting: 3}); err != nil {
		t.Fatal(err)
	}
	if m := sent.msgs[1]; m.Subject != "4draws: 3 identity submissions are waiting for review" || !strings.Contains(m.Text, "3 submissions are waiting in all") || !strings.HasPrefix(m.Text, "A player submitted") {
		t.Errorf("plural: %q\n%s", m.Subject, m.Text)
	}
}

func TestMailReviewNotifier_LinkEscapesTheUserID(t *testing.T) {
	n := kyc.MailReviewNotifier{BackOfficeURL: "http://localhost:5173"}
	if got := n.Link("a b&c"); got != "http://localhost:5173/kyc?user=a+b%26c" {
		t.Errorf("link = %q", got)
	}
}
