package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/itsivali/lottery-platform/internal/platform/mail"
)

// Email sends each event to the on-call addresses as plain text: what, since when, the details, and the link into the
// back office. Nothing about a player beyond what the alert's details already say (a masked number, an id).
type Email struct {
	Sender mail.Sender
	To     []string
	// BackOfficeURL is the back office's origin (https://backoffice.4draws.co.ke); empty leaves the link out.
	BackOfficeURL string
}

// Notify implements Notifier.
func (e Email) Notify(ctx context.Context, m Message) error {
	if len(e.To) == 0 {
		return nil
	}
	return e.Sender.Send(ctx, mail.Message{To: e.To, Subject: e.Subject(m), Text: e.Body(m)})
}

// Subject is "[4draws] PAGE kind: summary", with the event in front when it is not the first notice.
func (Email) Subject(m Message) string {
	prefix := ""
	switch m.Event {
	case "reminder":
		prefix = "REMINDER "
	case "escalated":
		prefix = "ESCALATED "
	case "resolved":
		prefix = "RESOLVED "
	}
	return oneLine("[4draws] " + prefix + strings.ToUpper(string(m.Alert.Severity)) + " " + m.Alert.Kind + ": " + m.Alert.Summary)
}

// Link is the alert's page in the back office.
func (e Email) Link(id string) string {
	if e.BackOfficeURL == "" {
		return ""
	}
	return strings.TrimRight(e.BackOfficeURL, "/") + "/alerts?alert=" + url.QueryEscape(id)
}

// Body is the plain-text message.
func (e Email) Body(m Message) string {
	var b strings.Builder
	b.WriteString(m.Text + "\n\n")
	a := m.Alert
	b.WriteString("Kind:       " + a.Kind + "\n")
	b.WriteString("Subject:    " + a.Subject + "\n")
	b.WriteString("Severity:   " + string(a.Severity) + "\n")
	b.WriteString("First seen: " + a.FirstSeenAt.UTC().Format(time.RFC3339) + "\n")
	b.WriteString("Last seen:  " + a.LastSeenAt.UTC().Format(time.RFC3339) + " (" + strconv.Itoa(a.Occurrences) + " occurrence(s))\n")
	if a.FreezesPayouts && m.Event != "resolved" {
		b.WriteString("Payouts are frozen until a person acknowledges this alert.\n")
	}
	if details := detailLines(a.Details); len(details) > 0 {
		b.WriteString("\nDetails:\n")
		for _, l := range details {
			b.WriteString("  " + l + "\n")
		}
	}
	if link := e.Link(a.ID); link != "" {
		b.WriteString("\nOpen it in the back office (sign in first):\n" + link + "\n")
	}
	b.WriteString("\nWhat to do about each kind of alert: docs/runbooks/alerts.md\n")
	return b.String()
}

// detailLines renders the details JSON as "key: value" lines, keys sorted.
func detailLines(raw []byte) []string {
	var d map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil || len(d) == 0 {
		return nil
	}
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+": "+oneLine(stringify(d[k])))
	}
	return out
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

// Multi sends every event to each channel. A channel that fails is logged and does not stop the others; the error is
// returned (so the watch sends the event again) only when no channel took it.
type Multi []Notifier

// Notify implements Notifier.
func (m Multi) Notify(ctx context.Context, msg Message) error {
	if len(m) == 0 {
		return LogNotifier{}.Notify(ctx, msg)
	}
	var errs []error
	for _, n := range m {
		if err := n.Notify(ctx, msg); err != nil {
			slog.ErrorContext(ctx, "alert channel failed", "channel", name(n), "alert", msg.Alert.ID, "event", msg.Event, "err", err)
			errs = append(errs, err)
		}
	}
	if len(errs) == len(m) {
		return errors.Join(errs...)
	}
	return nil
}

// name says which channel, for the log.
func name(n Notifier) string {
	switch n.(type) {
	case Webhook, *Webhook:
		return "webhook"
	case Email, *Email:
		return "email"
	case LogNotifier, *LogNotifier:
		return "log"
	}
	return "other"
}

// stringify renders a details value for a person: strings as they are, everything else as JSON.
func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
