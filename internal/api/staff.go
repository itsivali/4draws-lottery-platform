package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/4draws-lottery-platform/internal/alerts"
	"github.com/itsivali/4draws-lottery-platform/internal/aml"
	"github.com/itsivali/4draws-lottery-platform/internal/compliance"
	"github.com/itsivali/4draws-lottery-platform/internal/draws"
	"github.com/itsivali/4draws-lottery-platform/internal/finance"
	"github.com/itsivali/4draws-lottery-platform/internal/game"
	"github.com/itsivali/4draws-lottery-platform/internal/identity"
	"github.com/itsivali/4draws-lottery-platform/internal/kyc"
	"github.com/itsivali/4draws-lottery-platform/internal/metrics"
	"github.com/itsivali/4draws-lottery-platform/internal/oversight"
	"github.com/itsivali/4draws-lottery-platform/internal/payments"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/httpx"
	"github.com/itsivali/4draws-lottery-platform/internal/rg"
	"github.com/itsivali/4draws-lottery-platform/internal/staff"
	"github.com/itsivali/4draws-lottery-platform/internal/wallet"
)

// The back-office API (/v1/staff): the operations lotteryctl performs, for staff signed in with the staff realm
// (sessions opened with Google or Apple sign-in, ADR 0018). Every route needs a role; every action is audited by the service as the staff member
// ("staff:<id>"), so the two-person rules (draw sign-off, tax remittance) compare real people.

// StaffServices are the services the back office acts through. They run on their own database pool with the worker's
// role, because the player API's role may not change draws or payouts (00008 grants); in single-role mode they may
// share the API's pool.
type StaffServices struct {
	Identity *identity.Service
	KYC      *kyc.Service
	Payments *payments.Service
	Draws    *draws.Service
	Payouts  *payouts.Service
	Alerts   *alerts.Service
	Game     *game.Service
	Members  *staff.Service   // sign-in, sessions and members (ADR 0018, 0019)
	Finance  *finance.Service // the financial dashboard
	Metrics  *metrics.Service // the app metrics (admins)
	// Oversight reads plays, top-ups, the audit trail and the system's state for the back office.
	Oversight *oversight.Service
	// Jobs retries or cancels a background job (ADR 0036).
	Jobs *oversight.JobControl
	// RG reads a player's deposit limits and restrictions for their timeline (owner, 2026-10-31).
	RG *rg.Service
	// Gate is the operating mode (ADR 0027): its state, stop, resume and the two-admin live approval.
	Gate *compliance.Gate
	// AML is transaction monitoring (ADR 0033): its cases, holds and rule settings.
	AML *aml.Service
}

type staffKey struct{}

func staffFrom(ctx context.Context) (staff.Member, bool) {
	m, ok := ctx.Value(staffKey{}).(staff.Member)
	return m, ok
}

const (
	staffPrefix  = "/v1/staff/"
	staffSession = "/v1/staff/session"
	// a forgotten password is reset without a session: the request is answered by email, the reset by the link's token
	staffForgot = "/v1/staff/password/forgot"
	staffReset  = "/v1/staff/password/reset"
)

// staffAuth authenticates staff tokens on /v1/staff routes, before idempotency (so keys are scoped to the staff
// member). Player tokens never authenticate here: they are not issued by the staff realm.
func (h *handlers) staffAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, staffPrefix) || (r.Method == http.MethodPost && (r.URL.Path == staffSession || r.URL.Path == staffForgot || r.URL.Path == staffReset)) {
			next.ServeHTTP(w, r) // signing in (or resetting a forgotten password) is how a staff member gets a token
			return
		}
		auth := h.Staff
		if auth == nil {
			auth = staff.Unavailable{}
		}
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		m, err := auth.Authenticate(r.Context(), tok)
		switch {
		case err == nil:
		case errors.Is(err, staff.ErrUnavailable):
			httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "staff-unavailable", "Staff sign-in is not configured", "", 60)
			return
		case errors.Is(err, staff.ErrNoRole):
			httpx.WriteProblem(w, http.StatusForbidden, "forbidden", "No back-office role", "ask an administrator for a role")
			return
		default:
			w.Header().Set("WWW-Authenticate", "Bearer")
			httpx.WriteProblem(w, http.StatusUnauthorized, "unauthenticated", "Staff authentication required", "")
			return
		}
		if !h.limit(w, r, "staff:"+m.ID, 600, time.Minute) {
			return
		}
		// a temporary password opens the door only to changing it (and to seeing who one is, and leaving)
		if m.MustChangePassword && !allowedBeforePasswordChange(r) {
			httpx.WriteProblem(w, http.StatusForbidden, "password-change-required", "Change your temporary password first",
				"set your own password (PUT /v1/staff/password) before anything else")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), staffKey{}, m)))
	})
}

func allowedBeforePasswordChange(r *http.Request) bool {
	switch {
	case r.URL.Path == staffPrefix+"me" && r.Method == http.MethodGet,
		r.URL.Path == staffPrefix+"password" && r.Method == http.MethodPut,
		r.URL.Path == staffSession && r.Method == http.MethodDelete:
		return true
	}
	return false
}

// requireRole lets the request through only for members holding one of roles.
func requireRole(roles ...staff.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			m, ok := staffFrom(r.Context())
			if !ok {
				httpx.WriteProblem(w, http.StatusUnauthorized, "unauthenticated", "Staff authentication required", "")
				return
			}
			for _, role := range roles {
				if m.Has(role) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.WriteProblem(w, http.StatusForbidden, "forbidden", "Your role does not allow this", "")
		})
	}
}

