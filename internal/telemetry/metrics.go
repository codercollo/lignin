// Package telemetry defines Prometheus metrics used across all lignin
// subsystems
//
// A single Metrics instances is intended to be created at process startup and
// shared via dependency injection avoiding global state while still ensuring
// all subsystems report into a unified metrics registry
package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics groups all Prometheus instruments for the app
type Metrics struct {
	// HTTP
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec

	// Callbacks
	CallbacksTotal      *prometheus.CounterVec
	CallbackProcessTime *prometheus.HistogramVec

	// Authentication
	TokenRefreshesTotal *prometheus.CounterVec
	TokenCacheHits      *prometheus.CounterVec

	// Reconciliation
	ReconciliationRunsTotal    *prometheus.CounterVec
	ReconciliationDuration     *prometheus.HistogramVec
	ReconciliationRowsExported prometheus.Gauge

	// Cron
	CronJobsTotal   *prometheus.CounterVec
	CronJobDuration *prometheus.HistogramVec

	// Database
	DBQueryDuration *prometheus.HistogramVec
	DBPoolStats     *prometheus.GaugeVec

	// WebSocket
	WSConnections   prometheus.Gauge
	WSMessagesTotal *prometheus.CounterVec
}

// NewMetrics registers all Prometheus instruments and returns a Metrics bundle.
//
// Instruments are registered using promauto so they are immediately visible to the
// provided registry
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		HTTPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_http_requests_total",
			Help: "Total HTTP requests by method, path and status code.",
		}, []string{"method", "path", "status"}),

		HTTPRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lignin_http_request_duration_seconds",
			Help:    "HTTP request latency distribution.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),

		HTTPResponseSize: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lignin_http_response_size_bytes",
			Help:    "HTTP response body size distribution.",
			Buckets: []float64{256, 1024, 4096, 16384, 65536, 262144},
		}, []string{"method", "path"}),

		CallbacksTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_callbacks_total",
			Help: "Total M-Pesa callbacks by processing status.",
		}, []string{"status"}),

		CallbackProcessTime: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lignin_callback_process_duration_seconds",
			Help:    "Time to verify, deduplicate, and store a callback.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1},
		}, []string{"status"}),

		TokenRefreshesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_token_refreshes_total",
			Help: "Total OAuth2 token refresh attempts.",
		}, []string{"result"}),

		TokenCacheHits: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_token_cache_hits_total",
			Help: "Token cache lookups by cache layer hit.",
		}, []string{"layer"}),

		ReconciliationRunsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_reconciliation_runs_total",
			Help: "Total reconciliation batch runs.",
		}, []string{"status"}),

		ReconciliationDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lignin_reconciliation_duration_seconds",
			Help:    "Wall-clock time per reconciliation run.",
			Buckets: []float64{1, 5, 15, 30, 60, 120, 300},
		}, []string{"format"}),

		ReconciliationRowsExported: factory.NewGauge(prometheus.GaugeOpts{
			Name: "lignin_reconciliation_rows_exported",
			Help: "Number of rows in the last successful reconciliation export.",
		}),

		CronJobsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_cron_jobs_total",
			Help: "Total cron job executions by job name and status.",
		}, []string{"job", "status"}),

		CronJobDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lignin_cron_job_duration_seconds",
			Help:    "Cron job execution time.",
			Buckets: []float64{.1, .5, 1, 5, 15, 30, 60},
		}, []string{"job"}),

		DBQueryDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "lignin_db_query_duration_seconds",
			Help:    "Database query latency by query name.",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .5, 1},
		}, []string{"query"}),

		DBPoolStats: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "lignin_db_pool_stats",
			Help: "pgx connection pool statistics.",
		}, []string{"stat"}),

		WSConnections: factory.NewGauge(prometheus.GaugeOpts{
			Name: "lignin_ws_connections_active",
			Help: "Number of active WebSocket connections.",
		}),

		WSMessagesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "lignin_ws_messages_total",
			Help: "Total WebSocket messages by direction.",
		}, []string{"direction"}),
	}
}
