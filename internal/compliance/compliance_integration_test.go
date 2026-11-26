//go:build integration

package compliance_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/compliance"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/testpg"
)

const (
	alice   = "staff:0192f000-0000-7000-8000-00000000000a"
	bob     = "staff:0192f000-0000-7000-8000-00000000000b"
	licence = "GRA/OPL/0001"
)

var evidence = strings.Repeat("ab", 32)

type env struct {
	pool *pgxpool.Pool
	clk  *clock.Manual
}

func setup(t *testing.T) *env {
	t.Helper()
	return &env{pool: testpg.Pool(t, "app_worker"), clk: clock.NewManual(time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC))}
}

func (e *env) gate(mode compliance.Mode, licence string) *compliance.Gate {
	return compliance.NewGate(e.pool, e.clk, audit.NewWriter(), compliance.Config{Mode: mode, LicenceReference: licence})
}

func (e *env) assert(t *testing.T, g *compliance.Gate) error {
	t.Helper()
	return pgx.BeginFunc(context.Background(), e.pool, func(tx pgx.Tx) error { return g.Assert(context.Background(), tx, compliance.OpPlay) })
}

func (e *env) mode(t *testing.T, g *compliance.Gate) compliance.Mode {
	t.Helper()
	st, err := g.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode
}

func liveRequest() compliance.LiveRequest {
	return compliance.LiveRequest{LicenceReference: licence, ApprovalReference: "GRA/APPR/77", EvidenceSHA256: evidence,
		EvidenceLocation: "operator vault, folder licensing/2026", Reason: "licence granted 2026-11-01"}
}

func TestGate_TheConfigurationIsTheMostAndNothingRecordedMeansSandbox(t *testing.T) {
	e := setup(t)
	if m := e.mode(t, e.gate(compliance.Disabled, "")); m != compliance.Disabled {
		t.Errorf("configured disabled: %s", m)
	}
	if err := e.assert(t, e.gate(compliance.Disabled, "")); !errors.Is(err, compliance.ErrOperationsStopped) {
		t.Errorf("configured disabled: assert = %v, want stopped", err)
	}
	if m := e.mode(t, e.gate(compliance.Sandbox, "")); m != compliance.Sandbox {
		t.Errorf("configured sandbox: %s", m)
	}
	if err := e.assert(t, e.gate(compliance.Sandbox, "")); err != nil {
		t.Errorf("configured sandbox: assert = %v", err)
	}
	// production credentials and a licence in the configuration are not enough: nothing is approved, nothing moves
	if m := e.mode(t, e.gate(compliance.Live, licence)); m != compliance.Disabled {
		t.Errorf("configured live without approval: %s, want disabled", m)
	}
	if m := e.mode(t, e.gate("", "")); m != compliance.Disabled {
		t.Errorf("unset: %s, want disabled", m)
	}
}

