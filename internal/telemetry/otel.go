// Package telemetry provides OpenTelemetry (OTel) tracing setup for the
// lignin service
//
// It is responsible for initializing a global tracer provider, configuring
// exporters, and ensuring consistent propagation of trace context across services.
// The package supports two modes: Enable mode - exports traces via OTLP HTTP exporter
// and Disabled mode - installs a no-op tracer
//
// Callers must always invoke the returned ShutdownFunc on graceful shutdown
// to ensure all buffered traces are flushed.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// OTelConfig controls tracing initialization and export behavior.
type OTelConfig struct {
	Endpoint    string
	ServiceName string
	Enabled     bool
}

// ShutdownFunc defines a cleanup function that flushes and shuts down the
// tracer provider.
type ShutdownFunc func(context.Context) error

// InitTracer initialiazes the global OpenTelemetry tracer provider
//
// If telemetry is disabled, a no-op provider is installed so instrumentation
// remains safe without side effects.
//
// On success, it returns: a service tracer instance, a shutdown function, nil err
func InitTracer(ctx context.Context, cfg OTelConfig) (trace.Tracer, ShutdownFunc, error) {
	if !cfg.Enabled || cfg.Endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return noop.NewTracerProvider().Tracer(cfg.ServiceName), noopShutdown, nil
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)

	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.Endpoint),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithTimeout(5*time.Second),
	)

	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: create OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)

	// Configure propagation for distributed tracing across services
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	shutdown := func(ctx context.Context) error {
		return tp.Shutdown(ctx)
	}

	return tp.Tracer(cfg.ServiceName), shutdown, nil

}

// noopShutdown is used when telemetry is disabled
func noopShutdown(_ context.Context) error {
	return nil
}
