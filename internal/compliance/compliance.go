// Package compliance holds the operating mode (ADR 0027): whether the service may move money at all, and whether that
// money is real. The configuration sets the most the process may do; the database can only lower it, and makes it live
// only through a request approved by a second admin that names the configured licence. Every money path asks the Gate
// inside its own transaction, so a stop takes effect at the next operation and cannot interleave with one.
package compliance

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/compliance/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// Mode is what the service may do with money.
type Mode string

const (
	// Disabled moves no money: no top-up, play, withdrawal or payout starts.
	Disabled Mode = "disabled"
	// Sandbox moves sandbox money (Daraja's sandbox or a mock): a demonstration, never real stakes.
	Sandbox Mode = "sandbox"
	// Live moves real money. It needs a licence, production Daraja and two admins' approval.
	Live Mode = "live"
)

func (m Mode) rank() int {
	switch m {
	case Sandbox:
		return 1
	case Live:
		return 2
	}
	return 0 // Disabled and anything unknown: fail closed
}

// ParseMode reads a mode; anything else is an error.
func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case Disabled, Sandbox, Live:
		return m, nil
	}
	return "", fmt.Errorf("compliance: unknown operating mode %q (disabled, sandbox or live)", s)
}

// lower is the more restrictive of a and b.
func lower(a, b Mode) Mode {
	if a.rank() <= b.rank() {
		return a
	}
	return b
}

// Op names the operation asking, for errors and logs.
type Op string

const (
	OpTopUp      Op = "top-up"
	OpPush       Op = "top-up prompt"
	OpPaybill    Op = "paybill payment"
	OpPlay       Op = "play"
	OpWithdrawal Op = "withdrawal"
	OpPayout     Op = "payout"
)

// ErrOperationsStopped is every refusal of the gate: the effective mode is disabled.
var ErrOperationsStopped = errors.New("compliance: money operations are stopped")

var (
	// ErrInvalid is a request with a field missing or malformed.
	ErrInvalid = errors.New("compliance: invalid request")
	// ErrNotFound is an unknown live request.
	ErrNotFound = errors.New("compliance: no such live request")
	// ErrSamePerson is the requester approving their own live request.
	ErrSamePerson = errors.New("compliance: a live request needs a second admin")
	// ErrAlreadyDecided is a live request approved before.
	ErrAlreadyDecided = errors.New("compliance: the live request was already approved")
	// ErrExpired is a live request older than RequestTTL.
	ErrExpired = errors.New("compliance: the live request has expired; record a new one")
	// ErrNotConfigured is going live in a process not configured for it, or with another licence.
	ErrNotConfigured = errors.New("compliance: this service is not configured to operate live with that licence")
	// ErrNotLive is resuming to live without a request (use RequestLive and ApproveLive).
	ErrNotLive = errors.New("compliance: going live needs a request and a second admin's approval")
)

// RequestTTL is how long a live request may wait for its approval.
const RequestTTL = 7 * 24 * time.Hour

// Config is what the process was started with.
type Config struct {
	Mode             Mode   // OPERATING_MODE
	LicenceReference string // REGULATORY_LICENCE_REFERENCE; required for Live
}

// Querier is a transaction or the pool.
type Querier = store.DBTX

// Gate answers whether money may move. A nil *Gate allows everything: services built without one (tests) behave as
// before; the application always installs one.
type Gate struct {
	pool  *pgxpool.Pool
	clk   clock.Clock
	audit *audit.Writer
	cfg   Config
}

// NewGate returns the gate for cfg.
func NewGate(pool *pgxpool.Pool, clk clock.Clock, aw *audit.Writer, cfg Config) *Gate {
	if _, err := ParseMode(string(cfg.Mode)); err != nil {
		cfg.Mode = Disabled // fail closed; config.Load refuses an unknown mode before this
	}
	return &Gate{pool: pool, clk: clk, audit: aw, cfg: cfg}
}

// Status is the effective mode and how it came about.
type Status struct {
	Mode       Mode    `json:"mode"`        // effective
	Configured Mode    `json:"configured"`  // OPERATING_MODE
	RealMoney  bool    `json:"real_money"`  // Mode == Live
	Latest     *Change `json:"last_change"` // nil when nothing was ever recorded
}

// Change is one recorded change of mode.
type Change struct {
	ID               string    `json:"id"`
	Mode             Mode      `json:"mode"`
	Reason           string    `json:"reason"`
	ChangedBy        string    `json:"changed_by"`
	At               time.Time `json:"at"`
	LicenceReference string    `json:"licence_reference,omitempty"`
}

