package semconv

import "go.opentelemetry.io/otel/attribute"

// OpenInference span kind values.
var (
	SpanKindValueAgent = attribute.StringValue("AGENT")
	SpanKindValueTool  = attribute.StringValue("TOOL")
)

// LLM system values.
const (
	LLMSystemAnthropic = "anthropic"
)

// MIME type values.
const (
	MimeTypeText = "text/plain"
	MimeTypeJSON = "application/json"
)

// Abandoned span error types.
const (
	ErrorTypeToolSpanAbandoned    = "tool_span_abandoned"
	ErrorTypeSubagentSpanAbandoned = "subagent_span_abandoned"
)
