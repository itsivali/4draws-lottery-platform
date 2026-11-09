-- name: LatestModeChange :one
SELECT c.id, c.mode, c.reason, c.changed_by, c.created_at, r.licence_reference
  FROM compliance.mode_changes c
  LEFT JOIN compliance.live_requests r ON r.id = c.live_request_id
 ORDER BY c.created_at DESC, c.id DESC
 LIMIT 1;

-- name: ModeHistory :many
SELECT c.id, c.mode, c.reason, c.changed_by, c.created_at, r.licence_reference
  FROM compliance.mode_changes c
  LEFT JOIN compliance.live_requests r ON r.id = c.live_request_id
 ORDER BY c.created_at DESC, c.id DESC
 LIMIT sqlc.arg(lim);

-- name: InsertModeChange :one
INSERT INTO compliance.mode_changes (mode, live_request_id, reason, changed_by, created_at)
VALUES (sqlc.arg(mode), sqlc.narg(live_request_id), sqlc.arg(reason), sqlc.arg(changed_by), sqlc.arg(at))
RETURNING id;

-- name: InsertLiveRequest :one
INSERT INTO compliance.live_requests (licence_reference, approval_reference, evidence_sha256, evidence_location, reason, requested_by, created_at)
VALUES (sqlc.arg(licence_reference), sqlc.arg(approval_reference), sqlc.arg(evidence_sha256), sqlc.arg(evidence_location),
        sqlc.arg(reason), sqlc.arg(requested_by), sqlc.arg(at))
RETURNING id;

-- name: GetLiveRequest :one
SELECT r.id, r.licence_reference, r.approval_reference, r.evidence_sha256, r.evidence_location, r.reason, r.requested_by,
       r.created_at, a.approved_by, a.created_at AS approved_at
  FROM compliance.live_requests r
  LEFT JOIN compliance.live_approvals a ON a.request_id = r.id
 WHERE r.id = sqlc.arg(id);

-- name: OpenLiveRequests :many
SELECT r.id, r.licence_reference, r.approval_reference, r.evidence_sha256, r.evidence_location, r.reason, r.requested_by, r.created_at
  FROM compliance.live_requests r
 WHERE NOT EXISTS (SELECT 1 FROM compliance.live_approvals a WHERE a.request_id = r.id)
   AND r.created_at > sqlc.arg(since)
 ORDER BY r.created_at DESC;

-- name: InsertLiveApproval :exec
INSERT INTO compliance.live_approvals (request_id, approved_by, created_at) VALUES (sqlc.arg(request_id), sqlc.arg(approved_by), sqlc.arg(at));

-- name: InsertGeoSignal :exec
INSERT INTO compliance.geo_signals (user_id, action, country, refused, observed_at)
VALUES (sqlc.narg(user_id), sqlc.arg(action), sqlc.narg(country), sqlc.arg(refused), sqlc.arg(at));

-- name: GeoSignals :many
-- An account's geo signals, newest first.
SELECT action, country, refused, observed_at FROM compliance.geo_signals
WHERE user_id = sqlc.arg(user_id) ORDER BY id DESC LIMIT sqlc.arg(max_rows);
