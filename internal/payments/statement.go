package payments

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/payments/store"
)

// The M-Pesa statement (owner, 2026-10-31). Safaricom's organisation portal exports the business's completed
// transactions as CSV: one row per receipt with the completion time, the amount paid in, the payer (number, often
// masked, and name) and the account number given. An operator uploads it; the platform matches each paid row to the
// prompts it knows, reports what it found, and credits what the operator confirms. Nothing is credited on upload.

// Statement errors.
var (
	ErrStatementEmpty    = errors.New("payments: the statement has no rows the platform can read")
	ErrStatementTooLarge = errors.New("payments: the statement has too many rows; split it")
)

// MaxStatementRows bounds one import.
const MaxStatementRows = 20000

// StatementRow is one transaction on the statement.
type StatementRow struct {
	Line         int       `json:"line"`
	Receipt      string    `json:"receipt"`
	CompletedAt  time.Time `json:"completed_at"`
	PaidInKES    int64     `json:"paid_in_kes"`
	WithdrawnKES int64     `json:"withdrawn_kes,omitempty"`
	Party        string    `json:"party"`        // the other party as printed: "254796***457 - WILLIS IVALI"
	PartyNumber  string    `json:"party_number"` // the digits (and stars) of the party's number, normalised to 254…
	Account      string    `json:"account"`      // the account number / bill reference given
	Details      string    `json:"details"`
	Status       string    `json:"status"`
}

var nairobi = mustLoad("Africa/Nairobi")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("EAT", 3*3600)
	}
	return loc
}

// ParseStatement reads an M-Pesa statement CSV. Columns are found by their headers, wherever they are and whatever
// their case ("Receipt No.", "Completion Time", "Paid In", "Other Party Info", "A/C No.", "Details", "Transaction
// Status"); rows without a receipt or a time are skipped and reported by line. Times without a zone are Nairobi time.
func ParseStatement(r io.Reader) (rows []StatementRow, skipped []string, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	var cols map[string]int
	line := 0
	for {
		rec, rerr := cr.Read()
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			var pe *csv.ParseError
			if errors.As(rerr, &pe) {
				skipped = append(skipped, "line "+strconv.Itoa(pe.Line)+": "+pe.Err.Error())
				continue
			}
			return nil, nil, rerr
		}
		line++
		if cols == nil {
			if c := headerColumns(rec); c != nil {
				cols = c
			}
			continue
		}
		if len(rows) >= MaxStatementRows {
			return nil, nil, ErrStatementTooLarge
		}
		row, reason := statementRow(rec, cols, line)
		if reason != "" {
			if strings.TrimSpace(strings.Join(rec, "")) != "" {
				skipped = append(skipped, "line "+strconv.Itoa(line)+": "+reason)
			}
			continue
		}
		rows = append(rows, row)
	}
	if cols == nil || len(rows) == 0 {
		return nil, skipped, ErrStatementEmpty
	}
	return rows, skipped, nil
}

// headerColumns recognises the header row by its receipt and time columns.
func headerColumns(rec []string) map[string]int {
	cols := map[string]int{}
	for i, h := range rec {
		k := strings.ToLower(strings.TrimSpace(h))
		k = strings.NewReplacer(".", "", "_", " ").Replace(k)
		switch {
		case strings.HasPrefix(k, "receipt"):
			cols["receipt"] = i
		case strings.Contains(k, "completion"):
			cols["time"] = i
		case strings.Contains(k, "initiation") && cols["time"] == 0:
			cols["initiated"] = i
		case strings.Contains(k, "paid in"), k == "credit", k == "amount":
			if _, ok := cols["paid"]; !ok {
				cols["paid"] = i
			}
		case strings.Contains(k, "withdrawn"), k == "debit":
			cols["withdrawn"] = i
		case strings.Contains(k, "other party"), strings.Contains(k, "sender"), strings.Contains(k, "payer"):
			cols["party"] = i
		case strings.HasPrefix(k, "a/c"), strings.Contains(k, "account"), strings.Contains(k, "bill ref"):
			cols["account"] = i
		case k == "details", k == "description":
			cols["details"] = i
		case strings.Contains(k, "status"):
			cols["status"] = i
		}
	}
	if _, ok := cols["receipt"]; !ok {
		return nil
	}
	if _, ok := cols["time"]; !ok {
		if i, ok := cols["initiated"]; ok {
			cols["time"] = i
		} else {
			return nil
		}
	}
	return cols
}

