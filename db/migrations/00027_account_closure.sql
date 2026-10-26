-- +goose Up
-- A player may ask for their account to be closed. Closing is a staff step (back office): the balance must be paid out
-- first and the records the regulator and KRA require are kept, so the request is recorded here and the account stays
-- usable until staff close it. One request per player; the request itself never changes, only its closing is added.
CREATE TABLE identity.closure_requests (
  user_id       uuid PRIMARY KEY REFERENCES identity.users (id),
  requested_at  timestamptz NOT NULL,
  closed_at     timestamptz,
  closed_by     uuid,
  CONSTRAINT closure_requests_closed_has_staff CHECK ((closed_at IS NULL) = (closed_by IS NULL)),
  CONSTRAINT closure_requests_closed_after_request CHECK (closed_at IS NULL OR closed_at >= requested_at)
);

-- +goose StatementBegin
CREATE FUNCTION identity.check_closure_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.user_id IS DISTINCT FROM OLD.user_id OR NEW.requested_at IS DISTINCT FROM OLD.requested_at
     OR OLD.closed_at IS NOT NULL THEN
    RAISE EXCEPTION 'closure request %: only an open request can be closed, once', OLD.user_id USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER closure_requests_transition BEFORE UPDATE ON identity.closure_requests
  FOR EACH ROW EXECUTE FUNCTION identity.check_closure_transition();
CREATE TRIGGER closure_requests_no_delete BEFORE DELETE ON identity.closure_requests
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();
CREATE TRIGGER closure_requests_no_truncate BEFORE TRUNCATE ON identity.closure_requests
  FOR EACH STATEMENT EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  -- players ask (the API); staff close (the back office runs with the worker's role)
  GRANT SELECT, INSERT ON identity.closure_requests TO app_api;
  GRANT SELECT, UPDATE ON identity.closure_requests TO app_worker;
  GRANT SELECT ON identity.closure_requests TO app_readonly;
  ALTER TABLE identity.closure_requests OWNER TO app_migrator;
  ALTER FUNCTION identity.check_closure_transition() OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE identity.closure_requests;
DROP FUNCTION identity.check_closure_transition();
