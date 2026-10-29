-- name: UpsertDevice :one
INSERT INTO identity.devices (user_id, fingerprint_hash, platform, attestation_verdict, first_seen_at, last_seen_at)
VALUES (sqlc.arg(user_id), sqlc.arg(fingerprint_hash), sqlc.arg(platform), sqlc.arg(attestation_verdict), sqlc.arg(at), sqlc.arg(at))
ON CONFLICT (user_id, fingerprint_hash) DO UPDATE
   SET last_seen_at = EXCLUDED.last_seen_at, attestation_verdict = EXCLUDED.attestation_verdict
RETURNING id;

-- name: CountOtherAccountsOnDevice :one
SELECT count(DISTINCT user_id) FROM identity.devices WHERE fingerprint_hash = $1 AND user_id <> $2;

-- name: InsertSession :one
INSERT INTO identity.sessions (user_id, device_id, created_at, platform) VALUES ($1, $2, $3, sqlc.narg(platform)) RETURNING id;

-- name: GetSession :one
SELECT s.id, s.user_id, s.device_id, s.revoked_at, d.fingerprint_hash
FROM identity.sessions s LEFT JOIN identity.devices d ON d.id = s.device_id
WHERE s.id = $1;

-- name: RevokeSession :exec
UPDATE identity.sessions SET revoked_at = sqlc.arg(at), revoked_reason = sqlc.arg(reason)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: InsertRefreshToken :exec
INSERT INTO identity.refresh_tokens (session_id, parent_id, token_hash, issued_at, expires_at)
VALUES (sqlc.arg(session_id), sqlc.narg(parent_id), sqlc.arg(token_hash), sqlc.arg(issued_at), sqlc.arg(expires_at));

-- name: LockRefreshToken :one
SELECT id, session_id, expires_at, used_at FROM identity.refresh_tokens WHERE token_hash = $1 FOR UPDATE;

-- name: MarkRefreshUsed :exec
UPDATE identity.refresh_tokens SET used_at = $2 WHERE id = $1;

-- name: InsertAuthIdentity :exec
INSERT INTO identity.auth_identities (user_id, provider, subject) VALUES ($1, $2, $3);

-- name: FindAuthIdentity :one
SELECT user_id FROM identity.auth_identities WHERE provider = $1 AND subject = $2;

-- name: GetSessionDevice :one
SELECT d.platform, d.attestation_verdict
FROM identity.sessions s JOIN identity.devices d ON d.id = s.device_id
WHERE s.id = $1;

-- name: InsertAccount :one
INSERT INTO identity.users (msisdn, email, password_hash, password_changed_at, created_at, updated_at)
VALUES (sqlc.arg(msisdn), sqlc.narg(email), sqlc.arg(password_hash), sqlc.arg(at), sqlc.arg(at), sqlc.arg(at))
RETURNING id;

-- name: UserByEmail :one
SELECT id, msisdn, status FROM identity.users WHERE email = sqlc.arg(email)::text;

-- name: SetUserEmail :exec
UPDATE identity.users SET email = sqlc.arg(email)::text, updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- Emailed password-reset links (ADR 0021): the token's SHA-256 only, 30 minutes, used at most once.
-- name: InsertPlayerPasswordReset :exec
INSERT INTO identity.password_resets (user_id, token_hash, expires_at, created_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token_hash), sqlc.arg(expires_at), sqlc.arg(at));

-- name: PlayerPasswordResetByHash :one
SELECT r.id, r.user_id, r.expires_at, r.used_at, u.msisdn, u.status
FROM identity.password_resets r JOIN identity.users u ON u.id = r.user_id
WHERE r.token_hash = $1
FOR UPDATE OF r;

-- name: UsePlayerPasswordResets :exec
-- a reset consumes every open link of the player, not only the one used
UPDATE identity.password_resets SET used_at = sqlc.arg(at) WHERE user_id = sqlc.arg(user_id) AND used_at IS NULL;

-- name: RevokeSessionsForPasswordChange :exec
UPDATE identity.sessions SET revoked_at = sqlc.arg(at), revoked_reason = 'password_change'
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: VerifiedAccountExists :one
SELECT EXISTS (SELECT 1 FROM identity.users WHERE msisdn = $1 AND msisdn_verified_at IS NOT NULL) ::boolean AS exists;

-- A number with a proved owner admits only that owner; otherwise the newest few unproved claims are tried. An account
-- without a password (Google or Apple only) is still the owner that excludes the others; the caller skips its hash.
-- name: LoginCandidates :many
SELECT id, password_hash, status, (msisdn_verified_at IS NOT NULL)::boolean AS phone_verified, must_change_password
FROM identity.users
WHERE msisdn = $1
ORDER BY (msisdn_verified_at IS NULL), created_at DESC
LIMIT 5;

-- name: GetUser :one
SELECT id, msisdn, kyc_status, status, language, password_hash, (msisdn_verified_at IS NOT NULL)::boolean AS phone_verified
FROM identity.users WHERE id = $1;

-- name: SetPassword :exec
UPDATE identity.users SET password_hash = $2, password_changed_at = $3, must_change_password = false, updated_at = $3 WHERE id = $1;

