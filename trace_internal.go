package claude

// trace_internal.go provides the internal tracing hooks used by processQuery
// and ClaudeSDKClient when opts.TracerProvider is set.
//
// This file bridges the SDK core (processQuery, ClaudeSDKClient) with the
// tracing subsystem (span creation, hook injection, attribute extraction)
// without creating a circular dependency on the tracing package.
//
// The tracing package's public API (TracedQuery, TracedClient) is a thin
// wrapper that calls into these same hooks — users who prefer the decorator
// pattern can use tracing.TracedQuery, while users who set TracerProvider
// on ClaudeAgentOptions get the same tracing automatically.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/Arize-ai/openinference/go/openinference-instrumentation"
	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// sessionTracer holds per-session tracing state. Created when
// opts.TracerProvider is non-nil, lives for the lifetime of the session.
type sessionTracer struct {
	tracer          trace.Tracer
	toolTracker     *toolSpanTracker
	subagentTracker *subagentSpanTracker
}

// newSessionTracer creates a sessionTracer from a TracerProvider.
func newSessionTracer(tp trace.TracerProvider) *sessionTracer {
	tracer := tp.Tracer("claude-agent-sdk-go")
	slog.Debug("[trace] newSessionTracer", "provider_type", fmt.Sprintf("%T", tp), "tracer_nil", tracer == nil)
	if tracer == nil {
		slog.Warn("TracerProvider.Tracer returned nil, tracing disabled")
		return nil
	}
	st := &sessionTracer{
		tracer:          tracer,
		toolTracker:     newToolSpanTracker(tracer),
		subagentTracker: newSubagentSpanTracker(tracer),
	}
	// Invariant: all sub-trackers must share the same tracer instance.
	if st.toolTracker.tracer != tracer || st.subagentTracker.tracer != tracer {
		panic("sessionTracer: sub-trackers have mismatched tracer (programming error)")
	}
	return st
}

// startQuerySpan creates the root AGENT span for a Query/ReceiveResponse.
func (st *sessionTracer) startQuerySpan(ctx context.Context, spanName, prompt, model string) (context.Context, trace.Span) {
	inputValue, inputMimeType := formatPromptValueInternal(prompt)

	if model == "" {
		model = "claude"
	}

	ctx, span := st.tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.LLMSystem, semconv.LLMSystemAnthropic),
			attribute.String(semconv.LLMModelName, model),
			attribute.String("gen_ai.request.model", model),
			attribute.String(semconv.InputValue, inputValue),
			attribute.String(semconv.InputMimeType, inputMimeType),
		),
	)

	// Apply context attributes (session.id, user.id, metadata, tag.tags)
	applyContextAttributesInternal(ctx, span)

	// Wire up tool/subagent trackers
	st.toolTracker.setParentSpan(span)
	st.subagentTracker.setRootSpan(span)
	st.toolTracker.setSubagentCallback(func(toolUseID, agentID, agentType, toolName, parentToolUseID string) {
		st.subagentTracker.getOrCreate(toolUseID, agentID, agentType, toolName)
	})
	st.toolTracker.setParentContextResolver(func(parentToolUseID string) context.Context {
		if subSpan := st.subagentTracker.getByToolUseID(parentToolUseID); subSpan != nil {
			return trace.ContextWithSpan(context.Background(), subSpan)
		}
		return nil
	})
	st.toolTracker.setAgentIDContextResolver(func(agentID string) context.Context {
		if subSpan := st.subagentTracker.getByAgentID(agentID); subSpan != nil {
			return trace.ContextWithSpan(context.Background(), subSpan)
		}
		return nil
	})

	return ctx, span
}

// processTracedMessage extracts attributes and manages tool/subagent spans
// for a single message. Called from the message consumption goroutine.
// Panics are recovered to avoid crashing the consumer goroutine.
func (st *sessionTracer) processTracedMessage(ctx context.Context, rootSpan trace.Span, msg Message, outputMsgIndex *int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("panic in traced message processing (non-fatal)", "recover", r, "msg_type", fmt.Sprintf("%T", msg), "stack", string(debug.Stack()))
		}
	}()

	// ProcessMessage must run BEFORE resolveTargetSpan
	st.subagentTracker.processMessage(msg)

	targetSpan := st.resolveTargetSpan(rootSpan, msg)
	extractMessageAttributesInternal(targetSpan, msg, outputMsgIndex)

	st.updateToolSpansFromMessages(ctx, msg)
}

