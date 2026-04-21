// Package store defines the Go representations of all Postgres rows and
// parameter objects used by the lignin persistence layer
package store

import (
	"time"

	"github.com/google/uuid"
)

// TransactionStatus mirrors the "transaction_status" PostgreSQL enum
type TransactionStatus string

const (
	StatusPending   TransactionStatus = "pending"
	StatusCompleted TransactionStatus = "completed"
	StatusFailed    TransactionStatus = "failed"
	StatusReversed  TransactionStatus = "reversed"
	StatusCancelled TransactionStatus = "cancelled"
)

// TransactionType mirrors the `transaction_type` PostgreSQL enum.
type TransactionType string

const (
	TypeC2B      TransactionType = "c2b"
	TypeB2C      TransactionType = "b2c"
	TypeB2B      TransactionType = "b2b"
	TypeReversal TransactionType = "reversal"
)

// Transaction represents a single row in the "transactions" table
type Transaction struct {
	ID                 uuid.UUID         `db:"id"`
	MpesaReceiptNumber string            `db:"mpesa_receipt_number"`
	TransactionType    TransactionType   `db:"transaction_type"`
	Status             TransactionStatus `db:"status"`
	AmountCents        int64             `db:"amount_cents"`
	Currency           string            `db:"currency"`
	MSISDN             string            `db:"msisdn"`
	BusinessShortCode  string            `db:"business_short_code"`
	AccountReference   *string           `db:"account_reference"`
	ThirdPartyTxnID    *string           `db:"third_party_txn_id"`
	TransactedAt       time.Time         `db:"transacted_at"`
	ReceivedAt         time.Time         `db:"received_at"`
	UpdatedAt          time.Time         `db:"updated_at"`
	RawPayload         []byte            `db:"raw_payload"` // JSONB
	ReversedByID       *uuid.UUID        `db:"reversed_by_id"`
	ReversalReason     *string           `db:"reversal_reason"`
}

// CreateTransactionParams contains the required fields to insert a new
// transaction row.
type CreateTransactionParams struct {
	MpesaReceiptNumber string
	TransactionType    TransactionType
	AmountCents        int64
	Currency           string
	MSISDN             string
	BusinessShortCode  string
	AccountReference   *string
	ThirdPartyTxnID    *string
	TransactedAt       time.Time
	RawPayload         []byte
}

// UpdateTransactionStatusParams is used by background workers and callback
// handlers to change the processing status of a transaction.
type UpdateTransactionStatusParams struct {
	ID     uuid.UUID
	Status TransactionStatus
}

// ListTransactionsParams provides cursor-based pagination and filtering.
type ListTransactionsParams struct {
	Status *TransactionStatus
	MSISDN *string
	After  *uuid.UUID
	Limit  int
}

// ReconciliationStatus mirrors the `reconciliation_status` PostgreSQL enum.
type ReconciliationStatus string

const (
	ReconciliationPending   ReconciliationStatus = "pending"
	ReconciliationRunning   ReconciliationStatus = "running"
	ReconciliationCompleted ReconciliationStatus = "completed"
	ReconciliationFailed    ReconciliationStatus = "failed"
)

// ReconciliationRun represents a row in the 'reconciliation_runs' table
type ReconciliationRun struct {
	ID           uuid.UUID            `db:"id"`
	Status       ReconciliationStatus `db:"status"`
	PeriodStart  time.Time            `db:"period_start"`
	PeriodEnd    time.Time            `db:"period_end"`
	RowCount     *int                 `db:"row_count"`
	FilePath     *string              `db:"file_path"`
	FileSHA256   *string              `db:"file_sha256"`
	Format       string               `db:"format"`
	ErrorMessage *string              `db:"error_message"`
	StartedAt    *time.Time           `db:"started_at"`
	CompletedAt  *time.Time           `db:"completed_at"`
	CreatedAt    time.Time            `db:"created_at"`
	UpdatedAt    time.Time            `db:"updated_at"`
}

// CreateReconciliationRunParams contains the fields required to create a new
// reconciliation batch record before processing begins.
type CreateReconciliationRunParams struct {
	PeriodStart time.Time
	PeriodEnd   time.Time
	Format      string
}

// CompleteReconciliationRunParams is used when a reconciliation run
// finishes successfully and the export file has been generated.
type CompleteReconciliationRunParams struct {
	ID         uuid.UUID
	RowCount   int
	FilePath   string
	FileSHA256 string
}

// FailReconciliationRunParams is used to mark a reconciliation run as failed
// and store the error details.
type FailReconciliationRunParams struct {
	ID           uuid.UUID
	ErrorMessage string
}

// RetryQueueEntry represents a row in the `retry_queue` table used by
// background workers to retry failed callback processing.
type RetryQueueEntry struct {
	ID            uuid.UUID `db:"id"`
	TransactionID uuid.UUID `db:"transaction_id"`
	Attempt       int       `db:"attempt"`
	MaxAttempts   int       `db:"max_attempts"`
	NextRetryAt   time.Time `db:"next_retry_at"`
	LastError     *string   `db:"last_error"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// IdempotencyKey represents a row in the `idempotency_keys` table used to
// detect duplicate callback deliveries at the network boundary.
type IdempotencyKey struct {
	Key           string     `db:"key"`
	ReceivedAt    time.Time  `db:"received_at"`
	TransactionID *uuid.UUID `db:"transaction_id"`
}
