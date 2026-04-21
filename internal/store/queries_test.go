package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/codercollo/lignin/internal/errors"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	lignin "github.com/codercollo/lignin"
	"github.com/codercollo/lignin/internal/store"
	"github.com/codercollo/lignin/internal/telemetry"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// ── Test harness ───────────────────────────────────────────────────────────────

func setupStore(t *testing.T) *store.Store {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	pgContainer, err := tcpostgres.RunContainer(ctx,
		postgres.WithDatabase("lignin_test"),
		postgres.WithUsername("lignin"),
		postgres.WithPassword("lignin"),
		postgres.WithInitScripts(), // migrations applied by MigrateUp below
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, store.MigrateUp(ctx, dsn, telemetry.NewLogger(0, true)))

	reg := promRegistry(t)
	metrics := telemetry.NewMetrics(reg)

	s, err := store.New(ctx, lignin.DatabaseConfig{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: 30 * time.Second,
	}, metrics)
	require.NoError(t, err)
	t.Cleanup(s.Close)

	return s
}

// promRegistry creates an isolated Prometheus registry per test so counters
// don't bleed between parallel runs.
func promRegistry(t *testing.T) prometheus.Registerer {
	t.Helper()
	return prometheus.NewRegistry()
}

func rawPayload(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func newTxParams(receipt string) store.CreateTransactionParams {
	return store.CreateTransactionParams{
		MpesaReceiptNumber: receipt,
		TransactionType:    store.TypeC2B,
		AmountCents:        10000, // KES 100.00
		Currency:           "KES",
		MSISDN:             "+254700000001",
		BusinessShortCode:  "174379",
		TransactedAt:       time.Now().UTC().Truncate(time.Millisecond),
		RawPayload:         rawPayload(nil, map[string]string{"receipt": receipt}),
	}
}

// ── CreateTransaction ─────────────────────────────────────────────────────────

func TestCreateTransaction_Success(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	tx, err := s.CreateTransaction(ctx, newTxParams("OEI2AK12345"))
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, tx.ID)
	assert.Equal(t, "OEI2AK12345", tx.MpesaReceiptNumber)
	assert.Equal(t, store.StatusPending, tx.Status)
	assert.Equal(t, int64(10000), tx.AmountCents)
}

func TestCreateTransaction_DuplicateReceipt(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	_, err := s.CreateTransaction(ctx, newTxParams("DUPE_RECEIPT"))
	require.NoError(t, err)

	_, err = s.CreateTransaction(ctx, newTxParams("DUPE_RECEIPT"))
	require.Error(t, err)

	var de *errors.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, errors.CodeDuplicate, de.Code)
}

// ── GetTransaction ────────────────────────────────────────────────────────────

func TestGetTransactionByID_NotFound(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	_, err := s.GetTransactionByID(ctx, uuid.New())
	require.Error(t, err)

	var de *errors.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, errors.CodeNotFound, de.Code)
}

func TestGetTransactionByID_Found(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	created, err := s.CreateTransaction(ctx, newTxParams("GET_BY_ID_01"))
	require.NoError(t, err)

	got, err := s.GetTransactionByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "GET_BY_ID_01", got.MpesaReceiptNumber)
}

func TestGetTransactionByReceipt(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	_, err := s.CreateTransaction(ctx, newTxParams("GET_BY_REC_01"))
	require.NoError(t, err)

	got, err := s.GetTransactionByReceipt(ctx, "GET_BY_REC_01")
	require.NoError(t, err)
	assert.Equal(t, "GET_BY_REC_01", got.MpesaReceiptNumber)
}

// ── ListTransactions ──────────────────────────────────────────────────────────

