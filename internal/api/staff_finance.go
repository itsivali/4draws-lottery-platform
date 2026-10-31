package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/itsivali/lottery-platform/internal/finance"
	"github.com/itsivali/lottery-platform/internal/platform/httpx"
)

// The back office's Finance tab (owner, 2026-10-31): the ledger's figures by day, week or month with the sections an
// audit asks for, two CSV exports (each recorded in the audit trail with the file's hash), and the stored
// reconciliation runs.

// staffFinance is the financial dashboard over Nairobi days from..to (default: the last 30 days, today included),
// cut by group (default: day).
func (h *handlers) staffFinance(w http.ResponseWriter, r *http.Request) {
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	sum, err := h.StaffServices.Finance.Summary(r.Context(), from, to, finance.Group(r.URL.Query().Get("group")))
	if err != nil {
		staffError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sum)
}

// staffFinanceExport is the summary as a CSV file, with its SHA-256 in X-File-SHA256 as the tax files have.
func (h *handlers) staffFinanceExport(w http.ResponseWriter, r *http.Request) {
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	f, err := h.StaffServices.Finance.ExportSummary(r.Context(), from, to, finance.Group(r.URL.Query().Get("group")), member(r).Operator())
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

// staffLedgerExport streams every ledger entry of the range as CSV. The file is hashed from a snapshot before the
// first byte goes out, so X-File-SHA256 is a header here too; more than the cap of entries is refused.
func (h *handlers) staffLedgerExport(w http.ResponseWriter, r *http.Request) {
	from, to, ok := dateRange(w, r)
	if !ok {
		return
	}
	x, err := h.StaffServices.Finance.PrepareLedgerExport(r.Context(), from, to, member(r).Operator())
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
		slog.ErrorContext(r.Context(), "ledger export interrupted", "err", err, "from", x.From, "to", x.To)
	}
}

var reconciliationAreas = map[string]bool{"payments": true, "game": true, "draws": true, "payouts": true}

// staffReconciliations lists one area's stored reconciliation runs, newest first.
func (h *handlers) staffReconciliations(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	if !reconciliationAreas[qs.Get("area")] {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid-area", "Invalid area", "use payments, game, draws or payouts")
		return
	}
	limit, ok := pageLimit(w, r)
	if !ok {
		return
	}
	p, err := h.StaffServices.Oversight.Reconciliations(r.Context(), qs.Get("area"), limit, qs.Get("before"))
	if err != nil {
		oversightError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"runs": p.Rows, "next_before": nextOf(p.NextBefore)})
}
