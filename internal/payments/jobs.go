package payments

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// PushArgs sends the STK prompt for a payment.
type PushArgs struct {
	PaymentID string `json:"payment_id"`
}

// Kind implements river.JobArgs.
func (PushArgs) Kind() string { return "payments.stk_push" }

// VerifyArgs settles a payment from the status query. A payment has one verify job at a time, whatever asked for it:
// before 2026-10-30 the push, the callback and the sweep each ran their own, tripling the status queries Daraja
// throttles.
type VerifyArgs struct {
	PaymentID string       `json:"payment_id" river:"unique"`
	Source    VerifySource `json:"source"`
}

// Kind implements river.JobArgs.
func (VerifyArgs) Kind() string { return "payments.stk_verify" }

// C2BVerifyArgs verifies and settles a paybill confirmation.
type C2BVerifyArgs struct {
	ConfirmationID string       `json:"confirmation_id"`
	Source         VerifySource `json:"source"`
}

// Kind implements river.JobArgs.
func (C2BVerifyArgs) Kind() string { return "payments.c2b_verify" }

// ClaimCheckArgs asks M-Pesa about the receipt a player claimed for an uncredited top-up. One job per payment.
type ClaimCheckArgs struct {
	PaymentID string `json:"payment_id" river:"unique"`
}

// Kind implements river.JobArgs.
func (ClaimCheckArgs) Kind() string { return "payments.claim_check" }

// ReconcileArgs runs the daily internal reconciliation.
type ReconcileArgs struct{}

// Kind implements river.JobArgs.
func (ReconcileArgs) Kind() string { return "payments.reconcile" }

// SweepArgs looks for payments that have been pending too long.
type SweepArgs struct{}

// Kind implements river.JobArgs.
func (SweepArgs) Kind() string { return "payments.sweep" }

const verifyMaxAttempts = 20

// RiverEnqueuer enqueues payment jobs inside the caller's transaction.
type RiverEnqueuer struct{ client *river.Client[pgx.Tx] }

// NewRiverEnqueuer returns an Enqueuer on an (insert-only is enough) River client.
func NewRiverEnqueuer(c *river.Client[pgx.Tx]) *RiverEnqueuer { return &RiverEnqueuer{client: c} }

// EnqueuePush implements Enqueuer. The push job runs exactly once (see Service.Push).
func (e *RiverEnqueuer) EnqueuePush(ctx context.Context, tx pgx.Tx, paymentID string) error {
	_, err := e.client.InsertTx(ctx, tx, PushArgs{PaymentID: paymentID}, &river.InsertOpts{MaxAttempts: 1})
	return err
}

// EnqueueVerify implements Enqueuer. A callback brings forward the payment's verify job that is already waiting
// (snoozed or backing off), so the customer is not kept waiting for its next turn.
func (e *RiverEnqueuer) EnqueueVerify(ctx context.Context, tx pgx.Tx, paymentID string, source VerifySource) error {
	res, err := e.client.InsertTx(ctx, tx, VerifyArgs{PaymentID: paymentID, Source: source},
		&river.InsertOpts{MaxAttempts: verifyMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	if err != nil || source != AfterCallback || !res.UniqueSkippedAsDuplicate {
		return err
	}
	switch res.Job.State {
	case rivertype.JobStateScheduled, rivertype.JobStateRetryable:
		_, err = e.client.JobRetryTx(ctx, tx, res.Job.ID)
	}
	return err
}

// EnqueueC2B implements Enqueuer.
func (e *RiverEnqueuer) EnqueueC2B(ctx context.Context, tx pgx.Tx, confirmationID string, source VerifySource) error {
	_, err := e.client.InsertTx(ctx, tx, C2BVerifyArgs{ConfirmationID: confirmationID, Source: source},
		&river.InsertOpts{MaxAttempts: verifyMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// EnqueueClaimCheck implements Enqueuer.
func (e *RiverEnqueuer) EnqueueClaimCheck(ctx context.Context, tx pgx.Tx, paymentID string) error {
	_, err := e.client.InsertTx(ctx, tx, ClaimCheckArgs{PaymentID: paymentID},
		&river.InsertOpts{MaxAttempts: verifyMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

type claimWorker struct {
	river.WorkerDefaults[ClaimCheckArgs]
	svc *Service
}

func (w *claimWorker) Work(ctx context.Context, j *river.Job[ClaimCheckArgs]) error {
	d, err := w.svc.CheckClaim(ctx, j.Args.PaymentID)
	if err != nil {
		return err
	}
	if d > 0 {
		return river.JobSnooze(d)
	}
	return nil
}

type pushWorker struct {
	river.WorkerDefaults[PushArgs]
	svc *Service
}

func (w *pushWorker) Work(ctx context.Context, j *river.Job[PushArgs]) error {
	return w.svc.Push(ctx, j.Args.PaymentID)
}

type verifyWorker struct {
	river.WorkerDefaults[VerifyArgs]
	svc *Service
}

func (w *verifyWorker) Work(ctx context.Context, j *river.Job[VerifyArgs]) error {
	d, err := w.svc.Verify(ctx, j.Args.PaymentID)
	if err != nil {
		return err
	}
	if d > 0 {
		return river.JobSnooze(d)
	}
	return nil
}

type c2bWorker struct {
	river.WorkerDefaults[C2BVerifyArgs]
	svc *Service
}

func (w *c2bWorker) Work(ctx context.Context, j *river.Job[C2BVerifyArgs]) error {
	d, err := w.svc.VerifyC2B(ctx, j.Args.ConfirmationID)
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

type reconcileWorker struct {
	river.WorkerDefaults[ReconcileArgs]
	svc *Service
}

func (w *reconcileWorker) Work(ctx context.Context, _ *river.Job[ReconcileArgs]) error {
	_, err := w.svc.Reconcile(ctx)
	return err
}

// SweepEvery is how often the sweeper looks for stuck payments.
const SweepEvery = time.Minute

// AddWorkers registers the payment job workers.
func AddWorkers(ws *river.Workers, svc *Service) {
	river.AddWorker(ws, &pushWorker{svc: svc})
	river.AddWorker(ws, &verifyWorker{svc: svc})
	river.AddWorker(ws, &c2bWorker{svc: svc})
	river.AddWorker(ws, &claimWorker{svc: svc})
	river.AddWorker(ws, &sweepWorker{svc: svc})
	river.AddWorker(ws, &reconcileWorker{svc: svc})
}

// ReconcileEvery is how often the internal reconciliation runs.
const ReconcileEvery = 24 * time.Hour

// PeriodicJobs are the scheduled payment jobs.
func PeriodicJobs() []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(SweepEvery),
			func() (river.JobArgs, *river.InsertOpts) { return SweepArgs{}, nil }, &river.PeriodicJobOpts{ID: "payments.sweep"}),
		river.NewPeriodicJob(river.PeriodicInterval(ReconcileEvery),
			func() (river.JobArgs, *river.InsertOpts) { return ReconcileArgs{}, nil }, &river.PeriodicJobOpts{ID: "payments.reconcile", RunOnStart: true}),
	}
}
