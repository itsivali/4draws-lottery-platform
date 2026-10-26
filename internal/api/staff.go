package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/alerts"
	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/game"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/kyc"
	"github.com/itsivali/lottery-platform/internal/payments"
	"github.com/itsivali/lottery-platform/internal/payouts"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
	"github.com/itsivali/lottery-platform/internal/staff"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// The back-office API (/v1/staff): the operations lotteryctl performs, for staff signed in with the staff realm
// (Keycloak). Every route needs a role; every action is audited by the service as the staff member
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
}

type staffKey struct{}

func staffFrom(ctx context.Context) (staff.Member, bool) {
	m, ok := ctx.Value(staffKey{}).(staff.Member)
	return m, ok
}

const staffPrefix = "/v1/staff/"

// staffAuth authenticates staff tokens on /v1/staff routes, before idempotency (so keys are scoped to the staff
// member). Player tokens never authenticate here: they are not issued by the staff realm.
func (h *handlers) staffAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, staffPrefix) {
			next.ServeHTTP(w, r)
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
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), staffKey{}, m)))
	})
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
	r.Get("/me", h.staffMe)

	r.With(requireRole(staff.KYCReviewer)).Get("/kyc/pending", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCPending))
	r.With(requireRole(staff.KYCReviewer)).Post("/kyc/{user}/approve", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCReview(true)))
	r.With(requireRole(staff.KYCReviewer)).Post("/kyc/{user}/reject", h.needs(h.StaffServices.KYC != nil, "KYC review", h.staffKYCReview(false)))

	r.With(requireRole(staff.PaymentsOperator, staff.Auditor)).Post("/payments/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Payments.Reconcile(ctx) }, h.StaffServices.Payments != nil))
	r.With(requireRole(staff.PaymentsOperator)).Post("/payments/reversals", h.staffReverse)
	r.With(requireRole(staff.PaymentsOperator)).Post("/payments/{id}/resolve", h.staffResolve)
	r.With(requireRole(staff.PaymentsOperator)).Post("/suspense/{receipt}/credit", h.staffSuspenseCredit)
	r.With(requireRole(staff.PaymentsOperator)).Post("/suspense/{receipt}/refund", h.staffSuspenseRefund)
	r.With(requireRole(staff.PaymentsOperator)).Post("/paybill/{receipt}/recheck", h.staffRecheck)

	r.With(requireRole(staff.DrawOfficial, staff.Auditor)).Get("/draws", h.staffDraws)
	r.With(requireRole(staff.DrawOfficial, staff.Auditor)).Get("/draws/overdue", h.staffOverdue)
	r.With(requireRole(staff.DrawOfficial, staff.Auditor)).Post("/draws/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Draws.Reconcile(ctx) }, h.StaffServices.Draws != nil))
	r.With(requireRole(staff.DrawOfficial)).Post("/draws/{id}/sign", h.staffDrawStep("sign"))
	r.With(requireRole(staff.DrawOfficial)).Post("/draws/{id}/approve", h.staffDrawStep("approve"))
	r.With(requireRole(staff.DrawOfficial)).Post("/draws/{id}/void", h.staffDrawStep("void"))
	r.With(requireRole(staff.Auditor, staff.PaymentsOperator)).Post("/game/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Game.Reconcile(ctx) }, h.StaffServices.Game != nil))

	r.With(requireRole(staff.PayoutOperator, staff.Auditor)).Get("/payouts", h.staffPayouts)
	r.With(requireRole(staff.PayoutOperator, staff.Auditor)).Post("/payouts/reconcile", h.staffReconcile(func(ctx context.Context) (any, error) { return h.StaffServices.Payouts.Reconcile(ctx) }, h.StaffServices.Payouts != nil))
	r.With(requireRole(staff.PayoutOperator)).Post("/payouts/{id}/approve", h.staffPayoutAction("approve"))
	r.With(requireRole(staff.PayoutOperator)).Post("/payouts/{id}/reject", h.staffPayoutAction("reject"))
	r.With(requireRole(staff.PayoutOperator)).Post("/payouts/{id}/settle", h.staffPayoutAction("settle"))
	r.With(requireRole(staff.PayoutOperator)).Post("/prizes/{result}/retry", h.staffRetryPrize)

	r.With(requireRole(staff.TaxOfficer, staff.Auditor)).Get("/tax/batches", h.staffTaxBatches)
	r.With(requireRole(staff.TaxOfficer)).Post("/tax/batches", h.staffTaxPrepare)
	r.With(requireRole(staff.TaxOfficer, staff.Auditor)).Get("/tax/batches/{no}/file", h.staffTaxFile)
	r.With(requireRole(staff.TaxOfficer)).Post("/tax/batches/{no}/remit", h.staffTaxRemit)

	r.With(requireRole(staff.AlertsOperator, staff.Auditor)).Get("/alerts", h.staffAlerts)
	r.With(requireRole(staff.AlertsOperator)).Post("/alerts/{id}/acknowledge", h.staffAlertAck)

	r.With(requireRole(staff.Support, staff.Auditor)).Get("/closures", h.staffClosures)
	r.With(requireRole(staff.Support)).Post("/closures/{user}/close", h.staffClose)
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
	type m struct {
		status      int
		slug, title string
	}
	var known = []struct {
		err error
		m   m
	}{
		{kyc.ErrNothingToReview, m{http.StatusConflict, "nothing-to-review", "No submission awaits review"}},
		{payments.ErrNotFound, m{http.StatusNotFound, "not-found", "Not found"}},
		{payments.ErrNotReversible, m{http.StatusConflict, "state", "Only a credited payment can be reversed"}},
		{payments.ErrNotResolvable, m{http.StatusConflict, "state", "Only an unconfirmed payment can be resolved"}},
		{payments.ErrReceiptUsed, m{http.StatusConflict, "receipt-used", "This receipt already credited another payment"}},
		{payments.ErrReasonRequired, m{http.StatusBadRequest, "reason-required", "A reason is required"}},
		{payments.ErrNotInSuspense, m{http.StatusNotFound, "not-found", "No open suspense item for this receipt"}},
		{payments.ErrNotRecheckable, m{http.StatusConflict, "state", "Only an unverified paybill payment can be checked again"}},
		{payments.ErrRefundRefNeeded, m{http.StatusBadRequest, "refund-ref-required", "The M-Pesa reference of the refund is required"}},
		{payments.ErrInvalidAmount, m{http.StatusBadRequest, "invalid-amount", "Invalid amount"}},
		{draws.ErrNotFound, m{http.StatusNotFound, "not-found", "No such draw"}},
		{draws.ErrState, m{http.StatusConflict, "state", "The draw is not in a state that allows this"}},
		{draws.ErrTooLate, m{http.StatusConflict, "too-late", "The drand round is published; the outcome is public"}},
		{draws.ErrAlreadySigned, m{http.StatusConflict, "already-signed", "You have already signed this step; a second official must"}},
		{draws.ErrNoPrizeVersion, m{http.StatusConflict, "no-prize-structure", "No prize structure in force"}},
		{payouts.ErrNotFound, m{http.StatusNotFound, "not-found", "No such payout"}},
		{payouts.ErrState, m{http.StatusConflict, "state", "The payout is not in a state that allows this"}},
		{payouts.ErrOperator, m{http.StatusBadRequest, "reason-required", "A reason is required"}},
		{payouts.ErrReceiptRequired, m{http.StatusBadRequest, "receipt-required", "The M-Pesa receipt of the payment is required"}},
		{payouts.ErrUnavailable, m{http.StatusServiceUnavailable, "payouts-unavailable", "Payouts are unavailable"}},
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
	m := member(r)
	roles := make([]string, 0, len(m.Roles))
	for _, role := range m.Roles {
		roles = append(roles, string(role))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": m.ID, "name": m.Name, "email": m.Email, "roles": roles})
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
	list, err := h.StaffServices.Draws.List(r.Context(), limit)
	if err != nil {
		staffError(w, err)
		return
	}
	out := make([]drawJSON, 0, len(list))
	for _, d := range list {
		out = append(out, toDrawJSON(d))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"draws": out})
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
	list, err := h.StaffServices.Payouts.InState(r.Context(), state, 100)
	if err != nil {
		staffError(w, err)
		return
	}
	if list == nil {
		list = []payouts.Payout{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payouts": list})
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

func (h *handlers) staffAlerts(w http.ResponseWriter, r *http.Request) {
	if h.StaffServices.Alerts == nil {
		unavailable(w, "Alerts")
		return
	}
	list, err := h.StaffServices.Alerts.List(r.Context(), r.URL.Query().Get("all") == "true", 100)
	if err != nil {
		staffError(w, err)
		return
	}
	if list == nil {
		list = []alerts.View{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"alerts": list})
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
