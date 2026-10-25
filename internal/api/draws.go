package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/itsivali/lottery-platform/internal/draws"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
)

// The public view of a draw: what anyone needs to follow and check it. Officials' names are staff personal data and
// stay internal; each step shows its time and attestation hash.
type signoffJSON struct {
	Step        string    `json:"step"`
	Attestation string    `json:"attestation"`
	SignedAt    time.Time `json:"signed_at"`
}

type resultJSON struct {
	Rank      int   `json:"rank"`
	AmountKES int64 `json:"amount_kes"`
	EntryNo   int   `json:"entry_no"`
}

type drawJSON struct {
	ID              string          `json:"id"`
	Type            string          `json:"draw_type"`
	PeriodStart     string          `json:"period_start"`
	PeriodEnd       string          `json:"period_end"`
	Status          string          `json:"status"`
	FreezeAt        time.Time       `json:"freeze_at"`
	Entries         int             `json:"entries"`
	EntryListHash   *string         `json:"entry_list_hash"`
	SeedCommit      *string         `json:"seed_commit"`
	DrandRound      *int64          `json:"drand_round"`
	DrandRoundAt    *time.Time      `json:"drand_round_at"`
	RevealedSeed    *string         `json:"revealed_seed"`
	Beacon          json.RawMessage `json:"beacon"`
	CertificateHash *string         `json:"certificate_hash"`
	VoidedReason    *string         `json:"voided_reason"`
	RerunOf         *string         `json:"rerun_of"`
	Signoffs        []signoffJSON   `json:"signoffs"`
	Results         []resultJSON    `json:"results"`
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toDrawJSON(d draws.Draw) drawJSON {
	out := drawJSON{ID: d.ID, Type: d.Type, PeriodStart: d.PeriodStart, PeriodEnd: d.PeriodEnd, Status: d.Status, FreezeAt: d.FreezeAt,
		Entries: d.Entries, EntryListHash: optStr(d.EntryListHash), SeedCommit: optStr(d.SeedCommit), DrandRoundAt: d.RoundAt,
		RevealedSeed: optStr(d.RevealedSeed), Beacon: json.RawMessage("null"), CertificateHash: optStr(d.CertificateHash),
		VoidedReason: optStr(d.VoidedReason), RerunOf: optStr(d.RerunOf), Signoffs: make([]signoffJSON, 0, len(d.Signoffs)),
		Results: make([]resultJSON, 0, len(d.Results))}
	if d.BeaconRound != 0 {
		out.DrandRound = &d.BeaconRound
	}
	var cert struct {
		Beacon json.RawMessage `json:"beacon"`
	}
	if json.Unmarshal(d.Certificate, &cert) == nil && cert.Beacon != nil {
		out.Beacon = cert.Beacon
	}
	for _, s := range d.Signoffs {
		out.Signoffs = append(out.Signoffs, signoffJSON{Step: s.Step, Attestation: s.Signature, SignedAt: s.SignedAt})
	}
	for _, r := range d.Results {
		out.Results = append(out.Results, resultJSON{Rank: r.Rank, AmountKES: r.AmountKES, EntryNo: r.EntryNo})
	}
	return out
}

func drawsUnavailable(w http.ResponseWriter) {
	httpx.WriteProblemRetry(w, http.StatusServiceUnavailable, "draws-unavailable", "Draws are temporarily unavailable", "", 60)
}

func drawNotFound(w http.ResponseWriter) {
	httpx.WriteProblem(w, http.StatusNotFound, "not-found", "No such draw", "")
}

// publicDraws applies the per-address limit of the public draw routes and checks the service is there.
func (h *handlers) publicDraws(w http.ResponseWriter, r *http.Request) bool {
	if !h.limit(w, r, "draws:ip:"+h.clientIP(r), 120, time.Minute) {
		return false
	}
	if h.Draws == nil {
		drawsUnavailable(w)
		return false
	}
	return true
}

func (h *handlers) listDraws(w http.ResponseWriter, r *http.Request) {
	if !h.publicDraws(w, r) {
		return
	}
	limit := 20
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 100 {
			httpx.WriteProblem(w, http.StatusBadRequest, "invalid-limit", "Invalid limit", "use 1 to 100")
			return
		}
		limit = n
	}
	list, err := h.Draws.List(r.Context(), limit)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]drawJSON, 0, len(list))
	for _, d := range list {
		out = append(out, toDrawJSON(d))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"draws": out})
}

func (h *handlers) getDraw(w http.ResponseWriter, r *http.Request) {
	if !h.publicDraws(w, r) {
		return
	}
	d, err := h.Draws.Get(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, toDrawJSON(d))
	case errors.Is(err, draws.ErrNotFound):
		drawNotFound(w)
	default:
		internalError(w, err)
	}
}

// drawCertificate serves the certificate byte for byte as it was hashed, so the download hashes to certificate_hash.
func (h *handlers) drawCertificate(w http.ResponseWriter, r *http.Request) {
	if !h.publicDraws(w, r) {
		return
	}
	raw, sum, err := h.Draws.CertificateOf(r.Context(), chi.URLParam(r, "id"))
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Certificate-SHA256", sum)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw) //nolint:gosec // our own stored JSON, served as application/json, never HTML
	case errors.Is(err, draws.ErrNotFound):
		drawNotFound(w)
	case errors.Is(err, draws.ErrState):
		httpx.WriteProblem(w, http.StatusConflict, "not-conducted", "The draw has not been conducted yet", "the certificate exists once the winners are drawn")
	default:
		internalError(w, err)
	}
}

func (h *handlers) myDrawEntries(w http.ResponseWriter, r *http.Request) {
	p, _ := principalFrom(r.Context())
	if !h.limit(w, r, "draws:user:"+p.UserID, 120, time.Minute) {
		return
	}
	if h.Draws == nil {
		drawsUnavailable(w)
		return
	}
	mine, err := h.Draws.EntriesOf(r.Context(), chi.URLParam(r, "id"), p.UserID)
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, mine)
	case errors.Is(err, draws.ErrNotFound):
		drawNotFound(w)
	default:
		internalError(w, err)
	}
}
