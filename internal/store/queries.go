// Package store implements the PostgreSQL persistence layer for the service.
//
// It owns all database access, translating between relational rows and domain
// models while enforcing cross-cutting guarantees:
//
//   - Idempotency for webhook/at-least-once deliveries
//   - Audit logging for state transitions
//   - Retry queue coordination for background workers
//   - Reconciliation run tracking for external reporting (e.g. iTax/KRA)
//   - Query-level metrics via pgx tracing and histograms
//
// The Store is safe for concurrent use and should be constructed once at
// application startup via New, then shared across handlers, schedulers,
// and background workers.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	domainerrs "github.com/codercollo/lignin/internal/errors"
)

// pgErrUniqueViolation is the Postgres error code for unique constraint violations.
const pgErrUniqueViolation = "23505"

// isPgError checks whether err is a Postgres error with the given code.
func isPgError(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// ── Transactions ──────────────────────────────────────────────────────────────

// CreateTransaction inserts a new transaction row and returns it.
// Returns [domainerrs.CodeDuplicate] if mpesa_receipt_number already exists.
func (s *Store) CreateTransaction(ctx context.Context, p CreateTransactionParams) (*Transaction, error) {
	const query = `
		INSERT INTO transactions (
			mpesa_receipt_number, transaction_type, amount_cents, currency,
			msisdn, business_short_code, account_reference, third_party_txn_id,
			transacted_at, raw_payload
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
		RETURNING
			id, mpesa_receipt_number, transaction_type, status,
			amount_cents, currency, msisdn, business_short_code,
			account_reference, third_party_txn_id,
			transacted_at, received_at, updated_at,
			raw_payload, reversed_by_id, reversal_reason`

	timer := s.startTimer("create_transaction")
	defer timer()

	rows, err := s.pool.Query(ctx, query,
		p.MpesaReceiptNumber, p.TransactionType, p.AmountCents, p.Currency,
		p.MSISDN, p.BusinessShortCode, p.AccountReference, p.ThirdPartyTxnID,
		p.TransactedAt, p.RawPayload,
	)
	if err != nil {
		if isPgError(err, pgErrUniqueViolation) {
			return nil, domainerrs.Duplicate(p.MpesaReceiptNumber)
		}
		return nil, fmt.Errorf("store: create transaction: %w", err)
	}

	tx, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Transaction])
	if err != nil {
		if isPgError(err, pgErrUniqueViolation) {
			return nil, domainerrs.Duplicate(p.MpesaReceiptNumber)
		}
		return nil, fmt.Errorf("store: scan transaction: %w", err)
	}
	return tx, nil
}

// GetTransactionByID fetches a single transaction by primary key.
// Returns [domainerrs.CodeNotFound] if the row does not exist.
func (s *Store) GetTransactionByID(ctx context.Context, id uuid.UUID) (*Transaction, error) {
	const query = `
		SELECT id, mpesa_receipt_number, transaction_type, status,
		       amount_cents, currency, msisdn, business_short_code,
		       account_reference, third_party_txn_id,
		       transacted_at, received_at, updated_at,
		       raw_payload, reversed_by_id, reversal_reason
		FROM transactions
		WHERE id = $1`

	timer := s.startTimer("get_transaction_by_id")
	defer timer()

	rows, err := s.pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("store: get transaction: %w", err)
	}

	tx, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Transaction])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domainerrs.NotFound("transaction")
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan transaction: %w", err)
	}
	return tx, nil
}

// GetTransactionByReceipt fetches by the Safaricom receipt number (natural key).
func (s *Store) GetTransactionByReceipt(ctx context.Context, receipt string) (*Transaction, error) {
	const query = `
		SELECT id, mpesa_receipt_number, transaction_type, status,
		       amount_cents, currency, msisdn, business_short_code,
		       account_reference, third_party_txn_id,
		       transacted_at, received_at, updated_at,
		       raw_payload, reversed_by_id, reversal_reason
		FROM transactions
		WHERE mpesa_receipt_number = $1`

	timer := s.startTimer("get_transaction_by_receipt")
	defer timer()

	rows, err := s.pool.Query(ctx, query, receipt)
	if err != nil {
		return nil, fmt.Errorf("store: get transaction by receipt: %w", err)
	}

	tx, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[Transaction])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domainerrs.NotFound("transaction")
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan transaction: %w", err)
	}
	return tx, nil
}

