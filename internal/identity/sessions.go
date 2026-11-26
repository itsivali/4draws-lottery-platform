package identity

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/itsivali/4draws-lottery-platform/internal/audit"
	"github.com/itsivali/4draws-lottery-platform/internal/identity/store"
)

// Session is one of a player's open sessions, as they see it on their account (owner, 2026-10-31).
type Session struct {
	ID string
	// Platform is android, ios or web.
	Platform string
	// DeviceVerdict is the device's attestation verdict: none, passed or failed.
	DeviceVerdict string
	CreatedAt     time.Time
	// LastUsedAt is when a token was last issued to the session (its start, if never refreshed).
	LastUsedAt time.Time
}

// Sessions lists the player's open sessions, newest first.
func (s *Service) Sessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := store.New(s.pool).OpenSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, Session{ID: r.ID, Platform: r.Platform, DeviceVerdict: r.DeviceVerdict, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt})
	}
	return out, nil
}

// EndOtherSessions signs the player out everywhere but the session they are on, and says how many sessions it ended.
// Audited as the player's own act.
func (s *Service) EndOtherSessions(ctx context.Context, userID, keepSessionID string) (int64, error) {
	var n int64
	err := s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		var err error
		n, err = q.RevokeOtherSessionsByPlayer(ctx, store.RevokeOtherSessionsByPlayerParams{At: &now, UserID: userID, KeepSessionID: keepSessionID})
		if err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Player, ActorID: userID, Action: "auth.sessions_revoked",
			EntityType: "user", EntityID: userID, Details: map[string]any{"count": n, "kept_session_id": keepSessionID}})
	})
	return n, err
}

// LinkedSignIn is a Google or Apple identity that signs in to the account (ADR 0020).
type LinkedSignIn struct {
	Provider string
	LinkedAt time.Time
}

// SignInMethods says whether the account has a password and which Google or Apple identities sign in to it.
func (s *Service) SignInMethods(ctx context.Context, userID string) (hasPassword bool, linked []LinkedSignIn, err error) {
	q := store.New(s.pool)
	a, err := q.GetAccount(ctx, userID)
	if err != nil {
		return false, nil, err
	}
	rows, err := q.AuthIdentitiesFor(ctx, userID)
	if err != nil {
		return false, nil, err
	}
	linked = make([]LinkedSignIn, 0, len(rows))
	for _, r := range rows {
		linked = append(linked, LinkedSignIn{Provider: r.Provider, LinkedAt: r.CreatedAt})
	}
	return a.HasPassword, linked, nil
}