func (h *handlers) mountStaff(r chi.Router) {
	signIn := h.StaffServices.Members != nil
	admin, anyStaff := requireRole(staff.Admin), requireRole(staff.Staff) // admins pass anyStaff too (staff.Member.Has)
	r.Post("/session", h.needs(signIn, "Staff sign-in", h.staffSignIn))
	r.Delete("/session", h.needs(signIn, "Staff sign-in", h.staffSignOut))
	r.Get("/me", h.staffMe)
	r.Put("/password", h.needs(signIn, "Staff sign-in", h.staffChangePassword))
	r.Post("/password/forgot", h.needs(signIn, "Staff sign-in", h.staffForgotPassword))
	r.Post("/password/reset", h.needs(signIn, "Staff sign-in", h.staffResetWithToken))
	r.Put("/me/name", h.needs(signIn, "Staff sign-in", h.staffSetName(false)))
	r.With(admin).Get("/members", h.needs(signIn, "Staff members", h.staffMembers))
	r.With(admin).Post("/members", h.needs(signIn, "Staff members", h.staffInvite))
	r.With(admin).Put("/members/{id}/roles", h.needs(signIn, "Staff members", h.staffSetRoles))
	r.With(admin).Post("/members/{id}/disable", h.needs(signIn, "Staff members", h.staffSetActive(false)))
	r.With(admin).Post("/members/{id}/enable", h.needs(signIn, "Staff members", h.staffSetActive(true)))
	r.With(admin).Post("/members/{id}/password", h.needs(signIn, "Staff members", h.staffResetPassword))
	r.With(admin).Put("/members/{id}/name", h.needs(signIn, "Staff members", h.staffSetName(true)))
	r.With(admin).Get("/metrics", h.needs(h.StaffServices.Metrics != nil, "App metrics", h.staffMetrics))
	r.With(anyStaff).Get("/finance", h.needs(h.StaffServices.Finance != nil, "Finance", h.staffFinance))
	r.With(anyStaff).Get("/finance/export", h.needs(h.StaffServices.Finance != nil, "Finance", h.staffFinanceExport))
	r.With(anyStaff).Get("/ledger/export", h.needs(h.StaffServices.Finance != nil, "Finance", h.staffLedgerExport))
	r.With(anyStaff).Get("/reconciliations", h.needs(h.StaffServices.Oversight != nil, "Reconciliations", h.staffReconciliations))

	r.With(anyStaff).Get("/kyc/queue", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCQueue))
	r.With(anyStaff).Get("/kyc/pending", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCPending))
	r.With(anyStaff).Post("/kyc/{user}/approve", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCReview(true)))
	r.With(anyStaff).Post("/kyc/{user}/reject", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCReview(false)))

	r.With(anyStaff).Post("/payments/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Payments.Reconcile(ctx) }, h.StaffServices.Payments != nil))
	r.With(anyStaff).Post("/payments/reversals", h.staffReverse)
	r.With(anyStaff).Post("/payments/{id}/resolve", h.staffResolve)
	// top-up reconciliation (owner, 2026-10-31): the statement, and one top-up at a time
	r.With(anyStaff).Post("/payments/statement/match", h.staffStatementMatch)
	r.With(anyStaff).Post("/payments/statement/apply", h.staffStatementApply)
	r.With(anyStaff).Get("/payments/statements", h.staffStatementImports)
	r.With(anyStaff).Post("/topups/{id}/recheck", h.staffTopUpRecheck)
	r.With(anyStaff).Post("/topups/{id}/resolve", h.staffTopUpResolve)
	r.With(anyStaff).Post("/topups/{id}/dismiss", h.staffTopUpDismiss)
	r.With(anyStaff).Post("/suspense/{receipt}/credit", h.staffSuspenseCredit)
	r.With(anyStaff).Post("/suspense/{receipt}/refund", h.staffSuspenseRefund)
	r.With(anyStaff).Post("/paybill/{receipt}/recheck", h.staffRecheck)

	r.With(anyStaff).Get("/draws", h.staffDraws)
	r.With(anyStaff).Get("/draws/overdue", h.staffOverdue)
	r.With(anyStaff).Post("/draws/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Draws.Reconcile(ctx) }, h.StaffServices.Draws != nil))
	r.With(anyStaff).Post("/draws/{id}/sign", h.staffDrawStep("sign"))
	r.With(anyStaff).Post("/draws/{id}/approve", h.staffDrawStep("approve"))
	r.With(anyStaff).Post("/draws/{id}/void", h.staffDrawStep("void"))
	r.With(anyStaff).Post("/game/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Game.Reconcile(ctx) }, h.StaffServices.Game != nil))

	r.With(anyStaff).Get("/payouts", h.staffPayouts)
	r.With(anyStaff).Post("/payouts/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Payouts.Reconcile(ctx) }, h.StaffServices.Payouts != nil))
	r.With(anyStaff).Post("/payouts/{id}/approve", h.staffPayoutAction("approve"))
	r.With(anyStaff).Post("/payouts/{id}/reject", h.staffPayoutAction("reject"))
	r.With(anyStaff).Post("/payouts/{id}/settle", h.staffPayoutAction("settle"))
	r.With(anyStaff).Post("/prizes/{result}/retry", h.staffRetryPrize)

	r.With(anyStaff).Get("/tax/batches", h.staffTaxBatches)
	r.With(anyStaff).Post("/tax/batches", h.staffTaxPrepare)
	r.With(anyStaff).Get("/tax/batches/{no}/file", h.staffTaxFile)
	r.With(anyStaff).Post("/tax/batches/{no}/remit", h.staffTaxRemit)

	r.With(anyStaff).Get("/alerts", h.staffAlerts)
	r.With(anyStaff).Get("/alerts/{id}", h.staffAlert)
	r.With(anyStaff).Post("/alerts/{id}/acknowledge", h.staffAlertAck)

	hasGate := h.StaffServices.Gate != nil
	r.With(anyStaff).Get("/operating", h.needs(hasGate, "Operating mode", h.staffOperating))
	r.With(anyStaff).Post("/operating/stop", h.needs(hasGate, "Operating mode", h.staffOperatingStop))
	r.With(admin).Post("/operating/resume", h.needs(hasGate, "Operating mode", h.staffOperatingResume))
	r.With(admin).Post("/operating/live-requests", h.needs(hasGate, "Operating mode", h.staffLiveRequest))
	r.With(admin).Post("/operating/live-requests/{id}/approve", h.needs(hasGate, "Operating mode", h.staffLiveApprove))
	r.With(anyStaff).Get("/operating/switches", h.needs(hasGate, "Operating mode", h.staffSwitches))
	r.With(anyStaff).Post("/operating/switches/{area}/pause", h.needs(hasGate, "Operating mode", h.staffSwitch(true)))
	r.With(admin).Post("/operating/switches/{area}/resume", h.needs(hasGate, "Operating mode", h.staffSwitch(false)))

	r.With(anyStaff).Get("/closures", h.staffClosures)
	r.With(anyStaff).Post("/closures/{user}/close", h.staffClose)

	r.With(anyStaff).Get("/plays", h.needs(h.StaffServices.Oversight != nil, "Plays", h.staffPlays))
	r.With(anyStaff).Get("/topups", h.needs(h.StaffServices.Oversight != nil, "Top-ups", h.staffTopUps))
	r.With(anyStaff).Get("/topups/{id}", h.needs(h.StaffServices.Oversight != nil, "Top-ups", h.staffTopUp))
	r.With(anyStaff).Get("/system", h.needs(h.StaffServices.Oversight != nil, "System status", h.staffSystem))
	r.With(admin).Post("/jobs/{id}/retry", h.needs(h.StaffServices.Jobs != nil, "Background jobs", h.staffJob(true)))
	r.With(admin).Post("/jobs/{id}/cancel", h.needs(h.StaffServices.Jobs != nil, "Background jobs", h.staffJob(false)))
	r.With(admin).Get("/activity", h.needs(h.StaffServices.Oversight != nil, "Activity log", h.staffActivity))
	r.With(admin).Get("/activity/export", h.needs(h.StaffServices.Oversight != nil, "Activity log", h.staffActivityExport))

	r.With(anyStaff).Get("/players", h.staffPlayers)
	h.mountAML(r, anyStaff, admin)
	r.With(anyStaff).Get("/players/{user}", h.needs(h.StaffServices.Oversight != nil, "Players", h.staffPlayer))
	r.With(anyStaff).Get("/players/{user}/export", h.needs(h.StaffServices.Oversight != nil, "Players", h.staffPlayerExport))
	r.With(anyStaff).Post("/players/{user}/suspend", h.staffSuspend(true))
	r.With(anyStaff).Post("/players/{user}/reinstate", h.staffSuspend(false))
	r.With(anyStaff).Post("/players/{user}/password", h.staffPlayerPassword)
	// a balance the account cannot withdraw goes back to the number that paid it in (ADR 0031): an admin asks, a
	// second staff member approves the held payout
	r.With(admin).Post("/players/{user}/refund", h.staffRefundToSource)
	// a player who asks staff to exclude them (ADR 0032); there is no route to lift or shorten an exclusion
	r.With(anyStaff).Post("/players/{user}/self-exclusion", h.staffSelfExclude)
}

// needs answers 503 when the service behind a route is not configured.
func (h *handlers) needs(configured bool, what string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			unavailable(w, what)
			return
		}
		next(w, r)
	}
}

func member(r *http.Request) staff.Member { m, _ := staffFrom(r.Context()); return m }

func unavailable(w http.ResponseWriter, what string) {
	httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "unavailable", what+" is not configured", "", 60)
}

