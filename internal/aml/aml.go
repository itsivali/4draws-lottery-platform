// Package aml is transaction monitoring (ADR 0033): configurable rules raise cases about players for staff to look
// into; a case may hold the player's money (no top-ups, paybill payments, withdrawals or payouts) while they do. It is
// a foundation for the operator's AML programme, not a claim that it satisfies POCAMLA: the compliance owner decides the
// rules, their thresholds and what is reported to the Financial Reporting Centre. Until a threshold is approved it is
// shown as pending that approval.
package aml

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/aml/store"
	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// Rule is what raised a case.
type Rule string

// The rules, and Manual for a case staff opened.
const (
	DepositVelocity Rule = "deposit_velocity"   // much money in, quickly
	InOutLittlePlay Rule = "in_out_little_play" // money in and out with little play between
	FailedTopUps    Rule = "failed_topups"      // many failed top-ups (card-testing style probing of numbers)
	SharedIDNumber  Rule = "shared_id_number"   // an ID number declared by several accounts
	SharedDevice    Rule = "shared_device"      // a device used by several accounts
	Manual          Rule = "manual"
)

// Rules are the automatic rules, in the order they are shown.
var Rules = []Rule{DepositVelocity, InOutLittlePlay, FailedTopUps, SharedIDNumber, SharedDevice}

// Errors.
var (
	// ErrUnderReview: an open case holds the player's money. Player-facing messages must not say why (tipping off a
	// player that they are being looked into can itself be an offence): "not available on your account right now".
	ErrUnderReview = errors.New("aml: the account is under review")
	ErrInvalid     = errors.New("aml: invalid rule, parameters, staff member or note")
	ErrNotFound    = errors.New("aml: no such case")
	ErrClosed      = errors.New("aml: the case is closed")
	ErrHoldState   = errors.New("aml: the case is not in a state that allows this")
	ErrSamePerson  = errors.New("aml: someone other than who placed the hold must release it")
	ErrBadCursor   = errors.New("aml: bad cursor")
	ErrOpen        = errors.New("aml: the player already has an open case of this kind")
)

// Params are a rule's thresholds; each rule uses some of them. QuietDays is how long after a case of the rule is closed
// the same player is not raised again by it.
type Params struct {
	WindowHours      int64 `json:"window_hours,omitempty"`
	MaxKES           int64 `json:"max_kes,omitempty"`
	MaxCount         int64 `json:"max_count,omitempty"`
	MinWithdrawalKES int64 `json:"min_withdrawal_kes,omitempty"`
	MaxPlayPercent   int64 `json:"max_play_percent,omitempty"`
	MinFailed        int64 `json:"min_failed,omitempty"`
	MinAccounts      int64 `json:"min_accounts,omitempty"`
	QuietDays        int64 `json:"quiet_days,omitempty"`
}

// Defaults are placeholders until the compliance owner sets and approves thresholds (ADR 0033); every rule is on.
var Defaults = map[Rule]Params{
	DepositVelocity: {WindowHours: 24, MaxKES: 100_000, MaxCount: 10, QuietDays: 30},
	InOutLittlePlay: {WindowHours: 7 * 24, MinWithdrawalKES: 5_000, MaxPlayPercent: 20, QuietDays: 30},
	FailedTopUps:    {WindowHours: 24, MinFailed: 5, QuietDays: 7},
	SharedIDNumber:  {MinAccounts: 2, QuietDays: 90},
	SharedDevice:    {MinAccounts: 3, QuietDays: 90},
}

