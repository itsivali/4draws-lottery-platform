package alerts

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/alerts/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// Kinds of alert about the platform's health and its duties to players.
const (
	KindJobsFailing     = "jobs.failing"
	KindDarajaThrottled = "daraja.throttled"
	KindKYCQueueStale   = "kyc.queue_stale"
	KindClosureWaiting  = "closure.waiting"
)

// River's tables are made by rivermigrate, outside the sqlc schema, so the job checks read them directly.
const (
	failingJobsSQL = `SELECT kind, state::text, count(*)::bigint, max(attempt)::int,
       (array_agg(coalesce(errors[array_length(errors, 1)]->>'error', '') ORDER BY id DESC))[1],
       (array_agg(id ORDER BY id DESC))[1:5]
FROM river.river_job
WHERE (state = 'discarded' AND finalized_at >= $1) OR (state = 'retryable' AND attempt >= $2)
GROUP BY kind, state ORDER BY kind, state`
	throttledSQL = `SELECT count(*)::bigint FROM river.river_job j, unnest(j.errors) AS e
WHERE j.kind IN ('payments.stk_verify', 'payments.stk_push') AND (e->>'at')::timestamptz >= $1 AND e->>'error' ILIKE '%throttl%'`
)

// JobsCheck pages for every job kind with a job discarded in the last day (River gave up: the work did not happen) and
// warns for one still retrying at the configured attempt or later; the alert clears when the jobs are retried, done
// or cleaned up. It also warns when Daraja's throttling shows in the recent errors of the STK jobs.
func JobsCheck(pool *pgxpool.Pool, clk clock.Clock, th Thresholds) Check {
	return Check{Name: "jobs", Kinds: []string{KindJobsFailing, KindDarajaThrottled}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		type failing struct {
			kind, state, lastError string
			count                  int64
			maxAttempt             int32
			ids                    []int64
		}
		perKind := map[string]*failing{}
		var order []string
		byState := map[string]map[string]int64{}
		rows, err := pool.Query(ctx, failingJobsSQL, now.Add(-Lookback), th.JobRetryWarnAttempts)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var f failing
			if err := rows.Scan(&f.kind, &f.state, &f.count, &f.maxAttempt, &f.lastError, &f.ids); err != nil {
				return nil, err
			}
			cur, ok := perKind[f.kind]
			if !ok {
				cur = &failing{kind: f.kind}
				perKind[f.kind] = cur
				order = append(order, f.kind)
				byState[f.kind] = map[string]int64{}
			}
			byState[f.kind][f.state] = f.count
			cur.count += f.count
			if f.maxAttempt > cur.maxAttempt {
				cur.maxAttempt = f.maxAttempt
			}
			if cur.lastError == "" || f.state == "discarded" {
				cur.lastError = f.lastError
			}
			cur.ids = append(cur.ids, f.ids...)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		var out []Alert
		for _, kind := range order {
			f := perKind[kind]
			discarded, retrying := byState[kind]["discarded"], byState[kind]["retryable"]
			sev, what := Warn, strconv.FormatInt(retrying, 10)+" "+kind+" job(s) are still failing after "+strconv.Itoa(int(f.maxAttempt))+" attempts"
			if discarded > 0 {
				sev = Page
				what = strconv.FormatInt(discarded, 10) + " " + kind + " job(s) were given up on (discarded)"
				if retrying > 0 {
					what += " and " + strconv.FormatInt(retrying, 10) + " more are still failing"
				}
			}
			out = append(out, Alert{Kind: KindJobsFailing, Subject: kind, Severity: sev, Summary: what + ": " + firstLine(f.lastError),
				Details: map[string]any{"kind": kind, "discarded": discarded, "retrying": retrying, "max_attempt": f.maxAttempt, "last_error": f.lastError, "job_ids": f.ids}})
		}
		var throttled int64
		if err := pool.QueryRow(ctx, throttledSQL, now.Add(-th.DarajaThrottleWindow)).Scan(&throttled); err != nil {
			return nil, err
		}
		if th.DarajaThrottleCount > 0 && throttled >= int64(th.DarajaThrottleCount) {
			out = append(out, Alert{Kind: KindDarajaThrottled, Subject: "stk", Severity: Warn,
				Summary: "Daraja throttled " + strconv.FormatInt(throttled, 10) + " STK requests in the last " + ages(th.DarajaThrottleWindow) + ": top-ups are slow to confirm",
				Details: map[string]any{"throttled_errors": throttled, "window": ages(th.DarajaThrottleWindow)}})
		}
		return out, nil
	}}
}

// firstLine keeps the first line of an error, which is what a summary has room for.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return "see the System page"
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// KYCQueueCheck warns when the oldest identity submission awaiting a decision has waited a day, and pages at three
// (the player cannot withdraw until someone decides). It clears when the queue is younger than that.
func KYCQueueCheck(pool *pgxpool.Pool, clk clock.Clock, th Thresholds) Check {
	return Check{Name: "kyc.queue", Kinds: []string{KindKYCQueueStale}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		q, err := store.New(pool).KYCReviewQueue(ctx)
		if err != nil {
			return nil, err
		}
		if q.Waiting == 0 {
			return nil, nil
		}
		age := now.Sub(q.Oldest)
		if age < th.KYCQueueWarn {
			return nil, nil
		}
		sev := Warn
		if age >= th.KYCQueuePage {
			sev = Page
		}
		return []Alert{{Kind: KindKYCQueueStale, Subject: "review", Severity: sev,
			Summary: strconv.FormatInt(q.Waiting, 10) + " identity submission(s) await a decision; the oldest has waited " + ages(age),
			Details: map[string]any{"waiting": q.Waiting, "oldest": q.Oldest, "age": ages(age)}}}, nil
	}}
}

// ClosuresCheck warns for every account closure request open longer than the configured wait.
func ClosuresCheck(pool *pgxpool.Pool, clk clock.Clock, th Thresholds) Check {
	return Check{Name: "closures", Kinds: []string{KindClosureWaiting}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		rows, err := store.New(pool).OpenClosureRequestsBefore(ctx, now.Add(-th.ClosureWaitWarn))
		if err != nil {
			return nil, err
		}
		out := make([]Alert, 0, len(rows))
		for _, r := range rows {
			age := now.Sub(r.RequestedAt)
			out = append(out, Alert{Kind: KindClosureWaiting, Subject: r.UserID, Severity: Warn,
				Summary: "A player asked to close their account " + ages(age) + " ago and nobody has: refund what is left and close it",
				Details: map[string]any{"user": r.UserID, "requested_at": r.RequestedAt, "age": ages(age)}})
		}
		return out, nil
	}}
}
