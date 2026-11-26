// Package alerts tells a person what needs them: money waiting for a human (top-ups never confirmed, paybill money in
// suspense, payouts awaiting approval or failed), the platform's health (failing jobs, Daraja throttling, a low B2C
// float), and the duties with deadlines (late draws, reconciliations, withheld tax, identity reviews, account
// closures). Every few minutes the watch runs each check, keeps one alert per condition in audit.alerts, sends new
// alerts to every configured channel, reminds about pages until someone acknowledges them, escalates a warning that has
// waited too long into a page, and closes alerts whose condition has cleared (ADR 0025).
//
// An alert raised by a failed reconciliation freezes payouts: B2C sends stop (payouts.Service.Send) until a person
// acknowledges it with a note, and only a person closes it. Books that do not balance never pay out.
package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/alerts/store"
	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// Severity says how urgently a person must act: a page is re-sent until acknowledged, a warning is sent once.
type Severity string

// Severities.
const (
	Page Severity = "page"
	Warn Severity = "warn"
)

// Errors of acknowledgement.
var (
	ErrNotFound     = errors.New("alerts: not found")
	ErrOperator     = errors.New("alerts: an operator's name and a note are required")
	ErrAcknowledged = errors.New("alerts: the alert is already acknowledged")
	ErrClosed       = errors.New("alerts: the alert is already resolved")
)

// Alert is a condition a check found. (Kind, Subject) names it: raising it again while it is open counts another
// occurrence of the same alert.
type Alert struct {
	Kind           string
	Subject        string
	Severity       Severity
	FreezesPayouts bool
	Summary        string
	Details        map[string]any
}

// Check evaluates one condition. Run returns every alert that fires now; Kinds lists the kinds the check owns, so an
// open alert of those kinds that no longer fires is resolved (except one that freezes payouts: a person closes it).
type Check struct {
	Name  string
	Kinds []string
	Run   func(ctx context.Context) ([]Alert, error)
}

// View is a stored alert.
type View struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Subject        string          `json:"subject"`
	Severity       Severity        `json:"severity"`
	FreezesPayouts bool            `json:"freezes_payouts"`
	Summary        string          `json:"summary"`
	Details        json.RawMessage `json:"details"`
	FirstSeenAt    time.Time       `json:"first_seen_at"`
	LastSeenAt     time.Time       `json:"last_seen_at"`
	Occurrences    int             `json:"occurrences"`
	Notifications  int             `json:"notifications"`
	AcknowledgedBy string          `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time      `json:"acknowledged_at,omitempty"`
	AckNote        string          `json:"ack_note,omitempty"`
	ResolvedBy     string          `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time      `json:"resolved_at,omitempty"`
}

func view(a store.AuditAlert) View {
	return View{ID: a.ID, Kind: a.Kind, Subject: a.Subject, Severity: Severity(a.Severity), FreezesPayouts: a.Freezes != nil, Summary: a.Summary,
		Details: a.Details, FirstSeenAt: a.FirstSeenAt, LastSeenAt: a.LastSeenAt, Occurrences: int(a.Occurrences), Notifications: int(a.Notifications),
		AcknowledgedBy: deref(a.AcknowledgedBy), AcknowledgedAt: a.AcknowledgedAt, AckNote: deref(a.AckNote), ResolvedBy: deref(a.ResolvedBy),
		ResolvedAt: a.ResolvedAt}
}

// Config tunes the service; zero values take the defaults.
type Config struct {
	RemindEvery time.Duration // how often an unacknowledged page is sent again (default 1h)
	// Heartbeat, when set, is called after every watch that ran all its checks (a dead man's switch).
	Heartbeat func(ctx context.Context) error
}

// Service keeps and sends alerts.
type Service struct {
	pool   *pgxpool.Pool
	clk    clock.Clock
	notify Notifier
	audit  *audit.Writer
	cfg    Config
	checks []Check
}

// NewService returns the alert service. A nil notifier only logs.
func NewService(pool *pgxpool.Pool, clk clock.Clock, n Notifier, aw *audit.Writer, cfg Config, checks ...Check) *Service {
	if n == nil {
		n = LogNotifier{}
	}
	if cfg.RemindEvery <= 0 {
		cfg.RemindEvery = time.Hour
	}
	return &Service{pool: pool, clk: clk, notify: n, audit: aw, cfg: cfg, checks: checks}
}

