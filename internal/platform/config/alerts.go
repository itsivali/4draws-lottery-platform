package config

import (
	"errors"
	"net/url"
	"os"
	"time"
)

// Alerts configures where alerts go (internal/alerts). Both URLs are optional: without a webhook, alerts are logged
// (ERROR "alert") and kept in audit.alerts; without a heartbeat, nothing notices a worker that stopped running.
type Alerts struct {
	// WebhookURL receives each alert as JSON with a "text" field (a Slack-compatible incoming webhook). It is a secret.
	WebhookURL string
	// HeartbeatURL is requested after every watch that ran all its checks: a dead man's switch (healthchecks.io and
	// similar) that pages when the pings stop. It is a secret.
	HeartbeatURL string
	// Withheld tax not yet remitted to KRA warns after KRARemitWarnAfter and pages after KRARemitDueAfter (placeholders
	// until a tax adviser confirms the deadline).
	KRARemitWarnAfter time.Duration
	KRARemitDueAfter  time.Duration
}

func loadAlerts() (Alerts, error) {
	a := Alerts{WebhookURL: os.Getenv("ALERT_WEBHOOK_URL"), HeartbeatURL: os.Getenv("ALERT_HEARTBEAT_URL"),
		KRARemitWarnAfter: 72 * time.Hour, KRARemitDueAfter: 120 * time.Hour}
	for _, u := range []string{a.WebhookURL, a.HeartbeatURL} {
		if u == "" {
			continue
		}
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "https" || p.Host == "" {
			return Alerts{}, errors.New("config: ALERT_WEBHOOK_URL and ALERT_HEARTBEAT_URL must be https URLs")
		}
	}
	for env, d := range map[string]*time.Duration{"KRA_REMIT_WARN_AFTER": &a.KRARemitWarnAfter, "KRA_REMIT_DUE_AFTER": &a.KRARemitDueAfter} {
		if v := os.Getenv(env); v != "" {
			parsed, err := time.ParseDuration(v)
			if err != nil || parsed <= 0 {
				return Alerts{}, errors.New("config: " + env + " must be a positive duration such as 72h")
			}
			*d = parsed
		}
	}
	if a.KRARemitWarnAfter > a.KRARemitDueAfter {
		return Alerts{}, errors.New("config: KRA_REMIT_WARN_AFTER must not be after KRA_REMIT_DUE_AFTER")
	}
	return a, nil
}
