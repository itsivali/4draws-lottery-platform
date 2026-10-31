-- +goose Up
-- An open warning may become a page (ADR 0025): a condition that was worth a warning at first becomes a page when it
-- has waited too long. Nothing else about what was raised changes, and a page never becomes a warning again.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit.alert_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'alert %: alerts are never deleted', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF (NEW.id, NEW.kind, NEW.subject, NEW.freezes, NEW.first_seen_at)
     IS DISTINCT FROM (OLD.id, OLD.kind, OLD.subject, OLD.freezes, OLD.first_seen_at)
     OR (NEW.severity IS DISTINCT FROM OLD.severity
         AND NOT (OLD.severity = 'warn' AND NEW.severity = 'page' AND OLD.resolved_at IS NULL))
     OR (OLD.acknowledged_at IS NOT NULL AND (NEW.acknowledged_by, NEW.acknowledged_at, NEW.ack_note)
         IS DISTINCT FROM (OLD.acknowledged_by, OLD.acknowledged_at, OLD.ack_note))
     OR (OLD.resolved_at IS NOT NULL AND (NEW.resolved_by, NEW.resolved_at, NEW.summary, NEW.details, NEW.last_seen_at, NEW.occurrences)
         IS DISTINCT FROM (OLD.resolved_by, OLD.resolved_at, OLD.summary, OLD.details, OLD.last_seen_at, OLD.occurrences)) THEN
    RAISE EXCEPTION 'alert %: what was raised, acknowledged or resolved never changes', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit.alert_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'alert %: alerts are never deleted', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  IF (NEW.id, NEW.kind, NEW.subject, NEW.severity, NEW.freezes, NEW.first_seen_at)
     IS DISTINCT FROM (OLD.id, OLD.kind, OLD.subject, OLD.severity, OLD.freezes, OLD.first_seen_at)
     OR (OLD.acknowledged_at IS NOT NULL AND (NEW.acknowledged_by, NEW.acknowledged_at, NEW.ack_note)
         IS DISTINCT FROM (OLD.acknowledged_by, OLD.acknowledged_at, OLD.ack_note))
     OR (OLD.resolved_at IS NOT NULL AND (NEW.resolved_by, NEW.resolved_at, NEW.summary, NEW.details, NEW.last_seen_at, NEW.occurrences)
         IS DISTINCT FROM (OLD.resolved_by, OLD.resolved_at, OLD.summary, OLD.details, OLD.last_seen_at, OLD.occurrences)) THEN
    RAISE EXCEPTION 'alert %: what was raised, acknowledged or resolved never changes', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
