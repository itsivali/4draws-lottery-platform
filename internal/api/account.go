package api

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itsivali/lottery-platform/internal/dataexport"
	"github.com/itsivali/lottery-platform/internal/identity"
	"github.com/itsivali/lottery-platform/internal/kyc"
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
	// the player's name is the one on their identity submission, else the one they declared (owner, 2026-10-30): its
	// first two words; with it, where the identity stands and a hint of the ID number, never the number itself
	var first, second, hint *string
	var ident map[string]any
	if h.KYC != nil {
		own, err := h.KYC.OwnIdentity(r.Context(), p.UserID)
		if err != nil {
			internalError(w, err)
			return
		}
		if words := strings.Fields(own.FullName); len(words) >= 2 {
			first, second = &words[0], &words[1]
		} else if len(words) == 1 {
			first = &words[0]
		}
		if n := len(own.IDNumber); n >= 3 {
			last := own.IDNumber[n-3:]
			hint = &last
		}
		ident = map[string]any{"status": own.Status, "national_id_hint": hint}
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
		"must_change_password": a.MustChangePassword, "email": a.Email, "email_verified": a.EmailVerified, "has_password": a.HasPassword, "photo_url": nonEmpty(a.PhotoURL),
		"first_name": first, "second_name": second, "identity": ident,
	})
}

// declareIdentity takes the first name, second name and national ID number of an account that gave none at sign-up
// (owner, 2026-10-31): once, and only until an identity submission exists.
func (h *handlers) declareIdentity(w http.ResponseWriter, r *http.Request) {
	var in declaredJSON
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if h.KYC == nil {
		httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "unavailable", "Temporarily unavailable", "", 5)
		return
	}
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "identity:user:"+p.UserID, 10, time.Hour) {
		return
	}
	err := h.KYC.DeclareLater(r.Context(), p.UserID, kyc.Declaration{FirstName: in.FirstName, SecondName: in.SecondName, IDNumber: in.NationalID})
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, kyc.ErrInvalidInput):
		invalidIdentity(w)
	case errors.Is(err, kyc.ErrAlreadyDeclared):
		httpx.WriteProblem(w, http.StatusConflict, "identity-declared", "Your names and ID number are already on your account",
			"they are checked, not changed, before your first withdrawal; contact support if they are wrong")
	case errors.Is(err, kyc.ErrSubmitted):
		httpx.WriteProblem(w, http.StatusConflict, "kyc-state", "Your identity is already submitted for verification",
			"the name on that submission is the one shown")
	default:
		internalError(w, err)
	}
}

type sessionJSON struct {
	ID            string    `json:"id"`
	Platform      string    `json:"platform"`
	DeviceVerdict string    `json:"device_verdict"`
	CreatedAt     time.Time `json:"created_at"`
	LastUsedAt    time.Time `json:"last_used_at"`
	Current       bool      `json:"current"`
}

// listSessions shows the player every device signed in to their account (open sessions, newest first).
func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	list, err := h.Identity.Sessions(r.Context(), p.UserID)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]sessionJSON, 0, len(list))
	for _, s := range list {
		out = append(out, sessionJSON{ID: s.ID, Platform: s.Platform, DeviceVerdict: s.DeviceVerdict, CreatedAt: s.CreatedAt,
			LastUsedAt: s.LastUsedAt, Current: s.ID == p.SessionID})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// revokeOtherSessions signs every other device out; the one making the call stays signed in.
func (h *handlers) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "sessions:user:"+p.UserID, 10, time.Hour) {
		return
	}
	if _, err := h.Identity.EndOtherSessions(r.Context(), p.UserID, p.SessionID); err != nil {
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// signIns says how the account can be signed in to: with a password, and with which Google or Apple identities.
func (h *handlers) signIns(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	hasPassword, linked, err := h.Identity.SignInMethods(r.Context(), p.UserID)
	if err != nil {
		internalError(w, err)
		return
	}
	providers := make([]map[string]any, 0, len(linked))
	for _, l := range linked {
		providers = append(providers, map[string]any{"provider": l.Provider, "linked_at": l.LinkedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"password": hasPassword, "providers": providers})
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

// sendEmailVerification emails the player the link that confirms their address again (ADR 0040).
func (h *handlers) sendEmailVerification(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "verify-email:user:"+p.UserID, 5, time.Hour) {
		return
	}
	if err := h.Identity.SendEmailVerification(r.Context(), p.UserID); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
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

// requestDataExport emails the player a copy of their data (ADR 0038), at most once a day.
func (h *handlers) requestDataExport(w http.ResponseWriter, r *http.Request) {
	if h.DataExport == nil {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "data-export-unavailable", "Copies of your data cannot be emailed right now",
			"Write to privacy@4draws.co.ke for a copy.")
		return
	}
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "data-export:user:"+p.UserID, 5, time.Hour) {
		return
	}
	rc, err := h.DataExport.Send(r.Context(), p.UserID)
	var soon dataexport.TooSoonError
	switch {
	case errors.Is(err, dataexport.ErrNoEmail):
		httpx.WriteProblem(w, http.StatusConflict, "no-email", "Add an email address first", "Your copy is sent to the email address on your account.")
	case errors.Is(err, dataexport.ErrEmailNotVerified):
		httpx.WriteProblem(w, http.StatusConflict, "email-not-verified", "Confirm your email address first",
			"Your copy is sent only to an address you have confirmed. Open the link we emailed you, or ask for a new one.")
	case errors.Is(err, dataexport.ErrNotActive):
		httpx.WriteProblem(w, http.StatusConflict, "account-closed", "This account is closed", "")
	case errors.As(err, &soon):
		secs := int(time.Until(soon.Next).Seconds()) //nolint:forbidigo // Retry-After is wall-clock seconds
		httpx.WriteProblemRetry(w, http.StatusTooManyRequests, "data-export-too-soon", "A copy was sent in the last 24 hours",
			"Check your inbox and junk folder. You can ask again after "+soon.Next.UTC().Format(time.RFC3339)+".", max(secs, 1))
	case err != nil:
		internalError(w, err)
	default:
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"sent_to": rc.SentTo, "requested_at": rc.RequestedAt, "next_allowed_at": rc.Next})
	}
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

// nonEmpty is nil for "", so JSON says null rather than an empty string.
func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
