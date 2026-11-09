//go:build integration

package aml_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/aml"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/ledger"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
	"github.com/itsivali/lottery-platform/internal/platform/testpg"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

type env struct {
	pool *pgxpool.Pool
	svc  *aml.Service
}

func setup(t *testing.T) *env {
	t.Helper()
	pool := testpg.Pool(t, "app_worker")
	return &env{pool: pool, svc: aml.NewService(pool, clock.Postgres{}, audit.NewWriter())}
}

var seq atomic.Int64

func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (e *env) player(t *testing.T) string {
	t.Helper()
	n := seq.Add(1)
	var id string
	if err := e.pool.QueryRow(context.Background(), `INSERT INTO identity.users (msisdn, password_hash) VALUES ($1, 'x') RETURNING id::text`,
		fmt.Sprintf("2547%08d", 70_000_000+n)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *env) topUps(t *testing.T, user, status string, n int, amount int64) {
	t.Helper()
	for range n {
		k := seq.Add(1)
		var confirmed *int64
		if status == "credited" {
			confirmed = &amount
		}
		var receipt *string
		if status == "credited" {
			r := fmt.Sprintf("AML%07d", k)
			receipt = &r
		}
		e.exec(t, `INSERT INTO payments.payments (user_id, channel, status, amount_requested_kes, amount_confirmed_kes, mpesa_receipt, msisdn, idempotency_key)
			VALUES ($1, 'stk', $2, $3, $4, $5, '254700000000', $6)`, user, status, amount, confirmed, receipt, fmt.Sprint("aml-", k))
	}
}

// deposit credits the wallet as a credited top-up would (the deposit and the excise).
func (e *env) deposit(t *testing.T, user string, gross int64) {
	t.Helper()
	ctx := context.Background()
	ls := ledger.NewStore()
	if err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `SELECT platform.uuid_v7()::text`).Scan(&id); err != nil {
			return err
		}
		rule, err := ls.ActiveRule(ctx, tx, "deposit_excise", testpgNow(ctx, tx))
		if err != nil {
			return err
		}
		p, err := ledger.Deposit(id, user, gross, rule, true)
		if err != nil {
			return err
		}
		_, err = wallet.NewService(ls).Apply(ctx, tx, p)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func testpgNow(ctx context.Context, tx pgx.Tx) (now time.Time) {
	_ = tx.QueryRow(ctx, `SELECT now()`).Scan(&now)
	return now
}

func (e *env) cases(t *testing.T, view string) map[string][]string {
	t.Helper()
	list, _, err := e.svc.Cases(context.Background(), view, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for _, c := range list {
		out[string(c.Rule)] = append(out[string(c.Rule)], c.UserID)
	}
	return out
}

// ADR 0033: each rule finds the players it describes and opens one case per player and rule; running again opens
// nothing new, and a closed case keeps its player quiet for the rule's quiet period.
func TestScan_EachRuleOpensOneCasePerPlayerAndKeepsQuietAfterClosing(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	fast := e.player(t) // 11 top-ups today: more than 10
	e.topUps(t, fast, "credited", 11, 1_000)
	probing := e.player(t) // 5 failed top-ups
	e.topUps(t, probing, "failed", 5, 100)
	quiet := e.player(t) // ordinary
	e.topUps(t, quiet, "credited", 2, 500)

	inOut := e.player(t) // KES 10,000 in, KES 6,000 out, nothing played
	e.deposit(t, inOut, 10_000)
	e.exec(t, `INSERT INTO payouts.payouts (user_id, kind, status, gross_kes, net_kes, msisdn, originator_conversation_id, idempotency_key)
		VALUES ($1, 'withdrawal', 'pending', 6000, 6000, '254700000001', 'aml-orig-1', 'aml-w-1')`, inOut)

	idA, idB := e.player(t), e.player(t) // the same ID number declared twice
	for _, u := range []string{idA, idB} {
		e.exec(t, `INSERT INTO kyc.declared_identities (user_id, first_name_enc, second_name_enc, id_number_enc, id_number_hash) VALUES ($1, 'x', 'x', 'x', 'same-id')`, u)
	}
	dev := []string{e.player(t), e.player(t), e.player(t)} // one phone, three accounts
	for _, u := range dev {
		e.exec(t, `INSERT INTO identity.devices (user_id, fingerprint_hash, platform) VALUES ($1, 'one-phone', 'android')`, u)
	}

	n, err := e.svc.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := e.cases(t, "open")
	want := map[string]int{"deposit_velocity": 1, "failed_topups": 1, "in_out_little_play": 1, "shared_id_number": 2, "shared_device": 3}
	total := 0
	for rule, k := range want {
		if len(got[rule]) != k {
			t.Errorf("%s: %v, want %d players", rule, got[rule], k)
		}
		total += k
	}
	if n != total {
		t.Errorf("opened %d, want %d", n, total)
	}
	for _, users := range got {
		for _, u := range users {
			if u == quiet {
				t.Errorf("an ordinary player got a case")
			}
		}
	}
	if got["deposit_velocity"][0] != fast || got["failed_topups"][0] != probing || got["in_out_little_play"][0] != inOut {
		t.Errorf("cases went to the wrong players: %v", got)
	}

	// the evidence says what the rule saw, and that the thresholds await approval
	list, _, _ := e.svc.Cases(ctx, "open", 200, "")
	for _, c := range list {
		if c.Rule != aml.DepositVelocity {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal(c.Evidence, &ev); err != nil || ev["deposits"] != float64(11) || ev["thresholds_pending_approval"] != true {
			t.Errorf("evidence %s (%v)", c.Evidence, err)
		}
		// closed: the next scans leave the player alone for the quiet period
		if _, err := e.svc.Close(ctx, c.ID, "staff:alice", "salary day; no concern"); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := e.svc.Scan(ctx); err != nil || n != 0 {
		t.Fatalf("second scan opened %d (%v), want 0", n, err)
	}
	if len(e.cases(t, "closed")["deposit_velocity"]) != 1 {
		t.Fatal("the closed case is not listed as closed")
	}

	// a rule turned off raises nothing; settings are validated and recorded, pending until approved
	if err := e.svc.SetRule(ctx, aml.FailedTopUps, true, aml.Params{WindowHours: 24}, "", "staff:alice", "x"); !errors.Is(err, aml.ErrInvalid) {
		t.Fatalf("incomplete params: %v", err)
	}
	if err := e.svc.SetRule(ctx, aml.SharedDevice, false, aml.Defaults[aml.SharedDevice], "", "staff:alice", "families share phones"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetRule(ctx, aml.DepositVelocity, true, aml.Params{WindowHours: 24, MaxKES: 50_000, MaxCount: 20, QuietDays: 14}, "CO-2026-11", "staff:alice", "approved by the compliance owner"); err != nil {
		t.Fatal(err)
	}
	settings, err := e.svc.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range settings {
		switch s.Rule {
		case aml.SharedDevice:
			if s.Enabled || !s.Pending || s.Default {
				t.Errorf("shared device: %+v", s)
			}
		case aml.DepositVelocity:
			if !s.Enabled || s.Pending || s.Approval != "CO-2026-11" || s.Params.MaxKES != 50_000 {
				t.Errorf("deposit velocity: %+v", s)
			}
		case aml.FailedTopUps:
			if !s.Default || !s.Pending {
				t.Errorf("failed top-ups: %+v", s)
			}
		}
	}
}

// ADR 0033: a hold is placed by one staff member and lifted (released, or the case closed) by another; a closed case
// is final, and what a case is about never changes.
func TestCase_AHoldIsLiftedBySomeoneElseAndAClosedCaseIsFinal(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	u := e.player(t)
	if _, err := e.svc.OpenManual(ctx, u, "staff:alice", " "); !errors.Is(err, aml.ErrInvalid) {
		t.Fatalf("without a note: %v", err)
	}
	if _, err := e.svc.OpenManual(ctx, "not-a-player", "staff:alice", "tip-off from the bank"); !errors.Is(err, aml.ErrNotFound) {
		t.Fatalf("an unknown player: %v", err)
	}
	c, err := e.svc.OpenManual(ctx, u, "staff:alice", "tip-off from the bank")
	if err != nil || c.Rule != aml.Manual || c.OpenedBy != "staff:alice" || len(c.Events) != 1 {
		t.Fatalf("open: %+v %v", c, err)
	}
	if _, err := e.svc.OpenManual(ctx, u, "staff:bob", "again"); !errors.Is(err, aml.ErrOpen) {
		t.Fatalf("a second open manual case: %v", err)
	}
	held := func() bool {
		var on bool
		_ = pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error { on, err = e.svc.OnHold(ctx, tx, u); return err })
		return on
	}
	if held() {
		t.Fatal("held before anyone held it")
	}
	if c, err = e.svc.Hold(ctx, c.ID, "staff:alice", "until the bank answers"); err != nil || !c.Hold || !held() {
		t.Fatalf("hold: %+v %v", c, err)
	}
	if _, err := e.svc.Hold(ctx, c.ID, "staff:bob", "again"); !errors.Is(err, aml.ErrHoldState) {
		t.Fatalf("held twice: %v", err)
	}
	if _, err := e.svc.Release(ctx, c.ID, "staff:alice", "my own hold"); !errors.Is(err, aml.ErrSamePerson) {
		t.Fatalf("released by who placed it: %v", err)
	}
	if _, err := e.svc.Close(ctx, c.ID, "staff:alice", "closing my own hold"); !errors.Is(err, aml.ErrSamePerson) {
		t.Fatalf("closed by who placed the hold: %v", err)
	}
	if _, err := e.svc.Note(ctx, c.ID, "staff:bob", "bank says the funds are clean"); err != nil {
		t.Fatal(err)
	}
	if c, err = e.svc.Close(ctx, c.ID, "staff:bob", "no concern; not reported"); err != nil || c.Status != "closed" || c.Hold || held() {
		t.Fatalf("close: %+v %v", c, err)
	}
	if len(c.Events) != 4 {
		t.Fatalf("events %+v", c.Events)
	}
	if _, err := e.svc.Note(ctx, c.ID, "staff:bob", "afterthought"); !errors.Is(err, aml.ErrClosed) {
		t.Fatalf("a note on a closed case: %v", err)
	}
	for name, sql := range map[string]string{
		"reopen":          `UPDATE compliance.aml_cases SET status = 'open', outcome = NULL, closed_at = NULL, closed_by = NULL WHERE id = $1`,
		"rewrite events":  `UPDATE compliance.aml_case_events SET note = 'x' WHERE case_id = $1`,
		"delete the case": `DELETE FROM compliance.aml_cases WHERE id = $1`,
	} {
		if _, err := e.pool.Exec(ctx, sql, c.ID); err == nil {
			t.Errorf("%s: allowed", name)
		}
	}
}
