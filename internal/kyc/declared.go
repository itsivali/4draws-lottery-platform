package kyc

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/kyc/store"
)

// Errors of a declaration made after sign-up.
var (
	ErrAlreadyDeclared = errors.New("kyc: the names and ID number were already given")
	ErrSubmitted       = errors.New("kyc: an identity submission exists; the name comes from it")
)

// Declaration is what a new account gives at sign-up (owner, 2026-10-30): first name, second name and national ID
// number. It is stored encrypted and verified when the player first cashes out (ADR 0022): the identity submission
// starts from it and adds the date of birth. A declaration proves nothing by itself.
type Declaration struct {
	FirstName  string
	SecondName string
	IDNumber   string
}

// Full is the two names as one, as an identity submission takes them.
func (d Declaration) Full() string { return d.FirstName + " " + d.SecondName }

func oneName(s string) string { return strings.Join(strings.Fields(s), " ") }

// Normalized trims the names and the number, or answers ErrInvalidInput: each name a letter then letters, spaces,
// dots, apostrophes or hyphens (as on an ID card), the number 7 to 9 digits.
func (d Declaration) Normalized() (Declaration, error) {
	n := Declaration{FirstName: oneName(d.FirstName), SecondName: oneName(d.SecondName), IDNumber: strings.TrimSpace(d.IDNumber)}
	for _, name := range []string{n.FirstName, n.SecondName} {
		if utf8.RuneCountInString(name) < 2 || !namePattern.MatchString(name) {
			return Declaration{}, ErrInvalidInput
		}
	}
	if !idPattern.MatchString(n.IDNumber) {
		return Declaration{}, ErrInvalidInput
	}
	return n, nil
}

// Declare stores an account's declaration in the caller's transaction (the one creating the account, so neither
// exists without the other).
func (s *Service) Declare(ctx context.Context, tx pgx.Tx, userID string, d Declaration) error {
	n, err := d.Normalized()
	if err != nil {
		return err
	}
	now, err := s.clk.Now(ctx, tx)
	if err != nil {
		return err
	}
	p := store.InsertDeclaredIdentityParams{UserID: userID, IDNumberHash: s.idHash(n.IDNumber), CreatedAt: now}
	if p.FirstNameEnc, err = s.kr.Encrypt([]byte(n.FirstName), []byte("kyc.declared.first_name")); err != nil {
		return err
	}
	if p.SecondNameEnc, err = s.kr.Encrypt([]byte(n.SecondName), []byte("kyc.declared.second_name")); err != nil {
		return err
	}
	if p.IDNumberEnc, err = s.kr.Encrypt([]byte(n.IDNumber), []byte("kyc.declared.id_number")); err != nil {
		return err
	}
	return store.New(tx).InsertDeclaredIdentity(ctx, p)
}

// Declared returns the declarations of the given accounts (none for accounts from before 2026-10-30).
func (s *Service) Declared(ctx context.Context, userIDs []string) (map[string]Declaration, error) {
	out := map[string]Declaration{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := store.New(s.pool).DeclaredIdentitiesFor(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		var d Declaration
		for _, f := range []struct {
			enc []byte
			ad  string
			to  *string
		}{
			{r.FirstNameEnc, "kyc.declared.first_name", &d.FirstName},
			{r.SecondNameEnc, "kyc.declared.second_name", &d.SecondName},
			{r.IDNumberEnc, "kyc.declared.id_number", &d.IDNumber},
		} {
			b, err := s.kr.Decrypt(f.enc, []byte(f.ad))
			if err != nil {
				return nil, err
			}
			*f.to = string(b)
		}
		out[r.UserID] = d
	}
	return out, nil
}

// DeclareLater stores the declaration of an account from before sign-up asked for one (owner, 2026-10-31): once
// (ErrAlreadyDeclared), and only until an identity submission exists (ErrSubmitted: the name on the submission is the
// one shown and reviewed). Its own transaction; audited as the player's act, without the details.
func (s *Service) DeclareLater(ctx context.Context, userID string, d Declaration) error {
	n, err := d.Normalized()
	if err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if _, err := q.LockUserKYC(ctx, userID); err != nil {
			return err
		}
		if _, err := q.LatestKYCCheck(ctx, userID); err == nil {
			return ErrSubmitted
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		rows, err := q.DeclaredIdentitiesFor(ctx, []string{userID})
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			return ErrAlreadyDeclared
		}
		if err := s.Declare(ctx, tx, userID, n); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "identity.declared",
			EntityType: "user", EntityID: userID, Details: map[string]any{"source": "account"}})
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" { // two requests at once: the first one's declaration stands
		return ErrAlreadyDeclared
	}
	return err
}

// Own is what a player sees of their own identity: the name and ID number they gave, and where they stand. Status is
// none (nothing given), declared (given at sign-up or from the account, not checked yet), submitted (an identity
// submission exists, pending, in review or refused) or verified.
type Own struct {
	FullName string
	IDNumber string
	Status   string
}

// OwnIdentity returns the player's own name, ID number and status: from the latest identity submission, else the
// declaration (their own data, so no unmasking is audited).
func (s *Service) OwnIdentity(ctx context.Context, userID string) (Own, error) {
	rows, err := store.New(s.pool).LatestChecksFor(ctx, []string{userID})
	if err != nil {
		return Own{}, err
	}
	if len(rows) == 0 {
		d, err := s.Declared(ctx, []string{userID})
		if err != nil {
			return Own{}, err
		}
		if v, ok := d[userID]; ok {
			return Own{FullName: v.Full(), IDNumber: v.IDNumber, Status: "declared"}, nil
		}
		return Own{Status: "none"}, nil
	}
	nm, err := s.kr.Decrypt(rows[0].FullNameEnc, []byte("kyc.full_name"))
	if err != nil {
		return Own{}, err
	}
	id, err := s.kr.Decrypt(rows[0].IDNumberEnc, []byte("kyc.id_number"))
	if err != nil {
		return Own{}, err
	}
	st := "submitted"
	if Status(rows[0].Status) == Verified {
		st = "verified"
	}
	return Own{FullName: string(nm), IDNumber: string(id), Status: st}, nil
}