func (st *sessionTracer) resolveTargetSpan(rootSpan trace.Span, msg Message) trace.Span {
	if parentID := getParentToolUseIDInternal(msg); parentID != "" {
		if subSpan := st.subagentTracker.getByToolUseID(parentID); subSpan != nil {
			return subSpan
		}
	}
	return rootSpan
}

func (st *sessionTracer) updateToolSpansFromMessages(ctx context.Context, msg Message) {
	content := extractContentBlocksInternal(msg)

	parentCtx := ctx
	if parentID := getParentToolUseIDInternal(msg); parentID != "" {
		if subSpan := st.subagentTracker.getByToolUseID(parentID); subSpan != nil {
			parentCtx = trace.ContextWithSpan(ctx, subSpan)
		}
	}

	for _, block := range content {
		switch b := block.(type) {
		case *ToolUseBlock:
			st.toolTracker.start(parentCtx, b.ID, b.Name, b.Input, "")
		case *ToolResultBlock:
			if b.IsError != nil && *b.IsError {
				st.toolTracker.endWithError(b.ToolUseID, fmt.Errorf("tool result error"))
			} else {
				st.toolTracker.end(b.ToolUseID, b.Content)
			}
		}
	}
}

func (st *sessionTracer) endAll() {
	st.toolTracker.endAll()
	st.subagentTracker.endAll()
}

// injectHooks injects tracing hooks into ClaudeAgentOptions for tool span tracking.
func (st *sessionTracer) injectHooks(opts *ClaudeAgentOptions) {
	st.toolTracker.injectHooks(opts)
}

// --- Internal helpers (avoid circular dependency on tracing package) ---

func formatPromptValueInternal(prompt string) (string, string) {
	return prompt, semconv.MimeTypeText
}

func applyContextAttributesInternal(ctx context.Context, span trace.Span) {
	// Delegate to the openinference-instrumentation library which owns the
	// canonical context keys. The tracing package's WithSession/WithUser/
	// WithMetadata/WithTags functions set these same keys.
	instrumentation.ApplyContextAttributes(ctx, span)
}

// Compile-time assertion: processQuery depends on these sessionTracer methods.
// If any are renamed or removed, this line produces a compile error.
var _ interface {
	startQuerySpan(ctx context.Context, spanName, prompt, model string) (context.Context, trace.Span)
	processTracedMessage(ctx context.Context, rootSpan trace.Span, msg Message, outputMsgIndex *int)
	endAll()
	injectHooks(opts *ClaudeAgentOptions)
} = (*sessionTracer)(nil)

func getParentToolUseIDInternal(msg Message) string {
	switch m := msg.(type) {
	case *AssistantMessage:
		return m.ParentToolUseID
	case *UserMessage:
		return m.ParentToolUseID
	case *TaskStartedMessage:
		return m.ToolUseID
	case *TaskProgressMessage:
		return m.ToolUseID
	case *TaskNotificationMessage:
		return m.ToolUseID
	}
	return ""
}

func extractContentBlocksInternal(msg Message) []ContentBlock {
	switch m := msg.(type) {
	case *AssistantMessage:
		return m.Content
	case *UserMessage:
		if content, ok := m.Content.([]ContentBlock); ok {
			return content
		}
	}
	return nil
}

// --- tool span tracker (internal, mirrors tracing.ToolSpanTracker) ---

type toolSpanTracker struct {
	tracer                 trace.Tracer
	parentSpan             trace.Span
	spans                  map[string]trace.Span
	subagentCallback       func(toolUseID, agentID, agentType, toolName, parentToolUseID string)
	parentContextResolver  func(parentToolUseID string) context.Context
	agentIDContextResolver func(agentID string) context.Context
	hooksInjected          bool
}