// validate checks that a rule's parameters are the ones it uses, within sense.
func validate(r Rule, p Params) error {
	pos := func(vs ...int64) bool {
		for _, v := range vs {
			if v <= 0 {
				return false
			}
		}
		return true
	}
	ok := false
	switch r {
	case DepositVelocity:
		ok = pos(p.WindowHours, p.MaxKES, p.MaxCount, p.QuietDays)
	case InOutLittlePlay:
		ok = pos(p.WindowHours, p.MinWithdrawalKES, p.QuietDays) && p.MaxPlayPercent > 0 && p.MaxPlayPercent <= 100
	case FailedTopUps:
		ok = pos(p.WindowHours, p.MinFailed, p.QuietDays)
	case SharedIDNumber, SharedDevice:
		ok = pos(p.QuietDays) && p.MinAccounts >= 2
	}
	if !ok || p.WindowHours > 366*24 || p.QuietDays > 366 {
		return ErrInvalid
	}
	return nil
}

// Setting is a rule as it stands: enabled, its thresholds, and whether the compliance owner approved them.
type Setting struct {
	Rule     Rule   `json:"rule"`
	Enabled  bool   `json:"enabled"`
	Params   Params `json:"params"`
	Approval string `json:"approval_reference,omitempty"`
	// Pending is true until an approval reference is recorded: the thresholds are placeholders.
	Pending bool       `json:"pending_approval"`
	Default bool       `json:"default"`
	SetBy   string     `json:"set_by,omitempty"`
	Note    string     `json:"note,omitempty"`
	SetAt   *time.Time `json:"set_at,omitempty"`
}

// Event is something done on a case.
type Event struct {
	Action string    `json:"action"`
	Staff  string    `json:"staff,omitempty"`
	Note   string    `json:"note,omitempty"`
	At     time.Time `json:"at"`
}

