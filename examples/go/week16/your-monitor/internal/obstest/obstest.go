// Package obstest records the spans a test makes, in memory, where the
// test can read them.
package obstest

// region: record

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// RecordSpans makes the global tracer provider keep every span that ends
// in an in-memory exporter, and returns that exporter. When the test ends,
// tracing goes back to a provider that records nothing. Tests that call
// RecordSpans must not run in parallel: the provider is global.
func RecordSpans(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	// WithSyncer exports each span the moment it ends, with no batching.
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })
	return exporter
}

// endregion: record

// Find returns the first span named name, and fails the test when there
// is none.
func Find(t *testing.T, spans tracetest.SpanStubs, name string) tracetest.SpanStub {
	t.Helper()
	for _, s := range spans {
		if s.Name == name {
			return s
		}
	}
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	t.Fatalf("no span named %q; spans: %q", name, names)
	return tracetest.SpanStub{}
}

// Attrs returns the attributes of s as a map, for one-line checks such as
// Attrs(s)["http.route"].AsString().
func Attrs(s tracetest.SpanStub) map[attribute.Key]attribute.Value {
	m := make(map[attribute.Key]attribute.Value, len(s.Attributes))
	for _, kv := range s.Attributes {
		m[kv.Key] = kv.Value
	}
	return m
}