// ListTransactions returns a cursor-paginated slice of transactions.
// An empty slice (not nil) is returned when no rows match.
func (s *Store) ListTransactions(ctx context.Context, p ListTransactionsParams) ([]*Transaction, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 25
	}

	// Build a dynamic query; args are positional to prevent injection.
	args := []any{}
	argN := 1

	q := `
		SELECT id, mpesa_receipt_number, transaction_type, status,
		       amount_cents, currency, msisdn, business_short_code,
		       account_reference, third_party_txn_id,
		       transacted_at, received_at, updated_at,
		       raw_payload, reversed_by_id, reversal_reason
		FROM transactions
		WHERE TRUE`

	if p.Status != nil {
		q += fmt.Sprintf(" AND status = $%d", argN)
		args = append(args, *p.Status)
		argN++
	}
	if p.MSISDN != nil {
		q += fmt.Sprintf(" AND msisdn = $%d", argN)
		args = append(args, *p.MSISDN)
		argN++
	}
	if p.After != nil {
		// Keyset pagination: fetch rows whose (transacted_at, id) comes before
		// the cursor row (we sort DESC so "before" means earlier in time).
		q += fmt.Sprintf(`
			AND (transacted_at, id) < (
				SELECT transacted_at, id FROM transactions WHERE id = $%d
			)`, argN)
		args = append(args, *p.After)
		argN++
	}

	q += fmt.Sprintf(" ORDER BY transacted_at DESC, id DESC LIMIT $%d", argN)
	args = append(args, p.Limit)

	timer := s.startTimer("list_transactions")
	defer timer()

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list transactions: %w", err)
	}

	txns, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[Transaction])
	if err != nil {
		return nil, fmt.Errorf("store: scan transactions: %w", err)
	}
	if txns == nil {
		txns = []*Transaction{}
	}
	return txns, nil
}

