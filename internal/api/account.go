package api

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/payments/daraja"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
	"github.com/itsivali/lottery-platform/internal/wallet"
)

// pageLimit reads ?limit (1 to 100, default 20) and writes the problem when it is out of range.
func pageLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return 20, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 100 {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-limit", "Invalid limit", "use 1 to 100")
		return 0, false
	}
	return n, true
}

type walletTxJSON struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	AmountKES     int64     `json:"amount_kes"`
	ReferenceType *string   `json:"reference_type"`
	ReferenceID   *string   `json:"reference_id"`
	CreatedAt     time.Time `json:"created_at"`
}

func (h *handlers) getWallet(w http.ResponseWriter, r *http.Request) {
	if h.Wallet == nil {
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "unavailable", "Temporarily unavailable", "", 5)
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	st, err := h.Wallet.Statement(r.Context(), p.UserID, limit, r.URL.Query().Get("before"))
	switch {
	case errors.Is(err, wallet.ErrBadCursor):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-cursor", "Invalid cursor", "use next_before from the previous page")
		return
	case err != nil:
		internalError(w, err)
		return
	}
	txs := make([]walletTxJSON, 0, len(st.Transactions))
	for _, t := range st.Transactions {
		txs = append(txs, walletTxJSON{ID: t.ID, Kind: t.Kind, AmountKES: t.AmountKES, ReferenceType: t.ReferenceType,
			ReferenceID: t.ReferenceID, CreatedAt: t.CreatedAt})
	}
	var next *string
	if st.NextBefore != "" {
		next = &st.NextBefore
	}
	// the top-up and withdrawal bounds travel with the balance so the apps never hard-code them (null while that
	// money flow is unavailable: payments not configured, B2C not configured)
	var topUp, withdraw map[string]int64
	if h.Payments != nil {
		// wait_s: seconds until the player may ask for another M-Pesa prompt (the top-up cooldown; 0 means now)
		wait, err := h.Payments.TopUpWait(r.Context(), p.UserID)
		if err != nil {
			internalError(w, err)
			return
		}
		topUp = map[string]int64{"min_kes": h.Payments.MinTopUpKES(), "max_kes": daraja.MaxSTKAmountKES,
			"wait_s": int64(math.Ceil(wait.Seconds())), "cooldown_s": int64(h.Payments.TopUpCooldown().Seconds())}
	}
	if h.Payouts != nil && h.Payouts.Enabled() {
		lo, hi := h.Payouts.WithdrawalBounds()
		withdraw = map[string]int64{"min_kes": lo, "max_kes": hi}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"balance_kes": st.BalanceKES, "debt_kes": st.DebtKES, "transactions": txs,
		"next_before": next, "top_up": topUp, "withdraw": withdraw})
}

type restrictionJSON struct {
	Kind  string     `json:"kind"`
	Until *time.Time `json:"until"`
}

func (h *handlers) getAccount(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	a, err := h.Identity.Account(r.Context(), p.UserID)
	if err != nil {
		internalError(w, err)
		return
	}
	// the player's name is the one on their identity submission (owner, 2026-10-30): its first two words
	var first, second *string
	if h.KYC != nil {
		name, err := h.KYC.OwnName(r.Context(), p.UserID)
		if err != nil {
			internalError(w, err)
			return
		}
		if words := strings.Fields(name); len(words) >= 2 {
			first, second = &words[0], &words[1]
		} else if len(words) == 1 {
			first = &words[0]
		}
	}
	var restriction *restrictionJSON
	if h.RG != nil {
		re, err := h.RG.Restriction(r.Context(), p.UserID)
		if err != nil {
			internalError(w, err)
			return
		}
		if re != nil {
			restriction = &restrictionJSON{Kind: re.Kind, Until: re.Until}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id": a.UserID, "msisdn": a.MSISDN, "phone_verified": a.PhoneVerified, "kyc_status": a.KYCStatus, "status": a.Status,
		"created_at": a.CreatedAt, "restriction": restriction, "closure_requested_at": a.ClosureRequestedAt,
		"must_change_password": a.MustChangePassword, "email": a.Email, "has_password": a.HasPassword,
		"first_name": first, "second_name": second,
	})
}

// setEmail changes or adds the player's email address; an account with a password must give it (ADR 0021).
func (h *handlers) setEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"current_password"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "email:user:"+p.UserID, 10, time.Hour) {
		return
	}
	if err := h.Identity.SetEmail(r.Context(), p.UserID, in.Email, in.Password); err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			httpx.WriteProblem(w, http.StatusForbidden, "wrong-password", "Current password is incorrect", "")
			return
		}
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) requestClosure(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	at, err := h.Identity.RequestClosure(r.Context(), p.UserID)
	if err != nil {
		internalError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"closure_requested_at": at})
}

func (h *handlers) listPayments(w http.ResponseWriter, r *http.Request) {
	if h.Payments == nil {
		paymentsUnavailable(w)
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	p, _ := principalFrom(r.Context())
	list, err := h.Payments.List(r.Context(), p.UserID, limit)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]any, 0, len(list))
	for _, t := range list {
		out = append(out, toTopUpJSON(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"payments": out})
}