func newToolSpanTracker(tracer trace.Tracer) *toolSpanTracker {
	return &toolSpanTracker{spans: make(map[string]trace.Span), tracer: tracer}
}

func (t *toolSpanTracker) setParentSpan(span trace.Span) {
	t.parentSpan = span
}

func (t *toolSpanTracker) setSubagentCallback(cb func(toolUseID, agentID, agentType, toolName, parentToolUseID string)) {
	t.subagentCallback = cb
}

func (t *toolSpanTracker) setParentContextResolver(cb func(parentToolUseID string) context.Context) {
	t.parentContextResolver = cb
}

func (t *toolSpanTracker) setAgentIDContextResolver(cb func(agentID string) context.Context) {
	t.agentIDContextResolver = cb
}

func (t *toolSpanTracker) start(hookCtx context.Context, toolUseID, toolName string, input map[string]any, parentToolUseID string) bool {
	if t == nil || t.tracer == nil {
		return false
	}

	if _, exists := t.spans[toolUseID]; exists {
		return false
	}

	parentCtx := hookCtx
	if trace.SpanFromContext(hookCtx).SpanContext().IsValid() {
		// hook context has a span — use as-is
	} else if t.parentSpan != nil {
		parentCtx = trace.ContextWithSpan(context.Background(), t.parentSpan)
	}

	_, span := t.tracer.Start(parentCtx, toolName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindTool),
			attribute.String(semconv.ToolName, toolName),
			attribute.String(semconv.ToolID, toolUseID),
		),
	)

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

func (t *toolSpanTracker) end(toolUseID string, output any) {
	span, ok := t.spans[toolUseID]
	if !ok {
		return
	}
	delete(t.spans, toolUseID)

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

func (t *toolSpanTracker) endWithError(toolUseID string, err error) {
	span, ok := t.spans[toolUseID]
	if !ok {
		return
	}
	delete(t.spans, toolUseID)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.End()
}

func (t *toolSpanTracker) endAll() {
	for id, span := range t.spans {
		span.SetAttributes(attribute.String("error.type", "tool_span_abandoned"))
		span.SetStatus(codes.Error, fmt.Sprintf("tool span abandoned: %s", id))
		span.End()
	}
	t.spans = make(map[string]trace.Span)
}

func (t *toolSpanTracker) injectHooks(opts *ClaudeAgentOptions) {
	if t.hooksInjected {
		return
	}
	t.hooksInjected = true

	if opts.Hooks == nil {
		opts.Hooks = make(map[HookEvent][]HookMatcher)
	}

	preHook := HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecoverInternal("PreToolUse hook")
		toolName, _ := input["tool_name"].(string)
		toolInput, _ := input["tool_input"].(map[string]any)
		agentID, _ := input["agent_id"].(string)
		agentType, _ := input["agent_type"].(string)
		parentToolUseID, _ := input["parent_tool_use_id"].(string)

		slog.Debug("[trace] PreToolUse hook",
			"tool", toolName, "toolUseID", toolUseID,
			"agentID", agentID, "parentToolUseID", parentToolUseID,
			"parentSpan_nil", t.parentSpan == nil, "tracer_nil", t.tracer == nil)

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

		t.start(hookCtx, toolUseID, toolName, toolInput, parentToolUseID)

		if agentID != "" && t.subagentCallback != nil {
			t.subagentCallback(toolUseID, agentID, agentType, toolName, parentToolUseID)
		}
		return nil, nil
	})

	postHook := HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecoverInternal("PostToolUse hook")
		output := input["tool_response"]
		t.end(toolUseID, output)
		return nil, nil
	})

	postFailHook := HookCallback(func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		defer safeRecoverInternal("PostToolUseFailure hook")
		errMsg, _ := input["error"].(string)
		if errMsg == "" {
			errMsg = "tool execution failed"
		}
		t.endWithError(toolUseID, fmt.Errorf("%s", errMsg))
		return nil, nil
	})

	opts.Hooks[HookEventPreToolUse] = append(opts.Hooks[HookEventPreToolUse], HookMatcher{Hooks: []HookCallback{preHook}})
	opts.Hooks[HookEventPostToolUse] = append(opts.Hooks[HookEventPostToolUse], HookMatcher{Hooks: []HookCallback{postHook}})
	opts.Hooks[HookEventPostToolUseFailure] = append(opts.Hooks[HookEventPostToolUseFailure], HookMatcher{Hooks: []HookCallback{postFailHook}})
}

