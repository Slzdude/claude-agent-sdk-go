package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/Arize-ai/openinference/go/openinference-instrumentation"
	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// ---------------------------------------------------------------------------
// Guardrail 1: Tracer field is never nil (catches Bug 1)
// ---------------------------------------------------------------------------

func TestTracing_SpanHierarchyEndToEnd(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	tr := mockTransportWithToolUseHooks(t, tp)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opts := &ClaudeAgentOptions{TracerProvider: tp}
	msgs, err := mockProcessQuery(ctx, t, "run a tool", tr, opts)
	if err != nil {
		t.Fatalf("mockProcessQuery: %v", err)
	}

	for range msgs {
	}

	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans (AGENT + TOOL), got %d", len(spans))
	}

	var agentSpan, toolSpan *tracetest.SpanStub
	for i := range spans {
		attrs := spanAttrMapTest(spans[i].Attributes)
		switch attrs[semconv.OpenInferenceSpanKind] {
		case "AGENT":
			if agentSpan == nil {
				agentSpan = &spans[i]
			}
		case "TOOL":
			if toolSpan == nil {
				toolSpan = &spans[i]
			}
		}
	}

	if agentSpan == nil {
		t.Fatal("no AGENT span found — likely nil tracer (Bug 1)")
	}
	agentAttrs := spanAttrMapTest(agentSpan.Attributes)
	if agentAttrs[semconv.InputValue] != "run a tool" {
		t.Errorf("AGENT input.value = %q, want 'run a tool'", agentAttrs[semconv.InputValue])
	}

	if toolSpan == nil {
		t.Fatal("no TOOL span found — nil tracer silently disabled tool spans (Bug 1)")
	}

	toolAttrs := spanAttrMapTest(toolSpan.Attributes)
	if toolAttrs[semconv.ToolName] == "" {
		t.Error("TOOL span missing tool.name")
	}
	if toolAttrs[semconv.ToolID] == "" {
		t.Error("TOOL span missing tool.id")
	}

	// Verify parent-child: TOOL's parent must be the AGENT span.
	if toolSpan.Parent.SpanID() != agentSpan.SpanContext.SpanID() {
		t.Errorf("TOOL parent = %v, want AGENT %v", toolSpan.Parent.SpanID(), agentSpan.SpanContext.SpanID())
	}

	t.Logf("✓ AGENT(%s) -> TOOL(%s)", agentSpan.Name, toolSpan.Name)
}

// ---------------------------------------------------------------------------
// Guardrail 2: Hooks registered before Initialize (catches Bug 2)
// ---------------------------------------------------------------------------

