-- name: MemberByIdentity :one
SELECT m.id, m.email, m.name, m.roles, m.active FROM staff.identities i JOIN staff.members m ON m.id = i.member_id
WHERE i.provider = $1 AND i.subject = $2;

-- name: MemberByEmail :one
SELECT id, email, name, roles, active FROM staff.members WHERE email = $1;

-- name: MemberCredentials :one
-- what a password sign-in needs (the hash is compared in constant time by the service)
SELECT id, email, name, roles, active, password_hash, must_change_password FROM staff.members WHERE email = $1;

-- name: SetPassword :exec
UPDATE staff.members SET password_hash = sqlc.arg(password_hash), password_changed_at = sqlc.arg(at),
       must_change_password = sqlc.arg(must_change), updated_at = sqlc.arg(at)
WHERE id = sqlc.arg(id);

-- name: RevokeOtherSessions :exec
-- after a password change: every other session of the member ends, the one that changed it goes on
UPDATE staff.sessions SET revoked_at = sqlc.arg(at)
WHERE member_id = sqlc.arg(member_id) AND token_hash <> sqlc.arg(keep_token_hash) AND revoked_at IS NULL;

-- name: BindIdentity :exec
INSERT INTO staff.identities (provider, subject, member_id) VALUES ($1, $2, $3);

-- name: InsertSession :exec
INSERT INTO staff.sessions (token_hash, member_id, provider, created_at, expires_at, last_seen_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(member_id), sqlc.arg(provider), sqlc.arg(at), sqlc.arg(expires_at), sqlc.arg(at));

-- name: SessionMember :one
-- a live session and its member as they are now (roles and active state are read on every request)
SELECT s.id AS session_id, s.last_seen_at, s.provider, m.id, m.email, m.name, m.roles, m.active, m.must_change_password
FROM staff.sessions s JOIN staff.members m ON m.id = s.member_id
WHERE s.token_hash = sqlc.arg(token_hash) AND s.revoked_at IS NULL AND s.expires_at > sqlc.arg(now)
  AND s.last_seen_at > sqlc.arg(idle_after);

-- name: TouchSession :exec
UPDATE staff.sessions SET last_seen_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: RevokeSession :exec
UPDATE staff.sessions SET revoked_at = sqlc.arg(at) WHERE token_hash = sqlc.arg(token_hash) AND revoked_at IS NULL;

-- name: RevokeMemberSessions :exec
UPDATE staff.sessions SET revoked_at = sqlc.arg(at) WHERE member_id = sqlc.arg(member_id) AND revoked_at IS NULL;

-- name: ListMembers :many
SELECT m.id, m.email, m.name, m.roles, m.active, m.invited_by, m.created_at, m.updated_at,
       (m.password_hash IS NOT NULL)::boolean AS has_password, m.must_change_password,
       coalesce((SELECT array_agg(DISTINCT i.provider ORDER BY i.provider) FROM staff.identities i WHERE i.member_id = m.id), '{}')::text[] AS providers
FROM staff.members m
ORDER BY m.active DESC, m.email;

-- name: LastSignIns :many
SELECT member_id, max(created_at)::timestamptz AS last_sign_in FROM staff.sessions GROUP BY member_id;

-- name: InsertMember :one
INSERT INTO staff.members (email, name, roles, invited_by, created_at, updated_at, password_hash, password_changed_at, must_change_password)
VALUES (sqlc.arg(email), sqlc.arg(name), sqlc.arg(roles), sqlc.arg(invited_by), sqlc.arg(at), sqlc.arg(at),
        sqlc.narg(password_hash), CASE WHEN sqlc.narg(password_hash)::text IS NULL THEN NULL ELSE sqlc.arg(at)::timestamptz END, sqlc.narg(password_hash)::text IS NOT NULL)
RETURNING id;

-- name: LockMember :one
SELECT id, email, name, roles, active, password_hash FROM staff.members WHERE id = $1 FOR UPDATE;

-- name: UpdateMember :exec
UPDATE staff.members SET roles = sqlc.arg(roles), active = sqlc.arg(active), updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: SetName :execrows
UPDATE staff.members SET name = sqlc.arg(name), updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: ActiveAdmins :one
-- serialises role changes that could leave no admin (callers lock the member rows they change first)
SELECT count(*) FROM staff.members WHERE active AND 'admin' = ANY (roles);

-- name: InsertPasswordReset :exec
INSERT INTO staff.password_resets (member_id, token_hash, expires_at, created_at)
VALUES (sqlc.arg(member_id), sqlc.arg(token_hash), sqlc.arg(expires_at), sqlc.arg(at));

-- name: PasswordResetByHash :one
SELECT r.id, r.member_id, r.expires_at, r.used_at, m.email, m.active
FROM staff.password_resets r JOIN staff.members m ON m.id = r.member_id
WHERE r.token_hash = $1
FOR UPDATE OF r;

-- name: UseMemberPasswordResets :exec
-- a reset consumes every open link of the member, not only the one used
UPDATE staff.password_resets SET used_at = sqlc.arg(at) WHERE member_id = sqlc.arg(member_id) AND used_at IS NULL;