func safeRecoverInternal(hookName string) {
	if r := recover(); r != nil {
		slog.Error("panic in instrumentation hook", "hook", hookName, "recover", r)
	}
}

// --- subagent span tracker (internal) ---

type subagentSpanTracker struct {
	tracer      trace.Tracer
	rootSpan    trace.Span
	toolTracker *toolSpanTracker
	agents      map[string]trace.Span
	toolToAgent map[string]string
}

func newSubagentSpanTracker(tracer trace.Tracer) *subagentSpanTracker {
	return &subagentSpanTracker{
		tracer:      tracer,
		agents:      make(map[string]trace.Span),
		toolToAgent: make(map[string]string),
	}
}

func (s *subagentSpanTracker) setRootSpan(span trace.Span) {
	s.rootSpan = span
}

func (s *subagentSpanTracker) getOrCreate(toolUseID, agentID, agentType, toolName string) trace.Span {
	if span, ok := s.agents[agentID]; ok {
		return span
	}

	parentCtx := context.Background()
	if s.rootSpan != nil {
		parentCtx = trace.ContextWithSpan(context.Background(), s.rootSpan)
	}
	if s.toolTracker != nil {
		if toolSpan, ok := s.toolTracker.spans[toolUseID]; ok {
			parentCtx = trace.ContextWithSpan(context.Background(), toolSpan)
		}
	}

	spanName := fmt.Sprintf("ClaudeAgentSDK.%s", toolName)
	_, span := s.tracer.Start(parentCtx, spanName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.AgentName, agentID),
		),
	)

	s.agents[agentID] = span
	s.toolToAgent[toolUseID] = agentID
	return span
}

func (s *subagentSpanTracker) getByToolUseID(toolUseID string) trace.Span {
	agentID := s.toolToAgent[toolUseID]
	if agentID == "" {
		return nil
	}
	return s.agents[agentID]
}

func (s *subagentSpanTracker) getByAgentID(agentID string) trace.Span {
	return s.agents[agentID]
}

func (s *subagentSpanTracker) processMessage(msg Message) {
	switch m := msg.(type) {
	case *TaskNotificationMessage:
		s.handleTaskNotification(m)
	case *TaskStartedMessage:
		s.ensureSubagentSpan(m.ToolUseID, m.TaskID, m.Description)
	}
}

func (s *subagentSpanTracker) handleTaskNotification(msg *TaskNotificationMessage) {
	agentID := s.toolToAgent[msg.ToolUseID]
	if agentID == "" {
		for aid := range s.agents {
			agentID = aid
			break
		}
	}

	span, ok := s.agents[agentID]
	if !ok {
		return
	}
	delete(s.agents, agentID)
	delete(s.toolToAgent, msg.ToolUseID)

	switch msg.Status {
	case TaskStatusCompleted:
		span.SetStatus(codes.Ok, "")
	case TaskStatusFailed:
		span.SetStatus(codes.Error, "subagent task failed")
	case TaskStatusStopped:
		span.SetStatus(codes.Error, "subagent task stopped")
	}
	span.End()
}

