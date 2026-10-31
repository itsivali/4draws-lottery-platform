package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/lottery-platform/internal/oversight"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
	"github.com/itsivali/lottery-platform/internal/rg"
)

// The back office's audit aids (owner, 2026-10-31): the activity log taken away as a file, and one player's whole
// timeline, on screen and as a file. Exports carry their SHA-256 and row count in headers and are audited.

// staffNames maps staff actor ids ("staff:<id>") to names, for files an auditor reads without the back office.
func (h *handlers) staffNames(r *http.Request) (map[string]string, error) {
	names := map[string]string{}
	if h.StaffServices.Members == nil {
		return names, nil
	}
	list, err := h.StaffServices.Members.Members(r.Context())
	if err != nil {
		return nil, err
	}
	for _, m := range list {
		name := m.Name
		if name == "" {
			name = m.Email
		}
		names["staff:"+m.ID] = name
	}
	return names, nil
}

// staffActivityExport streams the filtered audit trail as CSV, oldest first. The file is hashed from a snapshot
// before the first byte goes out, so X-File-SHA256 is a header; more than the cap of records is refused.
func (h *handlers) staffActivityExport(w http.ResponseWriter, r *http.Request) {
	f, ok := eventFilter(w, r)
	if !ok {
		return
	}
	names, err := h.staffNames(r)
	if err != nil {
		staffError(w, err)
		return
	}
	x, err := h.StaffServices.Oversight.PrepareActivityExport(r.Context(), f, names, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	defer x.Close(r.Context())
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+x.Name+`"`)
	w.Header().Set("X-File-SHA256", x.SHA256)
	w.Header().Set("X-File-Rows", strconv.FormatInt(x.Rows, 10))
	if err := x.WriteTo(r.Context(), w); err != nil {
		// the status is out; the client sees a short or wrong file, and the log says why
		slog.ErrorContext(r.Context(), "activity export interrupted", "err", err, "file", x.Name)
	}
}

// playerTimeline reads one player's timeline with their name and ID number (unmasked and audited) and their
// responsible-gaming limits. viewer "" leaves the view itself unaudited (the export audits itself).
func (h *handlers) playerTimeline(r *http.Request, all bool, viewer string) (*oversight.Player, error) {
	p, err := h.StaffServices.Oversight.Player(r.Context(), chi.URLParam(r, "user"), all, viewer)
	if err != nil {
		return nil, err
	}
	if h.StaffServices.KYC != nil { // the name and ID number on the identity submission, else from sign-up: audited as unmasked
		who, err := h.StaffServices.KYC.Identities(r.Context(), []string{p.Player.UserID}, member(r).Operator())
		if err != nil {
			return nil, err
		}
		if id, ok := who[p.Player.UserID]; ok {
			p.Player.FullName, p.Player.NationalID = id.FullName, id.IDNumber
		}
	}
	if h.StaffServices.RG != nil {
		limits, err := h.StaffServices.RG.Limits(r.Context(), p.Player.UserID)
		if err != nil {
			return nil, err
		}
		for _, period := range []rg.Period{rg.Daily, rg.Weekly, rg.Monthly} {
			st := limits[period]
			if st.Current != nil {
				p.Limits.Deposit = append(p.Limits.Deposit, oversight.DepositLimit{Period: string(period), AmountKES: st.Current.AmountKES, EffectiveFrom: st.Current.EffectiveFrom})
			}
			if st.Pending != nil {
				p.Limits.Deposit = append(p.Limits.Deposit, oversight.DepositLimit{Period: string(period), AmountKES: st.Pending.AmountKES, EffectiveFrom: st.Pending.EffectiveFrom, Pending: true})
			}
		}
		restricted, err := h.StaffServices.RG.Restriction(r.Context(), p.Player.UserID)
		if err != nil {
			return nil, err
		}
		if restricted != nil {
			p.Limits.Restriction = &oversight.Restriction{Kind: restricted.Kind, Until: restricted.Until}
		}
	}
	return p, nil
}

// staffPlayer is one player's timeline: the account, wallet, top-ups, plays, payouts, identity history, limits and
// the audit records about them. The view is audited as identity.player_viewed, and the unmasking of the name.
func (h *handlers) staffPlayer(w http.ResponseWriter, r *http.Request) {
	p, err := h.playerTimeline(r, false, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// staffPlayerExport is the player pack: the whole timeline as one CSV file, hashed (X-File-SHA256) and audited as
// identity.player_exported.
func (h *handlers) staffPlayerExport(w http.ResponseWriter, r *http.Request) {
	p, err := h.playerTimeline(r, true, "")
	if err != nil {
		staffError(w, err)
		return
	}
	f, err := h.StaffServices.Oversight.PlayerPack(r.Context(), p, member(r).Operator())
	if err != nil {
		staffError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+f.Name+`"`)
	w.Header().Set("X-File-SHA256", f.SHA256)
	w.Header().Set("X-File-Rows", strconv.Itoa(f.Rows))
	_, _ = w.Write(f.Data)
}
