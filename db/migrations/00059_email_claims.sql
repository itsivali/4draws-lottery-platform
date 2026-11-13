-- +goose Up
-- An unconfirmed email address is only a claim (ADR 0041): it no longer stops anyone else from giving the same
-- address. Only one account may hold an address as confirmed, and confirming it takes it from every account that
-- merely claimed it.
DROP INDEX identity.users_email_key;
CREATE UNIQUE INDEX users_email_confirmed_key ON identity.users (email) WHERE email IS NOT NULL AND email_verified_at IS NOT NULL;
CREATE INDEX users_email_idx ON identity.users (email) WHERE email IS NOT NULL;

-- +goose Down
DROP INDEX identity.users_email_idx;
DROP INDEX identity.users_email_confirmed_key;
-- going back needs one account per address: later claims of an address lose it
UPDATE identity.users u SET email = NULL, email_verified_at = NULL
 WHERE u.email IS NOT NULL AND EXISTS (
   SELECT 1 FROM identity.users o
    WHERE o.email = u.email AND o.id <> u.id
      AND ((o.email_verified_at IS NOT NULL AND u.email_verified_at IS NULL)
        OR ((o.email_verified_at IS NULL) = (u.email_verified_at IS NULL) AND o.id < u.id)));
CREATE UNIQUE INDEX users_email_key ON identity.users (email) WHERE email IS NOT NULL;
