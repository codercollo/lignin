// Package store implements all PostgreSQL peristence for lignin using pgx.v5
//
// The Store type is intended to be created once ar process startup and shared
// across subsystems via dependency injection.
package store

import (
	"context"
	"fmt"

	lignin "github.com/codercollo/lignin"
	"github.com/codercollo/lignin/internal/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the single access point for all database operations.
//
// It owns the pgx connection pool and Peometheus instrumentation. The pool is
// not exposed directly; higher layers interact only through store methods or explicit
// transaction helpers
type Store struct {
	pool    *pgxpool.Pool
	metrics *telemetry.Metrics
}

// New creates a Store, configures the pgx pool, attaches query tracing for
// metrics collection, and verifies connectivity with a Ping
func New(ctx context.Context, cfg lignin.DatabaseConfig, metrics *telemetry.Metrics) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("store: parse dsn: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime
	poolCfg.MaxConnIdleTime = cfg.ConnMaxIdleTime

	// Attach a query tracer so per-query metrics can be recorded without
	// requiring callers to manually instrument every query site.
	poolCfg.ConnConfig.Tracer = &queryTracer{
		metrics: metrics,
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("store: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}

	return &Store{
		pool:    pool,
		metrics: metrics,
	}, nil
}

// Close releases all pool connections.
func (s *Store) Close() {
	s.pool.Close()
}

// PoolStats reads the currect pgx pool statistics and updates the Prometheus
// gauge. It could be periodically from a background goroutine so operational dashboards
// reflect live connection pool health
func (s *Store) PoolStats() {
	stat := s.pool.Stat()
	s.metrics.DBPoolStats.WithLabelValues("total_conns").Set(float64(stat.TotalConns()))
	s.metrics.DBPoolStats.WithLabelValues("idle_conns").Set(float64(stat.IdleConns()))
	s.metrics.DBPoolStats.WithLabelValues("acquired_conns").Set(float64(stat.AcquiredConns()))
}

// queryTracer implements pgx.QueryTracer and provides a hook point wher
// query execution can be correlated with Prometheis histograms.
//
// The tracer itself does not measure duration; instead, it propagates the
// query text through the context so higher-level helpers can record the elapsed
// time against a named query metric.
type queryTracer struct {
	metrics *telemetry.Metrics
}

// traceKey is an unexported context key used to store the SQL string for the currenty
// executing query
type traceKey struct{}

// TraceQueryStart stores the SQL string in the context so it can be used by
// instrumentation helpers when measuring query duration
func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, traceKey{}, data.SQL)
}

// TraceQueryEnd is required to satisfy the pgx.QueryTracer interface.
func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	_ = ctx
	_ = data
}
