// Package obs holds what every program of the monitor shares for
// observability: the JSON logger, the tracer setup, and the span for each
// SQL statement.
package obs

// region: logger

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger returns a logger that writes one JSON object per line to w.
// Every line carries service, and a line logged with a context that holds
// a span also carries the trace_id and span_id of that span.
func NewLogger(w io.Writer, service string) *slog.Logger {
	h := slog.NewJSONHandler(w, nil) // nil options: level INFO and up
	return slog.New(traceHandler{h}).With("service", service)
}

// traceHandler wraps another slog.Handler and adds the IDs of the span in
// the context to each record before it passes the record on.
type traceHandler struct{ slog.Handler }

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

// With and WithGroup return a new handler: wrap it again, or the IDs get lost.
func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

// endregion: logger

// Fatal logs err at level ERROR with the default logger and exits with
// status 1, as log.Fatal does for the log package. Deferred calls do not
// run.
func Fatal(err error) {
	slog.Error("fatal", "err", err)
	os.Exit(1)
}
