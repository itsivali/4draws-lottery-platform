package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

// Message is one alert event to send: raised, reminder or resolved.
type Message struct {
	Event string `json:"event"`
	Text  string `json:"text"`
	Alert View   `json:"alert"`
}

// Notifier sends alert events to people.
type Notifier interface {
	Notify(ctx context.Context, m Message) error
}

// LogNotifier only logs (alerts are also kept in audit.alerts and listed by lotteryctl alerts list).
type LogNotifier struct{}

// Notify implements Notifier.
func (LogNotifier) Notify(ctx context.Context, m Message) error {
	slog.WarnContext(ctx, "alert event (no ALERT_WEBHOOK_URL: logged only)", "event", m.Event, "alert", m.Alert.ID, "text", m.Text)
	return nil
}

// Webhook posts each event as JSON with a "text" field, the shape Slack-compatible incoming webhooks accept, plus the
// event and the alert for receivers that read them. The URL is a secret and never logged.
type Webhook struct {
	URL    string
	Client *http.Client
}

// Notify implements Notifier.
func (w Webhook) Notify(ctx context.Context, m Message) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return errors.New("alerts: bad webhook URL")
	}
	req.Header.Set("Content-Type", "application/json")
	return do(w.Client, req, "webhook")
}

// Heartbeat returns a ping of a dead man's switch: a GET of url, which pages someone when the pings stop.
func Heartbeat(url string, c *http.Client) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return errors.New("alerts: bad heartbeat URL")
		}
		return do(c, req, "heartbeat")
	}
}

func do(c *http.Client, req *http.Request, what string) error {
	res, err := c.Do(req) //nolint:gosec // the configured alert endpoint (https, checked by config)
	if err != nil {
		return errors.New("alerts: " + what + " unreachable") // the error would print the secret URL
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return errors.New("alerts: " + what + " answered " + strconv.Itoa(res.StatusCode))
	}
	return nil
}
