-- +goose Up
-- Email verification (ADR 0040). A player's email address is a claim until they open the link sent to it. The
-- account records when that happened; changing the address clears it. Addresses already on accounts start as
-- not confirmed.
ALTER TABLE identity.users ADD COLUMN email_verified_at timestamptz
  CONSTRAINT users_email_verified_has_email CHECK (email_verified_at IS NULL OR email IS NOT NULL);

-- One row per emailed link: only the SHA-256 of the token is kept, a link lives 24 hours and is used at most once.
-- The row names the address it was sent to, so a link sent before the player changed their address confirms nothing.
CREATE TABLE identity.email_verifications (
  id          uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  user_id     uuid NOT NULL REFERENCES identity.users (id),
  email       text NOT NULL,
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  created_at  timestamptz NOT NULL,
  CONSTRAINT email_verifications_expiry_after_creation CHECK (expires_at > created_at)
);
CREATE INDEX email_verifications_user_idx ON identity.email_verifications (user_id, created_at DESC);

-- Erasure at closure (ADR 0037) now also removes the confirmation links and the confirmation itself.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION identity.erase_closed_account(p_user uuid, p_at timestamptz)
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
DECLARE
  v_status text;
  sessions bigint;
  devices bigint;
  sign_in_links bigint;
  reset_links bigint;
  email_links bigint;
  had_password boolean;
  had_email boolean;
  had_photo boolean;
BEGIN
  SELECT u.status, u.password_hash IS NOT NULL, u.email IS NOT NULL, u.photo_url IS NOT NULL
    INTO v_status, had_password, had_email, had_photo
    FROM identity.users u WHERE u.id = p_user FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'erase_closed_account: no account %', p_user USING ERRCODE = 'no_data_found';
  END IF;
  IF v_status <> 'closed' THEN
    RAISE EXCEPTION 'erase_closed_account: account % is %, not closed', p_user, v_status USING ERRCODE = 'check_violation';
  END IF;

  DELETE FROM identity.refresh_tokens r USING identity.sessions s WHERE r.session_id = s.id AND s.user_id = p_user;
  DELETE FROM identity.sessions s WHERE s.user_id = p_user;
  GET DIAGNOSTICS sessions = ROW_COUNT;
  DELETE FROM identity.devices d WHERE d.user_id = p_user;
  GET DIAGNOSTICS devices = ROW_COUNT;
  DELETE FROM identity.auth_identities a WHERE a.user_id = p_user;
  GET DIAGNOSTICS sign_in_links = ROW_COUNT;
  DELETE FROM identity.password_resets p WHERE p.user_id = p_user;
  GET DIAGNOSTICS reset_links = ROW_COUNT;
  DELETE FROM identity.email_verifications v WHERE v.user_id = p_user;
  GET DIAGNOSTICS email_links = ROW_COUNT;

  UPDATE identity.users u
     SET password_hash = NULL, email = NULL, email_verified_at = NULL, photo_url = NULL, must_change_password = false,
         erased_at = coalesce(u.erased_at, p_at), updated_at = p_at
   WHERE u.id = p_user;
  -- what was removed, for the audit log (counts and yes/no only, never the values)
  RETURN jsonb_build_object('sessions', sessions, 'devices', devices, 'sign_in_links', sign_in_links,
    'reset_links', reset_links, 'email_links', email_links, 'password', had_password, 'email', had_email, 'photo', had_photo);
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  GRANT SELECT, INSERT, UPDATE ON identity.email_verifications TO app_api;
  GRANT SELECT ON identity.email_verifications TO app_worker, app_readonly;
  ALTER TABLE identity.email_verifications OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION identity.erase_closed_account(p_user uuid, p_at timestamptz)
RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
DECLARE
  v_status text;
  sessions bigint;
  devices bigint;
  sign_in_links bigint;
  reset_links bigint;
  had_password boolean;
  had_email boolean;
  had_photo boolean;
BEGIN
  SELECT u.status, u.password_hash IS NOT NULL, u.email IS NOT NULL, u.photo_url IS NOT NULL
    INTO v_status, had_password, had_email, had_photo
    FROM identity.users u WHERE u.id = p_user FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'erase_closed_account: no account %', p_user USING ERRCODE = 'no_data_found';
  END IF;
  IF v_status <> 'closed' THEN
    RAISE EXCEPTION 'erase_closed_account: account % is %, not closed', p_user, v_status USING ERRCODE = 'check_violation';
  END IF;

  DELETE FROM identity.refresh_tokens r USING identity.sessions s WHERE r.session_id = s.id AND s.user_id = p_user;
  DELETE FROM identity.sessions s WHERE s.user_id = p_user;
  GET DIAGNOSTICS sessions = ROW_COUNT;
  DELETE FROM identity.devices d WHERE d.user_id = p_user;
  GET DIAGNOSTICS devices = ROW_COUNT;
  DELETE FROM identity.auth_identities a WHERE a.user_id = p_user;
  GET DIAGNOSTICS sign_in_links = ROW_COUNT;
  DELETE FROM identity.password_resets p WHERE p.user_id = p_user;
  GET DIAGNOSTICS reset_links = ROW_COUNT;

  UPDATE identity.users u
     SET password_hash = NULL, email = NULL, photo_url = NULL, must_change_password = false,
         erased_at = coalesce(u.erased_at, p_at), updated_at = p_at
   WHERE u.id = p_user;
  RETURN jsonb_build_object('sessions', sessions, 'devices', devices, 'sign_in_links', sign_in_links,
    'reset_links', reset_links, 'password', had_password, 'email', had_email, 'photo', had_photo);
END $$;
-- +goose StatementEnd
DROP TABLE identity.email_verifications;
ALTER TABLE identity.users DROP COLUMN email_verified_at;
