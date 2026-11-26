package alerts

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/alerts/store"
	"github.com/itsivali/4draws-lottery-platform/internal/draws"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// Kinds of alert the checks raise.
const (
	KindDrawOverdue          = "draw.overdue"
	KindReconciliationFailed = "reconciliation.failed"
	KindReconciliationStale  = "reconciliation.missing"
	KindRemittanceDue        = "kra.remittance_due"
	KindRemittanceOverdue    = "kra.remittance_overdue"
)

// DrawsOverdue pages for every draw whose next step is late (draws.Service.Overdue); each clears when the draw moves on.
func DrawsOverdue(overdue func(ctx context.Context) ([]draws.Late, error)) Check {
	return Check{Name: "draws.overdue", Kinds: []string{KindDrawOverdue}, Run: func(ctx context.Context) ([]Alert, error) {
		late, err := overdue(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]Alert, 0, len(late))
		for _, l := range late {
			out = append(out, Alert{Kind: KindDrawOverdue, Subject: l.DrawID + "/" + l.Status, Severity: Page,
				Summary: "The " + l.DrawType + " draw of " + l.PeriodStart + " (" + l.Status + ", due " + l.Due.UTC().Format(time.RFC3339) + "): " + l.Problem,
				Details: map[string]any{"draw": l.DrawID, "type": l.DrawType, "period_start": l.PeriodStart, "status": l.Status, "due": l.Due}})
		}
		return out, nil
	}}
}

// Reconciliations are the daily reconciliations the watch follows, by source.
var Reconciliations = []string{"payments", "game", "draws", "payouts"}

// ReconciliationCheck pages when the latest run of a daily reconciliation found mismatches, which also freezes payouts
// until a person acknowledges it (each failed run is its own alert: a later clean run does not close it), and when a
// reconciliation has not run within staleAfter (cleared by its next run).
func ReconciliationCheck(pool *pgxpool.Pool, clk clock.Clock, staleAfter time.Duration) Check {
	return Check{Name: "reconciliations", Kinds: []string{KindReconciliationFailed, KindReconciliationStale}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		runs, err := store.New(pool).LatestReconciliations(ctx)
		if err != nil {
			return nil, err
		}
		latest := map[string]store.LatestReconciliationsRow{}
		for _, r := range runs {
			latest[r.Source] = r
		}
		var out []Alert
		for _, src := range Reconciliations {
			r, ok := latest[src]
			switch {
			case !ok:
				out = append(out, Alert{Kind: KindReconciliationStale, Subject: src, Severity: Page,
					Summary: "The " + src + " reconciliation has never run: is the worker running?"})
				continue
			case r.RanAt.Before(now.Add(-staleAfter)):
				out = append(out, Alert{Kind: KindReconciliationStale, Subject: src, Severity: Page,
					Summary: "The " + src + " reconciliation last ran " + r.RanAt.UTC().Format(time.RFC3339) + ": is the worker running?",
					Details: map[string]any{"last_run": r.ID, "ran_at": r.RanAt}})
			}
			if !r.Ok {
				names := mismatchNames(r.Report)
				out = append(out, Alert{Kind: KindReconciliationFailed, Subject: src + "/" + r.ID, Severity: Page, FreezesPayouts: true,
					Summary: "The " + src + " reconciliation found mismatches (" + strings.Join(names, ", ") + "); payouts are frozen",
					Details: map[string]any{"run": r.ID, "ran_at": r.RanAt, "mismatches": names}})
			}
		}
		return out, nil
	}}
}

// mismatchNames lists the invariants a reconciliation report says broke.
func mismatchNames(report []byte) []string {
	var r struct {
		Mismatches map[string]json.RawMessage `json:"mismatches"`
	}
	names := []string{}
	if json.Unmarshal(report, &r) == nil {
		for k := range r.Mismatches {
			names = append(names, k)
		}
	}
	if len(names) == 0 {
		names = append(names, "see the run's report")
	}
	sort.Strings(names)
	return names
}

// RemittanceCheck follows withheld tax not yet remitted to KRA (in no batch, or in a batch without a remittance): it
// warns once the oldest is warnAfter old and pages once it is dueAfter old. It clears when the tax is remitted.
func RemittanceCheck(pool *pgxpool.Pool, clk clock.Clock, warnAfter, dueAfter time.Duration) Check {
	return Check{Name: "kra.remittance", Kinds: []string{KindRemittanceDue, KindRemittanceOverdue}, Run: func(ctx context.Context) ([]Alert, error) {
		now, err := clk.Now(ctx, pool)
		if err != nil {
			return nil, err
		}
		rows, err := store.New(pool).UnremittedWithholding(ctx)
		if err != nil || len(rows) == 0 {
			return nil, err
		}
		var tax int64
		unbatched, batches := 0, map[int64]bool{}
		for _, r := range rows {
			tax += r.TaxKes
			if r.BatchNo == nil {
				unbatched++
			} else {
				batches[*r.BatchNo] = true
			}
		}
		oldest := rows[0].CreatedAt
		age := now.Sub(oldest)
		what := strconv.Itoa(len(rows)) + " withholding events (KES " + strconv.FormatInt(tax, 10) + ") are not remitted to KRA, the oldest from " +
			oldest.UTC().Format(time.RFC3339) + " (" + strconv.Itoa(unbatched) + " not yet in a batch, " + strconv.Itoa(len(batches)) + " batches awaiting remittance)"
		details := map[string]any{"events": len(rows), "tax_kes": tax, "oldest": oldest, "unbatched": unbatched, "batches_awaiting": len(batches)}
		switch {
		case age >= dueAfter:
			return []Alert{{Kind: KindRemittanceOverdue, Subject: "withholding", Severity: Page, Summary: what + ": past the remittance deadline", Details: details}}, nil
		case age >= warnAfter:
			return []Alert{{Kind: KindRemittanceDue, Subject: "withholding", Severity: Warn, Summary: what + ": remit before the deadline", Details: details}}, nil
		}
		return nil, nil
	}}
}
