package claude

import (
	"context"
	"testing"
	"time"

	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestQuery_WithTracerProvider(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	tr := mockTransportWithInit(t,
		assistantJSON("hello from traced query"),
		resultJSON(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := &ClaudeAgentOptions{TracerProvider: tp}
	msgs, err := mockProcessQuery(ctx, t, "test prompt", tr, opts)
	if err != nil {
		t.Fatalf("mockProcessQuery: %v", err)
	}

	for range msgs {
	}

	spans := exporter.GetSpans()
	if len(spans) < 1 {
		t.Fatal("expected at least 1 span")
	}

	// Find the root AGENT span
	var rootSpan *tracetest.SpanStub
	for i := range spans {
		attrs := spanAttrMap(spans[i].Attributes)
		if attrs[semconv.OpenInferenceSpanKind] == "AGENT" {
			rootSpan = &spans[i]
			break
		}
	}
	if rootSpan == nil {
		t.Fatal("no AGENT span found")
	}

	attrs := spanAttrMap(rootSpan.Attributes)
	if attrs[semconv.InputValue] != "test prompt" {
		t.Errorf("input.value = %q, want 'test prompt'", attrs[semconv.InputValue])
	}
	if attrs[semconv.LLMModelName] == "" {
		t.Error("llm.model_name should be set")
	}
	if attrs["gen_ai.request.model"] == "" {
		t.Error("gen_ai.request.model should be set")
	}
	t.Logf("Root span: %s, attributes: %v", rootSpan.Name, attrs)
}

func TestQuery_WithoutTracerProvider(t *testing.T) {
	tr := mockTransportWithInit(t,
		assistantJSON("hello"),
		resultJSON(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// No TracerProvider — should work without tracing.
	msgs, err := mockProcessQuery(ctx, t, "test", tr, nil)
	if err != nil {
		t.Fatalf("mockProcessQuery: %v", err)
	}

	collected := 0
	for range msgs {
		collected++
	}
	if collected < 2 {
		t.Errorf("expected >= 2 messages, got %d", collected)
	}
}

func spanAttrMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.AsString()
	}
	return m
}
