package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/identity/store"
)

// loadActive reads the account and holds its row (FOR SHARE) for the caller's transaction, so a top-up, play or
// withdrawal and the closing of the account cannot interleave.
func (s *Service) loadActive(ctx context.Context, tx pgx.Tx, userID string) (store.GetUserRow, error) {
	r, err := store.New(tx).GetUserForShare(ctx, userID)
	return activeUser(store.GetUserRow(r), err)
}

// peekActive is loadActive without the lock, for decisions taken in read-only transactions (it moves nothing).
func (s *Service) peekActive(ctx context.Context, tx pgx.Tx, userID string) (store.GetUserRow, error) {
	return activeUser(store.New(tx).GetUser(ctx, userID))
}

func activeUser(u store.GetUserRow, err error) (store.GetUserRow, error) {
	if err != nil {
		return u, err
	}
	if u.Status != "active" {
		return u, ErrAccountSuspended
	}
	return u, nil
}

// AssertCanTopUp refuses accounts that may not start a top-up: suspended or closed ones, and, when real money is in
// play (RequireVerifiedIDFirst, ADR 0029), an unverified national ID. The phone need not be proved yet (the first
// top-up is what proves it). In the sandbox the national ID need not be verified: ADR 0022 checks it before the first
// withdrawal. It returns the number the account claims, which is
// where the payment prompt must go (never a number the client supplies). Called inside the same transaction as the
// top-up (invariant 12).
func (s *Service) AssertCanTopUp(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	return s.canTopUp(s.loadActive(ctx, tx, userID))
}

// CanTopUp answers what AssertCanTopUp would without holding the account row (for read-only transactions, such as
// deciding whether to accept a paybill payment before it happens).
func (s *Service) CanTopUp(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	return s.canTopUp(s.peekActive(ctx, tx, userID))
}

func (s *Service) canTopUp(u store.GetUserRow, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if s.idFirst && u.KycStatus != "verified" {
		return "", ErrKYCRequired
	}
	return u.Msisdn, nil
}

// ProvedPhoneOwner returns the account that has proved msisdn, or ErrNoProvedOwner. Only a proved number identifies an
// account: an unproved one may be claimed by several accounts. Paybill payments use it to find whose account number
// (the registered phone) a customer typed; eligibility is checked separately (AssertCanTopUp).
func (s *Service) ProvedPhoneOwner(ctx context.Context, tx pgx.Tx, msisdn string) (string, error) {
	u, err := store.New(tx).ProvedPhoneOwner(ctx, msisdn)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoProvedOwner
	}
	return u.ID, err
}

// AssertCanPlay is AssertCanTopUp plus a proved phone number, which is what pays out and receives alerts. In the
// sandbox the national ID is checked at the first withdrawal (ADR 0022), not here; with real money, here too (ADR 0029). Called inside the same transaction as the debit.
func (s *Service) AssertCanPlay(ctx context.Context, tx pgx.Tx, userID string) error {
	u, err := s.loadActive(ctx, tx, userID)
	if err != nil {
		return err
	}
	if s.idFirst && u.KycStatus != "verified" {
		return ErrKYCRequired
	}
	if !u.PhoneVerified {
		return ErrPhoneNotVerified
	}
	return nil
}

// AssertDeviceTrusted refuses transactions from a session whose device failed attestation. With strict set (the
// production policy once the verifier is live) a mobile device must also have passed. Web sessions cannot be attested
// and the platform is the client's own claim, so a script can call itself "web": they pass only when webAllowed
// (ATTESTATION_WEB=allow), and are otherwise refused like an unattested phone.
func (s *Service) AssertDeviceTrusted(ctx context.Context, tx pgx.Tx, sessionID string, strict, webAllowed bool) error {
	d, err := store.New(tx).GetSessionDevice(ctx, sessionID)
	web := errors.Is(err, pgx.ErrNoRows) // a session without a device record is a web session
	if err != nil && !web {
		return err
	}
	if web || d.Platform == "web" {
		if webAllowed {
			return nil
		}
		return ErrAttestationFailed
	}
	if d.AttestationVerdict == "failed" || (strict && d.AttestationVerdict != "passed") {
		return ErrAttestationFailed
	}
	return nil
}

// AssertCanBePaid refuses accounts that may not receive a payout: suspended or closed accounts, an unverified national
// ID (the one place identity is checked, ADR 0022; it also keeps under-18s from ever being paid), or a phone not yet
// proved. It returns the proved number, the only one money is sent to (ADR 0005). Called inside the same transaction
// that records the payout.
func (s *Service) AssertCanBePaid(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	u, err := s.loadActive(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	if u.KycStatus != "verified" {
		return "", ErrKYCRequired
	}
	if !u.PhoneVerified {
		return "", ErrPhoneNotVerified
	}
	return u.Msisdn, nil
}

// DeviceTrusted is AssertDeviceTrusted in its own read-only transaction, for checks made before a service call.
func (s *Service) DeviceTrusted(ctx context.Context, sessionID string, strict, webAllowed bool) error {
	return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return s.AssertDeviceTrusted(ctx, tx, sessionID, strict, webAllowed)
	})
}
