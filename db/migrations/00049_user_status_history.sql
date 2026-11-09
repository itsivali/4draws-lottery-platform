-- +goose Up
-- ADR 0030: every change of an account's status (active, suspended, closed) is kept, with who made it and why. The
-- rows are written by a trigger on identity.users, so no code path can change a status without leaving one; the
-- service names the actor and reason for its transaction (identity.status_actor, identity.status_reason), and a change
-- made without them is still recorded, with them empty (a sign-up is the player's own). Never changed or deleted.
CREATE TABLE identity.user_status_history (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id      uuid NOT NULL REFERENCES identity.users (id),
  from_status  text CHECK (from_status IN ('active','suspended','closed')),
  to_status    text NOT NULL CHECK (to_status IN ('active','suspended','closed')),
  actor_type   text CHECK (actor_type IN ('player','staff','system')),
  actor_id     text,
  reason       text,
  changed_at   timestamptz NOT NULL
);
CREATE INDEX user_status_history_user_idx ON identity.user_status_history (user_id, id);
CREATE TRIGGER user_status_history_append_only BEFORE UPDATE OR DELETE ON identity.user_status_history
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
CREATE FUNCTION identity.record_status_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.status IS NOT DISTINCT FROM OLD.status THEN
    RETURN NEW;
  END IF;
  INSERT INTO identity.user_status_history (user_id, from_status, to_status, actor_type, actor_id, reason, changed_at)
  VALUES (NEW.id, CASE WHEN TG_OP = 'UPDATE' THEN OLD.status END, NEW.status,
          coalesce(nullif(current_setting('identity.status_actor_type', true), ''), CASE WHEN TG_OP = 'INSERT' THEN 'player' END),
          coalesce(nullif(current_setting('identity.status_actor', true), ''), CASE WHEN TG_OP = 'INSERT' THEN NEW.id::text END),
          coalesce(nullif(current_setting('identity.status_reason', true), ''), CASE WHEN TG_OP = 'INSERT' THEN 'signed up' END),
          CASE WHEN TG_OP = 'UPDATE' THEN NEW.updated_at ELSE NEW.created_at END);
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER users_status_history AFTER INSERT OR UPDATE OF status ON identity.users
  FOR EACH ROW EXECUTE FUNCTION identity.record_status_change();

-- Accounts that exist already: their status as of now, the history's first row.
INSERT INTO identity.user_status_history (user_id, from_status, to_status, actor_type, reason, changed_at)
SELECT id, NULL, status, 'system', 'status when the history began', now() FROM identity.users ORDER BY created_at, id;

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON identity.user_status_history TO app_api, app_worker;
  GRANT SELECT ON identity.user_status_history TO app_readonly;
  ALTER TABLE identity.user_status_history OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER users_status_history ON identity.users;
DROP FUNCTION identity.record_status_change();
DROP TABLE identity.user_status_history;