// staffError maps the services' refusals to problems. Anything unknown is a 500.
func staffError(w http.ResponseWriter, err error) {
	if status, slug, title, ok := complianceProblem(err); ok {
		httpx.WriteProblem(w, status, slug, title, "")
		return
	}
	if errors.Is(err, payouts.ErrPayeeIneligible) {
		httpx.WriteProblem(w, http.StatusConflict, "payee-ineligible", "The account cannot be paid now; the payout stays held",
			strings.TrimPrefix(err.Error(), payouts.ErrPayeeIneligible.Error()+": "))
		return
	}
	if errors.Is(err, staff.ErrSignInAgain) {
		httpx.WriteProblem(w, http.StatusForbidden, "sign-in-again", "Sign in again to set a password",
			"for your security, sign out, sign in with Google or Apple, then set it within 10 minutes")
		return
	}
	if errors.Is(err, staff.ErrWeakPassword) {
		httpx.WriteProblem(w, http.StatusBadRequest, "weak-password", "Password too weak", strings.TrimPrefix(err.Error(), "password: too weak: "))
		return
	}
	type m struct {
		status      int
		slug, title string
	}
	var known = []struct {
		err error
		m   m
	}{
		{kyc.ErrNothingToReview, m{http.StatusConflict, "nothing-to-review", "No submission awaits review"}},
		{staff.ErrExists, m{http.StatusConflict, "exists", "A member with this email address exists"}},
		{staff.ErrInvalid, m{http.StatusBadRequest, "invalid", "Invalid email address or role"}},
		{staff.ErrInvalidName, m{http.StatusBadRequest, "invalid-name", "A name is 1 to 80 characters"}},
		{staff.ErrInvalidToken, m{http.StatusGone, "invalid-token", "This reset link has expired or was already used"}},
		{identity.ErrNoSuchPlayer, m{http.StatusNotFound, "not-found", "No such player"}},
		{staff.ErrNotFound, m{http.StatusNotFound, "not-found", "No such member"}},
		{staff.ErrSelf, m{http.StatusConflict, "self", "You cannot change your own roles or disable yourself"}},
		{staff.ErrLastAdmin, m{http.StatusConflict, "last-admin", "The last admin cannot be removed"}},
		{staff.ErrWrongPassword, m{http.StatusForbidden, "wrong-password", "Current password is incorrect"}},
		{metrics.ErrRange, m{http.StatusBadRequest, "invalid-range", "The range must run forward and cover at most 366 days"}},
		{finance.ErrRange, m{http.StatusBadRequest, "invalid-range", "The range must run forward and cover at most 366 days"}},
		{finance.ErrGroup, m{http.StatusBadRequest, "invalid-group", "Group must be day, week or month"}},
		{finance.ErrTooManyRows, m{http.StatusBadRequest, "too-many-rows", "Too many ledger entries for one file; narrow the range"}},
		{payments.ErrNotFound, m{http.StatusNotFound, "not-found", "Not found"}},
		{payments.ErrNotReversible, m{http.StatusConflict, "state", "Only a credited payment can be reversed"}},
		{payments.ErrNotResolvable, m{http.StatusConflict, "state", "Only a settled, uncredited top-up can be resolved"}},
		{payments.ErrInvalidReceipt, m{http.StatusBadRequest, "invalid-receipt", "An M-Pesa receipt is 10 letters and digits"}},
		{payments.ErrSettledAlready, m{http.StatusConflict, "state", "A credited or reversed top-up is not checked again"}},
		{payments.ErrNotDismissable, m{http.StatusConflict, "state", "Only a settled, uncredited top-up can be dismissed"}},
		{payments.ErrDarajaUnavailable, m{http.StatusServiceUnavailable, "daraja-unavailable", "M-Pesa did not answer; ask again in a few minutes"}},
		{payments.ErrStatementEmpty, m{http.StatusBadRequest, "statement-empty", "The file has no statement rows the platform can read"}},
		{payments.ErrStatementTooLarge, m{http.StatusBadRequest, "statement-too-large", "The statement has too many rows; split it"}},
		{oversight.ErrNotFound, m{http.StatusNotFound, "not-found", "No such top-up"}},
		{payments.ErrReceiptUsed, m{http.StatusConflict, "receipt-used", "This receipt already credited another payment"}},
		{payments.ErrReasonRequired, m{http.StatusBadRequest, "reason-required", "A reason is required"}},
		{payments.ErrNotInSuspense, m{http.StatusNotFound, "not-found", "No open suspense item for this receipt"}},
		{payments.ErrNotRecheckable, m{http.StatusConflict, "state", "Only an unverified paybill payment can be checked again"}},
		{payments.ErrRefundRefNeeded, m{http.StatusBadRequest, "refund-ref-required", "The M-Pesa reference of the refund is required"}},
		{payments.ErrInvalidAmount, m{http.StatusBadRequest, "invalid-amount", "Invalid amount"}},
		{draws.ErrNotFound, m{http.StatusNotFound, "not-found", "No such draw"}},
		{draws.ErrBadCursor, m{http.StatusBadRequest, "invalid-cursor", "Invalid cursor"}},
		{payouts.ErrBadCursor, m{http.StatusBadRequest, "invalid-cursor", "Invalid cursor"}},
		{identity.ErrBadCursor, m{http.StatusBadRequest, "invalid-cursor", "Invalid cursor"}},
		{identity.ErrNotSuspendable, m{http.StatusConflict, "state", "The account is not in a state that allows this"}},
		{identity.ErrReasonRequired, m{http.StatusBadRequest, "reason-required", "A reason is required"}},
		{draws.ErrState, m{http.StatusConflict, "state", "The draw is not in a state that allows this"}},
		{draws.ErrTooLate, m{http.StatusConflict, "too-late", "The drand round is published; the outcome is public"}},
		{draws.ErrAlreadySigned, m{http.StatusConflict, "already-signed", "You have already signed this step; a second official must"}},
		{draws.ErrNoPrizeVersion, m{http.StatusConflict, "no-prize-structure", "No prize structure in force"}},
		{payouts.ErrNotFound, m{http.StatusNotFound, "not-found", "No such payout"}},
		{payouts.ErrState, m{http.StatusConflict, "state", "The payout is not in a state that allows this"}},
		{payouts.ErrOperator, m{http.StatusBadRequest, "reason-required", "A reason is required"}},
		{payouts.ErrInvalidReceipt, m{http.StatusBadRequest, "invalid-receipt", "An M-Pesa receipt is 10 letters and digits"}},
		{payouts.ErrReceiptUsed, m{http.StatusConflict, "receipt-used", "This receipt already settled another payout"}},
		{payouts.ErrCheckPending, m{http.StatusConflict, "check-pending", "M-Pesa has not answered about the last receipt yet; try again in a few minutes"}},
		{payouts.ErrNoAnswer, m{http.StatusServiceUnavailable, "daraja-unavailable", "M-Pesa did not answer; ask again in a few minutes"}},
		{payouts.ErrReceiptRequired, m{http.StatusBadRequest, "receipt-required", "The M-Pesa receipt of the payment is required"}},
		{payouts.ErrUnavailable, m{http.StatusServiceUnavailable, "payouts-unavailable", "Payouts are unavailable"}},
		{payouts.ErrPayable, m{http.StatusConflict, "payable", "The account can withdraw its balance itself; a refund is only for balances it cannot"}},
		{payouts.ErrNothingToRefund, m{http.StatusConflict, "nothing-to-refund", "The wallet holds nothing to refund"}},
		{payouts.ErrNoSource, m{http.StatusConflict, "no-source", "No credited top-up names a number to refund to"}},
		{payouts.ErrSameApprover, m{http.StatusConflict, "two-person-rule", "A second staff member must do this: you already approved, proposed or asked for it"}},
		{payouts.ErrInDebt, m{http.StatusConflict, "in-debt", "The player owes money from a reversed top-up; settle it first"}},
		{payouts.ErrNotRetryable, m{http.StatusConflict, "state", "Only a prize whose payout failed can be paid again"}},
		{payouts.ErrNothingToBatch, m{http.StatusConflict, "nothing-to-batch", "No withholding tax waits to be batched"}},
		{payouts.ErrRemitted, m{http.StatusConflict, "state", "The batch is already remitted"}},
		{payouts.ErrRemittanceAmount, m{http.StatusBadRequest, "remittance-amount", "The amount remitted must be exactly the batch's tax"}},
		{payouts.ErrSamePerson, m{http.StatusConflict, "two-person-rule", "Someone other than who prepared the batch must record the remittance"}},
		{payouts.ErrPRNUsed, m{http.StatusConflict, "prn-used", "That payment registration number is already recorded"}},
		{payouts.ErrBatchAltered, m{http.StatusConflict, "batch-altered", "The batch's events no longer match its recorded hash"}},
		{alerts.ErrNotFound, m{http.StatusNotFound, "not-found", "No such alert"}},
		{alerts.ErrOperator, m{http.StatusBadRequest, "note-required", "A note is required"}},
		{alerts.ErrAcknowledged, m{http.StatusConflict, "state", "The alert is already acknowledged"}},
		{alerts.ErrClosed, m{http.StatusConflict, "state", "The alert is already resolved"}},
		{identity.ErrNoClosureRequest, m{http.StatusNotFound, "not-found", "No open closure request for this account"}},
		{oversight.ErrNoSuchPlayer, m{http.StatusNotFound, "not-found", "No such player"}},
		{oversight.ErrTooManyRows, m{http.StatusBadRequest, "too-many-rows", "Too many records for one file; narrow the filters"}},
		{oversight.ErrRange, m{http.StatusBadRequest, "invalid-range", "Days are YYYY-MM-DD and the range must run forward"}},
	}
	if errors.Is(err, identity.ErrClosureBlocked) {
		httpx.WriteProblem(w, http.StatusConflict, "closure-blocked", "The account cannot be closed yet", strings.TrimPrefix(err.Error(), identity.ErrClosureBlocked.Error()+": "))
		return
	}
	for _, k := range known {
		if errors.Is(err, k.err) {
			httpx.WriteProblem(w, k.m.status, k.m.slug, k.m.title, "")
			return
		}
	}
	internalError(w, err)
}

