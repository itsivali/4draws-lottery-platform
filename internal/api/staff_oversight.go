package api

import (
	"errors"
	"net/http"

	"github.com/itsivali/lottery-platform/internal/oversight"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
)

// The back office's views of the whole system (owner, 2026-10-30): every play and top-up by player and time, the audit
// trail (administrators) and the system's state.

var topUpStatuses = map[string]bool{
	"initiated": true, "stk_sent": true, "callback_received": true, "verified": true, "credited": true, "failed": true,
	"cancelled": true, "expired": true, "unconfirmed": true, "reversed": true,
}

var actorTypes = map[string]bool{"system": true, "player": true, "staff": true}

func oversightError(w http.ResponseWriter, err error) {
	if errors.Is(err, oversight.ErrCursor) {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-cursor", "Invalid cursor or date", "use next_before from the previous page and dates as YYYY-MM-DD")
		return
	}
	internalError(w, err)
}

func nextOf(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// withNames adds the names on the players' identity submissions (decrypted, and audited as unmasked for the viewer).
func (h *handlers) withNames(r *http.Request, userIDs []string, set func(i int, name string)) error {
	if h.StaffServices.KYC == nil || len(userIDs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	who, err := h.StaffServices.KYC.Identities(r.Context(), unique, member(r).Operator())
	if err != nil {
		return err
	}
	for i, id := range userIDs {
		if v, ok := who[id]; ok && v.FullName != "" {
			set(i, v.FullName)
		}
	}
	return nil
}

func (h *handlers) staffPlays(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	p, err := h.StaffServices.Oversight.Plays(r.Context(), oversight.PlayFilter{Query: qs.Get("q"), Day: qs.Get("day")}, limit, qs.Get("before"))
	if err != nil {
		oversightError(w, err)
		return
	}
	ids := make([]string, len(p.Rows))
	for i, row := range p.Rows {
		ids[i] = row.UserID
	}
	if err := h.withNames(r, ids, func(i int, name string) { p.Rows[i].FullName = &name }); err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plays": p.Rows, "next_before": nextOf(p.NextBefore)})
}

func (h *handlers) staffTopUps(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	if st := qs.Get("status"); st != "" && !topUpStatuses[st] {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-status", "Invalid status", "")
		return
	}
	p, err := h.StaffServices.Oversight.TopUps(r.Context(), oversight.TopUpFilter{Query: qs.Get("q"), Status: qs.Get("status")}, limit, qs.Get("before"))
	if err != nil {
		oversightError(w, err)
		return
	}
	ids := make([]string, len(p.Rows))
	for i, row := range p.Rows {
		ids[i] = row.UserID
	}
	if err := h.withNames(r, ids, func(i int, name string) { p.Rows[i].FullName = &name }); err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"topups": p.Rows, "next_before": nextOf(p.NextBefore)})
}

func (h *handlers) staffActivity(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	qs := r.URL.Query()
	if a := qs.Get("actor_type"); a != "" && !actorTypes[a] {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-actor-type", "Invalid actor type", "use system, player or staff")
		return
	}
	p, err := h.StaffServices.Oversight.Activity(r.Context(), oversight.EventFilter{Query: qs.Get("q"), ActorType: qs.Get("actor_type")}, limit, qs.Get("before"))
	if err != nil {
		oversightError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"events": p.Rows, "next_before": nextOf(p.NextBefore)})
}

func (h *handlers) staffSystem(w http.ResponseWriter, r *http.Request) {
	s, err := h.StaffServices.Oversight.System(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}
