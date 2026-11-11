package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

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
	if errors.Is(err, oversight.ErrArea) {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-area", "Invalid area", "use payments, game, draws or payouts")
		return
	}
	if errors.Is(err, oversight.ErrCursor) {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-cursor", "Invalid cursor or date", "use next_before from the previous page and dates as YYYY-MM-DD")
		return
	}
	staffError(w, err)
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
	f := oversight.TopUpFilter{Query: qs.Get("q"), Status: qs.Get("status"), Reason: qs.Get("reason"), Channel: qs.Get("channel"), Claimed: qs.Get("claimed") == "true"}
	if f.Channel != "" && f.Channel != "stk" && f.Channel != "c2b" {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-channel", "Invalid channel", "stk or c2b")
		return
	}
	var ok2 bool
	if f.From, f.To, ok2 = dayWindow(w, qs.Get("from"), qs.Get("to")); !ok2 {
		return
	}
	p, err := h.StaffServices.Oversight.TopUps(r.Context(), f, limit, qs.Get("before"))
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

// eventFilter reads the activity log's filters (owner, 2026-10-31): q, actor_type, from and to (Nairobi days), action
// (a prefix), entity_type, entity_id and actor_id. The days are checked by the service.
func eventFilter(w http.ResponseWriter, r *http.Request) (oversight.EventFilter, bool) {
	qs := r.URL.Query()
	if a := qs.Get("actor_type"); a != "" && !actorTypes[a] {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-actor-type", "Invalid actor type", "use system, player or staff")
		return oversight.EventFilter{}, false
	}
	return oversight.EventFilter{Query: qs.Get("q"), ActorType: qs.Get("actor_type"), From: qs.Get("from"), To: qs.Get("to"),
		ActionPrefix: qs.Get("action"), EntityType: qs.Get("entity_type"), EntityID: qs.Get("entity_id"), ActorID: qs.Get("actor_id")}, true
}

// dayWindow reads from/to dates (YYYY-MM-DD, Nairobi days; to is inclusive) into a created-at window.
func dayWindow(w http.ResponseWriter, from, to string) (*time.Time, *time.Time, bool) {
	var f, t *time.Time
	if from != "" {
		d, err := time.ParseInLocation("2006-01-02", from, nairobi)
		if err != nil {
			httpx.WriteProblem(w, http.StatusBadRequest, "invalid-range", "Invalid from date", "YYYY-MM-DD")
			return nil, nil, false
		}
		f = &d
	}
	if to != "" {
		d, err := time.ParseInLocation("2006-01-02", to, nairobi)
		if err != nil {
			httpx.WriteProblem(w, http.StatusBadRequest, "invalid-range", "Invalid to date", "YYYY-MM-DD")
			return nil, nil, false
		}
		d = d.AddDate(0, 0, 1)
		t = &d
	}
	if f != nil && t != nil && !t.After(*f) {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-range", "The range is reversed", "")
		return nil, nil, false
	}
	return f, t, true
}

var nairobi = func() *time.Location {
	loc, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		return time.FixedZone("EAT", 3*3600)
	}
	return loc
}()

// staffTopUp shows one top-up in full, with the player's name.
func (h *handlers) staffTopUp(w http.ResponseWriter, r *http.Request) {
	d, err := h.StaffServices.Oversight.TopUp(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		staffError(w, err)
		return
	}
	if err := h.withNames(r, []string{d.UserID}, func(_ int, name string) { d.FullName = &name }); err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, d)
}

func (h *handlers) staffActivity(w http.ResponseWriter, r *http.Request) {
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	f, ok := eventFilter(w, r)
	if !ok {
		return
	}
	p, err := h.StaffServices.Oversight.Activity(r.Context(), f, limit, r.URL.Query().Get("before"))
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

// staffJob retries (retry) or cancels a background job: admins, audited (ADR 0036).
func (h *handlers) staffJob(retry bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil || id <= 0 {
			httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such job", "")
			return
		}
		act := h.StaffServices.Jobs.Cancel
		if retry {
			act = h.StaffServices.Jobs.Retry
		}
		switch err := act(r.Context(), id, member(r).Operator()); {
		case errors.Is(err, oversight.ErrJobNotFound):
			httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such job", "")
		case errors.Is(err, oversight.ErrJobState):
			httpx.WriteProblem(w, http.StatusConflict, "state", "The job is not in a state that allows this", "")
		case err != nil:
			staffError(w, err)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}
