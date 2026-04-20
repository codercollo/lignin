// Package telemetry provides observability primitives for the lignin service,
// including structured logging, trace correlation and integration hooks for
// distributed tracing systems
package telemetry

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// contextKey is an unexported type used to avoid context key collisions.
type contextKey int

const loggerKey contextKey = iota

// NewLogger constructs the root structured logger for the app
//
// In development mode, it uses a human-readable text handler for readability.
// In production mode, it emits structured JSON logs suitable for aggregation.
func NewLogger(level slog.Level, development bool) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: !development,
	}

	var handler slog.Handler
	if development {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// WithLogger injects a logger into the provided context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext retrieves a logger from the context
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// WithTraceID enriches a logger with OpenTelemetry trace metadata extracted
// from the current context span. It attaches: trace_id, span_id, allowing
// correlation between logs and distributed traces without requiring a dedicated
// OTel logging bridge
func WithTraceID(ctx context.Context, l *slog.Logger) *slog.Logger {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return l
	}

	sc := span.SpanContext()
	return l.With(
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	)
}

// ParseLevel converts a textual log level into a slog.Level
func ParseLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}
