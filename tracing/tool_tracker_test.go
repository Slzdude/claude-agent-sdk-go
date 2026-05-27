package tracing

import (
	"context"
	"errors"
	"sync"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer() (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	return tp, exporter
}

func TestToolSpanTracker_StartEnd(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)
	tracker.Start("tool_1", "Bash", map[string]any{"command": "ls"}, "")
	tracker.End("tool_1", map[string]any{"output": "file1.txt"})

	rootSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	toolSpan := spans[0]
	if toolSpan.Name != "Bash" {
		t.Errorf("tool span name = %q, want %q", toolSpan.Name, "Bash")
	}

	attrs := attrMap(toolSpan.Attributes)
	if attrs[string(semconv.SpanKindKey)] != "TOOL" {
		t.Errorf("span kind = %q, want TOOL", attrs[string(semconv.SpanKindKey)])
	}
	if attrs[string(semconv.ToolName)] != "Bash" {
		t.Errorf("tool name = %q, want Bash", attrs[string(semconv.ToolName)])
	}
	if attrs[string(semconv.ToolID)] != "tool_1" {
		t.Errorf("tool id = %q, want tool_1", attrs[string(semconv.ToolID)])
	}
	// Check tool.parameters is set
	if attrs[string(semconv.ToolParameters)] == "" {
		t.Error("tool.parameters should be set")
	}
}

func TestToolSpanTracker_Deduplication(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)
	ok1 := tracker.Start("tool_1", "Bash", nil, "")
	ok2 := tracker.Start("tool_1", "Bash", nil, "") // duplicate

	if !ok1 {
		t.Error("first Start should return true")
	}
	if ok2 {
		t.Error("duplicate Start should return false")
	}

	tracker.End("tool_1", nil)
	rootSpan.End()

	spans := exporter.GetSpans()
	// Should be root + 1 tool (not 2)
	if len(spans) != 2 {
		t.Errorf("expected 2 spans (root + 1 tool), got %d", len(spans))
	}
}

func TestToolSpanTracker_EndWithError(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)
	tracker.Start("tool_1", "Bash", nil, "")
	tracker.EndWithError("tool_1", errors.New("command failed"))

	rootSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	toolSpan := spans[0]
	if toolSpan.Status.Code != codes.Error {
		t.Errorf("status = %v, want ERROR", toolSpan.Status.Code)
	}
}

func TestToolSpanTracker_EndAll(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)
	tracker.Start("tool_1", "Bash", nil, "")
	tracker.Start("tool_2", "Read", nil, "")
	tracker.EndAll()

	rootSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	for _, span := range spans[:2] {
		if span.Status.Code != codes.Error {
			t.Errorf("span %q should have ERROR status", span.Name)
		}
	}
}

func TestToolSpanTracker_InjectHooks_MergesUserHooks(t *testing.T) {
	tp, _ := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)

	userHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		return nil, nil
	})

	opts := &claude.ClaudeAgentOptions{
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPreToolUse: {
				{Hooks: []claude.HookCallback{userHook}},
			},
		},
	}

	tracker.InjectHooks(opts)

	// Should have user hook + instrumentation hook + sentinel
	if len(opts.Hooks[claude.HookEventPreToolUse]) != 3 {
		t.Errorf("expected 3 PreToolUse hooks, got %d", len(opts.Hooks[claude.HookEventPreToolUse]))
	}
}

func TestToolSpanTracker_InjectHooks_NoAccumulation(t *testing.T) {
	tp, _ := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)

	opts := &claude.ClaudeAgentOptions{}

	// Inject multiple times
	tracker.InjectHooks(opts)
	tracker.InjectHooks(opts)
	tracker.InjectHooks(opts)

	// Should only have 1 sentinel + 1 instrumentation hook per event
	if len(opts.Hooks[claude.HookEventPreToolUse]) != 2 {
		t.Errorf("expected 2 PreToolUse hooks (no accumulation), got %d", len(opts.Hooks[claude.HookEventPreToolUse]))
	}
}

func TestToolSpanTracker_PanicRecovery(t *testing.T) {
	tp, _ := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)

	opts := &claude.ClaudeAgentOptions{}
	tracker.InjectHooks(opts)

	// The hooks should have panic recovery built in
	if len(opts.Hooks[claude.HookEventPreToolUse]) < 2 {
		t.Error("expected at least 2 hooks")
	}
}

func TestToolSpanTracker_ConcurrentAccess(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	tracker := NewToolSpanTracker(tracer, rootSpan, nil)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			toolID := "tool_" + string(rune('A'+id))
			tracker.Start(toolID, "Bash", nil, "")
			tracker.End(toolID, nil)
		}(i)
	}
	wg.Wait()

	rootSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 11 {
		t.Errorf("expected 11 spans, got %d", len(spans))
	}
}

func TestToolSpanTracker_WithAttributeFilter(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	// Filter that drops input.value (PII redaction)
	cfg := &TraceConfig{
		AttributeFilter: func(kv attribute.KeyValue) bool {
			return kv.Key != semconv.InputValue
		},
	}

	tracker := NewToolSpanTracker(tracer, rootSpan, cfg)
	tracker.Start("tool_1", "Bash", map[string]any{"secret": "password123"}, "")
	tracker.End("tool_1", nil)

	rootSpan.End()

	spans := exporter.GetSpans()
	attrs := attrMap(spans[0].Attributes)
	if attrs[string(semconv.InputValue)] != "" {
		t.Error("input.value should be filtered out by AttributeFilter")
	}
	if attrs[string(semconv.ToolName)] != "Bash" {
		t.Error("tool.name should still be present")
	}
}

func attrMap(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.AsString()
	}
	return m
}
