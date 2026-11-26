package api

import (
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/aml"
	"github.com/itsivali/4draws-lottery-platform/internal/compliance"
	"github.com/itsivali/4draws-lottery-platform/internal/identity"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/daraja"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/httpx"
)

type payoutJSON struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	GrossKES  int64     `json:"amount_kes"`
	TaxKES    int64     `json:"tax_kes"`
	DebtKES   int64     `json:"debt_repaid_kes"`
	NetKES    int64     `json:"net_kes"`
	Receipt   *string   `json:"receipt"`
	CreatedAt time.Time `json:"created_at"`
}

func toPayoutJSON(p payouts.Payout) payoutJSON {
	return payoutJSON{ID: p.ID, Kind: p.Kind, Status: p.Status, GrossKES: p.GrossKES, TaxKES: p.TaxKES, DebtKES: p.DebtKES,
		NetKES: p.NetKES, Receipt: optStr(p.Receipt), CreatedAt: p.CreatedAt}
}

func payoutsUnavailable(w http.ResponseWriter) {
	httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "payouts-unavailable", "Withdrawals are temporarily unavailable", "", 60)
}

func (h *handlers) withdraw(w http.ResponseWriter, r *http.Request) {
	if !h.deviceTrusted(w, r) {
		return
	}
	var in struct {
		AmountKES int64 `json:"amount_kes"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "withdraw:user:"+p.UserID, 10, time.Hour) || !h.geo(w, r, p.UserID, compliance.GeoWithdrawal) {
		return
	}
	if h.Payouts == nil {
		payoutsUnavailable(w)
		return
	}
	out, replayed, err := h.Payouts.RequestWithdrawal(r.Context(), p.UserID, in.AmountKES, r.Header.Get("Idempotency-Key"))
	switch {
	case err == nil:
		status := http.StatusCreated
		if replayed {
			status = http.StatusOK
		}
		httpx.WriteJSON(w, status, toPayoutJSON(out))
	case errors.Is(err, payouts.ErrInvalidAmount):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-amount", "Invalid amount", "withdraw KES 50 to 150,000 at a time")
	case errors.Is(err, payouts.ErrInsufficientFunds):
		httpx.WriteProblem(w, http.StatusConflict, "insufficient-funds", "Your balance is below the amount", "")
	case errors.Is(err, payouts.ErrDailyLimit):
		httpx.WriteProblem(w, http.StatusConflict, "daily-withdrawal-limit", "No withdrawals left today", "withdrawals reset at midnight Nairobi time")
	case errors.Is(err, payouts.ErrInDebt):
		httpx.WriteProblem(w, http.StatusForbidden, "in-debt", "Withdrawals are paused while you owe money from a reversed top-up",
			"your next top-up repays it first")
	case errors.Is(err, compliance.ErrOperationsStopped):
		operationsStopped(w)
	case errors.Is(err, aml.ErrUnderReview):
		underReview(w)
	case errors.Is(err, payouts.ErrUnavailable):
		payoutsUnavailable(w)
	case errors.Is(err, identity.ErrKYCRequired), errors.Is(err, identity.ErrPhoneNotVerified), errors.Is(err, identity.ErrAccountSuspended):
		authError(w, err)
	default:
		internalError(w, err)
	}
}

func (h *handlers) listPayouts(w http.ResponseWriter, r *http.Request) {
	if h.Payouts == nil {
		payoutsUnavailable(w)
		return
	}
	p, _ := principalFrom(r.Context())
	list, err := h.Payouts.List(r.Context(), p.UserID, 50)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]payoutJSON, 0, len(list))
	for _, x := range list {
		out = append(out, toPayoutJSON(x))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payouts": out})
}

func (h *handlers) getPayout(w http.ResponseWriter, r *http.Request) {
	if h.Payouts == nil {
		payoutsUnavailable(w)
		return
	}
	p, _ := principalFrom(r.Context())
	out, err := h.Payouts.Get(r.Context(), p.UserID, chi.URLParam(r, "id"))
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toPayoutJSON(out))
	case errors.Is(err, payouts.ErrNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such payout", "")
	default:
		internalError(w, err)
	}
}

// b2cHook receives what Daraja posts about payouts, behind the same secret path segment as the other hooks.
func (h *handlers) b2cHook(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := chi.URLParam(r, "path")
		if h.Payouts == nil || !h.Payouts.Enabled() || h.CallbackPath == "" || subtle.ConstantTimeCompare([]byte(path), []byte(h.CallbackPath)) != 1 {
			httpx.WriteProblem(w, http.StatusNotFound, "not-found", "Not found", "")
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
		if err != nil {
			httpx.WriteProblem(w, http.StatusBadRequest, "bad-callback", "Unreadable callback", "")
			return
		}
		switch err := h.Payouts.HandleCallback(r.Context(), kind, raw); {
		case errors.Is(err, daraja.ErrBadCallback):
			slog.WarnContext(r.Context(), "malformed payout callback", "kind", kind, "err", err)
			httpx.WriteProblem(w, http.StatusBadRequest, "bad-callback", "Malformed callback", "")
		case err != nil:
			internalError(w, err)
		default:
			darajaAck(w, 0, "Accepted")
		}
	}
}
