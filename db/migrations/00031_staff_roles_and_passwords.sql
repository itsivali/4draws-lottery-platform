-- +goose Up
-- Owner, 2026-10-28 (ADR 0019): two back-office roles, admin and staff, in place of the ten of ADR 0018, and a
-- password for each member so the back office signs in with email and password (Google and Apple stay optional).
-- Existing admins stay admins; every other member with a role becomes staff.
ALTER TABLE staff.members DROP CONSTRAINT IF EXISTS members_roles_check;
UPDATE staff.members
   SET roles = CASE WHEN 'admin' = ANY (roles) THEN ARRAY['admin'] WHEN cardinality(roles) > 0 THEN ARRAY['staff'] ELSE '{}' END,
       updated_at = now()
 WHERE NOT (roles <@ ARRAY['admin','staff']::text[]);
ALTER TABLE staff.members ADD CONSTRAINT members_roles_check CHECK (roles <@ ARRAY['admin','staff']::text[]);

-- argon2id (internal/platform/password); null until an admin sets one. A temporary password must be changed at the
-- next sign-in before anything else is allowed.
ALTER TABLE staff.members
  ADD COLUMN password_hash         text,
  ADD COLUMN password_changed_at   timestamptz,
  ADD COLUMN must_change_password  boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE staff.members DROP COLUMN must_change_password, DROP COLUMN password_changed_at, DROP COLUMN password_hash;
ALTER TABLE staff.members DROP CONSTRAINT IF EXISTS members_roles_check;
ALTER TABLE staff.members ADD CONSTRAINT members_roles_check CHECK (roles <@ ARRAY['admin','finance','kyc_reviewer','payments_operator',
  'draw_official','payout_operator','tax_officer','alerts_operator','support','auditor','staff']::text[]);
