package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/lottery-platform/internal/compliance"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
)

// The operating mode (ADR 0027): GET /v1/status for the apps, and the back office's stop, resume and the two-admin
// live approval under /v1/staff/operating.

// status tells the apps whether money moves and whether it is real, so they can say "sandbox" or "paused". Nothing
// here is secret.
func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	if h.Gate == nil {
		unavailable(w, "Status")
		return
	}
	st, err := h.Gate.Current(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"operating_mode": st.Mode, "real_money": st.RealMoney, "money_moves": st.Mode != compliance.Disabled})
}

// operationsStopped answers a player whose money operation the operating mode refused.
func operationsStopped(w http.ResponseWriter) {
	httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "operations-stopped", "Top-ups, plays and withdrawals are paused",
		"your balance is safe; try again later", 300)
}

func (h *handlers) staffOperating(w http.ResponseWriter, r *http.Request) {
	g := h.StaffServices.Gate
	st, err := g.Current(r.Context())
	if err != nil {
		staffError(w, err)
		return
	}
	reqs, err := g.OpenLiveRequests(r.Context())
	if err != nil {
		staffError(w, err)
		return
	}
	hist, err := g.History(r.Context(), 50)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": st, "live_requests": reqs, "history": hist})
}

// staffOperatingStop stops money moving: any staff member, with a reason.
func (h *handlers) staffOperatingStop(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	c, err := h.StaffServices.Gate.Stop(r.Context(), member(r).Operator(), in.Reason)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

// staffOperatingResume lifts a stop back to sandbox: admins, with a reason.
func (h *handlers) staffOperatingResume(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode   string `json:"mode"`
		Reason string `json:"reason"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	c, err := h.StaffServices.Gate.Resume(r.Context(), compliance.Mode(in.Mode), member(r).Operator(), in.Reason)
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *handlers) staffLiveRequest(w http.ResponseWriter, r *http.Request) {
	var in compliance.LiveRequest
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	out, err := h.StaffServices.Gate.RequestLive(r.Context(), in, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *handlers) staffLiveApprove(w http.ResponseWriter, r *http.Request) {
	c, err := h.StaffServices.Gate.ApproveLive(r.Context(), chi.URLParam(r, "id"), member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

// complianceProblem maps the operating mode's refusals for staffError.
func complianceProblem(err error) (status int, slug, title string, ok bool) {
	switch {
	case errors.Is(err, compliance.ErrInvalid):
		return http.StatusBadRequest, "invalid", "A reason is required, and a live request needs its licence, approval reference and the evidence's SHA-256 and location", true
	case errors.Is(err, compliance.ErrNotFound):
		return http.StatusNotFound, "not-found", "No such live request", true
	case errors.Is(err, compliance.ErrSamePerson):
		return http.StatusConflict, "second-admin", "A different admin must approve this request", true
	case errors.Is(err, compliance.ErrAlreadyDecided):
		return http.StatusConflict, "state", "This request was already approved", true
	case errors.Is(err, compliance.ErrExpired):
		return http.StatusConflict, "expired", "This request has expired; record a new one", true
	case errors.Is(err, compliance.ErrNotConfigured):
		return http.StatusConflict, "not-configured", "This service is not configured to operate live under that licence", true
	case errors.Is(err, compliance.ErrNotLive):
		return http.StatusBadRequest, "live-needs-approval", "Going live needs a live request and a second admin's approval", true
	case errors.Is(err, compliance.ErrOperationsStopped):
		return http.StatusConflict, "operations-stopped", "Money operations are stopped; resume them first", true
	}
	return 0, "", "", false
}
