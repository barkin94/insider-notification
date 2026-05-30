package otel

import (
	"context"
	"log/slog"
	"os"

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

	sharedlogger "github.com/barkin/insider-notification/shared/logger"
)

// Shutdown cleanly flushes and stops the OTel SDK.
type Shutdown func(context.Context) error

// Init initialises the global TracerProvider, MeterProvider, and LoggerProvider,
// all pushing via OTLP gRPC to the OTel Collector. Call the returned Shutdown on exit.
// Exits the process on any initialisation error.
func Init(ctx context.Context, serviceName, collectorEndpoint, logLevel string) Shutdown {
	exitIfErr := func(err error, msg string) {
		if err != nil {
			slog.Error(msg, "error", err)
			os.Exit(1)
		}
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)),
	)
	exitIfErr(err, "otel resource")

	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(collectorEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	exitIfErr(err, "otlp trace exporter")
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
	exitIfErr(err, "otlp metric exporter")
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	logExp, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(collectorEndpoint),
		otlploggrpc.WithInsecure(),
	)
	exitIfErr(err, "otlp log exporter")
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(lp)

	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Error("otel", "error", err)
	}))

	slog.SetDefault(slog.New(&levelHandler{
		min:  sharedlogger.ParseLevel(logLevel),
		next: otelslog.NewHandler("app"),
	}))

	return func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		if err := mp.Shutdown(ctx); err != nil {
			return err
		}
		return lp.Shutdown(ctx)
	}
}

// levelHandler enforces a minimum slog level; otelslog's Enabled() delegates to the
// OTel backend and does not apply a client-side threshold.
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
