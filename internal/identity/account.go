package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/itsivali/lottery-platform/internal/audit"
	"github.com/itsivali/lottery-platform/internal/identity/store"
	"github.com/itsivali/lottery-platform/internal/platform/password"
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
	// MustChangePassword: staff gave the player a temporary password they have not replaced yet.
	MustChangePassword bool
	// Email is nil for an account from before addresses were asked for (ADR 0021).
	Email *string
	// HasPassword is false for an account that signs in with Google or Apple only (ADR 0020).
	HasPassword bool
}

// Account returns the player's account.
func (s *Service) Account(ctx context.Context, userID string) (Account, error) {
	u, err := store.New(s.pool).GetAccount(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	return Account{UserID: u.ID, MSISDN: u.Msisdn, PhoneVerified: u.PhoneVerified, KYCStatus: u.KycStatus, Status: u.Status, MustChangePassword: u.MustChangePassword,
		CreatedAt: u.CreatedAt, ClosureRequestedAt: u.ClosureRequestedAt, Email: u.Email, HasPassword: u.HasPassword}, nil
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

// OpenClosures lists open closure requests, oldest first, with the players' numbers; the viewing is audited as
// viewer (like a KYC unmask), since it shows personal data.
func (s *Service) OpenClosures(ctx context.Context, limit int, viewer string) ([]ClosureRequest, error) {
	var out []ClosureRequest
	err := s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, _ time.Time) error {
		rows, err := q.OpenClosureRequests(ctx, int32(limit)) //nolint:gosec // callers pass small limits
		if err != nil {
			return err
		}
		out = make([]ClosureRequest, 0, len(rows))
		for _, r := range rows {
			out = append(out, ClosureRequest{UserID: r.UserID, MSISDN: r.Msisdn, Status: r.Status, RequestedAt: r.RequestedAt})
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "identity.closures_viewed",
			EntityType: "closure_requests", EntityID: "open", Details: map[string]any{"shown": len(out)}})
	})
	return out, err
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
		// the account row first: a top-up, play or withdrawal in flight holds it (FOR SHARE) until it commits
		if err := q.LockUser(ctx, userID); err != nil {
			return err
		}
		if _, err := q.LockOpenClosure(ctx, userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNoClosureRequest
			}
			return err
		}
		if err := guard(ctx, tx, userID); err != nil {
			return err
		}
		if err := q.NameStatusChange(ctx, store.NameStatusChangeParams{ActorType: string(audit.Staff), ActorID: staff, Reason: "closure requested by the player"}); err != nil {
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

// Player is a registered account as the back office lists it.
type Player struct {
	UserID             string     `json:"user_id"`
	MSISDN             string     `json:"msisdn"`
	Email              *string    `json:"email"`
	KYCStatus          string     `json:"kyc_status"`
	Status             string     `json:"status"`
	PhoneVerified      bool       `json:"phone_verified"`
	BalanceKES         int64      `json:"balance_kes"`
	Plays              int64      `json:"plays"`
	CreatedAt          time.Time  `json:"created_at"`
	LastSignInAt       *time.Time `json:"last_sign_in_at"`
	ClosureRequestedAt *time.Time `json:"closure_requested_at"`
	// FullName and NationalID come from the latest identity submission (the KYC service fills them in); empty when
	// the player never submitted one.
	FullName   string `json:"full_name"`
	NationalID string `json:"national_id"`
}

// PlayerFilter narrows the list: a fragment of the phone number or email address (or a user id), and a status.
type PlayerFilter struct {
	Query  string
	Status string
}

// PlayerPage is one page of players, newest first; NextBefore continues it (pass it as before), empty on the last.
type PlayerPage struct {
	Players    []Player
	NextBefore string
}

// Players lists registered accounts, newest first, with their balance, plays and last sign-in (ADR 0019). The viewing
// is audited as viewer, since it shows phone numbers.
func (s *Service) Players(ctx context.Context, f PlayerFilter, limit int, before, viewer string) (PlayerPage, error) {
	var q, status, cursor *string
	if v := strings.TrimSpace(f.Query); v != "" {
		if n, err := NormalizeMSISDN(v); err == nil {
			v = n
		}
		q = &v
	}
	if f.Status != "" {
		status = &f.Status
	}
	if before != "" {
		cursor = &before
	}
	var page PlayerPage
	err := s.withTx(ctx, func(tx pgx.Tx, st *store.Queries, _ time.Time) error {
		rows, err := st.ListPlayers(ctx, store.ListPlayersParams{Q: q, Status: status, Before: cursor, Lim: int32(limit) + 1}) //nolint:gosec // small limits
		if err != nil {
			return err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		page.Players = make([]Player, 0, len(rows))
		for _, r := range rows {
			page.Players = append(page.Players, Player{UserID: r.ID, MSISDN: r.Msisdn, Email: r.Email, KYCStatus: r.KycStatus, Status: r.Status,
				PhoneVerified: r.PhoneVerified, BalanceKES: r.BalanceKes, Plays: r.Plays, CreatedAt: r.CreatedAt,
				LastSignInAt: timePtr(r.LastSignInAt), ClosureRequestedAt: r.ClosureRequestedAt})
		}
		if more {
			page.NextBefore = rows[len(rows)-1].ID
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: viewer, Action: "identity.players_viewed",
			EntityType: "users", EntityID: "list", Details: map[string]any{"shown": len(page.Players), "query": f.Query != "", "status": f.Status}})
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "22P02" { // not a uuid
		return PlayerPage{}, ErrBadCursor
	}
	return page, err
}

// ErrBadCursor is a page cursor that is not a user id.
var ErrBadCursor = errors.New("identity: bad cursor")

// ErrNotSuspendable is a status change that does not apply: suspending a closed or already suspended account, or
// reinstating one that is not suspended.
var ErrNotSuspendable = errors.New("identity: the account is not in a state that allows this")

// Suspend stops an account (no sign-in, play, top-up or withdrawal) and ends its sessions; Reinstate lifts it. Both
// are audited with the staff member's reason. Closed accounts are left alone.
func (s *Service) Suspend(ctx context.Context, userID, staff, reason string, suspend bool) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}
	from, to, action := "active", "suspended", "identity.account_suspended"
	if !suspend {
		from, to, action = "suspended", "active", "identity.account_reinstated"
	}
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		if err := q.NameStatusChange(ctx, store.NameStatusChangeParams{ActorType: string(audit.Staff), ActorID: staff, Reason: reason}); err != nil {
			return err
		}
		n, err := q.SetUserSuspended(ctx, store.SetUserSuspendedParams{Status: to, At: now, ID: userID, FromStatus: from})
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "22P02" {
			return ErrNotSuspendable
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotSuspendable
		}
		if suspend {
			if err := q.RevokeAllSessions(ctx, store.RevokeAllSessionsParams{UserID: userID, At: &now}); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: staff, Action: action,
			EntityType: "user", EntityID: userID, Details: map[string]any{"reason": reason}})
	})
}