// checkFailed is the kind of the alert raised when a check itself cannot run.
const checkFailed = "alerts.check_failed"

// Watch runs every check, records what fires and what has cleared, sends what is due, and pings the heartbeat if every
// check ran. A check that cannot run is an alert of its own; it never clears the alerts that check owns.
func (s *Service) Watch(ctx context.Context) error {
	var errs []error
	allRan := true
	for _, c := range s.checks {
		firing, err := c.Run(ctx)
		if err != nil {
			allRan = false
			slog.ErrorContext(ctx, "alert check failed", "check", c.Name, "err", err)
			errs = append(errs, s.record(ctx, Check{Name: c.Name}, []Alert{{Kind: checkFailed, Subject: c.Name, Severity: Page,
				Summary: "The " + c.Name + " check cannot run: " + err.Error()}}, false))
			continue
		}
		errs = append(errs, s.record(ctx, c, firing, true))
	}
	if err := s.Deliver(ctx); err != nil {
		errs = append(errs, err)
	}
	err := errors.Join(errs...)
	if err == nil && allRan && s.cfg.Heartbeat != nil {
		if herr := s.cfg.Heartbeat(ctx); herr != nil {
			slog.ErrorContext(ctx, "alert heartbeat failed", "err", herr)
		}
	}
	return err
}

// record stores what a check found in one transaction. With ran, the check's owned kinds that stopped firing (and
// its own check_failed alert) are resolved.
func (s *Service) record(ctx context.Context, c Check, firing []Alert, ran bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		live := map[string]bool{}
		for _, a := range firing {
			details := a.Details
			if details == nil {
				details = map[string]any{}
			}
			raw, err := json.Marshal(details)
			if err != nil {
				return err
			}
			var freezes *string
			if a.FreezesPayouts {
				freezes = strPtr("payouts")
			}
			row, err := q.RaiseAlert(ctx, store.RaiseAlertParams{Kind: a.Kind, Subject: a.Subject, Severity: string(a.Severity), Freezes: freezes,
				Summary: a.Summary, Details: raw, At: now})
			if err != nil {
				return err
			}
			if row.Created {
				slog.ErrorContext(ctx, "alert", "alert", row.ID, "kind", a.Kind, "subject", a.Subject, "severity", a.Severity,
					"freezes_payouts", a.FreezesPayouts, "summary", a.Summary)
			}
			live[a.Kind+"\x00"+a.Subject] = true
		}
		if !ran {
			return nil
		}
		kinds := append([]string{}, c.Kinds...)
		for _, kind := range kinds {
			open, err := q.OpenAlertsOfKind(ctx, kind)
			if err != nil {
				return err
			}
			for _, o := range open {
				if !live[kind+"\x00"+o.Subject] && o.Freezes == nil {
					if err := q.ResolveAlert(ctx, store.ResolveAlertParams{ResolvedBy: strPtr("system"), At: &now, ID: o.ID}); err != nil {
						return err
					}
				}
			}
		}
		open, err := q.OpenAlertsOfKind(ctx, checkFailed)
		if err != nil {
			return err
		}
		for _, o := range open {
			if o.Subject == c.Name {
				if err := q.ResolveAlert(ctx, store.ResolveAlertParams{ResolvedBy: strPtr("system"), At: &now, ID: o.ID}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Deliver sends new alerts, reminders of unacknowledged pages and resolutions, each outside any transaction; one
// that cannot be sent is tried again at the next watch.
func (s *Service) Deliver(ctx context.Context) error {
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return err
	}
	remindBefore := now.Add(-s.cfg.RemindEvery)
	due, err := store.New(s.pool).AlertsToNotify(ctx, &remindBefore)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range due {
		event := "raised"
		switch {
		case a.ResolvedAt != nil:
			event = "resolved"
		case a.NotifiedAt != nil:
			event = "reminder"
		case a.Notifications > 0: // sent before as a warning, and since escalated to a page (RaiseAlert cleared notified_at)
			event = "escalated"
		}
		v := view(a)
		if err := s.notify.Notify(ctx, Message{Event: event, Alert: v, Text: Text(event, v)}); err != nil {
			slog.ErrorContext(ctx, "alert not delivered; tried again at the next watch", "alert", a.ID, "err", err)
			errs = append(errs, err)
			continue
		}
		at, err := s.clk.Now(ctx, s.pool)
		if err != nil {
			return err
		}
		q := store.New(s.pool)
		if event == "resolved" {
			err = q.MarkResolutionNotified(ctx, store.MarkResolutionNotifiedParams{At: &at, ID: a.ID})
		} else {
			err = q.MarkNotified(ctx, store.MarkNotifiedParams{At: &at, ID: a.ID})
		}
		if err != nil {
			return err
		}
	}
	return errors.Join(errs...)
}

// Text is the one-line message for an alert event.
func Text(event string, a View) string {
	var b strings.Builder
	switch event {
	case "resolved":
		b.WriteString("[RESOLVED] ")
	case "reminder":
		b.WriteString("[REMINDER] ")
	case "escalated":
		b.WriteString("[ESCALATED] ")
	}
	b.WriteString("[" + strings.ToUpper(string(a.Severity)) + "] " + a.Summary + " (" + a.Kind + " " + a.Subject + ", alert " + a.ID + ")")
	if event != "resolved" && a.FreezesPayouts {
		b.WriteString(" Payouts are frozen until a person acknowledges this: lotteryctl alerts ack -alert " + a.ID + " -note TEXT -operator NAME")
	} else if event != "resolved" && a.Severity == Page {
		b.WriteString(" Acknowledge: lotteryctl alerts ack -alert " + a.ID + " -note TEXT -operator NAME")
	}
	return b.String()
}

// Acknowledge records that a person has taken an alert on, with a note: reminders stop, and an alert that freezes
// payouts is closed (payouts resume). Others stay open until their condition clears.
func (s *Service) Acknowledge(ctx context.Context, id, operator, note string) (View, error) {
	if strings.TrimSpace(operator) == "" || strings.TrimSpace(note) == "" {
		return View{}, ErrOperator
	}
	var out View
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		a, err := q.LockAlert(ctx, id)
		var pe *pgconn.PgError
		if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		switch {
		case a.AcknowledgedAt != nil:
			return ErrAcknowledged
		case a.ResolvedAt != nil:
			return ErrClosed
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		resolve := a.Freezes != nil
		if err := q.AcknowledgeAlert(ctx, store.AcknowledgeAlertParams{Operator: &operator, At: &now, Note: &note, Resolve: resolve, ID: id}); err != nil {
			return err
		}
		if a, err = q.LockAlert(ctx, id); err != nil {
			return err
		}
		out = view(a)
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "alert.acknowledged", EntityType: "alert",
			EntityID: id, Details: map[string]any{"kind": a.Kind, "subject": a.Subject, "note": note, "unfroze_payouts": resolve}})
	})
	return out, err
}

