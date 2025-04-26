package goai

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// StartSpan starts a new span with the given name and options.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return trace.SpanFromContext(ctx).TracerProvider().
		Tracer("github.com/shaharia-lab/goai").
		Start(ctx, name, opts...)
}
