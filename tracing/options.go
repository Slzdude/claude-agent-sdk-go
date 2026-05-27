package tracing

import (
	"github.com/Arize-ai/openinference/go/openinference-instrumentation"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Re-export context attribute functions from the official OpenInference library.
// These use unexported context keys so metadata cannot leak via OTel baggage.

// WithSuppression returns a context that suppresses tracing instrumentation.
var WithSuppression = instrumentation.WithSuppression

// IsSuppressed reports whether ctx was marked by WithSuppression.
var IsSuppressed = instrumentation.IsSuppressed

// WithSession returns a context carrying sessionID.
// Applied as the OpenInference session.id attribute on every span.
var WithSession = instrumentation.WithSession

// WithUser returns a context carrying userID.
// Applied as the OpenInference user.id attribute on every span.
var WithUser = instrumentation.WithUser

// WithMetadata returns a context carrying free-form metadata as a JSON string.
// Applied as the OpenInference metadata attribute on every span.
var WithMetadata = instrumentation.WithMetadata

// WithTags returns a context carrying categorical tags.
// Applied as the OpenInference tag.tags attribute (string slice).
var WithTags = instrumentation.WithTags

// ApplyContextAttributes copies any OpenInference context attributes from
// ctx onto span. Called once per span right after tracer.Start.
var ApplyContextAttributes = instrumentation.ApplyContextAttributes

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