func field(rec []string, cols map[string]int, name string) string {
	i, ok := cols[name]
	if !ok || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

var timeLayouts = []string{"2006-01-02 15:04:05", "02-01-2006 15:04:05", "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04", "02/01/2006 15:04:05",
	"2/1/2006 15:04:05", "2006-01-02", "02-01-2006"}

func parseStatementTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, nairobi); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

var amountClean = regexp.MustCompile(`[^0-9.\-]`)

// parseKES reads "1,000.00", "KES 1,000" or "1000" as whole shillings (cents are dropped: M-Pesa pays whole shillings).
func parseKES(s string) (int64, bool) {
	s = amountClean.ReplaceAllString(s, "")
	if s == "" || s == "-" {
		return 0, true
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int64(f + 0.0001), true
}

func statementRow(rec []string, cols map[string]int, line int) (StatementRow, string) {
	receipt := strings.ToUpper(strings.TrimSpace(field(rec, cols, "receipt")))
	if receipt == "" {
		return StatementRow{}, "no receipt"
	}
	if !receiptRE.MatchString(receipt) {
		return StatementRow{}, "receipt " + receipt + " is not 10 letters and digits"
	}
	at, ok := parseStatementTime(field(rec, cols, "time"))
	if !ok {
		return StatementRow{}, "unreadable time " + field(rec, cols, "time")
	}
	paid, ok := parseKES(field(rec, cols, "paid"))
	if !ok {
		return StatementRow{}, "unreadable amount " + field(rec, cols, "paid")
	}
	withdrawn, _ := parseKES(field(rec, cols, "withdrawn"))
	party := field(rec, cols, "party")
	return StatementRow{Line: line, Receipt: receipt, CompletedAt: at, PaidInKES: paid, WithdrawnKES: withdrawn, Party: party,
		PartyNumber: partyNumber(party), Account: field(rec, cols, "account"), Details: field(rec, cols, "details"), Status: field(rec, cols, "status")}, ""
}

// partyNumber takes the number out of "254796***457 - NAME" or "0796992457 - NAME": digits and stars, in 254 form.
func partyNumber(party string) string {
	n, _, _ := strings.Cut(party, " - ")
	n = strings.TrimSpace(n)
	var b strings.Builder
	for _, r := range n {
		if (r >= '0' && r <= '9') || r == '*' {
			b.WriteRune(r)
		}
	}
	n = b.String()
	switch {
	case strings.HasPrefix(n, "0") && len(n) == 10:
		return "254" + n[1:]
	case strings.HasPrefix(n, "7") || strings.HasPrefix(n, "1"):
		if len(n) == 9 {
			return "254" + n
		}
	}
	return n
}

// samePhone compares a payment's number with the statement's, which M-Pesa often masks ("254796***457").
func samePhone(msisdn, party string) bool {
	if party == "" || msisdn == "" {
		return false
	}
	if !strings.Contains(party, "*") {
		return party == msisdn
	}
	if len(party) != len(msisdn) {
		return false
	}
	for i := range party {
		if party[i] != '*' && party[i] != msisdn[i] {
			return false
		}
	}
	return true
}

// StatementMatch is what the platform found for one statement row.
type StatementMatch struct {
	Row StatementRow `json:"row"`
	// Kind: credited (already credited with this receipt), claimed (the receipt a player gave for this top-up),
	// matched (one prompt with this amount, number and time), ambiguous (several), unmatched (no prompt fits), skipped
	// (not a payment in: a withdrawal, a charge, a reversal).
	Kind       string            `json:"kind"`
	PaymentID  string            `json:"payment_id,omitempty"`
	UserID     string            `json:"user_id,omitempty"`
	MSISDN     string            `json:"msisdn,omitempty"`
	Status     string            `json:"status,omitempty"` // the payment's status now
	Reason     string            `json:"reason,omitempty"`
	Candidates []StatementOption `json:"candidates,omitempty"`
}

// StatementOption is one prompt that may be a statement row.
type StatementOption struct {
	PaymentID string    `json:"payment_id"`
	UserID    string    `json:"user_id"`
	MSISDN    string    `json:"msisdn"`
	AmountKES int64     `json:"amount_kes"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Why       string    `json:"why"`
}

// MissingCredit is a credited prompt in the statement's window that the statement does not show: it was credited
// from a callback or a receipt the statement should carry, so either the statement is partial or the credit is wrong.
type MissingCredit struct {
	PaymentID string    `json:"payment_id"`
	UserID    string    `json:"user_id"`
	Receipt   string    `json:"receipt"`
	AmountKES int64     `json:"amount_kes"`
	CreatedAt time.Time `json:"created_at"`
}

// StatementReport is the result of matching a statement, before anything is credited.
type StatementReport struct {
	SHA256   string            `json:"sha256"`
	Rows     int               `json:"rows"`
	Skipped  []string          `json:"skipped,omitempty"`
	From     time.Time         `json:"from"`
	To       time.Time         `json:"to"`
	Counts   map[string]int    `json:"counts"`
	Matches  []StatementMatch  `json:"matches"`
	Missing  []MissingCredit   `json:"missing_from_statement,omitempty"`
	ToCredit []StatementCredit `json:"to_credit"` // the matched and claimed rows, ready to apply
}

// StatementCredit is one top-up the operator confirms from the statement.
type StatementCredit struct {
	PaymentID string `json:"payment_id"`
	Receipt   string `json:"receipt"`
	AmountKES int64  `json:"amount_kes"`
}

const (
	statementMargin = 3 * time.Hour // prompts are looked for this far around the statement's own span
	matchBefore     = 2 * time.Hour // a prompt may precede its M-Pesa completion by this much (a slow PIN, a retry)
	matchAfter      = 20 * time.Minute
)

// MatchStatement matches parsed statement rows to the platform's prompts. Reading only.
func (s *Service) MatchStatement(ctx context.Context, body []byte) (StatementReport, error) {
	rows, skipped, err := ParseStatement(strings.NewReader(string(body)))
	if err != nil {
		return StatementReport{}, err
	}
	sum := sha256.Sum256(body)
	rep := StatementReport{SHA256: hex.EncodeToString(sum[:]), Rows: len(rows), Skipped: skipped, Counts: map[string]int{}}
	rep.From, rep.To = rows[0].CompletedAt, rows[0].CompletedAt
	receipts := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.CompletedAt.Before(rep.From) {
			rep.From = r.CompletedAt
		}
		if r.CompletedAt.After(rep.To) {
			rep.To = r.CompletedAt
		}
		receipts = append(receipts, r.Receipt)
	}
	q := store.New(s.pool)
	known, err := q.PaymentsByReceipts(ctx, receipts)
	if err != nil {
		return rep, err
	}
	byReceipt := map[string]store.PaymentsByReceiptsRow{}
	byClaim := map[string]store.PaymentsByReceiptsRow{}
	for _, k := range known {
		if k.MpesaReceipt != nil {
			byReceipt[*k.MpesaReceipt] = k
		}
		if k.ClaimedReceipt != nil {
			byClaim[*k.ClaimedReceipt] = k
		}
	}
	cands, err := q.StatementCandidates(ctx, store.StatementCandidatesParams{FromAt: rep.From.Add(-statementMargin), ToAt: rep.To.Add(statementMargin)})
	if err != nil {
		return rep, err
	}
	credited, err := q.CreditedBetween(ctx, store.CreditedBetweenParams{FromAt: rep.From.Add(-matchBefore), ToAt: rep.To.Add(matchAfter)})
	if err != nil {
		return rep, err
	}
	taken := map[string]bool{} // candidates already matched to a row
	onStatement := map[string]bool{}
	for _, r := range rows {
		onStatement[r.Receipt] = true
		m := StatementMatch{Row: r}
		switch {
		case r.PaidInKES <= 0:
			m.Kind, m.Reason = "skipped", "not a payment in"
		case r.Status != "" && !strings.EqualFold(r.Status, "completed"):
			m.Kind, m.Reason = "skipped", "status "+r.Status
		default:
			if k, ok := byReceipt[r.Receipt]; ok {
				m.Kind, m.PaymentID, m.UserID, m.Status = "credited", k.ID, k.UserID, k.Status
				break
			}
			if k, ok := byClaim[r.Receipt]; ok && uncredited(k.Status) {
				m.Kind, m.PaymentID, m.UserID, m.Status, m.Reason = "claimed", k.ID, k.UserID, k.Status, "the player gave this receipt"
				taken[k.ID] = true
				break
			}
			var fit []StatementOption
			for _, c := range cands {
				if taken[c.ID] || c.Msisdn == nil || c.AmountRequestedKes != r.PaidInKES {
					continue
				}
				if c.CreatedAt.Before(r.CompletedAt.Add(-matchBefore)) || c.CreatedAt.After(r.CompletedAt.Add(matchAfter)) {
					continue
				}
				why := "same amount and time"
				if samePhone(*c.Msisdn, r.PartyNumber) {
					why = "same amount, number and time"
				} else if r.PartyNumber != "" {
					continue
				}
				fit = append(fit, StatementOption{PaymentID: c.ID, UserID: c.UserID, MSISDN: *c.Msisdn, AmountKES: c.AmountRequestedKes, Status: c.Status, CreatedAt: c.CreatedAt, Why: why})
			}
			switch len(fit) {
			case 0:
				m.Kind, m.Reason = "unmatched", "no prompt with this amount and number around this time"
			case 1:
				m.Kind, m.PaymentID, m.UserID, m.MSISDN, m.Status, m.Reason = "matched", fit[0].PaymentID, fit[0].UserID, fit[0].MSISDN, fit[0].Status, fit[0].Why
				taken[fit[0].PaymentID] = true
			default:
				sort.Slice(fit, func(i, j int) bool { return fit[i].CreatedAt.After(fit[j].CreatedAt) })
				m.Kind, m.Candidates, m.Reason = "ambiguous", fit, "several prompts fit; pick one"
			}
		}
		rep.Counts[m.Kind]++
		if m.Kind == "matched" || m.Kind == "claimed" {
			rep.ToCredit = append(rep.ToCredit, StatementCredit{PaymentID: m.PaymentID, Receipt: r.Receipt, AmountKES: r.PaidInKES})
		}
		rep.Matches = append(rep.Matches, m)
	}
	for _, c := range credited {
		if c.MpesaReceipt == nil || strings.HasPrefix(*c.MpesaReceipt, "SANDBOX-") || onStatement[*c.MpesaReceipt] {
			continue
		}
		rep.Missing = append(rep.Missing, MissingCredit{PaymentID: c.ID, UserID: c.UserID, Receipt: *c.MpesaReceipt, AmountKES: derefInt(c.AmountConfirmedKes), CreatedAt: c.CreatedAt})
	}
	if rep.ToCredit == nil {
		rep.ToCredit = []StatementCredit{}
	}
	return rep, nil
}

func derefInt(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// StatementApply is what an operator confirms from a matched statement.
type StatementApply struct {
	Filename string            `json:"filename"`
	SHA256   string            `json:"sha256"`
	Rows     int               `json:"rows"`
	Credits  []StatementCredit `json:"credits"`
	Report   json.RawMessage   `json:"report"` // the matching report shown to the operator, kept with the import
}

// StatementResult is the outcome of one confirmed credit.
type StatementResult struct {
	PaymentID string `json:"payment_id"`
	Receipt   string `json:"receipt"`
	AmountKES int64  `json:"amount_kes"`
	Credited  bool   `json:"credited"`
	Error     string `json:"error,omitempty"`
}

// StatementApplied is the record of an import.
type StatementApplied struct {
	ImportID    string            `json:"import_id"`
	Credited    int               `json:"credited"`
	CreditedKES int64             `json:"credited_kes"`
	Results     []StatementResult `json:"results"`
}

// ApplyStatement credits the top-ups the operator confirmed, each on its own (one refusal does not stop the others),
// and records the import with the report and the outcomes. Every credit is audited as a resolution from the statement.
func (s *Service) ApplyStatement(ctx context.Context, in StatementApply, operator string) (StatementApplied, error) {
	if len(in.Credits) > MaxStatementRows {
		return StatementApplied{}, ErrStatementTooLarge
	}
	if !shaRE.MatchString(in.SHA256) {
		return StatementApplied{}, errors.New("payments: the statement sha256 is not one MatchStatement produced")
	}
	out := StatementApplied{Results: make([]StatementResult, 0, len(in.Credits))}
	for _, c := range in.Credits {
		res := StatementResult{PaymentID: c.PaymentID, Receipt: strings.ToUpper(strings.TrimSpace(c.Receipt)), AmountKES: c.AmountKES}
		err := s.Resolve(ctx, Resolution{PaymentID: c.PaymentID, Receipt: c.Receipt, AmountKES: c.AmountKES, Source: "statement", Note: "statement " + in.Filename}, operator)
		if err != nil {
			res.Error = userError(err)
		} else {
			res.Credited = true
			out.Credited++
			out.CreditedKES += c.AmountKES
		}
		out.Results = append(out.Results, res)
	}
	report := in.Report
	if len(report) == 0 {
		report = json.RawMessage(`{}`)
	}
	results, _ := json.Marshal(out.Results)
	full, _ := json.Marshal(map[string]json.RawMessage{"matching": report, "results": results})
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = "statement.csv"
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		id, err := store.New(tx).InsertStatementImport(ctx, store.InsertStatementImportParams{ImportedBy: operator, ImportedAt: now, Filename: filename,
			Sha256: in.SHA256, RowsTotal: int32(min(in.Rows, MaxStatementRows)), Matched: int32(len(in.Credits)), Credited: int32(out.Credited), //nolint:gosec // bounded above
			CreditedKes: out.CreditedKES, Report: full})
		if err != nil {
			return err
		}
		out.ImportID = id
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "payment.statement.imported", EntityType: "statement_import",
			EntityID: id, Details: map[string]any{"filename": filename, "sha256": in.SHA256, "rows": in.Rows, "confirmed": len(in.Credits), "credited": out.Credited, "credited_kes": out.CreditedKES}})
	})
	return out, err
}

var shaRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// userError is the short reason an operator sees for a credit that was refused.
func userError(err error) string {
	switch {
	case errors.Is(err, ErrNotFound):
		return "no such top-up"
	case errors.Is(err, ErrNotResolvable):
		return "the top-up is not settled uncredited"
	case errors.Is(err, ErrReceiptUsed):
		return "the receipt already credited another top-up"
	case errors.Is(err, ErrInvalidReceipt):
		return "not an M-Pesa receipt"
	case errors.Is(err, ErrInvalidAmount):
		return "invalid amount"
	}
	return "could not be credited"
}

// StatementImport is a past import.
type StatementImport struct {
	ID          string          `json:"id"`
	ImportedBy  string          `json:"imported_by"`
	ImportedAt  time.Time       `json:"imported_at"`
	Filename    string          `json:"filename"`
	SHA256      string          `json:"sha256"`
	Rows        int32           `json:"rows"`
	Matched     int32           `json:"matched"`
	Credited    int32           `json:"credited"`
	CreditedKES int64           `json:"credited_kes"`
	Report      json.RawMessage `json:"report"`
}

// StatementImports lists past imports, newest first.
func (s *Service) StatementImports(ctx context.Context, limit int) ([]StatementImport, error) {
	rows, err := store.New(s.pool).ListStatementImports(ctx, int32(limit)) //nolint:gosec // callers pass small limits
	if err != nil {
		return nil, err
	}
	out := make([]StatementImport, 0, len(rows))
	for _, r := range rows {
		out = append(out, StatementImport{ID: r.ID, ImportedBy: r.ImportedBy, ImportedAt: r.ImportedAt, Filename: r.Filename, SHA256: r.Sha256,
			Rows: r.RowsTotal, Matched: r.Matched, Credited: r.Credited, CreditedKES: r.CreditedKes, Report: r.Report})
	}
	return out, nil
}
