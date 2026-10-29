package api

import (
	"errors"
	"net/http"
	"strconv"
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
		topUp = map[string]int64{"min_kes": h.Payments.MinTopUpKES(), "max_kes": daraja.MaxSTKAmountKES}
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
		"first_name": a.FirstName, "second_name": a.SecondName,
	})
}

// setName changes or adds the player's first and second names.
func (h *handlers) setName(w http.ResponseWriter, r *http.Request) {
	var in struct {
		First  string `json:"first_name"`
		Second string `json:"second_name"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "name:user:"+p.UserID, 10, time.Hour) {
		return
	}
	if err := h.Identity.SetName(r.Context(), p.UserID, in.First, in.Second); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
