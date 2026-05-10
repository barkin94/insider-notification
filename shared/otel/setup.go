package otel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown cleanly flushes and stops the OTel SDK.
type Shutdown func(context.Context) error

// Init initialises the global TracerProvider (OTLP gRPC → collector) and
// MeterProvider (Prometheus exporter → default registry). Call the returned
// Shutdown on service exit.
func Init(ctx context.Context, serviceName, collectorEndpoint string) (Shutdown, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	// --- traces: OTLP gRPC to OTel Collector ---
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(collectorEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// --- metrics: Prometheus exporter registers with default prometheus registry ---
	metricExp, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricExp),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Debug("otel", "error", err)
	}))

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}, nil
}

// InitLogger configures the global slog logger with JSON output, the given
// log level, and trace_id/span_id injection from active OTel spans.
func InitLogger(level string) {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(NewTraceHandler(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}),
	)))
}
