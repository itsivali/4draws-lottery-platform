package kyc

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

// VerifyArgs is the queue payload. It carries only the check id: the personal details stay encrypted in the database
// and never pass through the job table.
type VerifyArgs struct {
	CheckID string `json:"check_id"`
}

// Kind implements river.JobArgs.
func (VerifyArgs) Kind() string { return "kyc.verify" }

// ReviewNoticeArgs is the payload of the reviewers' notice: which account's submission waits. No personal details.
type ReviewNoticeArgs struct {
	CheckID string `json:"check_id"`
	UserID  string `json:"user_id"`
}

// Kind implements river.JobArgs.
func (ReviewNoticeArgs) Kind() string { return "kyc.review_notice" }

const defaultMaxAttempts = 8

// RiverEnqueuer enqueues verification jobs inside the caller's transaction.
type RiverEnqueuer struct {
	client      *river.Client[pgx.Tx]
	maxAttempts int
}

// NewRiverEnqueuer returns an Enqueuer on an (insert-only is enough) River client.
func NewRiverEnqueuer(c *river.Client[pgx.Tx], maxAttempts int) *RiverEnqueuer {
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	return &RiverEnqueuer{client: c, maxAttempts: maxAttempts}
}

// EnqueueVerify implements Enqueuer.
func (e *RiverEnqueuer) EnqueueVerify(ctx context.Context, tx pgx.Tx, checkID, _ string) error {
	_, err := e.client.InsertTx(ctx, tx, VerifyArgs{CheckID: checkID}, &river.InsertOpts{MaxAttempts: e.maxAttempts})
	return err
}

// EnqueueReviewNotice implements Enqueuer.
func (e *RiverEnqueuer) EnqueueReviewNotice(ctx context.Context, tx pgx.Tx, checkID, userID string) error {
	_, err := e.client.InsertTx(ctx, tx, ReviewNoticeArgs{CheckID: checkID, UserID: userID}, &river.InsertOpts{MaxAttempts: e.maxAttempts})
	return err
}

// reviewNoticeWorker runs Service.SendReviewNotice; a failed send is retried by the queue.
type reviewNoticeWorker struct {
	river.WorkerDefaults[ReviewNoticeArgs]
	svc *Service
}

// Work implements river.Worker.
func (w *reviewNoticeWorker) Work(ctx context.Context, job *river.Job[ReviewNoticeArgs]) error {
	return w.svc.SendReviewNotice(ctx, job.Args.CheckID, job.Args.UserID)
}

// verifyWorker runs Service.Process; when the provider stays down through the last attempt the submission is
// abandoned so the player is not stuck in "pending".
type verifyWorker struct {
	river.WorkerDefaults[VerifyArgs]
	svc *Service
}

// Work implements river.Worker.
func (w *verifyWorker) Work(ctx context.Context, job *river.Job[VerifyArgs]) error {
	err := w.svc.Process(ctx, job.Args.CheckID)
	if errors.Is(err, ErrProviderUnavailable) && job.Attempt >= job.MaxAttempts {
		return w.svc.Abandon(ctx, job.Args.CheckID)
	}
	return err
}

// AddWorkers registers the KYC job workers.
func AddWorkers(ws *river.Workers, svc *Service) {
	river.AddWorker(ws, &verifyWorker{svc: svc})
	river.AddWorker(ws, &reviewNoticeWorker{svc: svc})
}
