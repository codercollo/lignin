// Package lignin is the root of the Mpesa gateway SDK.
//
// It bootstraps the foundational runtime concerns required by higher
// subsystems: configuration, structured logging, distributed tracing, and
// Prometheus metrics.
package lignin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/codercollo/lignin/internal/telemetry"
	"github.com/prometheus/client_golang/prometheus"
)

// App represents the assembled lignin foundation layer
//
// It owns shared process-wide resources. The application must be shutdown
// cleanly via [App.Shutdown] to flush telemetry buffers.
type App struct {
	Config       *Config
	Logger       *slog.Logger
	Metrics      *telemetry.Metrics
	otelShutdown telemetry.ShutdownFunc
}

// New initializes the foundation layer: loger, tracing and metrics
func New(ctx context.Context, cfg *Config) (*App, error) {

	// Logger
	level := telemetry.ParseLevel(cfg.App.LogLevel)
	logger := telemetry.NewLogger(level, cfg.IsDevelopment())
	slog.SetDefault(logger)

	logger.Info("lignin starting...",
		slog.String("env", cfg.App.Env),
		slog.String("service", cfg.App.ServiceName),
	)

	// Tracing [OpenTelemetry]
	_, otelShutdown, err := telemetry.InitTracer(ctx, telemetry.OTelConfig{
		Endpoint:    cfg.OTel.Endpoint,
		ServiceName: cfg.App.ServiceName,
		Enabled:     cfg.OTel.Enabled,
	})
	if err != nil {
		return nil, fmt.Errorf("lignin: init tracer: %w", err)
	}

	// Metrics [Prometheus]
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)

	return &App{
		Config:       cfg,
		Logger:       logger,
		Metrics:      metrics,
		otelShutdown: otelShutdown,
	}, nil
}

// Shutdown gracefully tears down shared resources.
func (a *App) Shutdown(ctx context.Context) error {
	a.Logger.Info("lignin shutting down...")

	if err := a.otelShutdown(ctx); err != nil {
		a.Logger.Error("otel shutdown error", slog.Any("error", err))
		return fmt.Errorf("lignin: otel shutdown: %w", err)
	}

	return nil
}
