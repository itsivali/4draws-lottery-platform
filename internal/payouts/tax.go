package payouts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/ledger"
	"github.com/itsivali/4draws-lottery-platform/internal/payouts/store"
)

// Errors of KRA remittance batches.
var (
	ErrNothingToBatch   = errors.New("payouts: no withholding tax waits to be batched")
	ErrRemitted         = errors.New("payouts: the batch is already remitted")
	ErrRemittanceAmount = errors.New("payouts: the amount remitted must be exactly the batch's tax")
	ErrSamePerson       = errors.New("payouts: the remittance must be recorded by someone other than who prepared the batch")
	ErrPRNUsed          = errors.New("payouts: that payment registration number is already recorded")
	ErrBatchAltered     = errors.New("payouts: the batch's events no longer hash to its recorded SHA-256")
)

// TaxFileFormat names the layout of a batch file: a header line with the control totals, then one line per event.
const TaxFileFormat = "kra-withholding-v1"

// TaxEvent is one row of the KRA feed.
type TaxEvent struct {
	ID        int64           `json:"id"`
	EventType string          `json:"event_type"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// Batch is a remittance batch: withholding tax events frozen into one numbered file. PRN is KRA's payment
// registration number once the tax is remitted.
type Batch struct {
	ID           string     `json:"id"`
	No           int64      `json:"batch_no"`
	Events       int        `json:"events"`
	FirstEventAt time.Time  `json:"first_event_at"`
	LastEventAt  time.Time  `json:"last_event_at"`
	GrossKES     int64      `json:"gross_kes"`
	TaxKES       int64      `json:"tax_kes"`
	SHA256       string     `json:"file_sha256"`
	PreparedBy   string     `json:"prepared_by"`
	PreparedAt   time.Time  `json:"prepared_at"`
	PRN          string     `json:"prn,omitempty"`
	RemittedBy   string     `json:"remitted_by,omitempty"`
	RemittedAt   *time.Time `json:"remitted_at,omitempty"`
}

func batchView(b store.AuditKraBatch) Batch {
	return Batch{ID: b.ID, No: b.BatchNo, Events: int(b.Events), FirstEventAt: b.FirstEventAt, LastEventAt: b.LastEventAt, GrossKES: b.GrossKes,
		TaxKES: b.TaxKes, SHA256: b.FileSha256, PreparedBy: b.PreparedBy, PreparedAt: b.PreparedAt, PRN: deref(b.Prn),
		RemittedBy: deref(b.RemittedBy), RemittedAt: b.RemittedAt}
}

// batchHeader is a batch file's first line. Field order is fixed by the struct, so a file is rebuilt byte for byte.
type batchHeader struct {
	Format       string    `json:"format"`
	BatchNo      int64     `json:"batch_no"`
	PreparedBy   string    `json:"prepared_by"`
	PreparedAt   time.Time `json:"prepared_at"`
	Events       int       `json:"events"`
	GrossKES     int64     `json:"gross_kes"`
	TaxKES       int64     `json:"tax_kes"`
	FirstEventAt time.Time `json:"first_event_at"`
	LastEventAt  time.Time `json:"last_event_at"`
}

// taxFile writes a batch file: the header, then each event, one JSON object per line, times in UTC.
func taxFile(h batchHeader, events []TaxEvent) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	h.Format = TaxFileFormat
	h.PreparedAt, h.FirstEventAt, h.LastEventAt = h.PreparedAt.UTC(), h.FirstEventAt.UTC(), h.LastEventAt.UTC()
	if err := enc.Encode(h); err != nil {
		return nil, err
	}
	for _, e := range events {
		e.CreatedAt = e.CreatedAt.UTC()
		if err := enc.Encode(e); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func taxEvents(rows []store.KRABatchEventsRow) []TaxEvent {
	out := make([]TaxEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, TaxEvent{ID: r.ID, EventType: r.EventType, CreatedAt: r.CreatedAt, Payload: r.Payload})
	}
	return out
}

// eventAmounts reads what an event withheld.
func eventAmounts(e TaxEvent) (gross, tax int64, err error) {
	var v struct {
		Gross int64 `json:"gross_kes"`
		Tax   int64 `json:"tax_kes"`
	}
	err = json.Unmarshal(e.Payload, &v)
	return v.Gross, v.Tax, err
}

// PrepareBatch freezes every withholding event not yet batched (up to limit, oldest first) into the next numbered
// batch, with its control totals and the SHA-256 of its file, and returns the batch and the file. The file is what is
// filed with KRA; it can be rebuilt byte for byte later (BatchFile).
func (s *Service) PrepareBatch(ctx context.Context, operator string, limit int) (Batch, []byte, error) {
	if strings.TrimSpace(operator) == "" {
		return Batch{}, nil, ErrOperator
	}
	var out Batch
	var file []byte
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		// one batch at a time, so numbers have no gaps and no event lands in two batches
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('kra.batch'))"); err != nil {
			return err
		}
		rows, err := q.UnbatchedKRAEvents(ctx, int32(limit)) //nolint:gosec // small limits
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrNothingToBatch
		}
		events := make([]TaxEvent, 0, len(rows))
		ids := make([]int64, 0, len(rows))
		h := batchHeader{Events: len(rows), FirstEventAt: rows[0].CreatedAt, LastEventAt: rows[0].CreatedAt}
		for _, r := range rows {
			e := TaxEvent{ID: r.ID, EventType: r.EventType, CreatedAt: r.CreatedAt, Payload: r.Payload}
			gross, tax, err := eventAmounts(e)
			if err != nil {
				return err
			}
			h.GrossKES, h.TaxKES = h.GrossKES+gross, h.TaxKES+tax
			h.FirstEventAt, h.LastEventAt = minTime(h.FirstEventAt, r.CreatedAt), maxTime(h.LastEventAt, r.CreatedAt)
			events, ids = append(events, e), append(ids, r.ID)
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if h.BatchNo, err = q.NextKRABatchNo(ctx); err != nil {
			return err
		}
		// stored to the microsecond: the rebuilt header must carry the same time
		h.PreparedBy, h.PreparedAt = operator, now.Truncate(time.Microsecond)
		if file, err = taxFile(h, events); err != nil {
			return err
		}
		sum := sha256.Sum256(file)
		id, err := q.InsertKRABatch(ctx, store.InsertKRABatchParams{BatchNo: h.BatchNo, Events: int32(h.Events), FirstEventAt: h.FirstEventAt, //nolint:gosec // bounded by limit
			LastEventAt: h.LastEventAt, GrossKes: h.GrossKES, TaxKes: h.TaxKES, FileSha256: hex.EncodeToString(sum[:]), PreparedBy: operator,
			At: h.PreparedAt})
		if err != nil {
			return err
		}
		if err := q.AssignKRABatch(ctx, store.AssignKRABatchParams{BatchID: &id, At: h.PreparedAt, Ids: ids}); err != nil {
			return err
		}
		b, err := q.KRABatchByNo(ctx, h.BatchNo)
		if err != nil {
			return err
		}
		out = batchView(b)
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "kra.batch.prepared", EntityType: "kra_batch",
			EntityID: id, Details: map[string]any{"batch_no": h.BatchNo, "events": h.Events, "tax_kes": h.TaxKES, "file_sha256": out.SHA256}})
	})
	if err != nil {
		return Batch{}, nil, err
	}
	return out, file, nil
}

// BatchFile rebuilds a batch's file from its stored events and checks it still hashes to the recorded SHA-256.
func (s *Service) BatchFile(ctx context.Context, no int64) (Batch, []byte, error) {
	q := store.New(s.pool)
	b, err := q.KRABatchByNo(ctx, no)
	if notFound(err) {
		return Batch{}, nil, ErrNotFound
	} else if err != nil {
		return Batch{}, nil, err
	}
	file, err := rebuild(ctx, q, b)
	if err != nil {
		return Batch{}, nil, err
	}
	return batchView(b), file, nil
}

func rebuild(ctx context.Context, q *store.Queries, b store.AuditKraBatch) ([]byte, error) {
	rows, err := q.KRABatchEvents(ctx, &b.ID)
	if err != nil {
		return nil, err
	}
	file, err := taxFile(batchHeader{BatchNo: b.BatchNo, PreparedBy: b.PreparedBy, PreparedAt: b.PreparedAt, Events: int(b.Events),
		GrossKES: b.GrossKes, TaxKES: b.TaxKes, FirstEventAt: b.FirstEventAt, LastEventAt: b.LastEventAt}, taxEvents(rows))
	if err != nil {
		return nil, err
	}
	if sum := sha256.Sum256(file); hex.EncodeToString(sum[:]) != b.FileSha256 {
		return nil, ErrBatchAltered
	}
	return file, nil
}

// RecordRemittance records that a batch's tax was paid to KRA under the payment registration number prn: exactly the
// batch's tax, by someone other than who prepared it, once, and only while the batch still matches its file. It posts
// TAX_PAYABLE → OPERATOR_BANK.
func (s *Service) RecordRemittance(ctx context.Context, no int64, prn string, amount int64, operator string) (Batch, error) {
	prn = strings.TrimSpace(prn)
	if strings.TrimSpace(operator) == "" || prn == "" {
		return Batch{}, ErrOperator
	}
	var out Batch
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		b, err := q.LockKRABatchByNo(ctx, no)
		if notFound(err) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		switch {
		case b.Prn != nil:
			return ErrRemitted
		case amount != b.TaxKes:
			return ErrRemittanceAmount
		case operator == b.PreparedBy:
			return ErrSamePerson
		}
		// checked first: a unique violation would abort the transaction
		if used, err := q.PRNUsed(ctx, &prn); err != nil {
			return err
		} else if used {
			return ErrPRNUsed
		}
		if _, err := rebuild(ctx, q, b); err != nil {
			return err
		}
		now, err := s.clk.Now(ctx, tx)
		if err != nil {
			return err
		}
		if err := q.RecordKRARemittance(ctx, store.RecordKRARemittanceParams{Prn: &prn, RemittedKes: &amount, RemittedBy: &operator, At: &now, ID: b.ID}); err != nil {
			return err
		}
		posting, err := ledger.TaxRemittance(b.ID, amount)
		if err != nil {
			return err
		}
		if _, err := s.ledger.Post(ctx, tx, posting.Journal); err != nil {
			return err
		}
		if b, err = q.KRABatchByNo(ctx, no); err != nil {
			return err
		}
		out = batchView(b)
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: operator, Action: "kra.batch.remitted", EntityType: "kra_batch",
			EntityID: b.ID, Details: map[string]any{"batch_no": no, "prn": prn, "tax_kes": amount}})
	})
	return out, err
}

// Batches lists the latest remittance batches, newest first.
func (s *Service) Batches(ctx context.Context, limit int) ([]Batch, error) {
	rows, err := store.New(s.pool).KRABatches(ctx, int32(limit)) //nolint:gosec // small limits
	if err != nil {
		return nil, err
	}
	out := make([]Batch, 0, len(rows))
	for _, b := range rows {
		out = append(out, batchView(b))
	}
	return out, nil
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