func (h *handlers) staffMe(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, memberJSON(member(r)))
}

// staffKYCQueue is what the back office polls to tell reviewers that submissions wait: counts only, not audited.
func (h *handlers) staffKYCQueue(w http.ResponseWriter, r *http.Request) {
	q, err := h.StaffServices.KYC.ReviewQueue(r.Context())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"waiting": q.Waiting, "oldest_submitted_at": q.Oldest})
}

func (h *handlers) staffKYCPending(w http.ResponseWriter, r *http.Request) {
	list, err := h.StaffServices.KYC.AwaitingReview(r.Context(), member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, map[string]any{"user_id": p.UserID, "msisdn": p.MSISDN, "national_id": p.IDNumber, "full_name": p.FullName,
			"date_of_birth": p.DateOfBirth, "submitted_at": p.SubmittedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"submissions": out})
}

func (h *handlers) staffKYCReview(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Reason string `json:"reason"`
		}
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Reason) == "" {
			httpx.WriteProblem(w, http.StatusBadRequest, "reason-required", "A reason is required", "say what was checked, or why it was refused")
			return
		}
		st, err := h.StaffServices.KYC.Review(r.Context(), chi.URLParam(r, "user"), member(r).Operator(), approve, in.Reason)
		if err != nil {
			staffError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": string(st)})
	}
}

