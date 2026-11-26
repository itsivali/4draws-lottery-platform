package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/aml"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/httpx"
)

// Transaction monitoring in the back office (ADR 0033): the cases, what is done on them, and the rules as they stand.
// The rules' thresholds are the compliance owner's decision; an admin records it here or with lotteryctl (aml
// set-rule), with a note and, once given, the owner's approval reference (ADR 0036).
func (h *handlers) mountAML(r chi.Router, anyStaff, admin func(http.Handler) http.Handler) {
	has := h.StaffServices.AML != nil
	r.With(anyStaff).Get("/aml/rules", h.needs(has, "Transaction monitoring", h.staffAMLRules))
	r.With(admin).Put("/aml/rules/{rule}", h.needs(has, "Transaction monitoring", h.staffAMLSetRule))
	r.With(anyStaff).Get("/aml/cases", h.needs(has, "Transaction monitoring", h.staffAMLCases))
	r.With(anyStaff).Post("/aml/cases", h.needs(has, "Transaction monitoring", h.staffAMLOpen))
	r.With(anyStaff).Get("/aml/cases/{id}", h.needs(has, "Transaction monitoring", h.staffAMLCase))
	for _, action := range []string{"hold", "release", "note", "close"} {
		r.With(anyStaff).Post("/aml/cases/{id}/"+action, h.needs(has, "Transaction monitoring", h.staffAMLAction(action)))
	}
}

func amlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, aml.ErrInvalid):
		httpx.WriteProblem(w, http.StatusBadRequest, "note-required", "A note is required", "")
	case errors.Is(err, aml.ErrNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such case or player", "")
	case errors.Is(err, aml.ErrBadCursor):
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-cursor", "Invalid cursor", "")
	case errors.Is(err, aml.ErrClosed), errors.Is(err, aml.ErrHoldState):
		httpx.WriteProblem(w, http.StatusConflict, "state", "The case is not in a state that allows this", "")
	case errors.Is(err, aml.ErrSamePerson):
		httpx.WriteProblem(w, http.StatusConflict, "two-person-rule", "Someone other than who placed the hold must lift it", "")
	case errors.Is(err, aml.ErrOpen):
		httpx.WriteProblem(w, http.StatusConflict, "case-open", "The player already has an open case of this kind", "")
	default:
		staffError(w, err)
	}
}

func (h *handlers) staffAMLRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.StaffServices.AML.Settings(r.Context())
	if err != nil {
		amlError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

// staffAMLSetRule records a rule's setting: on or off, its thresholds, the approval reference and a note.
func (h *handlers) staffAMLSetRule(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled           bool       `json:"enabled"`
		Params            aml.Params `json:"params"`
		ApprovalReference string     `json:"approval_reference"`
		Note              string     `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	err := h.StaffServices.AML.SetRule(r.Context(), aml.Rule(chi.URLParam(r, "rule")), in.Enabled, in.Params, in.ApprovalReference, member(r).Operator(), in.Note)
	if errors.Is(err, aml.ErrInvalid) {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-rule", "Check the rule's thresholds and give a note",
			"every threshold is a positive whole number, play percent at most 100, accounts at least 2, windows and quiet periods at most a year")
		return
	} else if err != nil {
		amlError(w, err)
		return
	}
	h.staffAMLRules(w, r)
}

func (h *handlers) staffAMLCases(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	switch view {
	case "", "open", "held", "closed":
	default:
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-view", "The view is open, held or closed", "")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	list, next, err := h.StaffServices.AML.Cases(r.Context(), view, limit, r.URL.Query().Get("after"))
	if err != nil {
		amlError(w, err)
		return
	}
	var cursor *string
	if next != "" {
		cursor = &next
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"cases": list, "next_after": cursor})
}

func (h *handlers) staffAMLCase(w http.ResponseWriter, r *http.Request) {
	c, err := h.StaffServices.AML.Case(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		amlError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *handlers) staffAMLOpen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID string `json:"user_id"`
		Note   string `json:"note"`
	}
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	c, err := h.StaffServices.AML.OpenManual(r.Context(), in.UserID, member(r).Operator(), in.Note)
	if err != nil {
		amlError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *handlers) staffAMLAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Note string `json:"note"`
		}
		if !httpx.DecodeJSON(w, r, &in) {
			return
		}
		svc, id, op := h.StaffServices.AML, chi.URLParam(r, "id"), member(r).Operator()
		var c aml.Case
		var err error
		switch action {
		case "hold":
			c, err = svc.Hold(r.Context(), id, op, in.Note)
		case "release":
			c, err = svc.Release(r.Context(), id, op, in.Note)
		case "note":
			c, err = svc.Note(r.Context(), id, op, in.Note)
		case "close":
			c, err = svc.Close(r.Context(), id, op, in.Note)
		}
		if err != nil {
			amlError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, c)
	}
}
