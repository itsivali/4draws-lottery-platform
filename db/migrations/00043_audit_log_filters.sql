-- +goose Up
-- Owner, 2026-10-31: the back office must help with audits. The activity log is filtered by who acted, by the action's
-- prefix (payment., payout., ...), by day, and by the entity; a player's timeline lists every record about them,
-- whether they acted, were acted on, or are named in the details. The log had one index (entity type, entity id, at);
-- these carry the new filters. The table stays append-only: its triggers are untouched, and indexes need no grants
-- (they follow the table, owned by app_migrator like it).
CREATE INDEX audit_log_actor_idx ON audit.audit_log (actor_id, id);
CREATE INDEX audit_log_action_idx ON audit.audit_log (action text_pattern_ops, id);
CREATE INDEX audit_log_at_idx ON audit.audit_log (at);
CREATE INDEX audit_log_entity_id_idx ON audit.audit_log (entity_id, id);
CREATE INDEX audit_log_details_user_idx ON audit.audit_log ((details->>'user_id'), id) WHERE details ? 'user_id';

-- +goose Down
DROP INDEX IF EXISTS audit.audit_log_details_user_idx, audit.audit_log_entity_id_idx, audit.audit_log_at_idx,
  audit.audit_log_action_idx, audit.audit_log_actor_idx;
