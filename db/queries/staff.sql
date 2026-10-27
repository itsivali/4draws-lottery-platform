-- name: MemberByIdentity :one
SELECT m.id, m.email, m.name, m.roles, m.active FROM staff.identities i JOIN staff.members m ON m.id = i.member_id
WHERE i.provider = $1 AND i.subject = $2;

-- name: MemberByEmail :one
SELECT id, email, name, roles, active FROM staff.members WHERE email = $1;

-- name: BindIdentity :exec
INSERT INTO staff.identities (provider, subject, member_id) VALUES ($1, $2, $3);

-- name: InsertSession :exec
INSERT INTO staff.sessions (token_hash, member_id, provider, created_at, expires_at, last_seen_at)
VALUES (sqlc.arg(token_hash), sqlc.arg(member_id), sqlc.arg(provider), sqlc.arg(at), sqlc.arg(expires_at), sqlc.arg(at));

-- name: SessionMember :one
-- a live session and its member as they are now (roles and active state are read on every request)
SELECT s.id AS session_id, s.last_seen_at, m.id, m.email, m.name, m.roles, m.active
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
       coalesce((SELECT array_agg(DISTINCT i.provider ORDER BY i.provider) FROM staff.identities i WHERE i.member_id = m.id), '{}')::text[] AS providers
FROM staff.members m
ORDER BY m.active DESC, m.email;

-- name: LastSignIns :many
SELECT member_id, max(created_at)::timestamptz AS last_sign_in FROM staff.sessions GROUP BY member_id;

-- name: InsertMember :one
INSERT INTO staff.members (email, name, roles, invited_by, created_at, updated_at)
VALUES (sqlc.arg(email), sqlc.arg(name), sqlc.arg(roles), sqlc.arg(invited_by), sqlc.arg(at), sqlc.arg(at))
RETURNING id;

-- name: LockMember :one
SELECT id, email, name, roles, active FROM staff.members WHERE id = $1 FOR UPDATE;

-- name: UpdateMember :exec
UPDATE staff.members SET roles = sqlc.arg(roles), active = sqlc.arg(active), updated_at = sqlc.arg(at) WHERE id = sqlc.arg(id);

-- name: ActiveAdmins :one
-- serialises role changes that could leave no admin (callers lock the member rows they change first)
SELECT count(*) FROM staff.members WHERE active AND 'admin' = ANY (roles);
