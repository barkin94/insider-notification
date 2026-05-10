package otel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown cleanly flushes and stops the OTel SDK.
type Shutdown func(context.Context) error

// Init initialises the global TracerProvider, MeterProvider, and LoggerProvider,
// all pushing via OTLP gRPC to the OTel Collector. Call the returned Shutdown on exit.
func Init(ctx context.Context, serviceName, collectorEndpoint string) (Shutdown, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

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

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(collectorEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logExp, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(collectorEndpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("otlp log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Debug("otel", "error", err)
	}))

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		if err := mp.Shutdown(ctx); err != nil {
			return err
		}
		return lp.Shutdown(ctx)
	}, nil
}

// InitLogger configures the global slog logger to export via OTel log bridge.
// Call after Init so the global LoggerProvider is set.
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
	// otelslog.NewHandler uses global.LoggerProvider() which was set by Init.
	// The OTel log bridge automatically injects trace_id/span_id from active spans.
	slog.SetDefault(slog.New(&levelHandler{
		min:  l,
		next: otelslog.NewHandler("app"),
	}))
}

// levelHandler enforces a minimum slog level; otelslog's Enabled() passes everything through.
type levelHandler struct {
	min  slog.Level
	next slog.Handler
}

func (h *levelHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }
func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, r)
}
func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{h.min, h.next.WithAttrs(attrs)}
}
func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{h.min, h.next.WithGroup(name)}
}