// Status reads the effective mode on q.
func (g *Gate) Status(ctx context.Context, q Querier) (Status, error) {
	if g == nil {
		return Status{Mode: Sandbox, Configured: Sandbox}, nil
	}
	st := Status{Configured: g.cfg.Mode}
	row, err := store.New(q).LatestModeChange(ctx)
	db := Sandbox // nothing recorded: the configuration decides, short of live
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Status{}, err
	default:
		c := toChange(row.ID, row.Mode, row.Reason, row.ChangedBy, row.CreatedAt, row.LicenceReference)
		st.Latest = &c
		db = c.Mode
		// a live change counts only for the licence this process was configured with
		if db == Live && (g.cfg.LicenceReference == "" || c.LicenceReference != g.cfg.LicenceReference) {
			db = Disabled
		}
	}
	st.Mode = lower(g.cfg.Mode, db)
	if g.cfg.Mode == Live && db != Live {
		// production credentials without an approved live change move nothing, not sandbox money
		st.Mode = Disabled
	}
	st.RealMoney = st.Mode == Live
	return st, nil
}

// Current reads the effective mode now.
func (g *Gate) Current(ctx context.Context) (Status, error) {
	if g == nil {
		return g.Status(ctx, nil)
	}
	return g.Status(ctx, g.pool)
}

// Assert refuses op unless money may move. Call it inside the operation's transaction.
func (g *Gate) Assert(ctx context.Context, q Querier, op Op) error {
	if g == nil {
		return nil
	}
	st, err := g.Status(ctx, q)
	if err != nil {
		return err
	}
	if st.Mode.rank() == 0 {
		return fmt.Errorf("%w: %s refused", ErrOperationsStopped, op)
	}
	return nil
}

func toChange(id, mode, reason, by string, at time.Time, licence *string) Change {
	c := Change{ID: id, Mode: Mode(mode), Reason: reason, ChangedBy: by, At: at}
	if licence != nil {
		c.LicenceReference = *licence
	}
	return c
}

var staffActor = regexp.MustCompile(`^staff:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isBadUUID(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "22P02"
}

func validActor(a string) bool { return staffActor.MatchString(a) }

func text(s string, maxLen int) bool {
	s = strings.TrimSpace(s)
	return s != "" && len(s) <= maxLen
}

// changeLock serialises changes of mode, so two people acting at once are recorded one after the other.
const changeLock = "SELECT pg_advisory_xact_lock(hashtext('compliance.mode'))"

// Stop records a stop: nothing moves until an admin resumes. Anyone on staff may stop.
func (g *Gate) Stop(ctx context.Context, actor, reason string) (Change, error) {
	return g.change(ctx, Disabled, actor, reason)
}

// Resume lifts a stop back to sandbox. Live is only reached through RequestLive and ApproveLive.
func (g *Gate) Resume(ctx context.Context, mode Mode, actor, reason string) (Change, error) {
	if mode != Sandbox {
		return Change{}, ErrNotLive
	}
	return g.change(ctx, Sandbox, actor, reason)
}

func (g *Gate) change(ctx context.Context, mode Mode, actor, reason string) (Change, error) {
	if !validActor(actor) || !text(reason, 1000) {
		return Change{}, ErrInvalid
	}
	var c Change
	err := pgx.BeginFunc(ctx, g.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, changeLock); err != nil {
			return err
		}
		now, err := g.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		before, err := g.Status(ctx, tx)
		if err != nil {
			return err
		}
		id, err := store.New(tx).InsertModeChange(ctx, store.InsertModeChangeParams{Mode: string(mode), Reason: strings.TrimSpace(reason), ChangedBy: actor, At: now})
		if err != nil {
			return err
		}
		c = Change{ID: id, Mode: mode, Reason: strings.TrimSpace(reason), ChangedBy: actor, At: now}
		action := "compliance.operations_resumed"
		if mode == Disabled {
			action = "compliance.operations_stopped"
		}
		return g.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: actor, Action: action, EntityType: "operating_mode",
			EntityID: id, Details: map[string]any{"mode": string(mode), "was": string(before.Mode), "reason": c.Reason}})
	})
	return c, err
}

// LiveRequest asks to operate with real money under a licence.
type LiveRequest struct {
	ID                string     `json:"id"`
	LicenceReference  string     `json:"licence_reference"`
	ApprovalReference string     `json:"approval_reference"`
	EvidenceSHA256    string     `json:"evidence_sha256"`
	EvidenceLocation  string     `json:"evidence_location"`
	Reason            string     `json:"reason"`
	RequestedBy       string     `json:"requested_by"`
	At                time.Time  `json:"at"`
	ApprovedBy        string     `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time `json:"approved_at,omitempty"`
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// RequestLive records an admin's request to go live. It changes nothing until a second admin approves it. The
// licence must be the one this process was configured with, so a request cannot be prepared for another deployment.
func (g *Gate) RequestLive(ctx context.Context, r LiveRequest, actor string) (LiveRequest, error) {
	r.LicenceReference, r.ApprovalReference = strings.TrimSpace(r.LicenceReference), strings.TrimSpace(r.ApprovalReference)
	r.EvidenceSHA256, r.EvidenceLocation, r.Reason = strings.ToLower(strings.TrimSpace(r.EvidenceSHA256)), strings.TrimSpace(r.EvidenceLocation), strings.TrimSpace(r.Reason)
	if !validActor(actor) || !text(r.LicenceReference, 200) || !text(r.ApprovalReference, 200) || !sha256Hex.MatchString(r.EvidenceSHA256) ||
		!text(r.EvidenceLocation, 500) || !text(r.Reason, 1000) {
		return LiveRequest{}, ErrInvalid
	}
	if g.cfg.Mode != Live || r.LicenceReference != g.cfg.LicenceReference {
		return LiveRequest{}, ErrNotConfigured
	}
	err := pgx.BeginFunc(ctx, g.pool, func(tx pgx.Tx) error {
		now, err := g.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		r.RequestedBy, r.At = actor, now
		if r.ID, err = store.New(tx).InsertLiveRequest(ctx, store.InsertLiveRequestParams{LicenceReference: r.LicenceReference,
			ApprovalReference: r.ApprovalReference, EvidenceSha256: r.EvidenceSHA256, EvidenceLocation: r.EvidenceLocation,
			Reason: r.Reason, RequestedBy: actor, At: now}); err != nil {
			return err
		}
		return g.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: actor, Action: "compliance.live_requested",
			EntityType: "live_request", EntityID: r.ID, Details: map[string]any{"licence_reference": r.LicenceReference,
				"approval_reference": r.ApprovalReference, "evidence_sha256": r.EvidenceSHA256}})
	})
	return r, err
}