// State narrows a listing: open alerts are the ones not resolved (acknowledged or not), acknowledged the open ones a
// person has taken on, resolved the closed ones; All lists everything.
type State string

// States.
const (
	Open         State = "open"
	Acknowledged State = "acknowledged"
	Resolved     State = "resolved"
	All          State = "all"
)

// ParseState reads a state from a query; "" is Open.
func ParseState(s string) (State, bool) {
	switch State(s) {
	case "":
		return Open, true
	case Open, Acknowledged, Resolved, All:
		return State(s), true
	}
	return "", false
}

// Filter narrows a listing. Zero values mean no restriction except State ("" lists open alerts) and Limit (0 is 100).
type Filter struct {
	Kind     string
	Severity Severity
	State    State
	Limit    int
	// Before continues a listing after the page that ended with it: the next_before of that page.
	Before string
}

// ErrCursor says a Before was not one this service gave out.
var ErrCursor = errors.New("alerts: invalid cursor")

// Listing is one page of alerts, newest first, and where the next page starts (empty at the end).
type Listing struct {
	Alerts     []View
	NextBefore string
}

// List returns alerts matching f, newest first.
func (s *Service) List(ctx context.Context, f Filter) (Listing, error) {
	if f.State == "" {
		f.State = Open
	}
	if f.Limit <= 0 {
		f.Limit = 100
	}
	p := store.ListAlertsParams{State: string(f.State), MaxRows: int32(f.Limit + 1)} //nolint:gosec // small limits
	if f.Kind != "" {
		p.Kind = &f.Kind
	}
	if f.Severity != "" {
		sev := string(f.Severity)
		p.Severity = &sev
	}
	if f.Before != "" {
		at, id, ok := strings.Cut(f.Before, "|")
		t, err := time.Parse(time.RFC3339Nano, at)
		if !ok || err != nil || id == "" {
			return Listing{}, ErrCursor
		}
		p.BeforeAt, p.BeforeID = &t, &id
	}
	rows, err := store.New(s.pool).ListAlerts(ctx, p)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "22P02" {
			return Listing{}, ErrCursor
		}
		return Listing{}, err
	}
	out := Listing{Alerts: make([]View, 0, len(rows))}
	for _, r := range rows {
		out.Alerts = append(out.Alerts, view(r))
	}
	if len(out.Alerts) > f.Limit {
		out.Alerts = out.Alerts[:f.Limit]
		last := out.Alerts[f.Limit-1]
		out.NextBefore = last.FirstSeenAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID
	}
	return out, nil
}

