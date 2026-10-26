package identity

import (
	"context"
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