func (h *handlers) staffReconcile(run func(context.Context) (any, error), configured bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !configured {
			unavailable(w, "This service")
			return
		}
		rep, err := run(r.Context())
		if err != nil {
			staffError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, rep)
	}
}

func (h *handlers) staffReverse(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Receipt string `json:"receipt"`
		Reason  string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	rev, err := h.StaffServices.Payments.Reverse(r.Context(), in.Receipt, in.Reason, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payment_id": rev.PaymentID, "user_id": rev.UserID, "taken_kes": rev.TakenKES, "debt_kes": rev.DebtKES})
}

func (h *handlers) staffResolve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Receipt   string `json:"receipt"`
		AmountKES int64  `json:"amount_kes"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	if err := h.StaffServices.Payments.ResolveUnconfirmed(r.Context(), chi.URLParam(r, "id"), in.Receipt, in.AmountKES, member(r).Operator()); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// staffTopUpRecheck asks M-Pesa about a top-up again and reports what it said.
func (h *handlers) staffTopUpRecheck(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	out, err := h.StaffServices.Payments.Recheck(r.Context(), chi.URLParam(r, "id"), member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// staffTopUpResolve credits a settled, uncredited top-up from a receipt the operator found.
func (h *handlers) staffTopUpResolve(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Receipt   string `json:"receipt"`
		AmountKES int64  `json:"amount_kes"`
		Note      string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	res := payments.Resolution{PaymentID: chi.URLParam(r, "id"), Receipt: in.Receipt, AmountKES: in.AmountKES, Note: in.Note, Source: "operator"}
	if err := h.StaffServices.Payments.Resolve(r.Context(), res, member(r).Operator()); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// staffTopUpDismiss closes an unconfirmed top-up the operator could not find on the statement.
func (h *handlers) staffTopUpDismiss(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	if err := h.StaffServices.Payments.Dismiss(r.Context(), chi.URLParam(r, "id"), in.Note, member(r).Operator()); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxStatementBytes bounds an uploaded M-Pesa statement (20,000 rows are well under this).
const maxStatementBytes = 8 << 20

// statementBody reads the statement from a multipart upload (field "file") or a raw text/csv body.
func statementBody(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxStatementBytes)
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		f, hdr, err := r.FormFile("file")
		if err != nil {
			httpx.WriteProblem(w, http.StatusBadRequest, "no-file", "Attach the statement as the file field", "")
			return nil, "", false
		}
		defer func() { _ = f.Close() }()
		body, err := io.ReadAll(f)
		if err != nil {
			httpx.WriteProblem(w, http.StatusBadRequest, "unreadable-file", "The file could not be read", "")
			return nil, "", false
		}
		return body, hdr.Filename, true
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.WriteProblem(w, http.StatusRequestEntityTooLarge, "too-large", "The statement is too large", "")
		return nil, "", false
	}
	return body, "", true
}

// staffStatementMatch matches an uploaded M-Pesa statement to the platform's prompts; nothing is credited.
func (h *handlers) staffStatementMatch(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	body, filename, ok := statementBody(w, r)
	if !ok {
		return
	}
	rep, err := h.StaffServices.Payments.MatchStatement(r.Context(), body)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"filename": filename, "report": rep})
}

// staffStatementApply credits the top-ups the operator confirmed from a matched statement.
func (h *handlers) staffStatementApply(w http.ResponseWriter, r *http.Request) {
	var in payments.StatementApply
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	out, err := h.StaffServices.Payments.ApplyStatement(r.Context(), in, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// staffStatementImports lists past statement imports.
func (h *handlers) staffStatementImports(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	out, err := h.StaffServices.Payments.StatementImports(r.Context(), limit)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"imports": out})
}

func (h *handlers) staffSuspenseCredit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID string `json:"user_id"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	id, err := h.StaffServices.Payments.ResolveSuspense(r.Context(), chi.URLParam(r, "receipt"), in.UserID, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"payment_id": id})
}