// UpdateTransactionStatus changes a transaction's status and records the
// change in the audit log within a single transaction.
func (s *Store) UpdateTransactionStatus(ctx context.Context, p UpdateTransactionStatusParams) error {
	return s.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE transactions
			SET    status = $1
			WHERE  id     = $2
			RETURNING id`

		timer := s.startTimer("update_transaction_status")
		defer timer()

		var id uuid.UUID
		if err := tx.QueryRow(ctx, query, p.Status, p.ID).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domainerrs.NotFound("transaction")
			}
			return fmt.Errorf("store: update status: %w", err)
		}

		return s.appendAuditLog(ctx, tx, AuditEntry{
			EntityType: "transaction",
			EntityID:   p.ID,
			Action:     "status_changed",
			NewState:   []byte(fmt.Sprintf(`{"status":%q}`, p.Status)),
		})
	})
}

// ListTransactionsForReconciliation returns all completed transactions in a
// time range, ordered by transacted_at ascending (KRA iTax requirement).
func (s *Store) ListTransactionsForReconciliation(ctx context.Context, from, to time.Time) ([]*Transaction, error) {
	const query = `
		SELECT id, mpesa_receipt_number, transaction_type, status,
		       amount_cents, currency, msisdn, business_short_code,
		       account_reference, third_party_txn_id,
		       transacted_at, received_at, updated_at,
		       raw_payload, reversed_by_id, reversal_reason
		FROM transactions
		WHERE status = 'completed'
		  AND transacted_at >= $1
		  AND transacted_at <  $2
		ORDER BY transacted_at ASC, id ASC`

	timer := s.startTimer("list_transactions_for_reconciliation")
	defer timer()

	rows, err := s.pool.Query(ctx, query, from, to)
	if err != nil {
		return nil, fmt.Errorf("store: list for reconciliation: %w", err)
	}

	txns, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[Transaction])
	if err != nil {
		return nil, fmt.Errorf("store: scan reconciliation transactions: %w", err)
	}
	if txns == nil {
		txns = []*Transaction{}
	}
	return txns, nil
}

// ── Idempotency ───────────────────────────────────────────────────────────────

// AcquireIdempotencyKey attempts to insert a key.
// Returns (false, nil) if the key already exists (duplicate delivery).
// Returns (true, nil) on first insertion.
func (s *Store) AcquireIdempotencyKey(ctx context.Context, key string) (bool, error) {
	const query = `
		INSERT INTO idempotency_keys (key)
		VALUES ($1)
		ON CONFLICT (key) DO NOTHING
		RETURNING key`

	timer := s.startTimer("acquire_idempotency_key")
	defer timer()

	var got string
	err := s.pool.QueryRow(ctx, query, key).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // conflict: already exists
	}
	if err != nil {
		return false, fmt.Errorf("store: acquire idempotency key: %w", err)
	}
	return true, nil
}

// LinkIdempotencyKey sets the transaction_id back-reference after a
// successful transaction insert.
func (s *Store) LinkIdempotencyKey(ctx context.Context, key string, txID uuid.UUID) error {
	const query = `
		UPDATE idempotency_keys
		SET    transaction_id = $1
		WHERE  key            = $2`

	timer := s.startTimer("link_idempotency_key")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, txID, key); err != nil {
		return fmt.Errorf("store: link idempotency key: %w", err)
	}
	return nil
}

// PurgeExpiredIdempotencyKeys deletes keys older than the given cutoff.
// Called by the scheduler on a regular cadence.
func (s *Store) PurgeExpiredIdempotencyKeys(ctx context.Context, olderThan time.Time) (int64, error) {
	const query = `DELETE FROM idempotency_keys WHERE received_at < $1`

	timer := s.startTimer("purge_idempotency_keys")
	defer timer()

	tag, err := s.pool.Exec(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("store: purge idempotency keys: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ── ReconciliationRuns ────────────────────────────────────────────────────────

// CreateReconciliationRun inserts a new run in pending state.
func (s *Store) CreateReconciliationRun(ctx context.Context, p CreateReconciliationRunParams) (*ReconciliationRun, error) {
	const query = `
		INSERT INTO reconciliation_runs (period_start, period_end, format)
		VALUES ($1, $2, $3)
		RETURNING
			id, status, period_start, period_end,
			row_count, file_path, file_sha256, format,
			error_message, started_at, completed_at, created_at, updated_at`

	timer := s.startTimer("create_reconciliation_run")
	defer timer()

	rows, err := s.pool.Query(ctx, query, p.PeriodStart, p.PeriodEnd, p.Format)
	if err != nil {
		return nil, fmt.Errorf("store: create reconciliation run: %w", err)
	}
	run, err := pgx.CollectOneRow(rows, pgx.RowToAddrOfStructByName[ReconciliationRun])
	if err != nil {
		return nil, fmt.Errorf("store: scan reconciliation run: %w", err)
	}
	return run, nil
}

// MarkReconciliationRunning transitions a run to running status.
func (s *Store) MarkReconciliationRunning(ctx context.Context, id uuid.UUID) error {
	const query = `
		UPDATE reconciliation_runs
		SET status = 'running', started_at = NOW()
		WHERE id = $1`

	timer := s.startTimer("mark_reconciliation_running")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("store: mark reconciliation running: %w", err)
	}
	return nil
}

// CompleteReconciliationRun marks a run as completed with file metadata.
func (s *Store) CompleteReconciliationRun(ctx context.Context, p CompleteReconciliationRunParams) error {
	const query = `
		UPDATE reconciliation_runs
		SET status       = 'completed',
		    row_count    = $2,
		    file_path    = $3,
		    file_sha256  = $4,
		    completed_at = NOW()
		WHERE id = $1`

	timer := s.startTimer("complete_reconciliation_run")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, p.ID, p.RowCount, p.FilePath, p.FileSHA256); err != nil {
		return fmt.Errorf("store: complete reconciliation run: %w", err)
	}
	return nil
}

// FailReconciliationRun marks a run as failed with an error message.
func (s *Store) FailReconciliationRun(ctx context.Context, p FailReconciliationRunParams) error {
	const query = `
		UPDATE reconciliation_runs
		SET status        = 'failed',
		    error_message = $2,
		    completed_at  = NOW()
		WHERE id = $1`

	timer := s.startTimer("fail_reconciliation_run")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, p.ID, p.ErrorMessage); err != nil {
		return fmt.Errorf("store: fail reconciliation run: %w", err)
	}
	return nil
}

// ── RetryQueue ────────────────────────────────────────────────────────────────

// EnqueueRetry adds a transaction to the retry queue.
func (s *Store) EnqueueRetry(ctx context.Context, txID uuid.UUID, nextRetryAt time.Time) error {
	const query = `
		INSERT INTO retry_queue (transaction_id, next_retry_at)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`

	timer := s.startTimer("enqueue_retry")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, txID, nextRetryAt); err != nil {
		return fmt.Errorf("store: enqueue retry: %w", err)
	}
	return nil
}

// DequeueDueRetries fetches up to limit retry entries that are due now,
// locking them with SKIP LOCKED so concurrent scheduler workers don't race.
func (s *Store) DequeueDueRetries(ctx context.Context, limit int) ([]*RetryQueueEntry, error) {
	const query = `
		SELECT id, transaction_id, attempt, max_attempts, next_retry_at,
		       last_error, created_at, updated_at
		FROM retry_queue
		WHERE next_retry_at <= NOW()
		  AND attempt < max_attempts
		ORDER BY next_retry_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	timer := s.startTimer("dequeue_due_retries")
	defer timer()

	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("store: dequeue retries: %w", err)
	}

	entries, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[RetryQueueEntry])
	if err != nil {
		return nil, fmt.Errorf("store: scan retries: %w", err)
	}
	if entries == nil {
		entries = []*RetryQueueEntry{}
	}
	return entries, nil
}

