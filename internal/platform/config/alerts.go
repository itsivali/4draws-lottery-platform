package config

import (
	"errors"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Alerts configures where alerts go and when the checks warn and page (internal/alerts, docs/runbooks/alerts.md).
// Every channel is optional: without any, alerts are logged (ERROR "alert") and kept in audit.alerts; without a
// heartbeat, nothing notices a worker that stopped running.
type Alerts struct {
	// WebhookURL receives each alert as JSON with a "text" field (a Slack-compatible incoming webhook). It is a secret.
	WebhookURL string
	// Emails are the on-call addresses (ALERT_EMAILS, comma-separated) that receive each alert as plain text through the
	// SMTP server of Mail; with no SMTP_HOST the emails are only logged.
	Emails []string
	// HeartbeatURL is requested after every watch that ran all its checks: a dead man's switch (healthchecks.io and
	// similar) that pages when the pings stop. It is a secret.
	HeartbeatURL string
	// Withheld tax not yet remitted to KRA warns after KRARemitWarnAfter and pages after KRARemitDueAfter (placeholders
	// until a tax adviser confirms the deadline).
	KRARemitWarnAfter time.Duration
	KRARemitDueAfter  time.Duration
	// Thresholds are the waits of the other checks (ADR 0025); the same fields as alerts.Thresholds.
	Thresholds AlertThresholds
}

// AlertThresholds mirrors alerts.Thresholds field for field (the app converts between them), read from the
// environment variables named in loadAlerts.
type AlertThresholds struct {
	TopUpUnconfirmedWarn time.Duration
	TopUpUnconfirmedPage time.Duration
	TopUpPendingGrace    time.Duration
	SuspenseWarn         time.Duration
	SuspensePage         time.Duration
	JobRetryWarnAttempts int
	KYCQueueWarn         time.Duration
	KYCQueuePage         time.Duration
	PayoutApprovalWarn   time.Duration
	PayoutApprovalPage   time.Duration
	B2CFloatFloorKES     int64
	ClosureWaitWarn      time.Duration
	DarajaThrottleWindow time.Duration
	DarajaThrottleCount  int
}

// defaultAlertThresholds are alerts.DefaultThresholds (kept identical by TestLoad_AlertThresholdDefaults in internal/app).
func defaultAlertThresholds() AlertThresholds {
	return AlertThresholds{
		TopUpUnconfirmedWarn: 30 * time.Minute, TopUpUnconfirmedPage: 4 * time.Hour, TopUpPendingGrace: 10 * time.Minute,
		SuspenseWarn: time.Hour, SuspensePage: 24 * time.Hour,
		JobRetryWarnAttempts: 5,
		KYCQueueWarn:         24 * time.Hour, KYCQueuePage: 72 * time.Hour,
		PayoutApprovalWarn: 2 * time.Hour, PayoutApprovalPage: 24 * time.Hour,
		B2CFloatFloorKES:     0,
		ClosureWaitWarn:      7 * 24 * time.Hour,
		DarajaThrottleWindow: 15 * time.Minute, DarajaThrottleCount: 5,
	}
}

func loadAlerts() (Alerts, error) {
	a := Alerts{WebhookURL: os.Getenv("ALERT_WEBHOOK_URL"), HeartbeatURL: os.Getenv("ALERT_HEARTBEAT_URL"),
		KRARemitWarnAfter: 72 * time.Hour, KRARemitDueAfter: 120 * time.Hour, Thresholds: defaultAlertThresholds()}
	for _, u := range []string{a.WebhookURL, a.HeartbeatURL} {
		if u == "" {
			continue
		}
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "https" || p.Host == "" {
			return Alerts{}, errors.New("config: ALERT_WEBHOOK_URL and ALERT_HEARTBEAT_URL must be https URLs")
		}
	}
	for _, addr := range strings.Split(os.Getenv("ALERT_EMAILS"), ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		if _, err := mail.ParseAddress(addr); err != nil {
			return Alerts{}, errors.New("config: ALERT_EMAILS must be email addresses separated by commas")
		}
		a.Emails = append(a.Emails, addr)
	}
	th := &a.Thresholds
	durations := map[string]*time.Duration{
		"KRA_REMIT_WARN_AFTER": &a.KRARemitWarnAfter, "KRA_REMIT_DUE_AFTER": &a.KRARemitDueAfter,
		"TOPUP_UNCONFIRMED_WARN_AFTER": &th.TopUpUnconfirmedWarn, "TOPUP_UNCONFIRMED_PAGE_AFTER": &th.TopUpUnconfirmedPage,
		"TOPUP_PENDING_GRACE": &th.TopUpPendingGrace,
		"SUSPENSE_WARN_AFTER": &th.SuspenseWarn, "SUSPENSE_PAGE_AFTER": &th.SuspensePage,
		"KYC_QUEUE_WARN_AFTER": &th.KYCQueueWarn, "KYC_QUEUE_PAGE_AFTER": &th.KYCQueuePage,
		"PAYOUT_APPROVAL_WARN_AFTER": &th.PayoutApprovalWarn, "PAYOUT_APPROVAL_PAGE_AFTER": &th.PayoutApprovalPage,
		"CLOSURE_WAIT_WARN_AFTER": &th.ClosureWaitWarn, "DARAJA_THROTTLE_WINDOW": &th.DarajaThrottleWindow,
	}
	for env, d := range durations {
		if v := os.Getenv(env); v != "" {
			parsed, err := time.ParseDuration(v)
			if err != nil || parsed <= 0 {
				return Alerts{}, errors.New("config: " + env + " must be a positive duration such as 72h")
			}
			*d = parsed
		}
	}
	counts := map[string]*int{"JOB_RETRY_WARN_ATTEMPTS": &th.JobRetryWarnAttempts, "DARAJA_THROTTLE_COUNT": &th.DarajaThrottleCount}
	for env, n := range counts {
		if v := os.Getenv(env); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil || parsed < 1 {
				return Alerts{}, errors.New("config: " + env + " must be a whole number of at least 1")
			}
			*n = parsed
		}
	}
	if v := os.Getenv("B2C_FLOAT_FLOOR_KES"); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil || parsed < 0 {
			return Alerts{}, errors.New("config: B2C_FLOAT_FLOOR_KES must be whole KES (0 switches the check off)")
		}
		th.B2CFloatFloorKES = parsed
	}
	for _, pair := range []struct {
		warn, page string
		w, p       time.Duration
	}{
		{"KRA_REMIT_WARN_AFTER", "KRA_REMIT_DUE_AFTER", a.KRARemitWarnAfter, a.KRARemitDueAfter},
		{"TOPUP_UNCONFIRMED_WARN_AFTER", "TOPUP_UNCONFIRMED_PAGE_AFTER", th.TopUpUnconfirmedWarn, th.TopUpUnconfirmedPage},
		{"SUSPENSE_WARN_AFTER", "SUSPENSE_PAGE_AFTER", th.SuspenseWarn, th.SuspensePage},
		{"KYC_QUEUE_WARN_AFTER", "KYC_QUEUE_PAGE_AFTER", th.KYCQueueWarn, th.KYCQueuePage},
		{"PAYOUT_APPROVAL_WARN_AFTER", "PAYOUT_APPROVAL_PAGE_AFTER", th.PayoutApprovalWarn, th.PayoutApprovalPage},
	} {
		if pair.w > pair.p {
			return Alerts{}, errors.New("config: " + pair.warn + " must not be after " + pair.page)
		}
	}
	return a, nil
}