func TestTracing_HooksRegisteredBeforeInitialize(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	opts := &ClaudeAgentOptions{TracerProvider: tp}

	st := newSessionTracer(tp)
	if st == nil {
		t.Fatal("newSessionTracer returned nil")
	}
	st.injectHooks(opts)

	if len(opts.Hooks[HookEventPreToolUse]) == 0 {
		t.Fatal("Bug 2: PreToolUse hooks not registered before Initialize()")
	}
	if len(opts.Hooks[HookEventPostToolUse]) == 0 {
		t.Fatal("Bug 2: PostToolUse hooks not registered before Initialize()")
	}
	if len(opts.Hooks[HookEventPostToolUseFailure]) == 0 {
		t.Fatal("Bug 2: PostToolUseFailure hooks not registered before Initialize()")
	}

	// Verify Initialize actually sends them.
	tr := mockTransportWithInit(t, resultJSON())
	q := newQueryProto(tr, opts)

	if len(q.hookCallbacks) != 0 {
		t.Fatalf("hookCallbacks should be empty before Initialize, got %d", len(q.hookCallbacks))
	}

	rawCh := q.Run(context.Background())
	go func() {
		for range rawCh {
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := q.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if len(q.hookCallbacks) < 3 {
		t.Errorf("Bug 2: expected >= 3 hookCallbacks after Initialize, got %d", len(q.hookCallbacks))
	}

	_ = tr.close()
}

func TestTracing_HooksRegisteredBeforeInitialize_ClientPath(t *testing.T) {
	tp := sdktrace.NewTracerProvider()

	hooksReceived := make(chan bool, 1)
	tr := mockTransportWithInitInspect(t, func(initReq map[string]any) {
		req, _ := initReq["request"].(map[string]any)
		hooks, ok := req["hooks"].(map[string]any)
		hooksReceived <- ok && len(hooks) > 0
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := &ClaudeAgentOptions{TracerProvider: tp}
	// Simulate what NewClaudeSDKClient does: inject hooks before Initialize.
	st := newSessionTracer(tp)
	if st != nil {
		st.injectHooks(opts)
	}
	client, err := newMockSDKClient(ctx, t, opts, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}
	defer client.Close()

	select {
	case gotHooks := <-hooksReceived:
		if !gotHooks {
			t.Error("Bug 2: Initialize request did not contain hooks")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Initialize request")
	}
}

// ---------------------------------------------------------------------------
// Guardrail 3: parentSpan valid when hooks fire (catches Bug 3)
// ---------------------------------------------------------------------------

func TestTracing_HookFiresWithValidParentSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	st := newSessionTracer(tp)
	if st == nil {
		t.Fatal("newSessionTracer returned nil")
	}

	ctx := context.Background()
	ctx, rootSpan := st.startQuerySpan(ctx, "test-query", "test-prompt", "claude-sonnet-4-20250514")

	if st.toolTracker.parentSpan == nil {
		t.Fatal("Bug 3: toolTracker.parentSpan is nil after startQuerySpan")
	}

	// Simulate hook firing with empty context (no span in context).
	result := st.toolTracker.start(context.Background(), "tu_001", "Bash", map[string]any{"command": "echo hi"}, "")
	if !result {
		t.Fatal("toolTracker.start returned false — nil tracer (Bug 1)")
	}

	st.toolTracker.end("tu_001", "output")
	rootSpan.End()

	spans := exporter.GetSpans()

	var agentSpan, toolSpan *tracetest.SpanStub
	for i := range spans {
		attrs := spanAttrMapTest(spans[i].Attributes)
		switch attrs[semconv.OpenInferenceSpanKind] {
		case "AGENT":
			agentSpan = &spans[i]
		case "TOOL":
			toolSpan = &spans[i]
		}
	}

	if agentSpan == nil {
		t.Fatal("no AGENT span")
	}
	if toolSpan == nil {
		t.Fatal("no TOOL span — Bug 1 or Bug 3")
	}

	if toolSpan.Parent.SpanID() != agentSpan.SpanContext.SpanID() {
		t.Errorf("Bug 3: TOOL parent = %v, want AGENT %v",
			toolSpan.Parent.SpanID(), agentSpan.SpanContext.SpanID())
	}

	t.Logf("✓ AGENT(%s) -> TOOL(%s)", agentSpan.Name, toolSpan.Name)
}

func TestTracing_HookParentSpanSetBeforeHookRegistration(t *testing.T) {
	st := newSessionTracer(sdktrace.NewTracerProvider())
	if st == nil {
		t.Fatal("newSessionTracer returned nil")
	}

	if st.toolTracker.parentSpan != nil {
		t.Fatal("parentSpan should be nil before startQuerySpan")
	}

	opts := &ClaudeAgentOptions{}
	st.injectHooks(opts)

	if st.toolTracker.parentSpan != nil {
		t.Fatal("parentSpan should still be nil after injectHooks")
	}

	ctx, rootSpan := st.startQuerySpan(context.Background(), "test", "prompt", "model")
	defer rootSpan.End()

	if st.toolTracker.parentSpan == nil {
		t.Fatal("Bug 3: parentSpan still nil after startQuerySpan")
	}
	if st.toolTracker.parentSpan != rootSpan {
		t.Fatal("parentSpan should be the root span")
	}

	_ = ctx
}

// ---------------------------------------------------------------------------
// Guardrail 4: Context attributes propagated (catches Bug 4)
// ---------------------------------------------------------------------------

func TestTracing_ContextAttributesPropagated(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	st := newSessionTracer(tp)
	if st == nil {
		t.Fatal("newSessionTracer returned nil")
	}

	ctx := context.Background()
	ctx = instrumentation.WithSession(ctx, "sess-123")
	ctx = instrumentation.WithUser(ctx, "user-456")
	ctx = instrumentation.WithMetadata(ctx, `{"key":"value"}`)
	ctx = instrumentation.WithTags(ctx, "tag1", "tag2")

	ctx, rootSpan := st.startQuerySpan(ctx, "test-attrs", "prompt", "model")
	rootSpan.End()

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no spans")
	}

	attrs := spanAttrMapTest(spans[0].Attributes)
	if attrs[semconv.SessionID] != "sess-123" {
		t.Errorf("Bug 4: session.id = %q, want 'sess-123'", attrs[semconv.SessionID])
	}
	if attrs[semconv.UserID] != "user-456" {
		t.Errorf("Bug 4: user.id = %q, want 'user-456'", attrs[semconv.UserID])
	}
	if attrs[semconv.Metadata] != `{"key":"value"}` {
		t.Errorf("Bug 4: metadata = %q", attrs[semconv.Metadata])
	}
}

// ---------------------------------------------------------------------------
// Guardrail 5: Tracer field invariant (catches Bug 1 at construction)
// ---------------------------------------------------------------------------

func TestTracing_TracerInvariant(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	st := newSessionTracer(tp)
	if st == nil {
		t.Fatal("newSessionTracer returned nil")
	}

	// All sub-trackers must share the same tracer.
	if st.toolTracker.tracer == nil {
		t.Fatal("Bug 1: toolTracker.tracer is nil")
	}
	if st.subagentTracker.tracer == nil {
		t.Fatal("subagentTracker.tracer is nil")
	}
	if st.toolTracker.tracer != st.tracer {
		t.Fatal("toolTracker.tracer != sessionTracer.tracer")
	}
	if st.subagentTracker.tracer != st.tracer {
		t.Fatal("subagentTracker.tracer != sessionTracer.tracer")
	}
}

// ---------------------------------------------------------------------------
// Mock helpers
// ---------------------------------------------------------------------------

// mockTransportWithInitInspect is like mockTransportWithInit but calls onInit
// with the raw Initialize request before responding.
func mockTransportWithInitInspect(t *testing.T, onInit func(map[string]any)) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)

		if sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err == nil {
				if onInit != nil {
					onInit(req)
				}
				reqID, _ := req["request_id"].(string)
				resp := map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"request_id": reqID,
						"subtype":    "success",
						"response":   map[string]any{"commands": []any{}, "output_style": "default"},
					},
				}
				b, _ := json.Marshal(resp)
				w := bufio.NewWriter(outW)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}
		for sc.Scan() {
		}
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