-- name: SetTemporaryPassword :execrows
-- staff gave the player a password to sign in with once; the app makes them choose their own next
UPDATE identity.users SET password_hash = sqlc.arg(password_hash), password_changed_at = sqlc.arg(at), must_change_password = true, updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND status <> 'closed';

-- name: RevokeOtherSessions :exec
UPDATE identity.sessions SET revoked_at = sqlc.arg(at), revoked_reason = 'password_change'
WHERE user_id = sqlc.arg(user_id) AND id <> sqlc.arg(keep_session_id) AND revoked_at IS NULL;

-- name: LockUserPhone :one
SELECT msisdn, (msisdn_verified_at IS NOT NULL)::boolean AS verified FROM identity.users WHERE id = $1 FOR UPDATE;

-- name: MarkPhoneVerified :exec
UPDATE identity.users SET msisdn_verified_at = $2, updated_at = $2 WHERE id = $1 AND msisdn_verified_at IS NULL;


-- name: ProvedPhoneOwner :one
SELECT id, status, kyc_status FROM identity.users WHERE msisdn = $1 AND msisdn_verified_at IS NOT NULL;

-- name: GetAccount :one
SELECT u.id, u.msisdn, u.email, u.kyc_status, u.status, (u.msisdn_verified_at IS NOT NULL)::boolean AS phone_verified, u.created_at,
  u.must_change_password, (u.password_hash IS NOT NULL)::boolean AS has_password, c.requested_at AS closure_requested_at
FROM identity.users u LEFT JOIN identity.closure_requests c ON c.user_id = u.id
WHERE u.id = $1;

-- name: RequestClosure :one
-- Keeps the first request: asking again returns its time and changes nothing.
WITH ins AS (
  INSERT INTO identity.closure_requests (user_id, requested_at) VALUES (sqlc.arg(user_id), sqlc.arg(at))
  ON CONFLICT (user_id) DO NOTHING
  RETURNING requested_at, true AS created
)
SELECT requested_at, created FROM ins
UNION ALL
SELECT requested_at, false FROM identity.closure_requests WHERE user_id = sqlc.arg(user_id) AND NOT EXISTS (SELECT 1 FROM ins);

-- name: OpenClosureRequests :many
SELECT c.user_id, c.requested_at, u.msisdn, u.status FROM identity.closure_requests c
JOIN identity.users u ON u.id = c.user_id
WHERE c.closed_at IS NULL ORDER BY c.requested_at LIMIT sqlc.arg(lim);

-- name: LockOpenClosure :one
SELECT requested_at FROM identity.closure_requests WHERE user_id = $1 AND closed_at IS NULL FOR UPDATE;

-- name: CloseClosureRequest :exec
UPDATE identity.closure_requests SET closed_at = sqlc.arg(at), closed_by = sqlc.arg(staff)::uuid WHERE user_id = sqlc.arg(user_id);

-- name: SetUserClosed :exec
UPDATE identity.users SET status = 'closed', updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: RevokeAllSessions :exec
UPDATE identity.sessions SET revoked_at = sqlc.arg(at), revoked_reason = 'admin'
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL;

-- name: GetUserForShare :one
-- As GetUser, holding the row so the account cannot be closed (or suspended) until the caller's transaction ends.
SELECT id, msisdn, kyc_status, status, language, password_hash, (msisdn_verified_at IS NOT NULL)::boolean AS phone_verified
FROM identity.users WHERE id = $1 FOR SHARE;

-- name: LockUser :exec
SELECT 1 FROM identity.users WHERE id = $1 FOR UPDATE;

-- name: ListPlayers :many
-- the back office's player list (ADR 0019): newest first (ids are uuid v7), narrowed by a phone-number or email
-- fragment and a status; before (a user id) continues a page. Shows each player's balance, plays and last sign-in.
SELECT u.id, u.msisdn, u.email, u.kyc_status, u.status, (u.msisdn_verified_at IS NOT NULL)::boolean AS phone_verified, u.created_at,
       coalesce(w.balance_kes, 0)::bigint AS balance_kes,
       (SELECT count(*) FROM tickets.tickets t WHERE t.user_id = u.id AND t.status = 'paid')::bigint AS plays,
       (SELECT max(s.created_at) FROM identity.sessions s WHERE s.user_id = u.id) AS last_sign_in_at,
       c.requested_at AS closure_requested_at
FROM identity.users u
LEFT JOIN wallet.wallets w ON w.user_id = u.id
LEFT JOIN identity.closure_requests c ON c.user_id = u.id AND c.closed_at IS NULL
WHERE (sqlc.narg(q)::text IS NULL OR u.msisdn LIKE '%' || sqlc.narg(q)::text || '%' OR u.id::text = sqlc.narg(q)::text
       OR u.email LIKE '%' || lower(sqlc.narg(q)::text) || '%')
  AND (sqlc.narg(status)::text IS NULL OR u.status = sqlc.narg(status)::text)
  AND (sqlc.narg(before)::uuid IS NULL OR u.id < sqlc.narg(before)::uuid)
ORDER BY u.id DESC
LIMIT sqlc.arg(lim);

-- name: SetUserSuspended :execrows
-- suspend an active account or reinstate a suspended one; closed accounts are left alone
UPDATE identity.users SET status = sqlc.arg(status), updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id) AND status = sqlc.arg(from_status);
