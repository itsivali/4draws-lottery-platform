-- +goose Up
-- The platform a session was opened from (ADR 0019, app metrics). The web app sends no device fingerprint, so its
-- sessions have no device row; the platform is kept on the session itself so web use can be counted.
ALTER TABLE identity.sessions ADD COLUMN platform text CHECK (platform IN ('android','ios','web'));
UPDATE identity.sessions s SET platform = d.platform FROM identity.devices d WHERE d.id = s.device_id AND s.platform IS NULL;

-- +goose Down
ALTER TABLE identity.sessions DROP COLUMN platform;
