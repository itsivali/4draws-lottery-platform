-- +goose Up
-- Erasure at closure (ADR 0037). When staff close an account at the player's request, what the law does not require
-- the operator to keep is erased in the same transaction: the password, every session and refresh token, the
-- devices, the links to Google or Apple, the profile photo, the email address and any reset links. What the
-- law requires stays: the account row with its phone number and status, identity checks, payments, plays, prizes,
-- the ledger, tax, restrictions and the audit log.
--
-- One function does it, and only for an account that is already closed, so the api needs no right to delete
-- sessions or devices in general. Calling it again does nothing more and keeps the first time.
ALTER TABLE identity.users ADD COLUMN erased_at timestamptz;

-- +goose StatementBegin
CREATE FUNCTION identity.erase_closed_account(p_user uuid, p_at timestamptz)
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
  -- what was removed, for the audit log (counts and yes/no only, never the values)
  RETURN jsonb_build_object('sessions', sessions, 'devices', devices, 'sign_in_links', sign_in_links,
    'reset_links', reset_links, 'password', had_password, 'email', had_email, 'photo', had_photo);
END $$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION identity.erase_closed_account(uuid, timestamptz) FROM PUBLIC;

-- accounts closed before this existed are erased now
SELECT identity.erase_closed_account(id, now()) FROM identity.users WHERE status = 'closed' AND erased_at IS NULL;

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  ALTER FUNCTION identity.erase_closed_account(uuid, timestamptz) OWNER TO app_migrator;
  GRANT EXECUTE ON FUNCTION identity.erase_closed_account(uuid, timestamptz) TO app_api, app_worker;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION identity.erase_closed_account(uuid, timestamptz);
ALTER TABLE identity.users DROP COLUMN erased_at;
