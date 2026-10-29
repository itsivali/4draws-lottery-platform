package kyc

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/kyc/store"
	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

// ReviewNotice says that a player's submission waits for a staff decision. It carries the name on the submitted ID
// (owner, 2026-10-29); the ID number and date of birth are shown, and audited, only in the back office.
type ReviewNotice struct {
	UserID string
	// FullName is the name as written on the submitted ID.
	FullName    string
	SubmittedAt time.Time
	// Waiting is how many submissions wait in all, this one included.
	Waiting int64
}

// ReviewNotifier tells the reviewers about a waiting submission.
type ReviewNotifier interface {
	NotifyReview(ctx context.Context, n ReviewNotice) error
}

// SendReviewNotice tells the reviewers that the account's submission waits, if it still does: a submission decided
// before the job ran, or superseded by a newer one, sends nothing. It is the body of the kyc.review_notice job.
func (s *Service) SendReviewNotice(ctx context.Context, checkID, userID string) error {
	if s.notify == nil {
		return nil
	}
	q := store.New(s.pool)
	cur, err := q.GetUserKYCStatus(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if Status(cur) != ManualReview {
		return nil
	}
	last, err := q.LatestKYCCheck(ctx, userID)
	if err != nil || last.ID != checkID {
		return err
	}
	name, err := s.kr.Decrypt(last.FullNameEnc, []byte("kyc.full_name"))
	if err != nil {
		return err
	}
	queue, err := s.ReviewQueue(ctx)
	if err != nil {
		return err
	}
	return s.notify.NotifyReview(ctx, ReviewNotice{UserID: userID, FullName: string(name), SubmittedAt: last.CreatedAt, Waiting: queue.Waiting})
}

// MailReviewNotifier emails each notice to To, with a link that opens the submission in the back office.
type MailReviewNotifier struct {
	Sender mail.Sender
	To     string
	// BackOfficeURL is the back office's origin (https://backoffice.tajirika.co.ke).
	BackOfficeURL string
}

// NotifyReview implements ReviewNotifier.
func (m MailReviewNotifier) NotifyReview(ctx context.Context, n ReviewNotice) error {
	return m.Sender.Send(ctx, mail.Message{To: []string{m.To}, Subject: reviewSubject(n), Text: reviewText(n, m.Link(n.UserID))})
}

// Link is the back office page that shows the account's submission first.
func (m MailReviewNotifier) Link(userID string) string {
	return strings.TrimRight(m.BackOfficeURL, "/") + "/kyc?user=" + url.QueryEscape(userID)
}

func reviewSubject(n ReviewNotice) string {
	if n.Waiting > 1 {
		return "Tajirika: " + strconv.FormatInt(n.Waiting, 10) + " identity submissions are waiting for review"
	}
	return "Tajirika: an identity submission is waiting for review"
}

func reviewText(n ReviewNotice, link string) string {
	nairobi := time.FixedZone("EAT", 3*60*60)
	var b strings.Builder
	who := strings.TrimSpace(n.FullName)
	if who == "" {
		who = "A player"
	}
	b.WriteString(who + " submitted their national ID for verification on " + n.SubmittedAt.In(nairobi).Format("Mon 2 Jan 2006 at 15:04 MST") + ".\n\n")
	b.WriteString("Review it in the back office (sign in first):\n" + link + "\n\n")
	if n.Waiting > 1 {
		b.WriteString(strconv.FormatInt(n.Waiting, 10) + " submissions are waiting in all.\n")
	} else {
		b.WriteString("It is the only submission waiting.\n")
	}
	b.WriteString("\nThe ID number and date of birth are shown only in the back office, where every look is recorded in the audit log.\n")
	return b.String()
}
