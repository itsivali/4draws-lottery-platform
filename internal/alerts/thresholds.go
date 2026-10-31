package alerts

import "time"

// Thresholds are the waits after which the checks of ADR 0025 warn and page. The zero value is not usable: start from
// DefaultThresholds (config.Alerts reads the same fields from the environment, see docs/runbooks/alerts.md).
type Thresholds struct {
	// A top-up M-Pesa may have taken but never confirmed warns TopUpUnconfirmedWarn after it was started and pages
	// after TopUpUnconfirmedPage.
	TopUpUnconfirmedWarn time.Duration
	TopUpUnconfirmedPage time.Duration
	// A top-up still pending TopUpPendingGrace after its expiry is a stuck job (page).
	TopUpPendingGrace time.Duration
	// Paybill money in suspense warns after SuspenseWarn and pages after SuspensePage.
	SuspenseWarn time.Duration
	SuspensePage time.Duration
	// A River job still retrying at this attempt or later warns (a discarded job always pages).
	JobRetryWarnAttempts int
	// The identity review queue warns when its oldest submission is KYCQueueWarn old and pages at KYCQueuePage.
	KYCQueueWarn time.Duration
	KYCQueuePage time.Duration
	// A payout awaiting an operator warns after PayoutApprovalWarn and pages after PayoutApprovalPage.
	PayoutApprovalWarn time.Duration
	PayoutApprovalPage time.Duration
	// The B2C float warns below this many KES; 0 switches the check off.
	B2CFloatFloorKES int64
	// An account closure request still open after ClosureWaitWarn warns.
	ClosureWaitWarn time.Duration
	// Daraja counts as throttling us when DarajaThrottleCount job errors in the last DarajaThrottleWindow say so.
	DarajaThrottleWindow time.Duration
	DarajaThrottleCount  int
}

// DefaultThresholds are the owner's defaults of 2026-10-31.
func DefaultThresholds() Thresholds {
	return Thresholds{
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

// Lookback is how far back the checks look for things that happened (orphan callbacks, failed payouts, discarded
// jobs): such an alert clears when the thing is this old.
const Lookback = 24 * time.Hour
