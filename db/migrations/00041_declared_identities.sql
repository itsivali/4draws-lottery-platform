-- +goose Up
-- Owner, 2026-10-30: a new account gives its first name, second name and national ID number at sign-up. They are kept
-- here, encrypted like an identity submission, and verified when the player first cashes out (ADR 0022): the
-- verification form starts from them and adds the date of birth. One row per account, written once at sign-up.
CREATE TABLE kyc.declared_identities (
  user_id          uuid PRIMARY KEY REFERENCES identity.users (id),
  first_name_enc   bytea NOT NULL,
  second_name_enc  bytea NOT NULL,
  id_number_enc    bytea NOT NULL,
  id_number_hash   bytea NOT NULL, -- for staff lookups; not unique: a number is proved by verification, never by sign-up
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX declared_identities_id_hash_idx ON kyc.declared_identities (id_number_hash);
CREATE TRIGGER declared_identities_append_only BEFORE UPDATE OR DELETE ON kyc.declared_identities
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT ON kyc.declared_identities TO app_api, app_worker;
  GRANT SELECT ON kyc.declared_identities TO app_readonly;
  ALTER TABLE kyc.declared_identities OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE kyc.declared_identities;
