-- +goose Up
-- Owner, 2026-10-29: players give their first and second names at sign-up; the account screen shows them above the
-- phone number. Accounts from before have none until the player adds them. Not the legal name: that is the identity
-- submission's (kyc), checked at withdrawal.
ALTER TABLE identity.users
  ADD COLUMN first_name  text CONSTRAINT users_first_name_length CHECK (first_name IS NULL OR length(first_name) BETWEEN 1 AND 40),
  ADD COLUMN second_name text CONSTRAINT users_second_name_length CHECK (second_name IS NULL OR length(second_name) BETWEEN 1 AND 40);

-- +goose Down
ALTER TABLE identity.users DROP COLUMN first_name, DROP COLUMN second_name;
