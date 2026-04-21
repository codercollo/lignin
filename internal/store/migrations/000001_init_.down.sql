-- Reverse of 000001_init.up.sql
-- Drop in reverse dependency order.

DROP TRIGGER  IF EXISTS trg_reconciliation_runs_updated_at ON reconciliation_runs;
DROP TRIGGER  IF EXISTS trg_retry_queue_updated_at         ON retry_queue;
DROP TRIGGER  IF EXISTS trg_transactions_updated_at        ON transactions;
DROP FUNCTION IF EXISTS set_updated_at();

DROP TABLE IF EXISTS audit_log             CASCADE;
DROP TABLE IF EXISTS reconciliation_runs   CASCADE;
DROP TABLE IF EXISTS retry_queue           CASCADE;
DROP TABLE IF EXISTS transactions          CASCADE;

DROP TYPE IF EXISTS reconciliation_status;
DROP TYPE IF EXISTS transaction_type;
DROP TYPE IF EXISTS transaction_status;

DROP EXTENSION IF EXISTS "pg_trgm";
DROP EXTENSION IF EXISTS "pgcrypto";