func TestGate_AStopRefusesEveryOperationUntilAnAdminResumes(t *testing.T) {
	e := setup(t)
	g := e.gate(compliance.Sandbox, "")
	ctx := context.Background()
	if _, err := g.Stop(ctx, alice, " "); !errors.Is(err, compliance.ErrInvalid) {
		t.Fatalf("stop without a reason: %v", err)
	}
	if _, err := g.Stop(ctx, "operator:alice", "fraud suspected"); !errors.Is(err, compliance.ErrInvalid) {
		t.Fatalf("stop by a name, not a staff id: %v", err)
	}
	c, err := g.Stop(ctx, alice, "fraud suspected on paybill")
	if err != nil || c.Mode != compliance.Disabled {
		t.Fatalf("stop: %+v %v", c, err)
	}
	if err := e.assert(t, g); !errors.Is(err, compliance.ErrOperationsStopped) {
		t.Fatalf("after stop: assert = %v", err)
	}
	if _, err := g.Resume(ctx, compliance.Live, bob, "all clear"); !errors.Is(err, compliance.ErrNotLive) {
		t.Fatalf("resume to live: %v, want a refusal", err)
	}
	e.clk.Advance(time.Minute)
	if _, err := g.Resume(ctx, compliance.Sandbox, bob, "investigated: a test payment"); err != nil {
		t.Fatal(err)
	}
	if err := e.assert(t, g); err != nil {
		t.Fatalf("after resume: %v", err)
	}
	// the configuration still caps what the record allows
	if m := e.mode(t, e.gate(compliance.Disabled, "")); m != compliance.Disabled {
		t.Errorf("configured disabled after resume: %s", m)
	}
	var actions []string
	rows, err := e.pool.Query(ctx, `SELECT action FROM audit.audit_log WHERE entity_type = 'operating_mode' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		actions = append(actions, a)
	}
	if strings.Join(actions, ",") != "compliance.operations_stopped,compliance.operations_resumed" {
		t.Errorf("audit = %v", actions)
	}
	hist, err := g.History(ctx, 10)
	if err != nil || len(hist) != 2 || hist[0].Mode != compliance.Sandbox || hist[1].Reason != "fraud suspected on paybill" {
		t.Errorf("history = %+v %v", hist, err)
	}
}

func TestGate_LiveNeedsTheConfiguredLicenceAndASecondAdmin(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := e.gate(compliance.Sandbox, "").RequestLive(ctx, liveRequest(), alice); !errors.Is(err, compliance.ErrNotConfigured) {
		t.Fatalf("request on a sandbox service: %v", err)
	}
	g := e.gate(compliance.Live, licence)
	other := liveRequest()
	other.LicenceReference = "GRA/OPL/9999"
	if _, err := g.RequestLive(ctx, other, alice); !errors.Is(err, compliance.ErrNotConfigured) {
		t.Fatalf("request for another licence: %v", err)
	}
	bad := liveRequest()
	bad.EvidenceSHA256 = "not-a-hash"
	if _, err := g.RequestLive(ctx, bad, alice); !errors.Is(err, compliance.ErrInvalid) {
		t.Fatalf("request without the evidence's hash: %v", err)
	}
	r, err := g.RequestLive(ctx, liveRequest(), alice)
	if err != nil {
		t.Fatal(err)
	}
	if m := e.mode(t, g); m != compliance.Disabled {
		t.Fatalf("a request alone made the service %s", m)
	}
	if open, err := g.OpenLiveRequests(ctx); err != nil || len(open) != 1 || open[0].ID != r.ID {
		t.Fatalf("open requests = %+v %v", open, err)
	}
	if _, err := g.ApproveLive(ctx, r.ID, alice); !errors.Is(err, compliance.ErrSamePerson) {
		t.Fatalf("self-approval: %v", err)
	}
	if _, err := g.ApproveLive(ctx, "0192f000-0000-7000-8000-0000000000ff", bob); !errors.Is(err, compliance.ErrNotFound) {
		t.Fatalf("unknown request: %v", err)
	}
	if _, err := g.ApproveLive(ctx, "nope", bob); !errors.Is(err, compliance.ErrNotFound) {
		t.Fatalf("malformed id: %v", err)
	}
	c, err := g.ApproveLive(ctx, r.ID, bob)
	if err != nil || c.Mode != compliance.Live || c.LicenceReference != licence {
		t.Fatalf("approve: %+v %v", c, err)
	}
	st, err := g.Current(ctx)
	if err != nil || st.Mode != compliance.Live || !st.RealMoney {
		t.Fatalf("after approval: %+v %v", st, err)
	}
	if _, err := g.ApproveLive(ctx, r.ID, "staff:0192f000-0000-7000-8000-00000000000c"); !errors.Is(err, compliance.ErrAlreadyDecided) {
		t.Fatalf("second approval: %v", err)
	}
	// the approval counts only for the licence it names: redeployed under another licence, nothing moves
	if m := e.mode(t, e.gate(compliance.Live, "GRA/OPL/9999")); m != compliance.Disabled {
		t.Errorf("another licence: %s", m)
	}
	// redeployed as a sandbox over a live record, nothing moves until an admin records the return to sandbox
	sandbox := e.gate(compliance.Sandbox, "")
	if m := e.mode(t, sandbox); m != compliance.Disabled {
		t.Errorf("sandbox configuration over a live record: %s, want disabled", m)
	}
	e.clk.Advance(time.Minute)
	if _, err := sandbox.Resume(ctx, compliance.Sandbox, alice, "licence surrendered; back to the demo"); err != nil {
		t.Fatal(err)
	}
	if m := e.mode(t, sandbox); m != compliance.Sandbox {
		t.Errorf("after recording the return to sandbox: %s", m)
	}
	e.clk.Advance(time.Minute)
	// stopping a live service takes one person
	if _, err := g.Stop(ctx, alice, "regulator asked us to pause"); err != nil {
		t.Fatal(err)
	}
	if m := e.mode(t, g); m != compliance.Disabled {
		t.Errorf("after stopping live: %s", m)
	}
}

func TestGate_ALiveRequestExpires(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	g := e.gate(compliance.Live, licence)
	r, err := g.RequestLive(ctx, liveRequest(), alice)
	if err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(compliance.RequestTTL + time.Minute)
	if open, _ := g.OpenLiveRequests(ctx); len(open) != 0 {
		t.Errorf("an expired request is still listed: %+v", open)
	}
	if _, err := g.ApproveLive(ctx, r.ID, bob); !errors.Is(err, compliance.ErrExpired) {
		t.Fatalf("approve expired: %v", err)
	}
}

func TestSchema_RecordsAreAppendOnlyAndTheDatabaseRefusesSelfApproval(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	g := e.gate(compliance.Live, licence)
	r, err := g.RequestLive(ctx, liveRequest(), alice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO compliance.live_approvals (request_id, approved_by, created_at) VALUES ($1, $2, now())`, r.ID, alice); err == nil {
		t.Error("the database accepted a self-approval")
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO compliance.mode_changes (mode, reason, changed_by, created_at) VALUES ('live', 'x', $1, now())`, bob); err == nil {
		t.Error("the database accepted a live change without an approval")
	}
	if _, err := g.Stop(ctx, alice, "drill"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE compliance.mode_changes SET mode = 'sandbox'`,
		`DELETE FROM compliance.mode_changes`,
		`UPDATE compliance.live_requests SET licence_reference = 'other'`,
		`DELETE FROM compliance.live_requests`,
	} {
		if _, err := e.pool.Exec(ctx, q); err == nil {
			t.Errorf("%s succeeded", q)
		}
	}
}
