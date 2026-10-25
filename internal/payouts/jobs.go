package payouts

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// SendArgs makes the B2C request for a payout.
type SendArgs struct {
	PayoutID string `json:"payout_id"`
}

// Kind implements river.JobArgs.
func (SendArgs) Kind() string { return "payouts.send" }

// VerifyArgs moves an open payout forward.
type VerifyArgs struct {
	PayoutID string `json:"payout_id"`
	Source   Source `json:"source"`
}

// Kind implements river.JobArgs.
func (VerifyArgs) Kind() string { return "payouts.verify" }

// SweepArgs looks for payouts left behind.
type SweepArgs struct{}

// Kind implements river.JobArgs.
func (SweepArgs) Kind() string { return "payouts.sweep" }

// PrizesArgs creates the payouts of approved draws' prizes.
type PrizesArgs struct{}

// Kind implements river.JobArgs.
func (PrizesArgs) Kind() string { return "payouts.prizes" }

// ReconcileArgs runs the daily payout reconciliation.
type ReconcileArgs struct{}

// Kind implements river.JobArgs.
func (ReconcileArgs) Kind() string { return "payouts.reconcile" }

const verifyMaxAttempts = 20

// unique collapses a job into one already waiting or running for the same arguments, but not into one that finished:
// a payout approved, swept or called back later must get a new job (River's default also counts completed jobs).
var unique = river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStatePending,
	rivertype.JobStateRetryable, rivertype.JobStateRunning, rivertype.JobStateScheduled}}

// RiverEnqueuer enqueues payout jobs inside the caller's transaction.
type RiverEnqueuer struct{ client *river.Client[pgx.Tx] }

// NewRiverEnqueuer returns an Enqueuer on an (insert-only is enough) River client.
func NewRiverEnqueuer(c *river.Client[pgx.Tx]) *RiverEnqueuer { return &RiverEnqueuer{client: c} }

// EnqueueSend implements Enqueuer. The send job runs at most once (see Service.Send).
func (e *RiverEnqueuer) EnqueueSend(ctx context.Context, tx pgx.Tx, payoutID string) error {
	_, err := e.client.InsertTx(ctx, tx, SendArgs{PayoutID: payoutID}, &river.InsertOpts{MaxAttempts: 1, UniqueOpts: unique})
	return err
}

// EnqueueVerify implements Enqueuer.
func (e *RiverEnqueuer) EnqueueVerify(ctx context.Context, tx pgx.Tx, payoutID string, source Source) error {
	_, err := e.client.InsertTx(ctx, tx, VerifyArgs{PayoutID: payoutID, Source: source},
		&river.InsertOpts{MaxAttempts: verifyMaxAttempts, UniqueOpts: unique})
	return err
}

type sendWorker struct {
	river.WorkerDefaults[SendArgs]
	svc *Service
}

func (w *sendWorker) Work(ctx context.Context, j *river.Job[SendArgs]) error {
	return w.svc.Send(ctx, j.Args.PayoutID)
}

type verifyWorker struct {
	river.WorkerDefaults[VerifyArgs]
	svc *Service
}

func (w *verifyWorker) Work(ctx context.Context, j *river.Job[VerifyArgs]) error {
	d, err := w.svc.Verify(ctx, j.Args.PayoutID)
	if err != nil {
		return err
	}
	if d > 0 {
		return river.JobSnooze(d)
	}
	return nil
}

type sweepWorker struct {
	river.WorkerDefaults[SweepArgs]
	svc *Service
}

func (w *sweepWorker) Work(ctx context.Context, _ *river.Job[SweepArgs]) error {
	return w.svc.Sweep(ctx)
}

type prizesWorker struct {
	river.WorkerDefaults[PrizesArgs]
	svc *Service
}

func (w *prizesWorker) Work(ctx context.Context, _ *river.Job[PrizesArgs]) error {
	_, err := w.svc.PayPrizes(ctx)
	return err
}

type reconcileWorker struct {
	river.WorkerDefaults[ReconcileArgs]
	svc *Service
}

func (w *reconcileWorker) Work(ctx context.Context, _ *river.Job[ReconcileArgs]) error {
	_, err := w.svc.Reconcile(ctx)
	return err
}

// ReconcileEvery is how often the payout reconciliation runs.
const ReconcileEvery = 24 * time.Hour

// SweepEvery is how often the sweeper looks for payouts left behind (and prizes are paid).
const SweepEvery = 5 * time.Minute

// AddWorkers registers the payout workers.
func AddWorkers(ws *river.Workers, svc *Service) {
	river.AddWorker(ws, &sendWorker{svc: svc})
	river.AddWorker(ws, &verifyWorker{svc: svc})
	river.AddWorker(ws, &sweepWorker{svc: svc})
	river.AddWorker(ws, &prizesWorker{svc: svc})
	river.AddWorker(ws, &reconcileWorker{svc: svc})
}

// PeriodicJobs are the scheduled payout jobs.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(SweepEvery),
			func() (river.JobArgs, *river.InsertOpts) { return SweepArgs{}, nil }, &river.PeriodicJobOpts{ID: "payouts.sweep"}),
		river.NewPeriodicJob(river.PeriodicInterval(SweepEvery),
			func() (river.JobArgs, *river.InsertOpts) { return PrizesArgs{}, nil }, &river.PeriodicJobOpts{ID: "payouts.prizes", RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(ReconcileEvery),
			func() (river.JobArgs, *river.InsertOpts) { return ReconcileArgs{}, nil }, &river.PeriodicJobOpts{ID: "payouts.reconcile", RunOnStart: true}),
	}
}