// mockTransportWithToolUseHooks simulates the CLI:
// 1. Responds to Initialize (including extracting hook callback IDs)
// 2. Sends PreToolUse hook callback
// 3. Reads hook response
// 4. Emits assistant message with tool_use
// 5. Emits result
func mockTransportWithToolUseHooks(t *testing.T, tp *sdktrace.TracerProvider) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)
		w := bufio.NewWriter(outW)

		// 1. Handle initialize — extract hook callback IDs.
		if !sc.Scan() {
			return
		}
		var initReq map[string]any
		if err := json.Unmarshal(sc.Bytes(), &initReq); err != nil {
			return
		}

		reqID, _ := initReq["request_id"].(string)
		reqPayload, _ := initReq["request"].(map[string]any)

		var preHookID, postHookID string
		if hooks, ok := reqPayload["hooks"].(map[string]any); ok {
			if preToolUse, ok := hooks["PreToolUse"].([]any); ok && len(preToolUse) > 0 {
				if matcher, ok := preToolUse[0].(map[string]any); ok {
					if ids, ok := matcher["hookCallbackIds"].([]any); ok && len(ids) > 0 {
						preHookID, _ = ids[0].(string)
					}
				}
			}
			if postToolUse, ok := hooks["PostToolUse"].([]any); ok && len(postToolUse) > 0 {
				if matcher, ok := postToolUse[0].(map[string]any); ok {
					if ids, ok := matcher["hookCallbackIds"].([]any); ok && len(ids) > 0 {
						postHookID, _ = ids[0].(string)
					}
				}
			}
		}

		// Send init response.
		resp := map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"request_id": reqID,
				"subtype":    "success",
				"response":   map[string]any{"commands": []any{}, "output_style": "default"},
			},
		}
		b, _ := json.Marshal(resp)
		_, _ = w.Write(b)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// 2. Send PreToolUse hook callback.
		if preHookID != "" {
			hookLine, _ := json.Marshal(map[string]any{
				"type":       "control_request",
				"request_id": "pre_hook_1",
				"request": map[string]any{
					"subtype":     "hook_callback",
					"callback_id": preHookID,
					"input": map[string]any{
						"tool_name":  "Bash",
						"tool_input": map[string]any{"command": "echo hello"},
					},
					"tool_use_id": "tu_001",
				},
			})
			_, _ = w.Write(hookLine)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}

		// 3. Read hook response from stdin.
		sc.Scan()

		// 4. Emit assistant message with tool_use.
		assistantLine, _ := json.Marshal(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "tool_use", "id": "tu_001", "name": "Bash", "input": map[string]any{"command": "echo hello"}},
				},
				"model": "claude-sonnet-4-20250514",
			},
		})
		_, _ = w.Write(assistantLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// 5. Emit PostToolUse hook callback.
		if postHookID != "" {
			hookLine, _ := json.Marshal(map[string]any{
				"type":       "control_request",
				"request_id": "post_hook_1",
				"request": map[string]any{
					"subtype":     "hook_callback",
					"callback_id": postHookID,
					"input":       map[string]any{"tool_name": "Bash", "tool_response": "hello"},
					"tool_use_id": "tu_001",
				},
			})
			_, _ = w.Write(hookLine)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}

		// 6. Emit result.
		resultLine, _ := json.Marshal(map[string]any{
			"type":           "result",
			"subtype":        "success",
			"session_id":     "test",
			"total_cost_usd": 0.001,
		})
		_, _ = w.Write(resultLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Drain stdin.
		for sc.Scan() {
		}
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

func spanAttrMapTest(attrs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[string(a.Key)] = a.Value.AsString()
	}
	return m
}
