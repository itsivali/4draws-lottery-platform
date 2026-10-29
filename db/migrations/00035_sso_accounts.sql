-- +goose Up
-- An account created with Google or Apple (ADR 0020) has no password, and proves its phone number with its first
-- top-up like any other.
ALTER TABLE identity.users DROP CONSTRAINT users_verified_phone_has_password;

-- +goose Down
-- Fails while a verified account without a password exists.
ALTER TABLE identity.users ADD CONSTRAINT users_verified_phone_has_password
  CHECK (msisdn_verified_at IS NULL OR password_hash IS NOT NULL);
