package draws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/draws/store"
	"github.com/itsivali/lottery-platform/internal/ledger"
)

// Report is one draw reconciliation run. Mismatches break an invariant between entries, hashes, certificates, results
// and the prize pool ledger; they must never happen (a bug or tampering: investigate). Each lists the draws concerned.
type Report struct {
	RanAt      time.Time           `json:"ran_at"`
	OK         bool                `json:"ok"`
	Mismatches map[string][]string `json:"mismatches"`
	Totals     map[string]int64    `json:"totals"`
}

func (r *Report) flag(name, drawID, detail string) {
	r.Mismatches[name] = append(r.Mismatches[name], drawID+": "+detail)
}

// Reconcile recomputes every fixed draw from what is stored, checks the prize pool ledger, stores the run and returns
// it. Beacons are verified again offline; nothing is fetched.
func (s *Service) Reconcile(ctx context.Context) (Report, error) {
	r := Report{Mismatches: map[string][]string{}, Totals: map[string]int64{}}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// one consistent snapshot for every check
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		r.RanAt = now
		fundings, err := q.FundingByDraw(ctx)
		if err != nil {
			return err
		}
		funded := map[string]store.FundingByDrawRow{}
		for _, f := range fundings {
			funded[f.DrawID] = f
		}
		ids, err := q.DrawsToReconcile(ctx)
		if err != nil {
			return err
		}
		approvedPools := map[string]int64{}
		seen := map[string]bool{}
		for _, id := range ids {
			seen[id] = true
			d, err := q.GetDraw(ctx, id)
			if err != nil {
				return err
			}
			pool, err := s.reconcileDraw(ctx, q, d, funded[id], &r)
			if err != nil {
				return fmt.Errorf("draw %s: %w", id, err)
			}
			if d.Status == "approved" || d.Status == "paid" {
				approvedPools[d.DrawType] += pool
			}
		}
		for id := range funded {
			if !seen[id] {
				r.flag("funding_mismatch", id, "a prize pool was funded for a draw that is open or voided")
			}
		}
		balances, err := q.PoolBalances(ctx)
		if err != nil {
			return err
		}
		out, err := q.PrizesOutByType(ctx)
		if err != nil {
			return err
		}
		paidOut := map[string]int64{}
		for _, o := range out {
			paidOut[o.DrawType] = o.GrossKes
		}
		for _, b := range balances {
			typ := strings.ToLower(strings.TrimPrefix(b.Code, "PRIZE_POOL_"))
			r.Totals[strings.ToLower(b.Code)+"_kes"] = b.BalanceKes
			// prize payouts (not failed) take prizes out of the pools
			if want := approvedPools[typ] - paidOut[typ]; b.BalanceKes != want {
				r.flag("pool_balance_mismatch", b.Code, "balance "+kes(b.BalanceKes)+" != pools of approved "+typ+" draws "+kes(approvedPools[typ])+
					" - prizes paid out "+kes(paidOut[typ]))
			}
		}
		r.Totals["draws_checked"] = int64(len(ids))
		r.OK = len(r.Mismatches) == 0
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return Report{}, err
	}
	if err := store.New(s.pool).InsertReconciliationRun(ctx, store.InsertReconciliationRunParams{RanAt: r.RanAt, Ok: r.OK, Report: raw}); err != nil {
		return Report{}, err
	}
	if !r.OK {
		slog.ErrorContext(ctx, "draw reconciliation found mismatches", "mismatches", r.Mismatches, "totals", r.Totals)
	} else {
		slog.InfoContext(ctx, "draw reconciliation clean", "totals", r.Totals)
	}
	return r, nil
}

