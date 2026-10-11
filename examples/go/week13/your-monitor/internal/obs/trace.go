package obs

// region: tracing

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// StartTracing sets up tracing for the program named service. When
// OTEL_EXPORTER_OTLP_ENDPOINT is empty, tracing stays off: spans cost
// next to nothing and go nowhere. Otherwise, spans go in batches to that
// endpoint over OTLP/HTTP. Call the returned function before the program
// exits: it sends the spans still waiting in the batch.
func StartTracing(ctx context.Context, service string) (shutdown func(context.Context) error, err error) {
	// Read and write the W3C traceparent header, which carries a trace
	// from one program to the next.
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx) // reads OTEL_EXPORTER_OTLP_ENDPOINT
	if err != nil {
		return nil, err
	}
	// The resource describes the program on every span: service.name, the
	// SDK version, and OTEL_SERVICE_NAME or OTEL_RESOURCE_ATTRIBUTES, which
	// come last and win.
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", service)),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, err
	}
	// Without WithSampler, the SDK reads OTEL_TRACES_SAMPLER and
	// OTEL_TRACES_SAMPLER_ARG, and defaults to parentbased_always_on.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	// Spans that fail to send, for example while the collector is down,
	// become WARN lines in the program log.
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("tracing", "err", err)
	}))
	return tp.Shutdown, nil
}

// endregion: tracing