// ApproveLive is the second admin approving a live request: the service goes live in the same transaction.
func (g *Gate) ApproveLive(ctx context.Context, id, actor string) (Change, error) {
	if !validActor(actor) {
		return Change{}, ErrInvalid
	}
	var c Change
	err := pgx.BeginFunc(ctx, g.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, changeLock); err != nil {
			return err
		}
		q := store.New(tx)
		r, err := q.GetLiveRequest(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) || isBadUUID(err) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		now, err := g.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		switch {
		case r.ApprovedBy != nil:
			return ErrAlreadyDecided
		case r.RequestedBy == actor:
			return ErrSamePerson
		case now.Sub(r.CreatedAt) > RequestTTL:
			return ErrExpired
		case g.cfg.Mode != Live || r.LicenceReference != g.cfg.LicenceReference:
			return ErrNotConfigured
		}
		if err := q.InsertLiveApproval(ctx, store.InsertLiveApprovalParams{RequestID: id, ApprovedBy: actor, At: now}); err != nil {
			return err
		}
		reason := "live under licence " + r.LicenceReference + " (approval " + r.ApprovalReference + "), requested by " + r.RequestedBy
		cid, err := q.InsertModeChange(ctx, store.InsertModeChangeParams{Mode: string(Live), LiveRequestID: &id, Reason: reason, ChangedBy: actor, At: now})
		if err != nil {
			return err
		}
		c = Change{ID: cid, Mode: Live, Reason: reason, ChangedBy: actor, At: now, LicenceReference: r.LicenceReference}
		return g.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: actor, Action: "compliance.live_approved",
			EntityType: "live_request", EntityID: id, Details: map[string]any{"licence_reference": r.LicenceReference,
				"requested_by": r.RequestedBy, "mode_change": cid}})
	})
	return c, err
}

// OpenLiveRequests are the live requests waiting for a second admin, newest first.
func (g *Gate) OpenLiveRequests(ctx context.Context) ([]LiveRequest, error) {
	now, err := g.clk.Now(ctx, g.pool)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(g.pool).OpenLiveRequests(ctx, now.Add(-RequestTTL))
	if err != nil {
		return nil, err
	}
	out := make([]LiveRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, LiveRequest{ID: r.ID, LicenceReference: r.LicenceReference, ApprovalReference: r.ApprovalReference,
			EvidenceSHA256: r.EvidenceSha256, EvidenceLocation: r.EvidenceLocation, Reason: r.Reason, RequestedBy: r.RequestedBy, At: r.CreatedAt})
	}
	return out, nil
}

// History is the latest changes of mode, newest first.
func (g *Gate) History(ctx context.Context, limit int32) ([]Change, error) {
	rows, err := store.New(g.pool).ModeHistory(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Change, 0, len(rows))
	for _, r := range rows {
		out = append(out, toChange(r.ID, r.Mode, r.Reason, r.ChangedBy, r.CreatedAt, r.LicenceReference))
	}
	return out, nil
}

// Configured is the mode the process was started with.
func (g *Gate) Configured() Mode {
	if g == nil {
		return Sandbox
	}
	return g.cfg.Mode
}
