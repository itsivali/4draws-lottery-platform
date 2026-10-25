-- +goose Up
-- KRA remittance batches and operational alerts (docs/runbooks/tax.md, docs/runbooks/alerts.md).

-- The operator's bank account, from which withheld tax is remitted to KRA.
INSERT INTO ledger.ledger_accounts (code, name, type) VALUES ('OPERATOR_BANK', 'Operator bank account', 'asset');

-- A remittance batch: the withholding tax events not yet batched, frozen into one numbered file with control totals
-- and its SHA-256, numbered without gaps. One person prepares it; another records its remittance (the KRA payment registration number, for
-- exactly the batch's tax), which posts TAX_PAYABLE → OPERATOR_BANK. Nothing else about a batch ever changes.
CREATE TABLE audit.kra_batches (
  id              uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  batch_no        bigint NOT NULL CHECK (batch_no > 0),
  events          integer NOT NULL CHECK (events > 0),
  first_event_at  timestamptz NOT NULL,
  last_event_at   timestamptz NOT NULL,
  gross_kes       bigint NOT NULL CHECK (gross_kes >= 0),
  tax_kes         bigint NOT NULL CHECK (tax_kes > 0),
  file_sha256     text NOT NULL CHECK (file_sha256 ~ '^[0-9a-f]{64}$'),
  prepared_by     text NOT NULL CHECK (btrim(prepared_by) <> ''),
  prepared_at     timestamptz NOT NULL,
  prn             text CHECK (btrim(prn) <> ''),
  remitted_kes    bigint,
  remitted_by     text,
  remitted_at     timestamptz,
  CONSTRAINT kra_batches_batch_no_key UNIQUE (batch_no),
  CONSTRAINT kra_batches_prn_key UNIQUE (prn),
  CONSTRAINT kra_batches_events_in_order CHECK (first_event_at <= last_event_at),
  CONSTRAINT kra_batches_remittance_whole CHECK (num_nulls(prn, remitted_kes, remitted_by, remitted_at) IN (0, 4)),
  CONSTRAINT kra_batches_remitted_in_full CHECK (remitted_kes IS NULL OR remitted_kes = tax_kes),
  CONSTRAINT kra_batches_two_people CHECK (remitted_by IS NULL OR remitted_by <> prepared_by)
);

ALTER TABLE audit.kra_outbox ADD COLUMN batch_id uuid REFERENCES audit.kra_batches (id),
  ADD CONSTRAINT kra_outbox_batched_is_exported CHECK (batch_id IS NULL OR exported_at IS NOT NULL);
CREATE INDEX kra_outbox_unbatched_idx ON audit.kra_outbox (id) WHERE batch_id IS NULL;

-- +goose StatementBegin
CREATE FUNCTION audit.kra_batch_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'kra batch %: batches are never deleted', OLD.batch_no USING ERRCODE = 'restrict_violation';
  END IF;
  IF OLD.prn IS NOT NULL THEN
    RAISE EXCEPTION 'kra batch %: a remitted batch is final', OLD.batch_no USING ERRCODE = 'restrict_violation';
  END IF;
  IF (NEW.id, NEW.batch_no, NEW.events, NEW.first_event_at, NEW.last_event_at, NEW.gross_kes, NEW.tax_kes, NEW.file_sha256,
      NEW.prepared_by, NEW.prepared_at)
     IS DISTINCT FROM
     (OLD.id, OLD.batch_no, OLD.events, OLD.first_event_at, OLD.last_event_at, OLD.gross_kes, OLD.tax_kes, OLD.file_sha256,
      OLD.prepared_by, OLD.prepared_at) THEN
    RAISE EXCEPTION 'kra batch %: only its remittance may be recorded', OLD.batch_no USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER kra_batches_guard BEFORE UPDATE OR DELETE ON audit.kra_batches
  FOR EACH ROW EXECUTE FUNCTION audit.kra_batch_guard();

-- +goose StatementBegin
CREATE FUNCTION audit.kra_outbox_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.id, NEW.event_type, NEW.payload, NEW.created_at) IS DISTINCT FROM (OLD.id, OLD.event_type, OLD.payload, OLD.created_at)
     OR (OLD.batch_id IS NOT NULL AND (NEW.batch_id, NEW.exported_at) IS DISTINCT FROM (OLD.batch_id, OLD.exported_at)) THEN
    RAISE EXCEPTION 'kra event %: an event and its batch never change', OLD.id USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER kra_outbox_guard BEFORE UPDATE ON audit.kra_outbox
  FOR EACH ROW EXECUTE FUNCTION audit.kra_outbox_guard();