func (h *handlers) staffSuspenseRefund(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefundRef string `json:"refund_ref"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	if err := h.StaffServices.Payments.RefundSuspense(r.Context(), chi.URLParam(r, "receipt"), in.RefundRef, member(r).Operator()); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) staffRecheck(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payments == nil {
		unavailable(w, "Payments")
		return
	}
	if err := h.StaffServices.Payments.RecheckC2B(r.Context(), chi.URLParam(r, "receipt"), member(r).Operator()); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) staffDraws(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Draws == nil {
		unavailable(w, "Draws")
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	page, err := h.StaffServices.Draws.List(r.Context(), limit, r.URL.Query().Get("before"))
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, drawPageJSON(page))
}

func (h *handlers) staffOverdue(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Draws == nil {
		unavailable(w, "Draws")
		return
	}
	late, err := h.StaffServices.Draws.Overdue(r.Context())
	if err != nil {
		staffError(w, err)
		return
	}
	if late == nil {
		late = []draws.Late{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"overdue": late})
}

func (h *handlers) staffDrawStep(step string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Reason string `json:"reason"`
		}
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		if h.StaffServices.Draws == nil {
			unavailable(w, "Draws")
			return
		}
		id, official := chi.URLParam(r, "id"), member(r).Operator()
		var d draws.Draw
		var err error
		switch step {
		case "sign":
			d, err = h.StaffServices.Draws.Sign(r.Context(), id, official)
		case "approve":
			d, err = h.StaffServices.Draws.Approve(r.Context(), id, official)
		case "void":
			if strings.TrimSpace(in.Reason) == "" {
				httpx.WriteProblem(w, http.StatusBadRequest, "reason-required", "A reason is required to void a draw", "")
				return
			}
			d, err = h.StaffServices.Draws.Void(r.Context(), id, official, in.Reason)
		}
		if err != nil {
			staffError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, toDrawJSON(d))
	}
}

func (h *handlers) staffPayouts(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "held"
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	list, next, err := h.StaffServices.Payouts.InState(r.Context(), state, limit, r.URL.Query().Get("after"))
	if err != nil {
		staffError(w, err)
		return
	}
	if list == nil {
		list = []payouts.Payout{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payouts": list, "next_after": nullable(next)})
}

func (h *handlers) staffPayoutAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Reason  string `json:"reason"`
			Paid    *bool  `json:"paid"`
			Receipt string `json:"receipt"`
		}
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		if h.StaffServices.Payouts == nil {
			unavailable(w, "Payouts")
			return
		}
		id, op := chi.URLParam(r, "id"), member(r).Operator()
		var p payouts.Payout
		var err error
		switch action {
		case "approve":
			p, err = h.StaffServices.Payouts.Approve(r.Context(), id, op)
		case "reject":
			p, err = h.StaffServices.Payouts.Reject(r.Context(), id, op, in.Reason)
		case "settle":
			if in.Paid == nil {
				httpx.WriteProblem(w, http.StatusBadRequest, "paid-required", "Say whether M-Pesa paid it", "")
				return
			}
			p, err = h.StaffServices.Payouts.Settle(r.Context(), id, op, *in.Paid, in.Receipt, in.Reason)
		}
		if err != nil {
			staffError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, p)
	}
}

func (h *handlers) staffSelfExclude(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Term   string `json:"term"`
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.RG == nil {
		unavailable(w, "Responsible gaming")
		return
	}
	err := h.RG.SelfExcludeFor(r.Context(), chi.URLParam(r, "user"), in.Term, member(r).Operator(), in.Reason)
	var pe *pgconn.PgError
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": chi.URLParam(r, "user"), "term": in.Term})
	case errors.Is(err, rg.ErrInvalidDuration):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-term", "The term is 6m, 1y or permanent", "")
	case errors.Is(err, rg.ErrReasonRequired):
		httpx.WriteProblem(w, http.StatusBadRequest, "reason-required", "A reason is required", "")
	case errors.As(err, &pe) && (pe.Code == "22P02" || pe.Code == "23503"):
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such player", "")
	default:
		staffError(w, err)
	}
}

func (h *handlers) staffRefundToSource(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	p, err := h.StaffServices.Payouts.RefundToSource(r.Context(), chi.URLParam(r, "user"), member(r).Operator(), in.Reason, r.Header.Get("Idempotency-Key"))
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (h *handlers) staffRetryPrize(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	p, err := h.StaffServices.Payouts.RetryPrize(r.Context(), chi.URLParam(r, "result"), member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (h *handlers) staffTaxBatches(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	list, err := h.StaffServices.Payouts.Batches(r.Context(), 50)
	if err != nil {
		staffError(w, err)
		return
	}
	if list == nil {
		list = []payouts.Batch{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"batches": list})
}

func (h *handlers) staffTaxPrepare(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	b, _, err := h.StaffServices.Payouts.PrepareBatch(r.Context(), member(r).Operator(), 10000)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, b)
}

func batchNo(w http.ResponseWriter, r *http.Request) (int64, bool) {
	n, err := strconv.ParseInt(chi.URLParam(r, "no"), 10, 64)
	if err != nil || n < 1 {
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such batch", "")
		return 0, false
	}
	return n, true
}

func (h *handlers) staffTaxFile(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	no, ok := batchNo(w, r)
	if !ok {
		return
	}
	b, file, err := h.StaffServices.Payouts.BatchFile(r.Context(), no)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such batch", "")
		return
	}
	if err != nil {
		staffError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="kra-batch-`+strconv.FormatInt(b.No, 10)+`.csv"`)
	w.Header().Set("X-File-SHA256", b.SHA256)
	_, _ = w.Write(file)
}

