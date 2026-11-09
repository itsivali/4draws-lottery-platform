package kyc

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/kyc/store"
	"github.com/itsivali/lottery-platform/internal/platform/clock"
)

// ErrAgeRequired: sign-up needs a date of birth and the player's declaration that they are 18 or older (ADR 0029).
var ErrAgeRequired = errors.New("kyc: a date of birth and an 18+ declaration are required")

// AgeDeclaration is what every new account gives at sign-up (owner, 2026-11-07; ADR 0029): a date of birth
// (YYYY-MM-DD) and a declaration that the player is 18 or older. It proves nothing by itself; the date is checked
// against the ID when identity is verified.
type AgeDeclaration struct {
	DateOfBirth string
	Adult       bool
}

// adultDOB parses a date of birth and refuses a malformed or future date (ErrInvalidInput) and anyone under 18 on
// today's date in Nairobi (ErrUnderage).
func adultDOB(raw string, now time.Time) (time.Time, error) {
	dob, err := time.Parse(time.DateOnly, raw)
	if err != nil || dob.Year() < 1900 {
		return time.Time{}, ErrInvalidInput
	}
	n := clock.InNairobi(now)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
	if dob.After(today) {
		return time.Time{}, ErrInvalidInput
	}
	if age(dob, today) < 18 {
		return time.Time{}, ErrUnderage
	}
	return dob, nil
}

// CheckAge validates a sign-up's age declaration before the account is created: ErrAgeRequired without a date or the
// declaration, ErrInvalidInput for a date that is not one, ErrUnderage under 18. It returns the normalised date.
func (s *Service) CheckAge(ctx context.Context, a AgeDeclaration) (string, error) {
	if a.DateOfBirth == "" || !a.Adult {
		return "", ErrAgeRequired
	}
	now, err := s.clk.Now(ctx, s.pool)
	if err != nil {
		return "", err
	}
	dob, err := adultDOB(a.DateOfBirth, now)
	if err != nil {
		return "", err
	}
	return dob.Format(time.DateOnly), nil
}

// DeclareAge stores the checked date of birth and the declaration in the caller's transaction (the one creating the
// account, so neither exists without the other).
func (s *Service) DeclareAge(ctx context.Context, tx pgx.Tx, userID, dob string) error {
	now, err := s.clk.Now(ctx, tx)
	if err != nil {
		return err
	}
	enc, err := s.kr.Encrypt([]byte(dob), []byte("kyc.age.dob"))
	if err != nil {
		return err
	}
	return store.New(tx).InsertAgeDeclaration(ctx, store.InsertAgeDeclarationParams{UserID: userID, DateOfBirthEnc: enc, DeclaredAt: now})
}

// declaredDOB returns the date of birth the account declared at sign-up, or "" for accounts from before ADR 0029.
func (s *Service) declaredDOB(ctx context.Context, q *store.Queries, userID string) (string, error) {
	enc, err := q.AgeDeclaration(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	b, err := s.kr.Decrypt(enc, []byte("kyc.age.dob"))
	return string(b), err
}
