package obs

// region: sql

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// QueryTracer makes one span for each SQL statement that runs inside a
// trace. Set it on a pool with cfg.ConnConfig.Tracer = obs.QueryTracer{}.
// A statement outside any trace, such as a worker asking an empty queue
// for work every second, gets no span.
type QueryTracer struct{}

func (QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return ctx // not inside a trace
	}
	ctx, span := otel.Tracer("your-monitor/sql").Start(ctx, operation(data.SQL),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.query.text", data.SQL),
		))
	_ = span // TraceQueryEnd finds it in ctx
	return ctx
}

func (QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	// ctx is the context TraceQueryStart returned. Outside a trace, it holds
	// no span, and End does nothing.
	span := trace.SpanFromContext(ctx)
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	}
	span.End()
}

// operation names a span after the first word of the statement, in
// capitals: SELECT, INSERT, BEGIN, COMMIT.
func operation(sql string) string {
	words := strings.Fields(sql)
	if len(words) == 0 {
		return "SQL"
	}
	return strings.ToUpper(words[0])
}

// endregion: sql