// Case is a matter about one player.
type Case struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	MSISDN    string          `json:"msisdn,omitempty"`
	Rule      Rule            `json:"rule"`
	Status    string          `json:"status"`
	Hold      bool            `json:"hold"`
	HoldBy    string          `json:"hold_by,omitempty"`
	Evidence  json.RawMessage `json:"evidence"`
	OpenedBy  string          `json:"opened_by,omitempty"`
	Outcome   string          `json:"outcome,omitempty"`
	OpenedAt  time.Time       `json:"opened_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	ClosedAt  *time.Time      `json:"closed_at,omitempty"`
	ClosedBy  string          `json:"closed_by,omitempty"`
	Events    []Event         `json:"events,omitempty"`
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func caseView(c store.ComplianceAmlCase) Case {
	return Case{ID: c.ID, UserID: c.UserID, Rule: Rule(c.Rule), Status: c.Status, Hold: c.Hold, HoldBy: deref(c.HoldBy), Evidence: c.Evidence,
		OpenedBy: deref(c.OpenedBy), Outcome: deref(c.Outcome), OpenedAt: c.OpenedAt, UpdatedAt: c.UpdatedAt, ClosedAt: c.ClosedAt, ClosedBy: deref(c.ClosedBy)}
}

// Service runs the rules and the cases.
type Service struct {
	pool  *pgxpool.Pool
	clk   clock.Clock
	audit *audit.Writer
}

// NewService returns a Service.
func NewService(pool *pgxpool.Pool, clk clock.Clock, a *audit.Writer) *Service {
	return &Service{pool: pool, clk: clk, audit: a}
}

func staffActor(s string) bool { return strings.HasPrefix(s, "staff:") }

func text(s string, max int) bool { s = strings.TrimSpace(s); return s != "" && len(s) <= max }

// Settings returns every rule as it stands: its latest setting, else its default.
func (s *Service) Settings(ctx context.Context) ([]Setting, error) {
	rows, err := store.New(s.pool).LatestRuleSettings(ctx)
	if err != nil {
		return nil, err
	}
	set := map[Rule]store.LatestRuleSettingsRow{}
	for _, r := range rows {
		set[Rule(r.Rule)] = r
	}
	out := make([]Setting, 0, len(Rules))
	for _, rule := range Rules {
		r, ok := set[rule]
		if !ok {
			out = append(out, Setting{Rule: rule, Enabled: true, Params: Defaults[rule], Pending: true, Default: true})
			continue
		}
		var p Params
		if err := json.Unmarshal(r.Params, &p); err != nil {
			return nil, err
		}
		at := r.SetAt
		out = append(out, Setting{Rule: rule, Enabled: r.Enabled, Params: p, Approval: deref(r.ApprovalReference), Pending: r.ApprovalReference == nil,
			SetBy: r.SetBy, Note: r.Note, SetAt: &at})
	}
	return out, nil
}

// SetRule records a rule's new setting: on or off, its thresholds, and the compliance owner's approval reference when
// there is one (an empty one leaves the thresholds pending approval). Audited with the note.
func (s *Service) SetRule(ctx context.Context, rule Rule, enabled bool, p Params, approval, staff, note string) error {
	if _, known := Defaults[rule]; !known || !staffActor(staff) || !text(note, 1000) || len(approval) > 200 {
		return ErrInvalid
	}
	if err := validate(rule, p); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var ref *string
	if a := strings.TrimSpace(approval); a != "" {
		ref = &a
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if err := store.New(tx).InsertRuleSetting(ctx, store.InsertRuleSettingParams{Rule: string(rule), Enabled: enabled, Params: raw,
			ApprovalReference: ref, SetBy: staff, Note: strings.TrimSpace(note), At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: staff, Action: "aml.rule_set", EntityType: "aml_rule",
			EntityID: string(rule), Details: map[string]any{"enabled": enabled, "params": p, "approval_reference": deref(ref), "note": strings.TrimSpace(note)}})
	})
}

// hit is one player a rule found, with what it saw.
type hit struct {
	user     string
	evidence map[string]any
}

// Scan runs every enabled rule and opens a case for each player it finds who has no open case of that rule and none
// closed within the rule's quiet period. It returns how many cases it opened.
func (s *Service) Scan(ctx context.Context) (int, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return 0, err
	}
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return 0, err
	}
	opened := 0
	var errs []error
	for _, set := range settings {
		if !set.Enabled {
			continue
		}
		hits, err := s.find(ctx, set.Rule, set.Params, now)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, h := range hits {
			h.evidence["thresholds"] = set.Params
			h.evidence["thresholds_pending_approval"] = set.Pending
			id, err := s.open(ctx, h.user, set.Rule, h.evidence, "", "", now.AddDate(0, 0, -int(set.Params.QuietDays)))
			if err != nil {
				errs = append(errs, err)
			} else if id != "" {
				opened++
			}
		}
	}
	if opened > 0 {
		slog.InfoContext(ctx, "transaction monitoring opened cases", "opened", opened)
	}
	return opened, errors.Join(errs...)
}

func (s *Service) find(ctx context.Context, rule Rule, p Params, now time.Time) ([]hit, error) {
	q := store.New(s.pool)
	since := now.Add(-time.Duration(p.WindowHours) * time.Hour)
	var out []hit
	switch rule {
	case DepositVelocity:
		rows, err := q.DepositVelocity(ctx, store.DepositVelocityParams{Since: since, MaxKes: p.MaxKES, MaxCount: p.MaxCount})
		for _, r := range rows {
			out = append(out, hit{r.UserID, map[string]any{"deposited_kes": r.DepositedKes, "deposits": r.Deposits, "since": since}})
		}
		return out, err
	case InOutLittlePlay:
		rows, err := q.InOutLittlePlay(ctx, store.InOutLittlePlayParams{Since: since, MinWithdrawalKes: p.MinWithdrawalKES, MaxPlayPercent: p.MaxPlayPercent})
		for _, r := range rows {
			out = append(out, hit{r.UserID, map[string]any{"deposited_kes": r.DepositedKes, "staked_kes": r.StakedKes, "withdrawn_kes": r.WithdrawnKes, "since": since}})
		}
		return out, err
	case FailedTopUps:
		rows, err := q.FailedTopUps(ctx, store.FailedTopUpsParams{Since: since, MinFailed: p.MinFailed})
		for _, r := range rows {
			out = append(out, hit{r.UserID, map[string]any{"failed_topups": r.Failed, "since": since}})
		}
		return out, err
	case SharedIDNumber:
		rows, err := q.SharedIDNumbers(ctx, p.MinAccounts)
		for _, r := range rows {
			out = append(out, hit{r.UserID, map[string]any{"other_accounts": r.OtherAccounts}})
		}
		return out, err
	case SharedDevice:
		rows, err := q.SharedDevices(ctx, p.MinAccounts)
		for _, r := range rows {
			out = append(out, hit{r.UserID, map[string]any{"other_accounts": r.OtherAccounts}})
		}
		return out, err
	}
	return nil, nil
}

// open opens a case in its own transaction and returns its id; "" when the player already has one (or one closed
// within the quiet period).
func (s *Service) open(ctx context.Context, user string, rule Rule, evidence map[string]any, staff, note string, quietSince time.Time) (string, error) {
	raw, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	opened := ""
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		var by *string
		if staff != "" {
			by = &staff
		}
		id, err := q.OpenCase(ctx, store.OpenCaseParams{UserID: user, Rule: string(rule), Evidence: raw, OpenedBy: by, At: now, QuietSince: quietSince})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		opened = id
		var n *string
		if note != "" {
			n = &note
		}
		if err := q.InsertCaseEvent(ctx, store.InsertCaseEventParams{CaseID: id, Action: "opened", Staff: by, Note: n, At: now}); err != nil {
			return err
		}
		actor := audit.Event{ActorType: audit.System}
		if staff != "" {
			actor = audit.Event{ActorType: audit.Staff, ActorID: staff}
		}
		actor.Action, actor.EntityType, actor.EntityID = "aml.case_opened", "aml_case", id
		actor.Details = map[string]any{"user_id": user, "rule": string(rule)}
		return s.audit.Record(ctx, tx, actor)
	})
	return opened, err
}

// OpenManual opens a case staff decided to look into, with why. A player has at most one open manual case.
func (s *Service) OpenManual(ctx context.Context, userID, staff, note string) (Case, error) {
	if !staffActor(staff) || !text(note, 1000) {
		return Case{}, ErrInvalid
	}
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return Case{}, err
	}
	id, err := s.open(ctx, userID, Manual, map[string]any{"reason": strings.TrimSpace(note)}, staff, strings.TrimSpace(note), now)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && (pe.Code == "22P02" || pe.Code == "23503") {
		return Case{}, ErrNotFound
	}
	if err != nil {
		return Case{}, err
	}
	if id == "" {
		return Case{}, ErrOpen
	}
	return s.Case(ctx, id)
}

// act changes an open case in one transaction, records the event and audits it.
func (s *Service) act(ctx context.Context, id, staff, note, action string, f func(q *store.Queries, c store.ComplianceAmlCase, now time.Time) error) (Case, error) {
	if !staffActor(staff) || !text(note, 2000) {
		return Case{}, ErrInvalid
	}
	note = strings.TrimSpace(note)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		c, err := q.LockCase(ctx, id)
		var pe *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if c.Status == "closed" {
			return ErrClosed
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if err := f(q, c, now); err != nil {
			return err
		}
		if err := q.InsertCaseEvent(ctx, store.InsertCaseEventParams{CaseID: c.ID, Action: action, Staff: &staff, Note: &note, At: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: staff, Action: "aml.case_" + action, EntityType: "aml_case",
			EntityID: c.ID, Details: map[string]any{"user_id": c.UserID, "note": note}})
	})
	if err != nil {
		return Case{}, err
	}
	return s.Case(ctx, id)
}

// Hold stops the player's money while the case is looked into: no top-ups, paybill payments, withdrawals or payouts
// (prizes stay owed). The player is not told why.
func (s *Service) Hold(ctx context.Context, id, staff, note string) (Case, error) {
	return s.act(ctx, id, staff, note, "hold", func(q *store.Queries, c store.ComplianceAmlCase, now time.Time) error {
		if c.Hold {
			return ErrHoldState
		}
		return q.SetHold(ctx, store.SetHoldParams{Hold: true, HoldBy: &staff, At: now, ID: c.ID})
	})
}

// Release lifts a case's hold. Someone other than who placed it releases it.
func (s *Service) Release(ctx context.Context, id, staff, note string) (Case, error) {
	return s.act(ctx, id, staff, note, "released", func(q *store.Queries, c store.ComplianceAmlCase, now time.Time) error {
		if !c.Hold {
			return ErrHoldState
		}
		if deref(c.HoldBy) == staff {
			return ErrSamePerson
		}
		return q.SetHold(ctx, store.SetHoldParams{Hold: false, At: now, ID: c.ID})
	})
}

// Note adds what was found or done to a case.
func (s *Service) Note(ctx context.Context, id, staff, note string) (Case, error) {
	return s.act(ctx, id, staff, note, "note", func(q *store.Queries, c store.ComplianceAmlCase, now time.Time) error {
		return q.TouchCase(ctx, store.TouchCaseParams{At: now, ID: c.ID})
	})
}

// Close ends a case with its outcome (what was decided, and whether it was reported). Closing a held case lifts the
// hold, so, as with Release, someone other than who placed it closes it.
func (s *Service) Close(ctx context.Context, id, staff, outcome string) (Case, error) {
	return s.act(ctx, id, staff, outcome, "closed", func(q *store.Queries, c store.ComplianceAmlCase, now time.Time) error {
		if c.Hold && deref(c.HoldBy) == staff {
			return ErrSamePerson
		}
		o := strings.TrimSpace(outcome)
		return q.CloseCase(ctx, store.CloseCaseParams{Outcome: &o, At: &now, ClosedBy: &staff, ID: c.ID})
	})
}

// Case returns a case with its events.
func (s *Service) Case(ctx context.Context, id string) (Case, error) {
	q := store.New(s.pool)
	c, err := q.GetCase(ctx, id)
	var pe *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
		return Case{}, ErrNotFound
	} else if err != nil {
		return Case{}, err
	}
	out := caseView(c)
	evs, err := q.CaseEvents(ctx, id)
	if err != nil {
		return Case{}, err
	}
	for _, e := range evs {
		out.Events = append(out.Events, Event{Action: e.Action, Staff: deref(e.Staff), Note: deref(e.Note), At: e.At})
	}
	return out, nil
}

// Cases lists cases in a view, oldest first: open (the default), held or closed. after continues from a case id;
// next is empty on the last page.
func (s *Service) Cases(ctx context.Context, view string, limit int, after string) ([]Case, string, error) {
	var cursor *string
	if after != "" {
		cursor = &after
	}
	rows, err := store.New(s.pool).Cases(ctx, store.CasesParams{View: view, After: cursor, MaxRows: int32(limit) + 1}) //nolint:gosec // small limits
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "22P02" {
		return nil, "", ErrBadCursor
	} else if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = rows[len(rows)-1].ID
	}
	out := make([]Case, 0, len(rows))
	for _, r := range rows {
		c := caseView(store.ComplianceAmlCase{ID: r.ID, UserID: r.UserID, Rule: r.Rule, Status: r.Status, Hold: r.Hold, HoldBy: r.HoldBy, Evidence: r.Evidence,
			OpenedBy: r.OpenedBy, Outcome: r.Outcome, OpenedAt: r.OpenedAt, UpdatedAt: r.UpdatedAt, ClosedAt: r.ClosedAt, ClosedBy: r.ClosedBy})
		c.MSISDN = r.Msisdn
		out = append(out, c)
	}
	return out, next, nil
}

// OnHold reports whether an open case holds the player's money, in the caller's transaction (payments and payouts ask
// it before money moves).
func (s *Service) OnHold(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	if s == nil {
		return false, nil
	}
	return store.New(tx).OnHold(ctx, userID)
}