func (s *subagentSpanTracker) ensureSubagentSpan(toolUseID, taskID, description string) {
	if toolUseID == "" {
		return
	}
	if agentID := s.toolToAgent[toolUseID]; agentID != "" {
		if _, ok := s.agents[agentID]; ok {
			return
		}
	}

	agentID := taskID
	if agentID == "" {
		agentID = toolUseID
	}

	parentCtx := context.Background()
	if s.rootSpan != nil {
		parentCtx = trace.ContextWithSpan(context.Background(), s.rootSpan)
	}
	if s.toolTracker != nil {
		if toolSpan, ok := s.toolTracker.spans[toolUseID]; ok {
			parentCtx = trace.ContextWithSpan(context.Background(), toolSpan)
		}
	}

	spanName := "ClaudeAgentSDK.Agent"
	if description != "" {
		spanName = "ClaudeAgentSDK." + description
	}

	_, span := s.tracer.Start(parentCtx, spanName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.AgentName, agentID),
		),
	)

	s.agents[agentID] = span
	s.toolToAgent[toolUseID] = agentID
}

func (s *subagentSpanTracker) endAll() {
	for id, span := range s.agents {
		span.SetAttributes(attribute.String("error.type", "subagent_span_abandoned"))
		span.SetStatus(codes.Error, fmt.Sprintf("subagent span abandoned: %s", id))
		span.End()
	}
	s.agents = make(map[string]trace.Span)
	s.toolToAgent = make(map[string]string)
}

// --- attribute extraction (internal) ---

func extractMessageAttributesInternal(span trace.Span, msg Message, outputMsgIndex *int) {
	switch m := msg.(type) {
	case *SystemMessage:
		extractSystemMessageAttrs(span, m)
	case *AssistantMessage:
		extractAssistantMessageAttrs(span, m, outputMsgIndex)
	case *ResultMessage:
		extractResultMessageAttrs(span, m)
	case *TaskStartedMessage:
		if m.SessionID != "" {
			span.SetAttributes(attribute.String(semconv.SessionID, m.SessionID))
		}
		if m.Data != nil {
			if model := extractModelFromMapInternal(m.Data); model != "" {
				span.SetAttributes(attribute.String(semconv.LLMModelName, model))
			}
		}
	case *TaskProgressMessage:
		if m.Usage.TotalTokens > 0 {
			span.SetAttributes(attribute.Int64(semconv.LLMTokenCountTotal, int64(m.Usage.TotalTokens)))
		}
	case *TaskNotificationMessage:
		if m.SessionID != "" {
			span.SetAttributes(attribute.String(semconv.SessionID, m.SessionID))
		}
		if m.Usage != nil && m.Usage.TotalTokens > 0 {
			span.SetAttributes(attribute.Int64(semconv.LLMTokenCountTotal, int64(m.Usage.TotalTokens)))
		}
	}
}

func extractSystemMessageAttrs(span trace.Span, msg *SystemMessage) {
	if msg.Data == nil {
		return
	}
	if sid, ok := msg.Data["session_id"].(string); ok && sid != "" {
		span.SetAttributes(attribute.String(semconv.SessionID, sid))
	}
	if model := extractModelFromMapInternal(msg.Data); model != "" {
		span.SetAttributes(attribute.String(semconv.LLMModelName, model))
	}
}

func extractAssistantMessageAttrs(span trace.Span, msg *AssistantMessage, outputMsgIndex *int) {
	if msg == nil {
		return
	}
	if model := extractModelFromAssistantInternal(msg); model != "" {
		span.SetAttributes(attribute.String(semconv.LLMModelName, model))
	}
	if msg.Usage != nil {
		setUsageFromMapInternal(span, msg.Usage)
	}
	if len(msg.Content) > 0 {
		extractOutputMessagesInternal(span, msg.Content, *outputMsgIndex)
		*outputMsgIndex++
	}
}