CREATE TRIGGER kra_outbox_no_delete BEFORE DELETE ON audit.kra_outbox
  FOR EACH ROW EXECUTE FUNCTION platform.forbid_mutation();

-- An alert: something a person must look at. One open alert per (kind, subject) however often its check fires. A page
-- is re-sent until someone acknowledges it; an alert that freezes payouts stops B2C sends until a person acknowledges
-- it, and only a person closes it (never the check that raised it).
CREATE TABLE audit.alerts (
  id                    uuid PRIMARY KEY DEFAULT platform.uuid_v7(),
  kind                  text NOT NULL,
  subject               text NOT NULL,
  severity              text NOT NULL CHECK (severity IN ('page','warn')),
  freezes               text CHECK (freezes IN ('payouts')),
  summary               text NOT NULL,
  details               jsonb NOT NULL DEFAULT '{}',
  first_seen_at         timestamptz NOT NULL,
  last_seen_at          timestamptz NOT NULL,
  occurrences           integer NOT NULL DEFAULT 1 CHECK (occurrences > 0),
  notified_at           timestamptz,
  notifications         integer NOT NULL DEFAULT 0 CHECK (notifications >= 0),
  acknowledged_by       text,
  acknowledged_at       timestamptz,
  ack_note              text,
  resolved_by           text,
  resolved_at           timestamptz,
  resolution_notified_at timestamptz,
  CONSTRAINT alerts_ack_whole CHECK (num_nulls(acknowledged_by, acknowledged_at, ack_note) IN (0, 3)),
  CONSTRAINT alerts_ack_note CHECK (ack_note IS NULL OR btrim(ack_note) <> ''),
  CONSTRAINT alerts_resolution_whole CHECK ((resolved_by IS NULL) = (resolved_at IS NULL)),
  CONSTRAINT alerts_freeze_closed_by_a_person CHECK (freezes IS NULL OR resolved_at IS NULL OR acknowledged_at IS NOT NULL)
);
CREATE UNIQUE INDEX alerts_open_once ON audit.alerts (kind, subject) WHERE resolved_at IS NULL;
CREATE INDEX alerts_freezes_idx ON audit.alerts (freezes) WHERE resolved_at IS NULL AND freezes IS NOT NULL;

-- +goose StatementBegin
CREATE FUNCTION audit.alert_guard() RETURNS trigger
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
CREATE TRIGGER alerts_guard BEFORE UPDATE OR DELETE ON audit.alerts
  FOR EACH ROW EXECUTE FUNCTION audit.alert_guard();

-- +goose StatementBegin
DO $$
BEGIN
  IF (SELECT count(*) FROM pg_roles WHERE rolname IN ('app_migrator','app_api','app_worker','app_readonly')) < 4
     OR NOT pg_has_role(current_user, 'app_migrator', 'SET') THEN
    RAISE WARNING 'application roles unavailable: skipping grants (SINGLE-ROLE MODE)';
    RETURN;
  END IF;
  -- the worker (and lotteryctl) batch events, record remittances and keep alerts; the api only reads
  GRANT SELECT, INSERT ON audit.kra_batches TO app_worker;
  GRANT UPDATE (prn, remitted_kes, remitted_by, remitted_at) ON audit.kra_batches TO app_worker;
  GRANT SELECT ON audit.kra_batches TO app_api, app_readonly;
  GRANT UPDATE (batch_id) ON audit.kra_outbox TO app_worker;
  GRANT SELECT, INSERT, UPDATE ON audit.alerts TO app_worker;
  GRANT SELECT ON audit.alerts TO app_api, app_readonly;
  ALTER TABLE audit.kra_batches OWNER TO app_migrator;
  ALTER TABLE audit.alerts OWNER TO app_migrator;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE audit.alerts;
DROP FUNCTION audit.alert_guard();
DROP TRIGGER kra_outbox_no_delete ON audit.kra_outbox;
DROP TRIGGER kra_outbox_guard ON audit.kra_outbox;
DROP FUNCTION audit.kra_outbox_guard();
DROP INDEX audit.kra_outbox_unbatched_idx;
ALTER TABLE audit.kra_outbox DROP CONSTRAINT kra_outbox_batched_is_exported, DROP COLUMN batch_id;
DROP TABLE audit.kra_batches;
DROP FUNCTION audit.kra_batch_guard();
DELETE FROM ledger.ledger_accounts WHERE code = 'OPERATOR_BANK';
