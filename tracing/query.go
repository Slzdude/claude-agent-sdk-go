package tracing

import (
	"context"
	"fmt"
	"log/slog"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing/semconv"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracedQuery wraps claude.Query with an AGENT root span and automatic
// tool/subagent span tracking.
func TracedQuery(ctx context.Context, prompt string, opts *claude.ClaudeAgentOptions, traceOpts ...TraceOption) (<-chan claude.Message, error) {
	// Check OTel instrumentation suppression (matches Python's _SUPPRESS_INSTRUMENTATION_KEY check)
	if IsInstrumentationSuppressed(ctx) {
		return claude.Query(ctx, prompt, opts)
	}

	cfg := &TraceConfig{}
	for _, opt := range traceOpts {
		opt(cfg)
	}

	tracer := cfg.resolveTracer()

	// Determine span name
	spanName := "ClaudeAgentSDK.Query"
	if cfg.SpanNamer != nil {
		spanName = cfg.SpanNamer(prompt)
	}

	// Determine input attributes
	inputValue, inputMimeType := formatPromptValue(prompt)

	ctx, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(
			semconv.SpanKindKey.String("AGENT"),
			semconv.LLMSystem.String(semconv.LLMSystemAnthropic),
			semconv.InputValue.String(inputValue),
			semconv.InputMimeType.String(inputMimeType),
		),
	)
	span = wrapSpan(span, cfg)

	// Deep copy opts to avoid mutating the caller's options
	if opts == nil {
		opts = &claude.ClaudeAgentOptions{}
	}
	instrumentedOpts := *opts

	// Create tool span tracker and inject hooks
	toolTracker := NewToolSpanTracker(tracer, span, cfg)
	subagentTracker := NewSubagentSpanTracker(tracer, span, toolTracker, cfg)

	// Wire up subagent detection
	toolTracker.SetSubagentCallback(func(toolUseID, agentID, agentType, toolName, parentToolUseID string) {
		subagentTracker.GetOrCreate(toolUseID, agentID, agentType, toolName)
	})

	toolTracker.InjectHooks(&instrumentedOpts)

	// Call the original Query
	msgs, err := claude.Query(ctx, prompt, &instrumentedOpts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}

	// Wrap the message channel to extract attributes
	return wrapMessageChannel(span, msgs, toolTracker, subagentTracker, cfg), nil
}

// wrapMessageChannel creates a new channel that processes messages for tracing.
func wrapMessageChannel(
	rootSpan trace.Span,
	msgs <-chan claude.Message,
	toolTracker *ToolSpanTracker,
	subagentTracker *SubagentSpanTracker,
	cfg *TraceConfig,
) <-chan claude.Message {
	out := make(chan claude.Message, 64)

	go func() {
		defer close(out)
		defer rootSpan.End()
		defer toolTracker.EndAll()
		defer subagentTracker.EndAll()

		// Recover panics during message iteration
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in traced message channel", "recover", r)
				rootSpan.RecordError(fmt.Errorf("panic in message iteration: %v", r))
				rootSpan.SetStatus(codes.Error, fmt.Sprintf("panic: %v", r))
			}
		}()

		outputMsgIndex := 0
		for msg := range msgs {
			// Extract attributes on the appropriate span
			targetSpan := resolveTargetSpan(rootSpan, subagentTracker, msg)
			extractMessageAttributes(targetSpan, msg, &outputMsgIndex)

			// Route subagent messages
			subagentTracker.ProcessMessage(msg)

			// Message-based fallback for tool spans
			updateToolSpansFromMessages(msg, toolTracker)

			// Forward message
			out <- msg
		}
	}()

	return out
}

// resolveTargetSpan returns the subagent span if the message belongs to a subagent,
// otherwise the root span.
func resolveTargetSpan(rootSpan trace.Span, subagentTracker *SubagentSpanTracker, msg claude.Message) trace.Span {
	if parentID := getParentToolUseID(msg); parentID != "" {
		if subSpan := subagentTracker.GetByToolUseID(parentID); subSpan != nil {
			return subSpan
		}
	}
	return rootSpan
}

// getParentToolUseID extracts parent_tool_use_id from a message.
func getParentToolUseID(msg claude.Message) string {
	switch m := msg.(type) {
	case *claude.AssistantMessage:
		return m.ParentToolUseID
	case *claude.UserMessage:
		return m.ParentToolUseID
	case *claude.TaskStartedMessage:
		return m.ToolUseID
	case *claude.TaskProgressMessage:
		return m.ToolUseID
	case *claude.TaskNotificationMessage:
		return m.ToolUseID
	}
	return ""
}

// updateToolSpansFromMessages provides a message-based fallback for tool span tracking.
// Scans message content for tool_use and tool_result blocks.
// Handles both AssistantMessage (carries tool_use) and UserMessage (carries tool_result).
func updateToolSpansFromMessages(msg claude.Message, tracker *ToolSpanTracker) {
	content := extractContentBlocks(msg)
	for _, block := range content {
		switch b := block.(type) {
		case *claude.ToolUseBlock:
			tracker.Start(b.ID, b.Name, b.Input, "")
		case *claude.ToolResultBlock:
			if b.IsError != nil && *b.IsError {
				tracker.EndWithError(b.ToolUseID, fmt.Errorf("tool result error"))
			} else {
				tracker.End(b.ToolUseID, b.Content)
			}
		}
	}
}

// extractContentBlocks extracts content blocks from any message type.
func extractContentBlocks(msg claude.Message) []claude.ContentBlock {
	switch m := msg.(type) {
	case *claude.AssistantMessage:
		return m.Content
	case *claude.UserMessage:
		if content, ok := m.Content.([]claude.ContentBlock); ok {
			return content
		}
	}
	return nil
}