// IncrementRetryAttempt advances the attempt counter and schedules the next
// retry using exponential backoff computed by the caller.
func (s *Store) IncrementRetryAttempt(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, lastError string) error {
	const query = `
		UPDATE retry_queue
		SET attempt      = attempt + 1,
		    next_retry_at = $2,
		    last_error   = $3
		WHERE id = $1`

	timer := s.startTimer("increment_retry_attempt")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, id, nextRetryAt, lastError); err != nil {
		return fmt.Errorf("store: increment retry attempt: %w", err)
	}
	return nil
}

// DeleteRetry removes a retry entry after a successful reprocessing.
func (s *Store) DeleteRetry(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM retry_queue WHERE id = $1`

	timer := s.startTimer("delete_retry")
	defer timer()

	if _, err := s.pool.Exec(ctx, query, id); err != nil {
		return fmt.Errorf("store: delete retry: %w", err)
	}
	return nil
}

// ── Audit log ─────────────────────────────────────────────────────────────────

// AuditEntry is the input to appendAuditLog.
type AuditEntry struct {
	EntityType string
	EntityID   uuid.UUID
	Action     string
	Actor      string
	OldState   []byte // JSONB
	NewState   []byte // JSONB
}

// appendAuditLog inserts a row into audit_log within the provided transaction.
// It is intentionally unexported; callers go through the higher-level methods
// (e.g. UpdateTransactionStatus) that own the transaction lifecycle.
func (s *Store) appendAuditLog(ctx context.Context, tx pgx.Tx, e AuditEntry) error {
	const query = `
		INSERT INTO audit_log (entity_type, entity_id, action, actor, old_state, new_state)
		VALUES ($1, $2, $3, $4, $5, $6)`

	if _, err := tx.Exec(ctx, query,
		e.EntityType, e.EntityID, e.Action,
		nullString(e.Actor), nullBytes(e.OldState), nullBytes(e.NewState),
	); err != nil {
		return fmt.Errorf("store: append audit log: %w", err)
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// startTimer returns a function that observes elapsed time in the query
// histogram when called (via defer).
func (s *Store) startTimer(queryName string) func() {
	start := time.Now()
	return func() {
		s.metrics.DBQueryDuration.
			WithLabelValues(queryName).
			Observe(time.Since(start).Seconds())
	}
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullBytes(b []byte) *[]byte {
	if len(b) == 0 {
		return nil
	}
	return &b
}
