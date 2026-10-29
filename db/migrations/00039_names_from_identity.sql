-- +goose Up
-- Owner, 2026-10-30: the player's name is the one on their identity submission, not something asked at sign-up; the
-- columns of migration 00038 go (they were never asked of a live player: 00038 and this shipped the same day).
ALTER TABLE identity.users DROP COLUMN first_name, DROP COLUMN second_name;

-- +goose Down
ALTER TABLE identity.users
  ADD COLUMN first_name  text CONSTRAINT users_first_name_length CHECK (first_name IS NULL OR length(first_name) BETWEEN 1 AND 40),
  ADD COLUMN second_name text CONSTRAINT users_second_name_length CHECK (second_name IS NULL OR length(second_name) BETWEEN 1 AND 40);