func extractResultMessageAttrs(span trace.Span, msg *ResultMessage) {
	if msg.SessionID != "" {
		span.SetAttributes(attribute.String(semconv.SessionID, msg.SessionID))
	}
	if msg.Usage != nil {
		setUsageFromMapInternal(span, msg.Usage)
	}
	if msg.ModelUsage != nil {
		for _, v := range msg.ModelUsage {
			if mu, ok := v.(map[string]any); ok {
				setUsageFromMapInternal(span, mu)
				if model := extractModelFromMapInternal(mu); model != "" {
					span.SetAttributes(attribute.String(semconv.LLMModelName, model))
				}
			}
		}
	}
	if msg.TotalCostUSD != nil && *msg.TotalCostUSD > 0 {
		span.SetAttributes(attribute.Float64(semconv.LLMCostTotal, *msg.TotalCostUSD))
	}
	if msg.Result != "" {
		span.SetAttributes(
			attribute.String(semconv.OutputValue, msg.Result),
			attribute.String(semconv.OutputMimeType, semconv.MimeTypeText),
			attribute.String("gen_ai.completion", msg.Result),
		)
	}
	if msg.Subtype == "success" {
		span.SetStatus(codes.Ok, "")
	} else if msg.Subtype == "error" || msg.IsError {
		errMsg := "agent error"
		if len(msg.Errors) > 0 {
			errMsg = msg.Errors[0]
		}
		span.SetStatus(codes.Error, errMsg)
	}
}

func extractModelFromMapInternal(data map[string]any) string {
	for _, key := range []string{"model", "model_name", "name"} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func extractModelFromAssistantInternal(msg *AssistantMessage) string {
	if msg == nil {
		return ""
	}
	if msg.Model != "" {
		return msg.Model
	}
	if msg.Usage != nil {
		if model := extractModelFromMapInternal(msg.Usage); model != "" {
			return model
		}
	}
	return ""
}

func setUsageFromMapInternal(span trace.Span, usage map[string]any) {
	attrs := make([]attribute.KeyValue, 0, 6)
	if v := safeIntInternal(usage, "input_tokens"); v > 0 {
		attrs = append(attrs, attribute.Int64(semconv.LLMTokenCountPrompt, int64(v)))
	}
	if v := safeIntInternal(usage, "output_tokens"); v > 0 {
		attrs = append(attrs, attribute.Int64(semconv.LLMTokenCountCompletion, int64(v)))
	}
	total := safeIntInternal(usage, "input_tokens") + safeIntInternal(usage, "output_tokens")
	if total > 0 {
		attrs = append(attrs, attribute.Int64(semconv.LLMTokenCountTotal, int64(total)))
	}
	if v := safeIntInternal(usage, "cache_read_input_tokens"); v > 0 {
		attrs = append(attrs, attribute.Int64(semconv.LLMTokenCountPromptDetailsCacheRead, int64(v)))
	}
	cacheWrite := safeIntInternal(usage, "cache_write_input_tokens")
	if cacheWrite == 0 {
		cacheWrite = safeIntInternal(usage, "cache_creation_input_tokens")
	}
	if cacheWrite > 0 {
		attrs = append(attrs, attribute.Int64(semconv.LLMTokenCountPromptDetailsCacheWrite, int64(cacheWrite)))
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

func extractOutputMessagesInternal(span trace.Span, content []ContentBlock, msgIndex int) {
	span.SetAttributes(attribute.String(semconv.LLMOutputMessageRoleKey(msgIndex), "assistant"))
	contentIdx := 0
	toolCallIdx := 0
	for _, block := range content {
		switch b := block.(type) {
		case *TextBlock:
			if b.Text != "" {
				key := semconv.LLMOutputMessages + "." + itoaInternal(msgIndex) + ".message.contents." + itoaInternal(contentIdx) + ".message_content.text"
				span.SetAttributes(attribute.String(key, b.Text))
				contentIdx++
			}
		case *ToolUseBlock:
			span.SetAttributes(
				attribute.String(semconv.LLMOutputMessageToolCallKey(msgIndex, toolCallIdx, semconv.ToolCallID), b.ID),
				attribute.String(semconv.LLMOutputMessageToolCallKey(msgIndex, toolCallIdx, semconv.ToolCallFunctionName), b.Name),
			)
			if b.Input != nil {
				if inputJSON, err := json.Marshal(b.Input); err == nil {
					span.SetAttributes(attribute.String(
						semconv.LLMOutputMessageToolCallKey(msgIndex, toolCallIdx, semconv.ToolCallFunctionArgumentsJSON),
						string(inputJSON),
					))
				}
			}
			toolCallIdx++
		}
	}
}

func safeIntInternal(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

func itoaInternal(i int) string {
	return fmt.Sprintf("%d", i)
}