// ErrReasonRequired: a suspension or reinstatement needs a reason for the audit log.
var ErrReasonRequired = errors.New("identity: a reason is required")

// ErrNoSuchPlayer: no open account with that id.
var ErrNoSuchPlayer = errors.New("identity: no such player")

// SetTemporaryPassword is staff giving a player a password to sign in with once, after checking who they are (players
// have no email, so there is no link to send). Every session the player has ends, the password must pass the player
// rules, and the app makes them choose their own at the next sign-in (Tokens.MustChangePassword). Audited with the
// reason; the password itself is never recorded.
func (s *Service) SetTemporaryPassword(ctx context.Context, userID, staff, reason, temporary string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}
	u, err := store.New(s.pool).GetUser(ctx, userID)
	var pe *pgconn.PgError
	if errors.Is(err, pgx.ErrNoRows) || (errors.As(err, &pe) && pe.Code == "22P02") {
		return ErrNoSuchPlayer
	} else if err != nil {
		return err
	}
	if err := password.CheckPolicy(temporary, u.Msisdn); err != nil {
		return err
	}
	hash, err := password.Hash(temporary, s.pepper)
	if err != nil {
		return err
	}
	return s.withTx(ctx, func(tx pgx.Tx, q *store.Queries, now time.Time) error {
		n, err := q.SetTemporaryPassword(ctx, store.SetTemporaryPasswordParams{PasswordHash: &hash, At: &now, ID: userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNoSuchPlayer
		}
		if err := q.RevokeAllSessions(ctx, store.RevokeAllSessionsParams{UserID: userID, At: &now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Event{ActorType: audit.Staff, ActorID: staff, Action: "identity.temporary_password",
			EntityType: "user", EntityID: userID, Details: map[string]any{"reason": reason}})
	})
}

// timePtr reads an optional timestamp the store could not type (an aggregate in a sub-select): nil when NULL.
func timePtr(v any) *time.Time {
	if t, ok := v.(time.Time); ok {
		return &t
	}
	return nil
}

// StatusChange is one change of an account's status, as identity.user_status_history keeps it (ADR 0030). From is
// empty for the account's first status (its sign-up, or when the history began).
type StatusChange struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	ActorType string    `json:"actor_type"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	At        time.Time `json:"at"`
}

// StatusHistory returns every status the account has had, oldest first.
func (s *Service) StatusHistory(ctx context.Context, userID string) ([]StatusChange, error) {
	rows, err := store.New(s.pool).StatusHistory(ctx, userID)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "22P02" {
		return nil, ErrNoSuchPlayer
	}
	if err != nil {
		return nil, err
	}
	out := make([]StatusChange, 0, len(rows))
	for _, r := range rows {
		out = append(out, StatusChange{From: deref(r.FromStatus), To: r.ToStatus, ActorType: deref(r.ActorType), Actor: deref(r.ActorID),
			Reason: deref(r.Reason), At: r.ChangedAt})
	}
	return out, nil
}

// EndSessionsForSelfExclusion ends every session of the account, in the caller's transaction: a player who excludes
// themself is signed out everywhere, at once (ADR 0030). They may sign in again, to withdraw what is theirs; play and
// top-ups stay refused.
func (s *Service) EndSessionsForSelfExclusion(ctx context.Context, tx pgx.Tx, userID string) error {
	now, err := s.clk.Now(ctx, tx)
	if err != nil {
		return err
	}
	return store.New(tx).EndSessionsFor(ctx, store.EndSessionsForParams{At: &now, Reason: ptr("self_exclusion"), UserID: userID})
}
