//go:build integration

package oversight_test

import (
	"context"
	"errors"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/oversight"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/db"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/testpg"
)

type noopArgs struct{}

func (noopArgs) Kind() string { return "test.noop" }

func TestJobControl_RetriesAndCancelsOnlyFromTheRightStatesAndAuditsBoth(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t, "app_api")
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Schema: db.RiverSchema})
	if err != nil {
		t.Fatal(err)
	}
	jc := oversight.NewJobControl(pool, client, audit.NewWriter())
	const ada = "staff:0192f000-0000-7000-8000-00000000000a"
	state := func(id int64) string {
		var s string
		if err := pool.QueryRow(ctx, `SELECT state::text FROM river.river_job WHERE id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	res, err := client.Insert(ctx, noopArgs{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Job.ID
	if err := jc.Retry(ctx, id, ada); !errors.Is(err, oversight.ErrJobState) {
		t.Fatalf("retry a waiting job: %v", err)
	}
	if err := jc.Cancel(ctx, id, ada); err != nil || state(id) != "cancelled" {
		t.Fatalf("cancel: %v %s", err, state(id))
	}
	if err := jc.Cancel(ctx, id, ada); !errors.Is(err, oversight.ErrJobState) {
		t.Fatalf("cancel twice: %v", err)
	}
	if err := jc.Retry(ctx, id, ada); err != nil || state(id) != "available" {
		t.Fatalf("retry a cancelled job: %v %s", err, state(id))
	}
	// a completed job is never run again
	if _, err := pool.Exec(ctx, `UPDATE river.river_job SET state = 'completed', finalized_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := jc.Retry(ctx, id, ada); !errors.Is(err, oversight.ErrJobState) {
		t.Fatalf("retry a completed job: %v", err)
	}
	if err := jc.Cancel(ctx, 1<<40, ada); !errors.Is(err, oversight.ErrJobNotFound) {
		t.Fatalf("unknown job: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE entity_type = 'job' AND actor_id = $1`, ada).Scan(&n); err != nil || n != 2 {
		t.Errorf("audited %d %v, want 2 (the cancel and the retry)", n, err)
	}
}
