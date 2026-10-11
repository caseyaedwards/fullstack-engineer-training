package api

// region: trace

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Trace wraps next in one server span per request. A request with a
// traceparent header joins the trace that header names; any other request
// starts a new trace. Once next has answered, the span takes the name of
// the route, such as GET /api/monitors/{id}, and records the status code.
func Trace(next http.Handler) http.Handler {
	tracer := otel.Tracer("your-monitor/api")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		r = r.WithContext(ctx) // handlers further in see the span through r.Context()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		if r.Pattern != "" {
			span.SetName(r.Pattern)
		}
		span.SetAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", route(r)),
			attribute.Int("http.response.status_code", rec.status),
		)
		if rec.status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		}
	})
}

// endregion: trace
