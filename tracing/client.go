package tracing

import (
	"context"
	"sync"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TracedClient wraps ClaudeSDKClient with per-turn AGENT spans and
// automatic tool/subagent span tracking.
type TracedClient struct {
	client     *claude.ClaudeSDKClient
	cfg        *TraceConfig
	tracer     trace.Tracer
	lastPrompt string
	mu         sync.Mutex
	currentToolSpanTracker     *ToolSpanTracker
	currentSubagentSpanTracker *SubagentSpanTracker
	currentSpan                trace.Span
}

// NewTracedClient creates a TracedClient wrapping an existing ClaudeSDKClient.
func NewTracedClient(client *claude.ClaudeSDKClient, traceOpts ...TraceOption) *TracedClient {
	cfg := &TraceConfig{}
	for _, opt := range traceOpts {
		opt(cfg)
	}

	return &TracedClient{
		client: client,
		cfg:    cfg,
		tracer: cfg.resolveTracer(),
	}
}

// Query wraps ClaudeSDKClient.Query, recording the prompt for the next ReceiveResponse span.
func (c *TracedClient) Query(ctx context.Context, prompt string) error {
	c.mu.Lock()
	c.lastPrompt = prompt
	c.mu.Unlock()
	return c.client.Query(ctx, prompt)
}

// ReceiveResponse wraps ClaudeSDKClient.ReceiveResponse with a per-turn AGENT span.
func (c *TracedClient) ReceiveResponse(ctx context.Context) <-chan claude.Message {
	if IsSuppressed(ctx) {
		return c.client.ReceiveResponse(ctx)
	}

	spanName := "ClaudeAgentSDK.ReceiveResponse"

	c.mu.Lock()
	prompt := c.lastPrompt
	c.lastPrompt = ""
	span := c.currentSpan
	toolTracker := c.currentToolSpanTracker
	subagentTracker := c.currentSubagentSpanTracker
	c.mu.Unlock()

	if toolTracker != nil {
		toolTracker.EndAll()
	}
	if subagentTracker != nil {
		subagentTracker.EndAll()
	}
	if span != nil {
		span.End()
	}

	inputValue, inputMimeType := formatPromptValue(prompt)

	// Set model name so Langfuse treats this as a "generation" span
	modelName := "claude"
	if c.client != nil {
		if info := c.client.GetServerInfo(); info != nil {
			if m, ok := info["model"].(string); ok && m != "" {
				modelName = m
			}
		}
	}

	tracer := c.tracer
	ctx, newSpan := tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.LLMSystem, semconv.LLMSystemAnthropic),
			attribute.String(semconv.LLMModelName, modelName),
			attribute.String("gen_ai.request.model", modelName),
			attribute.String(semconv.InputValue, inputValue),
			attribute.String(semconv.InputMimeType, inputMimeType),
		),
	)
	newSpan = wrapSpan(newSpan, c.cfg)
	ApplyContextAttributes(ctx, newSpan)

	newToolTracker := NewToolSpanTracker(tracer, newSpan, c.cfg)
	newSubagentTracker := NewSubagentSpanTracker(tracer, newSpan, newToolTracker, c.cfg)

	newToolTracker.SetSubagentCallback(func(toolUseID, agentID, agentType, toolName, parentToolUseID string) {
		newSubagentTracker.GetOrCreate(toolUseID, agentID, agentType, toolName)
	})
	newToolTracker.SetParentContextResolver(func(parentToolUseID string) context.Context {
		if subSpan := newSubagentTracker.GetByToolUseID(parentToolUseID); subSpan != nil {
			return trace.ContextWithSpan(context.Background(), subSpan)
		}
		return nil
	})

	c.mu.Lock()
	c.currentSpan = newSpan
	c.currentToolSpanTracker = newToolTracker
	c.currentSubagentSpanTracker = newSubagentTracker
	c.mu.Unlock()

	msgs := c.client.ReceiveResponse(ctx)

	return wrapMessageChannel(ctx, newSpan, msgs, newToolTracker, newSubagentTracker, c.cfg)
}

// ReceiveMessages wraps ClaudeSDKClient.ReceiveMessages.
func (c *TracedClient) ReceiveMessages(ctx context.Context) <-chan claude.Message {
	if IsSuppressed(ctx) {
		return c.client.ReceiveMessages(ctx)
	}

	tracer := c.tracer
	ctx, span := tracer.Start(ctx, "ClaudeAgentSDK.ReceiveMessages",
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.LLMSystem, semconv.LLMSystemAnthropic),
			attribute.String(semconv.LLMModelName, "claude"),
			attribute.String("gen_ai.request.model", "claude"),
		),
	)
	span = wrapSpan(span, c.cfg)
	ApplyContextAttributes(ctx, span)

	toolTracker := NewToolSpanTracker(tracer, span, c.cfg)
	subagentTracker := NewSubagentSpanTracker(tracer, span, toolTracker, c.cfg)

	toolTracker.SetSubagentCallback(func(toolUseID, agentID, agentType, toolName, parentToolUseID string) {
		subagentTracker.GetOrCreate(toolUseID, agentID, agentType, toolName)
	})
	toolTracker.SetParentContextResolver(func(parentToolUseID string) context.Context {
		if subSpan := subagentTracker.GetByToolUseID(parentToolUseID); subSpan != nil {
			return trace.ContextWithSpan(context.Background(), subSpan)
		}
		return nil
	})

	msgs := c.client.ReceiveMessages(ctx)

	return wrapMessageChannel(ctx, span, msgs, toolTracker, subagentTracker, c.cfg)
}

// Close ends all pending spans and closes the underlying client.
func (c *TracedClient) Close() error {
	c.mu.Lock()
	toolTracker := c.currentToolSpanTracker
	subagentTracker := c.currentSubagentSpanTracker
	span := c.currentSpan
	c.mu.Unlock()

	if toolTracker != nil {
		toolTracker.EndAll()
	}
	if subagentTracker != nil {
		subagentTracker.EndAll()
	}
	if span != nil {
		span.SetStatus(codes.Ok, "")
		span.End()
	}

	return c.client.Close()
}

// Client returns the underlying ClaudeSDKClient for direct access.
func (c *TracedClient) Client() *claude.ClaudeSDKClient {
	return c.client
}
