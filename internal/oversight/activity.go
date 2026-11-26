package oversight

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/platform/clock"
)

// The audit trail as the back office reads it (owner, 2026-10-31: the back office must help with audits). An
// administrator narrows it by who acted, by the action's prefix, by Nairobi day and by the entity, and takes it away
// as a CSV file whose hash is announced before the first byte and whose export is itself audited.

// ErrRange is a day filter that is not YYYY-MM-DD, or runs backwards.
var ErrRange = errors.New("oversight: invalid day range")

// ErrTooManyRows is an export of more records than the cap.
var ErrTooManyRows = errors.New("oversight: too many records for one file; narrow the filters")

// ExportRowCap is the most records one activity export may hold.
const ExportRowCap = 100_000

// Event is one audit record.
type Event struct {
	ID         string          `json:"id"`
	At         time.Time       `json:"at"`
	ActorType  string          `json:"actor_type"` // system, player or staff
	ActorID    *string         `json:"actor_id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   *string         `json:"entity_id"`
	Details    json.RawMessage `json:"details"`
}

// EventFilter narrows the audit trail. Query matches a fragment of the action, the entity id or type, or the actor;
// ActorType is one kind; From and To are Nairobi days (YYYY-MM-DD, both inclusive); ActionPrefix is the start of the
// action ("payment." for every payment event); EntityType, EntityID and ActorID match exactly.
type EventFilter struct {
	Query        string
	ActorType    string
	From, To     string
	ActionPrefix string
	EntityType   string
	EntityID     string
	ActorID      string
}

// bounds turns the day range into instants: [from 00:00, to 24:00) Nairobi.
func (f EventFilter) bounds() (from, to *time.Time, err error) {
	day := func(s string) (*time.Time, error) {
		if s == "" {
			return nil, nil
		}
		t, err := time.ParseInLocation(time.DateOnly, s, clock.Nairobi)
		if err != nil {
			return nil, ErrRange
		}
		return &t, nil
	}
	if from, err = day(f.From); err != nil {
		return nil, nil, err
	}
	if to, err = day(f.To); err != nil {
		return nil, nil, err
	}
	if to != nil {
		if from != nil && to.Before(*from) {
			return nil, nil, ErrRange
		}
		next := to.AddDate(0, 0, 1)
		to = &next
	}
	return from, to, nil
}

// prefix is a LIKE pattern matching strings that start with p (escaped), or nil for no prefix.
func prefix(p string) *string {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil
	}
	p = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(p) + "%"
	return &p
}

func exact(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// eventsWhere is shared by the page, the export and its count: $1 query, $2 actor type, $3 from, $4 to, $5 action
// prefix, $6 entity type, $7 entity id, $8 actor id.
const eventsWhere = `
  ($1::text IS NULL OR action LIKE $1 OR entity_id LIKE $1 OR actor_id LIKE $1 OR entity_type LIKE $1)
  AND ($2::text IS NULL OR actor_type = $2)
  AND ($3::timestamptz IS NULL OR at >= $3)
  AND ($4::timestamptz IS NULL OR at < $4)
  AND ($5::text IS NULL OR action LIKE $5)
  AND ($6::text IS NULL OR entity_type = $6)
  AND ($7::text IS NULL OR entity_id = $7)
  AND ($8::text IS NULL OR actor_id = $8)`

const (
	eventColumns = `id::text, at, actor_type, actor_id, action, entity_type, entity_id, details`
	eventsSQL    = `SELECT ` + eventColumns + ` FROM audit.audit_log WHERE ($9::bigint IS NULL OR id < $9) AND ` + eventsWhere + ` ORDER BY id DESC LIMIT $10`
	exportSQL    = `SELECT ` + eventColumns + ` FROM audit.audit_log WHERE ` + eventsWhere + ` ORDER BY id`
	countSQL     = `SELECT count(*) FROM audit.audit_log WHERE ` + eventsWhere
)

func (f EventFilter) args() ([]any, error) {
	from, to, err := f.bounds()
	if err != nil {
		return nil, err
	}
	var actor *string
	if f.ActorType != "" {
		actor = &f.ActorType
	}
	return []any{like(f.Query), actor, from, to, prefix(f.ActionPrefix), exact(f.EntityType), exact(f.EntityID), exact(f.ActorID)}, nil
}

func scanEvent(r pgx.CollectableRow) (Event, error) {
	var e Event
	err := r.Scan(&e.ID, &e.At, &e.ActorType, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID, &e.Details)
	return e, err
}

// Activity lists audit records, newest first. Its cursor is the last record's id.
func (s *Service) Activity(ctx context.Context, f EventFilter, limit int, before string) (Page[Event], error) {
	var beforeID *int64
	if before != "" {
		n, err := strconv.ParseInt(before, 10, 64)
		if err != nil || n <= 0 {
			return Page[Event]{}, ErrCursor
		}
		beforeID = &n
	}
	args, err := f.args()
	if err != nil {
		return Page[Event]{}, err
	}
	rows, err := s.pool.Query(ctx, eventsSQL, append(args, beforeID, limit+1)...)
	if err != nil {
		return Page[Event]{}, err
	}
	out, err := pgx.CollectRows(rows, scanEvent)
	if err != nil {
		return Page[Event]{}, err
	}
	return page(out, limit, func(e Event) string { return e.ID }), nil
}

// ActivityColumns are the activity export's columns, in order.
var ActivityColumns = []string{"id", "at_eat", "actor_type", "actor_id", "actor_name", "action", "entity_type", "entity_id", "details_json"}

// ActivityExport is an export ready to stream: how many records it holds and the file's SHA-256, known before the
// first byte is sent because the file was hashed once already from the same database snapshot.
type ActivityExport struct {
	Name   string
	Rows   int64
	SHA256 string
	filter EventFilter
	args   []any
	names  map[string]string
	tx     pgx.Tx
}

// PrepareActivityExport checks the filters, refuses more than ExportRowCap records (ErrTooManyRows), opens a read-only
// snapshot, hashes the file and records the export as activity.exported by viewer with the filters, the row count
// and the hash. names turns actor ids into names (staff members) in the file. The caller streams the file with
// WriteTo and must Close the export.
func (s *Service) PrepareActivityExport(ctx context.Context, f EventFilter, names map[string]string, viewer string) (*ActivityExport, error) {
	args, err := f.args()
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	x := &ActivityExport{filter: f, args: args, names: names, tx: tx}
	ok := false
	defer func() {
		if !ok {
			x.Close(ctx)
		}
	}()
	if err := tx.QueryRow(ctx, countSQL, args...).Scan(&x.Rows); err != nil {
		return nil, err
	}
	if x.Rows > ExportRowCap {
		return nil, ErrTooManyRows
	}
	h := sha256.New()
	if _, err := x.stream(ctx, h); err != nil {
		return nil, err
	}
	x.SHA256 = hex.EncodeToString(h.Sum(nil))
	x.Name = exportName("activity", f)
	details := map[string]any{"rows": x.Rows, "sha256": x.SHA256, "file": x.Name}
	for k, v := range map[string]string{"q": f.Query, "actor_type": f.ActorType, "from": f.From, "to": f.To, "action": f.ActionPrefix,
		"entity_type": f.EntityType, "entity_id": f.EntityID, "actor_id": f.ActorID} {
		if strings.TrimSpace(v) != "" {
			details[k] = strings.TrimSpace(v)
		}
	}
	err = pgx.BeginFunc(ctx, s.pool, func(atx pgx.Tx) error {
		return audit.NewWriter().Record(ctx, atx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "activity.exported",
			EntityType: "activity_export", Details: details})
	})
	if err != nil {
		return nil, err
	}
	ok = true
	return x, nil
}

// exportName names a file after its day range: activity-2026-10-01-2026-10-31.csv, activity-all.csv when there is none.
func exportName(what string, f EventFilter) string {
	switch {
	case f.From != "" && f.To != "":
		return what + "-" + f.From + "-" + f.To + ".csv"
	case f.From != "":
		return what + "-from-" + f.From + ".csv"
	case f.To != "":
		return what + "-to-" + f.To + ".csv"
	}
	return what + "-all.csv"
}

// WriteTo streams the file to w from the same snapshot it was hashed from, and checks the bytes sent hash to SHA256.
func (x *ActivityExport) WriteTo(ctx context.Context, w io.Writer) error {
	h := sha256.New()
	rows, err := x.stream(ctx, io.MultiWriter(w, h))
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != x.SHA256 || rows != x.Rows {
		return fmt.Errorf("oversight: activity export changed while streaming (%d rows, sha %s; announced %d, %s)", rows, got, x.Rows, x.SHA256)
	}
	return nil
}

// Close ends the snapshot.
func (x *ActivityExport) Close(ctx context.Context) {
	if err := x.tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		slog.WarnContext(ctx, "oversight: closing an activity export snapshot", "err", err)
	}
}

func (x *ActivityExport) stream(ctx context.Context, w io.Writer) (int64, error) {
	rows, err := x.tx.Query(ctx, exportSQL, x.args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cw := csv.NewWriter(w)
	if err := cw.Write(ActivityColumns); err != nil {
		return 0, err
	}
	var count int64
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return count, err
		}
		actor, entity := deref(e.ActorID), deref(e.EntityID)
		if err := cw.Write([]string{e.ID, EAT(e.At), e.ActorType, actor, x.names[actor], e.Action, e.EntityType, entity, string(e.Details)}); err != nil {
			return count, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	cw.Flush()
	return count, cw.Error()
}

// EAT writes an instant as a Nairobi wall-clock time, the way the files show times.
func EAT(t time.Time) string { return clock.InNairobi(t).Format("2006-01-02 15:04:05") }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
