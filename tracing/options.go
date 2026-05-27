package tracing

import (
	"context"

	"github.com/Slzdude/claude-agent-sdk-go/tracing/semconv"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Unexported key types ensure these values can only be set via the
// With* helpers in this package — no risk of accidental key collisions,
// and (unlike OTel baggage) they cannot escape the process via a propagator.
type (
	suppressKey struct{}
	sessionKey  struct{}
	userKey     struct{}
	metadataKey struct{}
	tagsKey     struct{}
)

// WithSuppression returns a context that suppresses tracing instrumentation.
// Matches Python's context_api._SUPPRESS_INSTRUMENTATION_KEY.
func WithSuppression(ctx context.Context) context.Context {
	return context.WithValue(ctx, suppressKey{}, true)
}

// IsSuppressed reports whether ctx was marked by WithSuppression.
func IsSuppressed(ctx context.Context) bool {
	v, _ := ctx.Value(suppressKey{}).(bool)
	return v
}

// WithSession returns a context carrying sessionID.
// Applied as the OpenInference session.id attribute on every span.
// Returns ctx unchanged if sessionID is empty.
func WithSession(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, sessionID)
}

// WithUser returns a context carrying userID.
// Applied as the OpenInference user.id attribute on every span.
// Returns ctx unchanged if userID is empty.
func WithUser(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, userKey{}, userID)
}

// WithMetadata returns a context carrying free-form metadata as a JSON string.
// Applied as the OpenInference metadata attribute on every span.
// Caller is responsible for JSON-encoding the map.
// Returns ctx unchanged if metadataJSON is empty.
func WithMetadata(ctx context.Context, metadataJSON string) context.Context {
	if metadataJSON == "" {
		return ctx
	}
	return context.WithValue(ctx, metadataKey{}, metadataJSON)
}

// WithTags returns a context carrying categorical tags.
// Applied as the OpenInference tag.tags attribute (string slice).
// Returns ctx unchanged if no tags are provided.
func WithTags(ctx context.Context, tags ...string) context.Context {
	if len(tags) == 0 {
		return ctx
	}
	// Defensive copy so caller mutation doesn't affect the span.
	copied := make([]string, len(tags))
	copy(copied, tags)
	return context.WithValue(ctx, tagsKey{}, copied)
}

// ApplyContextAttributes copies any OpenInference context attributes from
// ctx onto span. Call this once per span (right after tracer.Start) so
// customer-set session.id / user.id / metadata / tags appear on every span.
func ApplyContextAttributes(ctx context.Context, span trace.Span) {
	if v, ok := ctx.Value(sessionKey{}).(string); ok && v != "" {
		span.SetAttributes(semconv.SessionID.String(v))
	}
	if v, ok := ctx.Value(userKey{}).(string); ok && v != "" {
		span.SetAttributes(semconv.UserID.String(v))
	}
	if v, ok := ctx.Value(metadataKey{}).(string); ok && v != "" {
		span.SetAttributes(semconv.Metadata.String(v))
	}
	if v, ok := ctx.Value(tagsKey{}).([]string); ok && len(v) > 0 {
		span.SetAttributes(attribute.StringSlice(string(semconv.TagTags), v))
	}
}

// AttributeFilter is a function that can redact or modify span attributes.
// Return true to keep the attribute, false to drop it.
type AttributeFilter func(kv attribute.KeyValue) bool

// TraceConfig controls instrumentation behavior.
type TraceConfig struct {
	// TracerProvider to use. If nil, uses the global provider.
	TracerProvider *sdktrace.TracerProvider
	// Tracer to use. If nil, creates one from TracerProvider.
	Tracer trace.Tracer
	// SpanNamer overrides the default span name for Query.
	// Default: "ClaudeAgentSDK.Query"
	SpanNamer func(prompt string) string
	// AttributeFilter is called before each attribute is set on a span.
	// Use this for PII redaction. If nil, all attributes are kept.
	AttributeFilter AttributeFilter
}

// TraceOption configures TraceConfig.
type TraceOption func(*TraceConfig)

// WithTracerProvider sets the TracerProvider.
func WithTracerProvider(tp *sdktrace.TracerProvider) TraceOption {
	return func(c *TraceConfig) {
		c.TracerProvider = tp
	}
}

// WithTracer sets a specific tracer.
func WithTracer(tracer trace.Tracer) TraceOption {
	return func(c *TraceConfig) {
		c.Tracer = tracer
	}
}

// WithSpanNamer sets a custom span namer.
func WithSpanNamer(namer func(prompt string) string) TraceOption {
	return func(c *TraceConfig) {
		c.SpanNamer = namer
	}
}

// WithAttributeFilter sets a filter for PII redaction on span attributes.
func WithAttributeFilter(f AttributeFilter) TraceOption {
	return func(c *TraceConfig) {
		c.AttributeFilter = f
	}
}

func (c *TraceConfig) resolveTracer() trace.Tracer {
	if c.Tracer != nil {
		return c.Tracer
	}
	name := "claude-agent-sdk-go"
	if c.TracerProvider != nil {
		return c.TracerProvider.Tracer(name)
	}
	// Use a no-op tracer when no provider is configured.
	return trace.NewNoopTracerProvider().Tracer(name) //nolint:staticcheck
}

// filteredSpan wraps a trace.Span and applies the AttributeFilter before setting attributes.
// Embeds the inner span so it satisfies the full trace.Span interface.
type filteredSpan struct {
	trace.Span
	filter AttributeFilter
}

func (s *filteredSpan) SetAttributes(kv ...attribute.KeyValue) {
	if s.filter == nil {
		s.Span.SetAttributes(kv...)
		return
	}
	filtered := make([]attribute.KeyValue, 0, len(kv))
	for _, attr := range kv {
		if s.filter(attr) {
			filtered = append(filtered, attr)
		}
	}
	if len(filtered) > 0 {
		s.Span.SetAttributes(filtered...)
	}
}

// wrapSpan wraps a span with the filter if one is configured.
func wrapSpan(span trace.Span, cfg *TraceConfig) trace.Span {
	if cfg != nil && cfg.AttributeFilter != nil {
		return &filteredSpan{Span: span, filter: cfg.AttributeFilter}
	}
	return span
}