func (h *handlers) staffTaxRemit(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PRN       string `json:"prn"`
		AmountKES int64  `json:"amount_kes"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Payouts == nil {
		unavailable(w, "Payouts")
		return
	}
	no, ok := batchNo(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(in.PRN) == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "prn-required", "The payment registration number is required", "")
		return
	}
	b, err := h.StaffServices.Payouts.RecordRemittance(r.Context(), no, in.PRN, in.AmountKES, member(r).Operator())
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such batch", "")
		return
	}
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, b)
}

// staffAlerts lists alerts, newest first, by kind, severity and state (open unless said otherwise; all=true is
// state=all, kept for older clients), with the open count per kind for the filter bar.
func (h *handlers) staffAlerts(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Alerts == nil {
		unavailable(w, "Alerts")
		return
	}
	qs := r.URL.Query()
	f := alerts.Filter{Kind: qs.Get("kind"), Before: qs.Get("before"), Limit: 100}
	state, ok := alerts.ParseState(qs.Get("state"))
	if !ok {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-state", "Invalid state", "use open, acknowledged, resolved or all")
		return
	}
	if qs.Get("all") == "true" && qs.Get("state") == "" {
		state = alerts.All
	}
	f.State = state
	switch sev := qs.Get("severity"); sev {
	case "":
	case string(alerts.Page), string(alerts.Warn):
		f.Severity = alerts.Severity(sev)
	default:
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-severity", "Invalid severity", "use page or warn")
		return
	}
	if v := qs.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpx.WriteProblem(w, http.StatusBadRequest, "invalid-limit", "Invalid limit", "use 1 to 200")
			return
		}
		f.Limit = n
	}
	list, err := h.StaffServices.Alerts.List(r.Context(), f)
	if errors.Is(err, alerts.ErrCursor) {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-cursor", "Invalid cursor", "use next_before from the previous page")
		return
	} else if err != nil {
		staffError(w, err)
		return
	}
	kinds, err := h.StaffServices.Alerts.Kinds(r.Context())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"alerts": list.Alerts, "next_before": nextOf(list.NextBefore), "kinds": kinds})
}

// staffAlert is one alert with its history and the check that owns it.
func (h *handlers) staffAlert(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Alerts == nil {
		unavailable(w, "Alerts")
		return
	}
	d, err := h.StaffServices.Alerts.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *handlers) staffAlertAck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.StaffServices.Alerts == nil {
		unavailable(w, "Alerts")
		return
	}
	v, err := h.StaffServices.Alerts.Acknowledge(r.Context(), chi.URLParam(r, "id"), member(r).Operator(), in.Note)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (h *handlers) staffClosures(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Identity == nil {
		unavailable(w, "Account closure")
		return
	}
	list, err := h.StaffServices.Identity.OpenClosures(r.Context(), 100, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{"user_id": c.UserID, "msisdn": c.MSISDN, "status": c.Status, "requested_at": c.RequestedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"closures": out})
}

// staffPlayers lists registered players, newest first, narrowed by a phone-number fragment and a status (ADR 0019).
func (h *handlers) staffPlayers(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Identity == nil {
		unavailable(w, "Players")
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	if st := qs.Get("status"); st != "" && st != "active" && st != "suspended" && st != "closed" {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-status", "Invalid status", "use active, suspended or closed")
		return
	}
	page, err := h.StaffServices.Identity.Players(r.Context(), identity.PlayerFilter{Query: qs.Get("q"), Status: qs.Get("status")}, limit, qs.Get("before"), member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	if page.Players == nil {
		page.Players = []identity.Player{}
	}
	// names and ID numbers come from the latest identity submission (decrypted and audited as unmasked)
	if h.StaffServices.KYC != nil && len(page.Players) > 0 {
		ids := make([]string, 0, len(page.Players))
		for _, p := range page.Players {
			ids = append(ids, p.UserID)
		}
		who, err := h.StaffServices.KYC.Identities(r.Context(), ids, member(r).Operator())
		if err != nil {
			staffError(w, err)
			return
		}
		for i := range page.Players {
			if id, ok := who[page.Players[i].UserID]; ok {
				page.Players[i].FullName, page.Players[i].NationalID = id.FullName, id.IDNumber
			}
		}
	}
	var next *string
	if page.NextBefore != "" {
		next = &page.NextBefore
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"players": page.Players, "next_before": next})
}

// staffSuspend suspends or reinstates a player's account, with a reason for the audit log.
func (h *handlers) staffSuspend(suspend bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.StaffServices.Identity == nil {
			unavailable(w, "Players")
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		if err := h.StaffServices.Identity.Suspend(r.Context(), chi.URLParam(r, "user"), member(r).Operator(), in.Reason, suspend); err != nil {
			staffError(w, err)
			return
		}
		status := "active"
		if suspend {
			status = "suspended"
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": chi.URLParam(r, "user"), "status": status})
	}
}

func (h *handlers) staffClose(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Identity == nil || h.StaffServices.Payments == nil || h.StaffServices.Payouts == nil {
		unavailable(w, "Account closure")
		return
	}
	m := member(r)
	at, err := h.StaffServices.Identity.CloseAccount(r.Context(), chi.URLParam(r, "user"), m.ID, m.Operator(), h.closureGuard)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"closed_at": at})
}

// closureGuard keeps an account open while money is in it, still moving, owed by the player, or still to be won or
// paid from a draw: a balance (pay it out first), debt from a reversed top-up, a top-up or payout not final, tickets in
// a draw still taking entries, entries in a draw not yet paid, or a prize not yet paid.
func (h *handlers) closureGuard(ctx context.Context, tx pgx.Tx, userID string) error {
	w := wallet.NewService(nil)
	bal, err := w.Balance(ctx, tx, userID)
	if err != nil {
		return err
	}
	if bal > 0 {
		return fmt.Errorf("%w: the wallet still holds KES %d; pay it out first", identity.ErrClosureBlocked, bal)
	}
	debt, err := w.Debt(ctx, tx, userID)
	if err != nil {
		return err
	}
	if debt > 0 {
		return fmt.Errorf("%w: the player owes KES %d from a reversed top-up; settle it first", identity.ErrClosureBlocked, debt)
	}
	if n, err := h.StaffServices.Payments.Unsettled(ctx, tx, userID); err != nil {
		return err
	} else if n > 0 {
		return fmt.Errorf("%w: %d top-up(s) are still being settled", identity.ErrClosureBlocked, n)
	}
	if n, err := h.StaffServices.Payouts.Unsettled(ctx, tx, userID); err != nil {
		return err
	} else if n > 0 {
		return fmt.Errorf("%w: %d payout(s) are not final yet", identity.ErrClosureBlocked, n)
	}
	pi, err := h.StaffServices.Payouts.PrizeInterestOf(ctx, tx, userID)
	if err != nil {
		return err
	}
	if pi.Any() {
		return fmt.Errorf("%w: the player still has chances or prizes in draws (%d tickets in open draws, %d entries in draws not yet paid, %d prizes not yet paid); close it once they are settled",
			identity.ErrClosureBlocked, pi.OpenDrawTickets, pi.UndecidedEntries, pi.UnpaidPrizes)
	}
	return nil
}

// staffSignIn opens a back-office session: with the member's email address and password (provider "password",
// ADR 0019), or with a Google or Apple ID token of an invited member (ADR 0018).
func (h *handlers) staffSignIn(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
		IDToken  string `json:"id_token"`
		Nonce    string `json:"nonce"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "staff-sign-in:ip:"+h.clientIP(r), 30, time.Hour) {
		return
	}
	var s staff.Session
	var err error
	if in.Provider == "password" {
		email := strings.ToLower(strings.TrimSpace(in.Email))
		if email == "" || in.Password == "" {
			httpx.WriteProblem(w, http.StatusBadRequest, "invalid-request", "Email address and password are required", "")
			return
		}
		if !h.limit(w, r, "staff-sign-in:email:"+email, 10, time.Hour) {
			return
		}
		s, err = h.StaffServices.Members.SignInWithPassword(r.Context(), email, in.Password)
	} else {
		s, err = h.StaffServices.Members.SignIn(r.Context(), in.Provider, in.IDToken, in.Nonce)
	}
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusCreated, map[string]any{"token": s.Token, "expires_at": s.ExpiresAt, "member": memberJSON(s.Member)})
	case errors.Is(err, staff.ErrUnavailable):
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "staff-unavailable", "Google and Apple sign-in are not configured", "sign in with your email address and password", 60)
	case errors.Is(err, staff.ErrNotInvited):
		httpx.WriteProblem(w, http.StatusForbidden, "not-invited", "This account is not a staff member",
			"ask an admin to invite the email address of the Google or Apple account you used")
	case errors.Is(err, staff.ErrDisabled):
		httpx.WriteProblem(w, http.StatusForbidden, "disabled", "This staff member is disabled", "")
	case errors.Is(err, staff.ErrNoRole):
		httpx.WriteProblem(w, http.StatusForbidden, "forbidden", "No back-office role", "ask an admin for a role")
	case errors.Is(err, staff.ErrUnauthenticated):
		w.Header().Set("WWW-Authenticate", "Bearer")
		if in.Provider == "password" {
			httpx.WriteProblem(w, http.StatusUnauthorized, "invalid-credentials", "Wrong email address or password", "")
		} else {
			httpx.WriteProblem(w, http.StatusUnauthorized, "unauthenticated", "The sign-in could not be verified", "sign in again")
		}
	default:
		internalError(w, err)
	}
}

