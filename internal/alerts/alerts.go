// Package alerts tells a person what needs them: draws whose next step is late, reconciliations that failed or did
// not run, withheld tax nearing its remittance deadline. Every few minutes the watch runs each check, keeps one alert
// per condition in audit.alerts, sends new alerts to the configured channel, reminds about pages until someone
// acknowledges them, and closes alerts whose condition has cleared.
//
// An alert raised by a failed reconciliation freezes payouts: B2C sends stop (payouts.Service.Send) until a person
// acknowledges it with a note, and only a person closes it. Books that do not balance never pay out.
package alerts

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

	"github.com/itsivali/lottery-platform/internal/alerts/store"
	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
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

// List returns open alerts (and resolved ones with all), newest first.
func (s *Service) List(ctx context.Context, all bool, limit int) ([]View, error) {
	rows, err := store.New(s.pool).ListAlerts(ctx, store.ListAlertsParams{IncludeResolved: all, MaxRows: int32(limit)}) //nolint:gosec // small limits
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		out = append(out, view(r))
	}
	return out, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func strPtr(s string) *string { return &s }