func TestListTransactions_Pagination(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	// Insert 5 transactions.
	for i := 0; i < 5; i++ {
		_, err := s.CreateTransaction(ctx, newTxParams(
			fmt.Sprintf("LIST_PAG_%02d", i),
		))
		require.NoError(t, err)
	}

	// First page.
	page1, err := s.ListTransactions(ctx, store.ListTransactionsParams{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, page1, 3)

	// Second page via cursor.
	lastID := page1[len(page1)-1].ID
	page2, err := s.ListTransactions(ctx, store.ListTransactionsParams{
		Limit: 3,
		After: &lastID,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(page2), 2)

	// No overlap between pages.
	page1IDs := make(map[uuid.UUID]bool, len(page1))
	for _, tx := range page1 {
		page1IDs[tx.ID] = true
	}
	for _, tx := range page2 {
		assert.False(t, page1IDs[tx.ID], "ID %s appears in both pages", tx.ID)
	}
}

func TestListTransactions_FilterByStatus(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	created, err := s.CreateTransaction(ctx, newTxParams("FILT_STATUS_01"))
	require.NoError(t, err)

	// Promote to completed.
	err = s.UpdateTransactionStatus(ctx, store.UpdateTransactionStatusParams{
		ID:     created.ID,
		Status: store.StatusCompleted,
	})
	require.NoError(t, err)

	status := store.StatusCompleted
	results, err := s.ListTransactions(ctx, store.ListTransactionsParams{
		Status: &status,
		Limit:  25,
	})
	require.NoError(t, err)

	for _, tx := range results {
		assert.Equal(t, store.StatusCompleted, tx.Status)
	}
}

// ── UpdateTransactionStatus ───────────────────────────────────────────────────

func TestUpdateTransactionStatus(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	created, err := s.CreateTransaction(ctx, newTxParams("UPDATE_STAT_01"))
	require.NoError(t, err)
	assert.Equal(t, store.StatusPending, created.Status)

	err = s.UpdateTransactionStatus(ctx, store.UpdateTransactionStatusParams{
		ID:     created.ID,
		Status: store.StatusCompleted,
	})
	require.NoError(t, err)

	updated, err := s.GetTransactionByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, store.StatusCompleted, updated.Status)
}

func TestUpdateTransactionStatus_NotFound(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	err := s.UpdateTransactionStatus(ctx, store.UpdateTransactionStatusParams{
		ID:     uuid.New(),
		Status: store.StatusFailed,
	})
	require.Error(t, err)

	var de *errors.Error
	require.True(t, errors.As(err, &de))
	assert.Equal(t, errors.CodeNotFound, de.Code)
}

// ── Idempotency ───────────────────────────────────────────────────────────────

func TestAcquireIdempotencyKey(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	key := "sha256:aabbccddeeff0011"

	// First acquisition succeeds.
	acquired, err := s.AcquireIdempotencyKey(ctx, key)
	require.NoError(t, err)
	assert.True(t, acquired)

	// Second acquisition for same key returns false (duplicate).
	acquired, err = s.AcquireIdempotencyKey(ctx, key)
	require.NoError(t, err)
	assert.False(t, acquired)
}

func TestPurgeExpiredIdempotencyKeys(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	_, err := s.AcquireIdempotencyKey(ctx, "sha256:old_key")
	require.NoError(t, err)

	// Purge everything (cutoff = far future).
	n, err := s.PurgeExpiredIdempotencyKeys(ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1))
}

// ── ReconciliationRuns ────────────────────────────────────────────────────────

func TestReconciliationRunLifecycle(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	run, err := s.CreateReconciliationRun(ctx, store.CreateReconciliationRunParams{
		PeriodStart: now.Add(-24 * time.Hour),
		PeriodEnd:   now,
		Format:      "csv",
	})
	require.NoError(t, err)
	assert.Equal(t, store.ReconciliationPending, run.Status)

	require.NoError(t, s.MarkReconciliationRunning(ctx, run.ID))

	require.NoError(t, s.CompleteReconciliationRun(ctx, store.CompleteReconciliationRunParams{
		ID:         run.ID,
		RowCount:   42,
		FilePath:   "exports/2024_01.csv",
		FileSHA256: "deadbeef",
	}))
}

// ── RetryQueue ────────────────────────────────────────────────────────────────

func TestRetryQueueRoundtrip(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	tx, err := s.CreateTransaction(ctx, newTxParams("RETRY_RT_01"))
	require.NoError(t, err)

	require.NoError(t, s.EnqueueRetry(ctx, tx.ID, time.Now().Add(-time.Second)))

	entries, err := s.DequeueDueRetries(ctx, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, tx.ID, entries[0].TransactionID)

	require.NoError(t, s.IncrementRetryAttempt(ctx, entries[0].ID,
		time.Now().Add(30*time.Second), "connection refused"))

	require.NoError(t, s.DeleteRetry(ctx, entries[0].ID))

	after, err := s.DequeueDueRetries(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, after)
}
