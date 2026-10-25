package draws

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/draws/store"
)

// Draw is what is known about a draw. RevealedSeed, the beacon and the certificate exist once it is conducted.
type Draw struct {
	ID              string          `json:"id"`
	Type            string          `json:"draw_type"`
	PeriodStart     string          `json:"period_start"`
	PeriodEnd       string          `json:"period_end"`
	Status          string          `json:"status"`
	FreezeAt        time.Time       `json:"freeze_at"`
	EntryListHash   string          `json:"entry_list_hash,omitempty"`
	SeedCommit      string          `json:"seed_commit,omitempty"`
	BeaconRound     int64           `json:"drand_round,omitempty"`
	RoundAt         *time.Time      `json:"drand_round_at,omitempty"`
	RevealedSeed    string          `json:"revealed_seed,omitempty"`
	Certificate     json.RawMessage `json:"certificate,omitempty"`
	CertificateHash string          `json:"certificate_hash,omitempty"`
	VoidedReason    string          `json:"voided_reason,omitempty"`
	RerunOf         string          `json:"rerun_of,omitempty"`
	Entries         int             `json:"entries"`
	Signoffs        []Signoff       `json:"signoffs"`
	Results         []Result        `json:"results"`
}

// Signoff is one official's step.
type Signoff struct {
	Step      string    `json:"step"`
	Official  string    `json:"official"`
	Signature string    `json:"attestation"`
	SignedAt  time.Time `json:"signed_at"`
}

// Result is one awarded rank.
type Result struct {
	Rank      int    `json:"rank"`
	AmountKES int64  `json:"amount_kes"`
	EntryNo   int    `json:"entry_no"`
	UserID    string `json:"user_id"`
}

// Get returns one draw with its sign-offs and results.
func (s *Service) Get(ctx context.Context, id string) (Draw, error) {
	q := store.New(s.pool)
	d, err := q.GetDraw(ctx, id)
	var pe *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
		return Draw{}, ErrNotFound
	}
	if err != nil {
		return Draw{}, err
	}
	out := Draw{ID: d.ID, Type: d.DrawType, PeriodStart: d.PeriodStart.Format(time.DateOnly), PeriodEnd: d.PeriodEnd.Format(time.DateOnly),
		Status: d.Status, FreezeAt: FreezeTime(d.PeriodEnd), EntryListHash: deref(d.EntryListHash), SeedCommit: deref(d.SeedCommit),
		BeaconRound: derefInt(d.BeaconRound), RevealedSeed: deref(d.RevealedSeed), Certificate: d.CertificateJson,
		CertificateHash: deref(d.CertificateHash), VoidedReason: deref(d.VoidedReason), RerunOf: deref(d.RerunOf),
		Signoffs: []Signoff{}, Results: []Result{}}
	if d.BeaconRound != nil {
		t := s.roundTime(d)
		out.RoundAt = &t
	}
	n, err := q.EntryCount(ctx, d.ID)
	if err != nil {
		return Draw{}, err
	}
	out.Entries = int(n)
	signs, err := q.Signoffs(ctx, d.ID)
	if err != nil {
		return Draw{}, err
	}
	for _, so := range signs {
		out.Signoffs = append(out.Signoffs, Signoff{Step: so.Step, Official: so.Official, Signature: so.Signature, SignedAt: so.SignedAt})
	}
	results, err := q.Results(ctx, d.ID)
	if err != nil {
		return Draw{}, err
	}
	for _, r := range results {
		out.Results = append(out.Results, Result{Rank: int(r.PrizeRank), AmountKES: r.AmountKes, EntryNo: int(r.EntryNo), UserID: r.UserID})
	}
	return out, nil
}

// List returns the most recent draws, newest period first.
func (s *Service) List(ctx context.Context, limit int) ([]Draw, error) {
	ids, err := store.New(s.pool).ListDraws(ctx, int32(limit)) //nolint:gosec // callers pass small limits
	if err != nil {
		return nil, err
	}
	out := make([]Draw, 0, len(ids))
	for _, id := range ids {
		d, err := s.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// CertificateOf returns a conducted draw's certificate exactly as it was hashed, with its hash.
func (s *Service) CertificateOf(ctx context.Context, id string) ([]byte, string, error) {
	d, err := store.New(s.pool).GetDraw(ctx, id)
	var pe *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if d.CertificateJson == nil {
		return nil, "", ErrState
	}
	return d.CertificateJson, deref(d.CertificateHash), nil
}

// MyEntry is one of a player's entries in a draw.
type MyEntry struct {
	EntryNo  int    `json:"entry_no"`
	PlayDate string `json:"play_date"`
	TicketID string `json:"ticket_id"`
}

// MyEntries is what a player can see of their part in a draw.
type MyEntries struct {
	DrawID  string    `json:"draw_id"`
	UserID  string    `json:"user_id"`
	Entries []MyEntry `json:"entries"`
	Prizes  []Result  `json:"prizes"`
}

// EntriesOf returns a player's own entries in a draw and any prize they won.
func (s *Service) EntriesOf(ctx context.Context, drawID, userID string) (MyEntries, error) {
	d, err := s.Get(ctx, drawID)
	if err != nil {
		return MyEntries{}, err
	}
	rows, err := store.New(s.pool).EntriesOf(ctx, store.EntriesOfParams{DrawID: d.ID, UserID: userID})
	if err != nil {
		return MyEntries{}, err
	}
	out := MyEntries{DrawID: d.ID, UserID: userID, Entries: make([]MyEntry, 0, len(rows)), Prizes: []Result{}}
	for _, e := range rows {
		out.Entries = append(out.Entries, MyEntry{EntryNo: int(e.EntryNo), PlayDate: e.PlayDate.Format(time.DateOnly), TicketID: e.TicketID})
	}
	for _, r := range d.Results {
		if r.UserID == userID {
			out.Prizes = append(out.Prizes, r)
		}
	}
	return out, nil
}

// ListEntry is one line of a draw's full entry list, the input that hashes to entry_list_hash.
type ListEntry struct {
	EntryNo  int    `json:"entry_no"`
	UserID   string `json:"user_id"`
	PlayDate string `json:"play_date"`
	TicketID string `json:"ticket_id"`
}

// EntryList returns a draw's full entry list in entry order (for the regulator and tools/verifier; never public).
func (s *Service) EntryList(ctx context.Context, drawID string) ([]ListEntry, error) {
	d, err := s.Get(ctx, drawID)
	if err != nil {
		return nil, err
	}
	rows, err := store.New(s.pool).Entries(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ListEntry, 0, len(rows))
	for _, e := range rows {
		out = append(out, ListEntry{EntryNo: int(e.EntryNo), UserID: e.UserID, PlayDate: e.PlayDate.Format(time.DateOnly), TicketID: e.TicketID})
	}
	return out, nil
}
