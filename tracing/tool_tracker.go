package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing/semconv"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ToolSpanTracker manages TOOL spans for tool executions.
type ToolSpanTracker struct {
	tracer     trace.Tracer
	parentSpan trace.Span
	cfg        *TraceConfig
	mu         sync.Mutex
	spans      map[string]trace.Span
	// subagentCallback is called when a tool has agent_id (subagent detection).
	subagentCallback func(toolUseID, agentID, agentType, toolName, parentToolUseID string)
}

// NewToolSpanTracker creates a new tracker.
func NewToolSpanTracker(tracer trace.Tracer, parentSpan trace.Span, cfg *TraceConfig) *ToolSpanTracker {
	return &ToolSpanTracker{
		tracer:     tracer,
		parentSpan: parentSpan,
		cfg:        cfg,
		spans:      make(map[string]trace.Span),
	}
}

// SetSubagentCallback sets the callback for subagent detection.
func (t *ToolSpanTracker) SetSubagentCallback(cb func(toolUseID, agentID, agentType, toolName, parentToolUseID string)) {
	t.subagentCallback = cb
}

// Start creates a TOOL span for a tool execution. Returns false if a span with
// this toolUseID already exists (deduplication).
func (t *ToolSpanTracker) Start(toolUseID, toolName string, input map[string]any, parentToolUseID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Deduplication: don't create duplicate spans
	if _, exists := t.spans[toolUseID]; exists {
		return false
	}

	ctx := trace.ContextWithSpan(context.Background(), t.parentSpan)
	_, span := t.tracer.Start(ctx, toolName,
		trace.WithAttributes(
			semconv.SpanKindKey.String("TOOL"),
			semconv.ToolName.String(toolName),
			semconv.ToolID.String(toolUseID),
		),
	)

	span = wrapSpan(span, t.cfg)

	if input != nil {
		if inputJSON, err := json.Marshal(input); err == nil {
			span.SetAttributes(
				semconv.InputValue.String(string(inputJSON)),
				semconv.InputMimeType.String(semconv.MimeTypeJSON),
				semconv.ToolParameters.String(string(inputJSON)),
			)
		}
	}

	t.spans[toolUseID] = span
	return true
}

// End ends a TOOL span with success output.
func (t *ToolSpanTracker) End(toolUseID string, output any) {
	t.mu.Lock()
	span, ok := t.spans[toolUseID]
	if ok {
		delete(t.spans, toolUseID)
	}
	t.mu.Unlock()

	if !ok {
		return
	}

	if output != nil {
		if outputJSON, err := json.Marshal(output); err == nil {
			span.SetAttributes(
				semconv.OutputValue.String(string(outputJSON)),
				semconv.OutputMimeType.String(semconv.MimeTypeJSON),
			)
		}
	}

	span.SetStatus(codes.Ok, "")
	span.End()
}

// EndWithError ends a TOOL span with an error.
func (t *ToolSpanTracker) EndWithError(toolUseID string, err error) {
	t.mu.Lock()
	span, ok := t.spans[toolUseID]
	if ok {
		delete(t.spans, toolUseID)
	}
	t.mu.Unlock()

	if !ok {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}

// EndAll ends all in-flight TOOL spans as abandoned.
func (t *ToolSpanTracker) EndAll() {
	t.mu.Lock()
	spans := make(map[string]trace.Span, len(t.spans))
	for k, v := range t.spans {
		spans[k] = v
	}
	t.spans = make(map[string]trace.Span)
	t.mu.Unlock()

	for id, span := range spans {
		span.SetAttributes(semconv.ErrorType.String(semconv.ErrorTypeToolSpanAbandoned))
		span.SetStatus(codes.Error, fmt.Sprintf("tool span abandoned: %s", id))
		span.End()
	}
}

// GetInFlightSpan returns the in-flight span for a tool use ID.
func (t *ToolSpanTracker) GetInFlightSpan(toolUseID string) (trace.Span, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	span, ok := t.spans[toolUseID]
	return span, ok
}

// InjectHooks injects instrumentation hooks into ClaudeAgentOptions.
// Uses a sentinel key in the hooks map to avoid duplicate injection.
func (t *ToolSpanTracker) InjectHooks(opts *claude.ClaudeAgentOptions) {
	if opts.Hooks == nil {
		opts.Hooks = make(map[claude.HookEvent][]claude.HookMatcher)
	}

	// Check if already injected (avoid accumulation on multi-turn)
	if t.hooksInjected(opts) {
		return
	}

	// PreToolUse hook
	preHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecover("PreToolUse hook")

		toolName, _ := input["tool_name"].(string)
		toolInput, _ := input["tool_input"].(map[string]any)
		agentID, _ := input["agent_id"].(string)
		agentType, _ := input["agent_type"].(string)
		parentToolUseID, _ := input["parent_tool_use_id"].(string)

		t.Start(toolUseID, toolName, toolInput, parentToolUseID)

		// Subagent detection
		if agentID != "" && t.subagentCallback != nil {
			t.subagentCallback(toolUseID, agentID, agentType, toolName, parentToolUseID)
		}

		return nil, nil
	})

	// PostToolUse hook
	postHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecover("PostToolUse hook")

		output := input["tool_response"]
		t.End(toolUseID, output)
		return nil, nil
	})

	// PostToolUseFailure hook
	postFailHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecover("PostToolUseFailure hook")

		errMsg, _ := input["error"].(string)
		if errMsg == "" {
			errMsg = "tool execution failed"
		}
		t.EndWithError(toolUseID, fmt.Errorf("%s", errMsg))
		return nil, nil
	})

	// Merge with existing hooks (preserve user hooks)
	opts.Hooks[claude.HookEventPreToolUse] = append(opts.Hooks[claude.HookEventPreToolUse], claude.HookMatcher{
		Hooks: []claude.HookCallback{preHook},
	})
	opts.Hooks[claude.HookEventPostToolUse] = append(opts.Hooks[claude.HookEventPostToolUse], claude.HookMatcher{
		Hooks: []claude.HookCallback{postHook},
	})
	opts.Hooks[claude.HookEventPostToolUseFailure] = append(opts.Hooks[claude.HookEventPostToolUseFailure], claude.HookMatcher{
		Hooks: []claude.HookCallback{postFailHook},
	})

	// Mark as injected
	t.markHooksInjected(opts)
}

// hooksInjected checks if our hooks are already in the options.
func (t *ToolSpanTracker) hooksInjected(opts *claude.ClaudeAgentOptions) bool {
	// Check if any PreToolUse hook matcher has our sentinel callback count.
	// We use the number of matchers as a simple heuristic: if there are more
	// than the original user hooks, we've already injected.
	// A more robust approach: check if the last hook is ours by marker.
	// For simplicity, we check if the hooks map has our marker key.
	if opts.Hooks == nil {
		return false
	}
	// Use a special nil-matcher entry as sentinel
	for _, m := range opts.Hooks[claude.HookEventPreToolUse] {
		if m.Matcher != nil && *m.Matcher == "__tracing_injected__" {
			return true
		}
	}
	return false
}

func (t *ToolSpanTracker) markHooksInjected(opts *claude.ClaudeAgentOptions) {
	sentinel := "__tracing_injected__"
	opts.Hooks[claude.HookEventPreToolUse] = append(opts.Hooks[claude.HookEventPreToolUse], claude.HookMatcher{
		Matcher: &sentinel,
		Hooks:   nil, // marker only, no callback
	})
}

func safeRecover(hookName string) {
	if r := recover(); r != nil {
		slog.Error("panic in instrumentation hook", "hook", hookName, "recover", r)
	}
}
