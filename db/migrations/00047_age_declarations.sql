-- +goose Up
-- ADR 0029: every new account gives a date of birth and declares it is 18 or older, at sign-up, checked by the server.
-- Kept encrypted (as the identity submission's date of birth) and never changed; the identity submission starts from it.
CREATE TABLE kyc.age_declarations (
  user_id            uuid PRIMARY KEY REFERENCES identity.users (id),
  date_of_birth_enc  bytea NOT NULL,
  adult_declared     boolean NOT NULL CHECK (adult_declared),
  declared_at        timestamptz NOT NULL
);
CREATE TRIGGER age_declarations_append_only BEFORE UPDATE OR DELETE ON kyc.age_declarations
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON kyc.age_declarations TO app_api, app_worker;
  GRANT SELECT ON kyc.age_declarations TO app_readonly;
  ALTER TABLE kyc.age_declarations OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE kyc.age_declarations;
