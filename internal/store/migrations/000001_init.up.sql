-- ── Extensions ────────────────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pg_trgm";   -- trigram indexes for search

-- ── Enumerations ──────────────────────────────────────────────────────────────
CREATE TYPE transaction_status AS ENUM (
    'pending',
    'completed',
    'failed',
    'reversed',
    'cancelled'
);

CREATE TYPE transaction_type AS ENUM (
    'c2b',        -- customer to business (STK push / paybill)
    'b2c',        -- business to customer (disbursement)
    'b2b',        -- business to business
    'reversal'
);

CREATE TYPE reconciliation_status AS ENUM (
    'pending',
    'running',
    'completed',
    'failed'
);

-- ── transactions ──────────────────────────────────────────────────────────────
-- Central fact table. One row per M-Pesa callback received.
-- mpesa_receipt_number is the Safaricom-assigned unique ID; we treat it
-- as the natural key for deduplication.
CREATE TABLE transactions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mpesa_receipt_number  TEXT        NOT NULL,
    transaction_type      transaction_type NOT NULL,
    status                transaction_status NOT NULL DEFAULT 'pending',

    -- Monetary fields stored as integer (KES cents) to avoid float precision.
    amount_cents          BIGINT      NOT NULL CHECK (amount_cents >= 0),
    currency              CHAR(3)     NOT NULL DEFAULT 'KES',

    -- Parties
    msisdn                TEXT        NOT NULL,   -- phone number, E.164
    business_short_code   TEXT        NOT NULL,
    account_reference     TEXT,                   -- paybill account number
    third_party_txn_id    TEXT,                   -- downstream system ref

    -- Timestamps (all UTC)
    transacted_at         TIMESTAMPTZ NOT NULL,   -- Safaricom transaction time
    received_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(), -- our receipt time
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Raw payload stored for audit / replay
    raw_payload           JSONB       NOT NULL DEFAULT '{}',

    -- Soft-delete / reversal tracking
    reversed_by_id        UUID REFERENCES transactions(id),
    reversal_reason       TEXT,

    CONSTRAINT uq_mpesa_receipt UNIQUE (mpesa_receipt_number)
);

CREATE INDEX idx_transactions_status        ON transactions (status);
CREATE INDEX idx_transactions_msisdn        ON transactions (msisdn);
CREATE INDEX idx_transactions_transacted_at ON transactions (transacted_at DESC);
CREATE INDEX idx_transactions_type_status   ON transactions (transaction_type, status);
-- GIN index for JSON payload search (operator dashboard)
CREATE INDEX idx_transactions_payload       ON transactions USING GIN (raw_payload jsonb_path_ops);

-- Auto-update updated_at on any write
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_transactions_updated_at
    BEFORE UPDATE ON transactions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── retry_queue ───────────────────────────────────────────────────────────────
-- Tracks callbacks that failed processing and need to be retried.
CREATE TABLE retry_queue (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id  UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    attempt         INT  NOT NULL DEFAULT 1 CHECK (attempt >= 1),
    max_attempts    INT  NOT NULL DEFAULT 5,
    next_retry_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_retry_queue_next ON retry_queue (next_retry_at)
    WHERE attempt < max_attempts;

CREATE TRIGGER trg_retry_queue_updated_at
    BEFORE UPDATE ON retry_queue
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── reconciliation_runs ───────────────────────────────────────────────────────
-- One row per export batch. The actual file is stored externally (S3 / disk);
-- we keep the metadata and a SHA-256 content hash for integrity checks.
CREATE TABLE reconciliation_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status          reconciliation_status NOT NULL DEFAULT 'pending',
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    row_count       INT,
    file_path       TEXT,                 -- relative path on storage backend
    file_sha256     TEXT,                 -- hex-encoded SHA-256 of the file
    format          TEXT NOT NULL DEFAULT 'csv' CHECK (format IN ('csv', 'pdf')),
    error_message   TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_period CHECK (period_end > period_start)
);

CREATE INDEX idx_reconciliation_runs_status ON reconciliation_runs (status);
CREATE INDEX idx_reconciliation_runs_period ON reconciliation_runs (period_start, period_end);

CREATE TRIGGER trg_reconciliation_runs_updated_at
    BEFORE UPDATE ON reconciliation_runs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ── audit_log ─────────────────────────────────────────────────────────────────
-- Immutable append-only record of significant state changes.
-- No updates, no deletes — enforced by the application layer.
CREATE TABLE audit_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  TEXT        NOT NULL,   -- 'transaction' | 'reconciliation_run'
    entity_id    UUID        NOT NULL,
    action       TEXT        NOT NULL,   -- 'created' | 'status_changed' | ...
    actor        TEXT,                   -- user or service that triggered it
    old_state    JSONB,
    new_state    JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_entity ON audit_log (entity_type, entity_id);
CREATE INDEX idx_audit_log_created ON audit_log (created_at DESC);