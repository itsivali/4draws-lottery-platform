package oversight

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/itsivali/lottery-platform/internal/audit"
)

var (
	// ErrJobNotFound is an unknown job.
	ErrJobNotFound = errors.New("oversight: no such job")
	// ErrJobState is a job in a state that does not allow the action (a completed job is never run again).
	ErrJobState = errors.New("oversight: the job is not in a state that allows this")
)

// JobControl retries or cancels a background job from the back office (ADR 0036), audited. Every job is safe to run
// again (each checks its record's state before acting: a payout is sent only while pending), and a cancelled one's
// work is picked up by its area's sweep.
type JobControl struct {
	pool  *pgxpool.Pool
	river *river.Client[pgx.Tx]
	audit *audit.Writer
}

// NewJobControl returns a JobControl over client (an insert-only client is enough).
func NewJobControl(pool *pgxpool.Pool, client *river.Client[pgx.Tx], aw *audit.Writer) *JobControl {
	return &JobControl{pool: pool, river: client, audit: aw}
}

// retryable and cancellable are the states each action is allowed from.
var (
	retryable   = map[string]bool{"retryable": true, "discarded": true, "cancelled": true}
	cancellable = map[string]bool{"retryable": true, "scheduled": true, "available": true}
)

// Retry runs a failing, discarded or cancelled job again now.
func (j *JobControl) Retry(ctx context.Context, id int64, staff string) error {
	return j.act(ctx, id, staff, "retry", retryable, func(tx pgx.Tx) error { _, err := j.river.JobRetryTx(ctx, tx, id); return err })
}

// Cancel stops a job that is waiting or retrying.
func (j *JobControl) Cancel(ctx context.Context, id int64, staff string) error {
	return j.act(ctx, id, staff, "cancel", cancellable, func(tx pgx.Tx) error { _, err := j.river.JobCancelTx(ctx, tx, id); return err })
}

func (j *JobControl) act(ctx context.Context, id int64, staff, action string, allowed map[string]bool, do func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, j.pool, func(tx pgx.Tx) error {
		var kind, state string
		err := tx.QueryRow(ctx, `SELECT kind, state::text FROM river.river_job WHERE id = $1 FOR UPDATE`, id).Scan(&kind, &state)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJobNotFound
		} else if err != nil {
			return err
		}
		if !allowed[state] {
			return ErrJobState
		}
		if err := do(tx); err != nil {
			if errors.Is(err, river.ErrNotFound) {
				return ErrJobNotFound
			}
			return err
		}
		return j.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: staff, Action: "system.job_" + action, EntityType: "job",
			EntityID: strconv.FormatInt(id, 10), Details: map[string]any{"kind": kind, "was": state}})
	})
}