// reconcileDraw checks one fixed draw and returns its certified pool (0 before it is conducted).
func (s *Service) reconcileDraw(ctx context.Context, q *store.Queries, d store.DrawsDraw, f store.FundingByDrawRow, r *Report) (int64, error) {
	entries, err := q.Entries(ctx, d.ID)
	if err != nil {
		return 0, err
	}
	r.Totals["entries_checked"] += int64(len(entries))
	h := sha256.New()
	users := make([]string, len(entries))
	for i, e := range entries {
		if int(e.EntryNo) != i+1 {
			r.flag("entry_numbering", d.ID, "entry "+strconv.Itoa(i+1)+" is numbered "+strconv.Itoa(int(e.EntryNo)))
			break
		}
		users[i] = e.UserID
		_, _ = fmt.Fprintf(h, "%d|%s|%s|%s\n", e.EntryNo, e.UserID, e.PlayDate.Format(time.DateOnly), e.TicketID)
	}
	if hex.EncodeToString(h.Sum(nil)) != deref(d.EntryListHash) {
		r.flag("entry_list_hash_mismatch", d.ID, "the entries do not hash to the frozen entry list hash")
	}
	approved := d.Status == "approved" || d.Status == "paid"
	if d.CertificateJson == nil {
		if f.Journals > 0 {
			r.flag("funding_mismatch", d.ID, "a prize pool was funded before the draw was conducted")
		}
		return 0, nil
	}

	if sum := sha256.Sum256(d.CertificateJson); hex.EncodeToString(sum[:]) != deref(d.CertificateHash) {
		r.flag("certificate_hash_mismatch", d.ID, "the stored certificate does not hash to certificate_hash")
	}
	var cert Certificate
	if err := json.Unmarshal(d.CertificateJson, &cert); err != nil {
		r.flag("certificate_unreadable", d.ID, err.Error())
		return 0, nil
	}
	seed, err := hex.DecodeString(cert.RevealedSeed)
	if c := sha256.Sum256(seed); err != nil || hex.EncodeToString(c[:]) != deref(d.SeedCommit) {
		r.flag("seed_commit_mismatch", d.ID, "the revealed seed is not the one committed to at freeze")
	}
	if cert.DrawID != d.ID || cert.DrawType != d.DrawType || cert.EntryListHash != deref(d.EntryListHash) || cert.SeedCommit != deref(d.SeedCommit) ||
		cert.RevealedSeed != deref(d.RevealedSeed) || int64(cert.Beacon.Round) != derefInt(d.BeaconRound) || //nolint:gosec // rounds stay far below 2^63
		cert.Beacon.Randomness != deref(d.BeaconValue) || cert.Entries != len(entries) {
		r.flag("certificate_vs_draw", d.ID, "the certificate disagrees with the draw or its entries")
	}
	if err := s.beacon.Verify(cert.Beacon); err != nil {
		r.flag("beacon_invalid", d.ID, err.Error())
	}

	wins, err := s.recompute(ctx, q, d, cert, seed, users)
	switch {
	case errors.Is(err, errNotReproducible):
		r.flag("winners_not_reproducible", d.ID, err.Error())
	case err != nil:
		return 0, err
	case !reflect.DeepEqual(wins, cert.Prizes):
		r.flag("winners_not_reproducible", d.ID, "the winners recomputed from the certificate differ from the certified ones")
	}

	results, err := q.Results(ctx, d.ID)
	if err != nil {
		return 0, err
	}
	var paid int64
	same := len(results) == len(cert.Prizes)
	for i, res := range results {
		paid += res.AmountKes
		if same {
			w := cert.Prizes[i]
			same = int(res.PrizeRank) == w.Rank && res.AmountKes == w.AmountKES && int(res.EntryNo) == w.EntryNo && res.UserID == w.UserID
		}
	}
	if !same {
		r.flag("results_vs_certificate", d.ID, "the result rows differ from the certified prizes")
	}
	if paid > cert.PoolKES {
		r.flag("prizes_exceed_pool", d.ID, "prizes "+kes(paid)+" > pool "+kes(cert.PoolKES))
	}

	switch want := cert.PoolKES > 0 && approved; {
	case want && (f.Journals != 1 || f.AmountKes != cert.PoolKES):
		r.flag("funding_mismatch", d.ID, "approved with pool "+kes(cert.PoolKES)+" but funded "+kes(f.AmountKes)+" in "+strconv.Itoa(int(f.Journals))+" journals")
	case !want && f.Journals > 0:
		r.flag("funding_mismatch", d.ID, "a prize pool was funded for a draw that is not approved")
	}
	return cert.PoolKES, nil
}

func kes(n int64) string { return strconv.FormatInt(n, 10) }

var errNotReproducible = errors.New("draws: not reproducible")

// recompute derives the pool and the winners again from the certificate, the stored entries and the prize structure.
func (s *Service) recompute(ctx context.Context, q *store.Queries, d store.DrawsDraw, cert Certificate, seed []byte, users []string) ([]CertifiedWin, error) {
	ps, err := q.PrizeStructureFor(ctx, store.PrizeStructureForParams{DrawType: d.DrawType, OnDate: d.PeriodEnd})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: no prize structure in force", errNotReproducible)
	} else if err != nil {
		return nil, err
	}
	if ps.PoolShare != cert.PoolShare || ps.Rounding != cert.Rounding {
		return nil, fmt.Errorf("%w: prize structure %s %s, certificate %s %s", errNotReproducible, ps.PoolShare, ps.Rounding, cert.PoolShare, cert.Rounding)
	}
	mode := ledger.Rounding(cert.Rounding)
	rate, err := ledger.ParseRate(cert.PoolShare)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNotReproducible, err)
	}
	if pool, err := ledger.Apply(cert.StakesKES, rate, mode); err != nil || pool != cert.PoolKES {
		return nil, fmt.Errorf("%w: pool %d is not %s of stakes %d", errNotReproducible, cert.PoolKES, cert.PoolShare, cert.StakesKES)
	}
	var ranks []RankShare
	if err := json.Unmarshal(ps.Prizes, &ranks); err != nil {
		return nil, err
	}
	randomness, err := hex.DecodeString(cert.Beacon.Randomness)
	if err != nil || len(users) == 0 {
		return nil, fmt.Errorf("%w: no randomness or no entries", errNotReproducible)
	}
	wins, err := Award(Key(randomness, seed, cert.EntryListHash, d.ID), users, ranks, cert.PoolKES, mode)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNotReproducible, err)
	}
	return wins, nil
}
