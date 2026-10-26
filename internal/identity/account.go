package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity/store"
)

// Account is what a player sees about their own account.
type Account struct {
	UserID             string
	MSISDN             string
	PhoneVerified      bool
	KYCStatus          string
	Status             string
	CreatedAt          time.Time
	ClosureRequestedAt *time.Time
}

// Account returns the player's account.
func (s *Service) Account(ctx context.Context, userID string) (Account, error) {
	u, err := store.New(s.pool).GetAccount(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	return Account{UserID: u.ID, MSISDN: u.Msisdn, PhoneVerified: u.PhoneVerified, KYCStatus: u.KycStatus, Status: u.Status,
		CreatedAt: u.CreatedAt, ClosureRequestedAt: u.ClosureRequestedAt}, nil
}

// RequestClosure records that the player wants their account closed and returns when they first asked. Asking again
// changes nothing. Staff close the account (after paying out any balance); records the law requires are kept.
func (s *Service) RequestClosure(ctx context.Context, userID string) (time.Time, error) {
	var at time.Time
	err := s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		r, err := q.RequestClosure(ctx, store.RequestClosureParams{UserID: userID, At: now})
		if err != nil {
			return err
		}
		at = r.RequestedAt
		if !r.Created {
			return nil
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "identity.closure_requested",
			EntityType: "user", EntityID: userID})
	})
	return at, err
}

// Closure errors.
var (
	ErrNoClosureRequest = errors.New("identity: no open closure request")
	// ErrClosureBlocked wraps the reason an account cannot be closed yet (money still in it or still moving).
	ErrClosureBlocked = errors.New("identity: account cannot be closed yet")
)

// ClosureRequest is an open request to close an account.
type ClosureRequest struct {
	UserID      string
	MSISDN      string
	Status      string
	RequestedAt time.Time
}

// OpenClosures lists open closure requests, oldest first.
func (s *Service) OpenClosures(ctx context.Context, limit int) ([]ClosureRequest, error) {
	rows, err := store.New(s.pool).OpenClosureRequests(ctx, int32(limit)) //nolint:gosec // callers pass small limits
	if err != nil {
		return nil, err
	}
	out := make([]ClosureRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, ClosureRequest{UserID: r.UserID, MSISDN: r.Msisdn, Status: r.Status, RequestedAt: r.RequestedAt})
	}
	return out, nil
}

// ClosureGuard refuses to close an account (returning an error wrapping ErrClosureBlocked) while money is in it or
// still moving. It runs in the closing transaction, after the request row is locked.
type ClosureGuard func(ctx context.Context, tx pgx.Tx, userID string) error

// CloseAccount closes the account of an open closure request: the account becomes `closed` (no sign-in, play, top-up
// or withdrawal), every session ends, and the request records who closed it. The player's records stay. staffID is
// the staff member's id (a uuid); staff is how they appear in the audit log.
func (s *Service) CloseAccount(ctx context.Context, userID, staffID, staff string, guard ClosureGuard) (time.Time, error) {
	var at time.Time
	err := s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if _, err := q.LockOpenClosure(ctx, userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoClosureRequest
			}
			return err
		}
		if err := guard(ctx, tx, userID); err != nil {
			return err
		}
		if err := q.SetUserClosed(ctx, store.SetUserClosedParams{ID: userID, At: now}); err != nil {
			return err
		}
		if err := q.RevokeAllSessions(ctx, store.RevokeAllSessionsParams{UserID: userID, At: &now}); err != nil {
			return err
		}
		if err := q.CloseClosureRequest(ctx, store.CloseClosureRequestParams{UserID: userID, At: &now, Staff: staffID}); err != nil {
			return err
		}
		at = now
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: staff, Action: "identity.account_closed",
			EntityType: "user", EntityID: userID})
	})
	return at, err
}
