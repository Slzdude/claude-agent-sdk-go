package tracing

import (
	"context"
	"fmt"
	"log/slog"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracedQuery wraps claude.Query with an AGENT root span and automatic
// tool/subagent span tracking.
func TracedQuery(ctx context.Context, prompt string, opts *claude.ClaudeAgentOptions, traceOpts ...TraceOption) (<-chan claude.Message, error) {
	if IsSuppressed(ctx) {
		return claude.Query(ctx, prompt, opts)
	}

	cfg := &TraceConfig{}
	for _, opt := range traceOpts {
		opt(cfg)
	}

	tracer := cfg.resolveTracer()

	spanName := "ClaudeAgentSDK.Query"
	if cfg.SpanNamer != nil {
		spanName = cfg.SpanNamer(prompt)
	}

	inputValue, inputMimeType := formatPromptValue(prompt)

	// Determine model name: from opts.Model, or default to "claude"
	// Langfuse requires a model attribute to treat the span as a "generation"
	// (which enables Input/Output display). Set it at creation time.
	modelName := "claude"
	if opts != nil && opts.Model != "" {
		modelName = opts.Model
	}

	ctx, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.LLMSystem, semconv.LLMSystemAnthropic),
			attribute.String(semconv.LLMModelName, modelName),
			attribute.String("gen_ai.request.model", modelName),
			attribute.String(semconv.InputValue, inputValue),
			attribute.String(semconv.InputMimeType, inputMimeType),
		),
	)
	span = wrapSpan(span, cfg)
	ApplyContextAttributes(ctx, span)

	if opts == nil {
		opts = &claude.ClaudeAgentOptions{}
	}
	instrumentedOpts := *opts

	toolTracker := NewToolSpanTracker(tracer, span, cfg)
	subagentTracker := NewSubagentSpanTracker(tracer, span, toolTracker, cfg)

	toolTracker.SetSubagentCallback(func(toolUseID, agentID, agentType, toolName, parentToolUseID string) {
		subagentTracker.GetOrCreate(toolUseID, agentID, agentType, toolName)
	})

	toolTracker.InjectHooks(&instrumentedOpts)

	msgs, err := claude.Query(ctx, prompt, &instrumentedOpts)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}

	return wrapMessageChannel(ctx, span, msgs, toolTracker, subagentTracker, cfg), nil
}

func wrapMessageChannel(
	ctx context.Context,
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

		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in traced message channel", "recover", r)
				rootSpan.RecordError(fmt.Errorf("panic in message iteration: %v", r))
				rootSpan.SetStatus(codes.Error, fmt.Sprintf("panic: %v", r))
			}
		}()

		outputMsgIndex := 0
		for msg := range msgs {
			// ProcessMessage must run BEFORE resolveTargetSpan, because
			// TaskStartedMessage creates the subagent span that subsequent
			// messages need to route to.
			subagentTracker.ProcessMessage(msg)

			targetSpan := resolveTargetSpan(rootSpan, subagentTracker, msg)
			extractMessageAttributes(targetSpan, msg, &outputMsgIndex)

			updateToolSpansFromMessages(ctx, msg, toolTracker)

			out <- msg
		}
	}()

	return out
}

func resolveTargetSpan(rootSpan trace.Span, subagentTracker *SubagentSpanTracker, msg claude.Message) trace.Span {
	if parentID := getParentToolUseID(msg); parentID != "" {
		if subSpan := subagentTracker.GetByToolUseID(parentID); subSpan != nil {
			return subSpan
		}
	}
	return rootSpan
}

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

func updateToolSpansFromMessages(ctx context.Context, msg claude.Message, tracker *ToolSpanTracker) {
	content := extractContentBlocks(msg)
	for _, block := range content {
		switch b := block.(type) {
		case *claude.ToolUseBlock:
			tracker.Start(ctx, b.ID, b.Name, b.Input, "")
		case *claude.ToolResultBlock:
			if b.IsError != nil && *b.IsError {
				tracker.EndWithError(b.ToolUseID, fmt.Errorf("tool result error"))
			} else {
				tracker.End(b.ToolUseID, b.Content)
			}
		}
	}
}

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