// KindCount is a kind of alert and how many of it are open.
type KindCount struct {
	Kind string `json:"kind"`
	Open int    `json:"open"`
}

// Kinds lists every kind the checks can raise, and any other kind with an open alert, each with its open count.
func (s *Service) Kinds(ctx context.Context) ([]KindCount, error) {
	rows, err := store.New(s.pool).OpenAlertCounts(ctx)
	if err != nil {
		return nil, err
	}
	open := map[string]int{checkFailed: 0}
	for _, c := range s.checks {
		for _, k := range c.Kinds {
			open[k] = 0
		}
	}
	for _, r := range rows {
		open[r.Kind] = int(r.Open)
	}
	out := make([]KindCount, 0, len(open))
	for k, n := range open {
		out = append(out, KindCount{Kind: k, Open: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}

// Event is one thing that happened to an alert, in the order it happened.
type Event struct {
	At    time.Time `json:"at"`
	Event string    `json:"event"` // raised, escalated, alert.acknowledged (and any other audited action), resolved
	Actor string    `json:"actor"` // system, or the operator
	Note  string    `json:"note,omitempty"`
}

// Detail is an alert with its history and the check that owns it.
type Detail struct {
	View
	Check   string  `json:"check"`
	History []Event `json:"history"`
}

// Get returns one alert with everything known about it.
func (s *Service) Get(ctx context.Context, id string) (Detail, error) {
	q := store.New(s.pool)
	a, err := q.GetAlert(ctx, id)
	var pe *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
		return Detail{}, ErrNotFound
	} else if err != nil {
		return Detail{}, err
	}
	d := Detail{View: view(a), Check: s.owner(a.Kind), History: []Event{{At: a.FirstSeenAt, Event: "raised", Actor: "system"}}}
	var details struct {
		EscalatedAt *time.Time `json:"escalated_at"`
	}
	if json.Unmarshal(a.Details, &details) == nil && details.EscalatedAt != nil {
		d.History = append(d.History, Event{At: *details.EscalatedAt, Event: "escalated", Actor: "system", Note: "a warning became a page"})
	}
	events, err := q.AlertAuditEvents(ctx, &id)
	if err != nil {
		return Detail{}, err
	}
	for _, e := range events {
		var det struct {
			Note string `json:"note"`
		}
		_ = json.Unmarshal(e.Details, &det)
		d.History = append(d.History, Event{At: e.At, Event: e.Action, Actor: deref(e.ActorID), Note: det.Note})
	}
	if a.ResolvedAt != nil {
		d.History = append(d.History, Event{At: *a.ResolvedAt, Event: "resolved", Actor: deref(a.ResolvedBy)})
	}
	sort.SliceStable(d.History, func(i, j int) bool { return d.History[i].At.Before(d.History[j].At) })
	return d, nil
}

// owner names the check that raises (and clears) alerts of kind.
func (s *Service) owner(kind string) string {
	if kind == checkFailed {
		return "watch"
	}
	for _, c := range s.checks {
		for _, k := range c.Kinds {
			if k == kind {
				return c.Name
			}
		}
	}
	return ""
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strPtr(s string) *string { return &s }
