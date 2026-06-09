package tracing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	claude "github.com/Slzdude/claude-agent-sdk-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ToolSpanTracker manages TOOL spans for tool executions.
type ToolSpanTracker struct {
	tracer           trace.Tracer
	parentSpan       trace.Span
	cfg              *TraceConfig
	mu               sync.Mutex
	spans            map[string]trace.Span
	subagentCallback func(toolUseID, agentID, agentType, toolName, parentToolUseID string)
	// parentContextResolver resolves the parent context for a parent_tool_use_id.
	parentContextResolver func(parentToolUseID string) context.Context
	// agentIDContextResolver resolves the parent context for an agent_id.
	// Used when the CLI provides agent_id but not parent_tool_use_id.
	agentIDContextResolver func(agentID string) context.Context
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

// SetParentContextResolver sets a callback that resolves the parent context
// for a given parent_tool_use_id. Used to parent TOOL spans under subagent
// AGENT spans when hooks fire within a subagent context.
func (t *ToolSpanTracker) SetParentContextResolver(cb func(parentToolUseID string) context.Context) {
	t.parentContextResolver = cb
}

// SetAgentIDContextResolver sets a callback that resolves the parent context
// for a given agent_id. Used when the CLI provides agent_id but not
// parent_tool_use_id in the hook input.
func (t *ToolSpanTracker) SetAgentIDContextResolver(cb func(agentID string) context.Context) {
	t.agentIDContextResolver = cb
}

// Start creates a TOOL span for a tool execution. Returns false if a span with
// this toolUseID already exists (deduplication).
// hookCtx is the context from the hook callback, which carries the parent span chain.
func (t *ToolSpanTracker) Start(hookCtx context.Context, toolUseID, toolName string, input map[string]any, parentToolUseID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.spans[toolUseID]; exists {
		return false
	}

	// Use the hook's context (which carries the AGENT span from the caller chain).
	// If it doesn't have a span, fall back to the stored parentSpan.
	parentCtx := hookCtx
	if trace.SpanFromContext(hookCtx).SpanContext().IsValid() {
		// Hook context already has a span — use it directly as parent.
	} else {
		parentCtx = trace.ContextWithSpan(context.Background(), t.parentSpan)
	}

	_, span := t.tracer.Start(parentCtx, toolName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindTool),
			attribute.String(semconv.ToolName, toolName),
			attribute.String(semconv.ToolID, toolUseID),
		),
	)
	span = wrapSpan(span, t.cfg)

	if input != nil {
		if inputJSON, err := json.Marshal(input); err == nil {
			span.SetAttributes(
				attribute.String(semconv.InputValue, string(inputJSON)),
				attribute.String(semconv.InputMimeType, semconv.MimeTypeJSON),
				attribute.String(semconv.ToolParameters, string(inputJSON)),
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
				attribute.String(semconv.OutputValue, string(outputJSON)),
				attribute.String(semconv.OutputMimeType, semconv.MimeTypeJSON),
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
		span.SetAttributes(attribute.String("error.type", "tool_span_abandoned"))
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
func (t *ToolSpanTracker) InjectHooks(opts *claude.ClaudeAgentOptions) {
	if opts.Hooks == nil {
		opts.Hooks = make(map[claude.HookEvent][]claude.HookMatcher)
	}

	if t.hooksInjected(opts) {
		return
	}

	preHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecover("PreToolUse hook")
		toolName, _ := input["tool_name"].(string)
		toolInput, _ := input["tool_input"].(map[string]any)
		agentID, _ := input["agent_id"].(string)
		agentType, _ := input["agent_type"].(string)
		parentToolUseID, _ := input["parent_tool_use_id"].(string)

		// Resolve parent context: try parent_tool_use_id first, then agent_id.
		hookCtx := ctx
		if parentToolUseID != "" && t.parentContextResolver != nil {
			if resolved := t.parentContextResolver(parentToolUseID); resolved != nil {
				hookCtx = resolved
			}
		} else if agentID != "" && t.agentIDContextResolver != nil {
			if resolved := t.agentIDContextResolver(agentID); resolved != nil {
				hookCtx = resolved
			}
		}

		t.Start(hookCtx, toolUseID, toolName, toolInput, parentToolUseID)

		if agentID != "" && t.subagentCallback != nil {
			t.subagentCallback(toolUseID, agentID, agentType, toolName, parentToolUseID)
		}

		return nil, nil
	})

	postHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecover("PostToolUse hook")
		output := input["tool_response"]
		t.End(toolUseID, output)
		return nil, nil
	})

	postFailHook := claude.HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecover("PostToolUseFailure hook")
		errMsg, _ := input["error"].(string)
		if errMsg == "" {
			errMsg = "tool execution failed"
		}
		t.EndWithError(toolUseID, fmt.Errorf("%s", errMsg))
		return nil, nil
	})

	opts.Hooks[claude.HookEventPreToolUse] = append(opts.Hooks[claude.HookEventPreToolUse], claude.HookMatcher{
		Hooks: []claude.HookCallback{preHook},
	})
	opts.Hooks[claude.HookEventPostToolUse] = append(opts.Hooks[claude.HookEventPostToolUse], claude.HookMatcher{
		Hooks: []claude.HookCallback{postHook},
	})
	opts.Hooks[claude.HookEventPostToolUseFailure] = append(opts.Hooks[claude.HookEventPostToolUseFailure], claude.HookMatcher{
		Hooks: []claude.HookCallback{postFailHook},
	})

	t.markHooksInjected(opts)
}

func (t *ToolSpanTracker) hooksInjected(opts *claude.ClaudeAgentOptions) bool {
	if opts.Hooks == nil {
		return false
	}
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
		Hooks:   nil,
	})
}

func safeRecover(hookName string) {
	if r := recover(); r != nil {
		slog.Error("panic in instrumentation hook", "hook", hookName, "recover", r)
	}
}
