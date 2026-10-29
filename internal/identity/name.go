package identity

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity/store"
)

// Names (owner, 2026-10-29): the player's first and second names, given at sign-up and shown on the account screen.
// They are how the app addresses the player, not the legal name (that is the identity submission's, checked at
// withdrawal). Accounts from before have none until the player adds them.
var ErrInvalidName = errors.New("identity: a name is missing or not a name")

var nameRE = regexp.MustCompile(`^[\p{L}][\p{L}\p{M}' .-]*$`)

// NormalizeName trims and collapses spaces and refuses what is not a name (empty, over 40 characters, digits).
func NormalizeName(raw string) (string, error) {
	n := strings.Join(strings.Fields(raw), " ")
	if n == "" || utf8.RuneCountInString(n) > 40 || !nameRE.MatchString(n) {
		return "", ErrInvalidName
	}
	return n, nil
}

func normalizeNames(first, second string) (string, string, error) {
	f, err := NormalizeName(first)
	if err != nil {
		return "", "", err
	}
	s, err := NormalizeName(second)
	if err != nil {
		return "", "", err
	}
	return f, s, nil
}

// SetName changes, or adds, the player's first and second names. Audited; the names themselves are not logged.
func (s *Service) SetName(ctx context.Context, userID, first, second string) error {
	f, sec, err := normalizeNames(first, second)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if err := q.SetUserName(ctx, store.SetUserNameParams{FirstName: f, SecondName: sec, At: now, ID: userID}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "identity.name_changed", EntityType: "user", EntityID: userID})
	})
}
