-- ── idempotency_keys ──────────────────────────────────────────────────────────
-- Stores hashed callback identifiers to detect and reject duplicate deliveries.
-- The key is a SHA-256 of the raw request body so replay attacks with a
-- different Content-Type or header ordering are still caught.
--
-- TTL cleanup is handled by the scheduler (retry_job.go) which deletes rows
-- older than CALLBACK_DEDUP_TTL; PostgreSQL itself never auto-purges rows.

CREATE TABLE idempotency_keys (
    key         TEXT        PRIMARY KEY,        -- SHA-256 hex of raw body
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- optional back-reference; NULL if the original processing failed before
    -- a transaction row was created.
    transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL
);

-- Partial index: only live (recent) keys are queried at high frequency.
-- The scheduler prunes old rows; this index stays lean automatically.
CREATE INDEX idx_idempotency_keys_received_at
    ON idempotency_keys (received_at DESC);