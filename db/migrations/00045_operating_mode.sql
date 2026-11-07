-- +goose Up
-- ADR 0027 (owner, 2026-11-07): the operating mode. The configuration sets the most the service may do; these tables
-- can only lower it, and make it live only through a request approved by a second admin. All three are append-only.
CREATE SCHEMA IF NOT EXISTS compliance;

-- A request to operate with real money, with the licence it relies on. The evidence itself (the licence, the
-- regulator's approval) stays with the operator; its SHA-256 and where it is kept are recorded here.
CREATE TABLE compliance.live_requests (
  id                  uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  licence_reference   text NOT NULL CHECK (length(btrim(licence_reference)) BETWEEN 1 AND 200),
  approval_reference  text NOT NULL CHECK (length(btrim(approval_reference)) BETWEEN 1 AND 200),
  evidence_sha256     text NOT NULL CHECK (evidence_sha256 ~ '^[0-9a-f]{64}$'),
  evidence_location   text NOT NULL CHECK (length(btrim(evidence_location)) BETWEEN 1 AND 500),
  reason              text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
  requested_by        text NOT NULL CHECK (requested_by LIKE 'staff:%'),
  created_at          timestamptz NOT NULL
);
CREATE TRIGGER live_requests_append_only BEFORE UPDATE OR DELETE ON compliance.live_requests
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- The second admin's approval: one per request, never the requester.
CREATE TABLE compliance.live_approvals (
  request_id   uuid PRIMARY KEY REFERENCES compliance.live_requests (id),
  approved_by  text NOT NULL CHECK (approved_by LIKE 'staff:%'),
  created_at   timestamptz NOT NULL
);
CREATE TRIGGER live_approvals_append_only BEFORE UPDATE OR DELETE ON compliance.live_approvals
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
CREATE FUNCTION compliance.check_live_approval() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.approved_by = (SELECT requested_by FROM compliance.live_requests WHERE id = NEW.request_id) THEN
    RAISE EXCEPTION 'live request %: the requester cannot approve it', NEW.request_id USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER live_approvals_second_person BEFORE INSERT ON compliance.live_approvals
  FOR EACH ROW EXECUTE FUNCTION compliance.check_live_approval();

-- Every change of mode. The latest row is the database's mode; with none it is sandbox (the configuration decides).
-- A live change must carry an approved request.
CREATE TABLE compliance.mode_changes (
  id               uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  mode             text NOT NULL CHECK (mode IN ('disabled','sandbox','live')),
  live_request_id  uuid REFERENCES compliance.live_approvals (request_id),
  reason           text NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
  changed_by       text NOT NULL CHECK (changed_by LIKE 'staff:%'),
  created_at       timestamptz NOT NULL,
  CONSTRAINT mode_changes_live_has_approval CHECK ((mode = 'live') = (live_request_id IS NOT NULL))
);
CREATE INDEX mode_changes_latest_idx ON compliance.mode_changes (created_at DESC, id DESC);
CREATE TRIGGER mode_changes_append_only BEFORE UPDATE OR DELETE ON compliance.mode_changes
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT USAGE ON SCHEMA compliance TO app_api, app_worker, app_readonly;
  GRANT SELECT, INSERT ON compliance.live_requests, compliance.live_approvals, compliance.mode_changes TO app_api, app_worker;
  GRANT SELECT ON compliance.live_requests, compliance.live_approvals, compliance.mode_changes TO app_readonly;
  ALTER SCHEMA compliance OWNER TO app_migrator; -- foreign-key checks run as the tables' owner
  ALTER TABLE compliance.live_requests OWNER TO app_migrator;
  ALTER TABLE compliance.live_approvals OWNER TO app_migrator;
  ALTER TABLE compliance.mode_changes OWNER TO app_migrator;
  ALTER FUNCTION compliance.check_live_approval() OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE compliance.mode_changes;
DROP TABLE compliance.live_approvals;
DROP FUNCTION compliance.check_live_approval();
DROP TABLE compliance.live_requests;
DROP SCHEMA compliance;
