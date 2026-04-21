package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TxFunc is a function that runs inside a database transaction.
// If it returns an error the transaction is rolled back; otherwise committed.
type TxFunc func(ctx context.Context, tx pgx.Tx) error

// WithTx executes fn inside a serializable transaction.
// The transaction is automatically committed on success and rolled back on
// any error returned from fn or from commit itself.
//
// Callers should not call tx.Commit or tx.Rollback — WithTx owns the lifecycle.
func (s *Store) WithTx(ctx context.Context, fn TxFunc) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted, // safe default; upgrade per-call if needed
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}

	// Ensure rollback is always attempted if the tx is still open.
	defer func() {
		// Rollback is a no-op after a successful commit, so it's always safe.
		_ = tx.Rollback(ctx)
	}()

	if err := fn(ctx, tx); err != nil {
		return err // rollback happens in defer
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit tx: %w", err)
	}
	return nil
}

// WithSerializableTx is like WithTx but uses Serializable isolation.
// Use this for operations that must not observe phantom reads, e.g. the
// idempotency-key + transaction double-write in the callback handler.
func (s *Store) WithSerializableTx(ctx context.Context, fn TxFunc) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return fmt.Errorf("store: begin serializable tx: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(ctx, tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: commit serializable tx: %w", err)
	}
	return nil
}