// staffChangePassword sets the signed-in member's own password; a temporary one must be changed through here first.
func (h *handlers) staffChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	m := member(r)
	if !h.limit(w, r, "staff-pwchange:"+m.ID, 10, time.Hour) {
		return
	}
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := h.StaffServices.Members.ChangePassword(r.Context(), m, tok, in.Current, in.New); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// staffSetName sets the name the back office shows: the signed-in member's own, or (admin) any member's.
func (h *handlers) staffSetName(anyMember bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
		}
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		by := member(r)
		id := by.ID
		if anyMember {
			id = chi.URLParam(r, "id")
		}
		rec, err := h.StaffServices.Members.SetName(r.Context(), by, id, in.Name)
		if err != nil {
			staffError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, rec)
	}
}

// staffForgotPassword emails a reset link to a member who forgot their password. It always answers 202 so the
// address book stays private; the per-address budget stops it being used to flood someone's inbox.
func (h *handlers) staffForgotPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "staff-forgot:ip:"+h.clientIP(r), 10, time.Hour) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-request", "An email address is required", "")
		return
	}
	if !h.limit(w, r, "staff-forgot:email:"+email, 3, time.Hour) {
		return
	}
	switch err := h.StaffServices.Members.RequestPasswordReset(r.Context(), email); {
	case err == nil:
		w.WriteHeader(http.StatusAccepted)
	case errors.Is(err, staff.ErrUnavailable):
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "staff-unavailable", "Password reset by email is not configured", "ask an administrator for a temporary password", 60)
	default:
		internalError(w, err)
	}
}

// staffResetWithToken sets a new password from an emailed link.
func (h *handlers) staffResetWithToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
		New   string `json:"new_password"`
	}
	if !httpx.DecodeJSON(w, r, &in) || !h.limit(w, r, "staff-reset:ip:"+h.clientIP(r), 20, time.Hour) {
		return
	}
	if err := h.StaffServices.Members.ResetPasswordWithToken(r.Context(), in.Token, in.New); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// staffPlayerPassword is staff giving a player a temporary password (players have no email to send a link to).
func (h *handlers) staffPlayerPassword(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Identity == nil {
		unavailable(w, "Players")
		return
	}
	var in struct {
		Password string `json:"password"`
		Reason   string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.StaffServices.Identity.SetTemporaryPassword(r.Context(), chi.URLParam(r, "user"), member(r).Operator(), in.Reason, in.Password); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// staffResetPassword is an admin giving a member a temporary password.
func (h *handlers) staffResetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.StaffServices.Members.ResetPassword(r.Context(), member(r), chi.URLParam(r, "id"), in.Password); err != nil {
		staffError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// staffMetrics is the app metrics page over Nairobi days from..to (default: the last 30 days, today included).
func (h *handlers) staffMetrics(w http.ResponseWriter, r *http.Request) {
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	sum, err := h.StaffServices.Metrics.Summary(r.Context(), from, to)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sum)
}

// dateRange reads optional from and to (YYYY-MM-DD) query parameters.
func dateRange(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	for name, into := range map[string]*time.Time{"from": &from, "to": &to} {
		if v := r.URL.Query().Get(name); v != "" {
			t, err := time.Parse(time.DateOnly, v)
			if err != nil {
				httpx.WriteProblem(w, http.StatusBadRequest, "invalid-range", "Invalid date", name+" must be YYYY-MM-DD")
				return from, to, false
			}
			*into = t
		}
	}
	return from, to, true
}

func (h *handlers) staffSignOut(w http.ResponseWriter, r *http.Request) {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if err := h.StaffServices.Members.SignOut(r.Context(), tok); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func memberJSON(m staff.Member) map[string]any {
	roles := make([]string, 0, len(m.Roles))
	for _, role := range m.Roles {
		roles = append(roles, string(role))
	}
	return map[string]any{"id": m.ID, "name": m.Name, "email": m.Email, "roles": roles, "must_change_password": m.MustChangePassword,
		"has_password": m.HasPassword, "photo_url": nonEmpty(m.PhotoURL)}
}

func (h *handlers) staffMembers(w http.ResponseWriter, r *http.Request) {
	list, err := h.StaffServices.Members.Members(r.Context())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"members": list, "roles": staff.AllRoles})
}

func (h *handlers) staffInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string   `json:"email"`
		Name     string   `json:"name"`
		Roles    []string `json:"roles"`
		Password string   `json:"password"` // a temporary password, told to the member out of band; optional
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	roles, err := staff.ParseRoles(in.Roles)
	if err == nil {
		var rec staff.Record
		if rec, err = h.StaffServices.Members.Invite(r.Context(), member(r), in.Email, in.Name, roles, in.Password); err == nil {
			httpx.WriteJSON(w, http.StatusCreated, rec)
			return
		}
	}
	staffError(w, err)
}

func (h *handlers) staffSetRoles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Roles []string `json:"roles"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	roles, err := staff.ParseRoles(in.Roles)
	if err == nil {
		var rec staff.Record
		if rec, err = h.StaffServices.Members.SetRoles(r.Context(), member(r), chi.URLParam(r, "id"), roles); err == nil {
			httpx.WriteJSON(w, http.StatusOK, rec)
			return
		}
	}
	staffError(w, err)
}

func (h *handlers) staffSetActive(active bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec, err := h.StaffServices.Members.SetActive(r.Context(), member(r), chi.URLParam(r, "id"), active)
		if err != nil {
			staffError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, rec)
	}
}

// staffFinance and the other Finance tab handlers are in staff_finance.go.
