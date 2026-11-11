-- +goose Up
-- ADR 0035: the photo of the Google account a player or staff member signs in with, as Google serves it (an https URL
-- on googleusercontent.com, refreshed at each Google sign-in). Only Google's own image host is accepted, so a link can
-- never point the app or the back office at another server. Apple gives no photo.
ALTER TABLE identity.users ADD COLUMN photo_url text
  CHECK (length(photo_url) <= 1600 AND photo_url ~ '^https://[a-z0-9-]+\.googleusercontent\.com/[^[:space:]"''<>]+$');
ALTER TABLE staff.members ADD COLUMN photo_url text
  CHECK (length(photo_url) <= 1600 AND photo_url ~ '^https://[a-z0-9-]+\.googleusercontent\.com/[^[:space:]"''<>]+$');

-- +goose Down
ALTER TABLE staff.members DROP COLUMN photo_url;
ALTER TABLE identity.users DROP COLUMN photo_url;
