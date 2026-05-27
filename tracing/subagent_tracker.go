package tracing

import (
	"context"
	"fmt"
	"sync"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	semconv "github.com/Arize-ai/openinference/go/openinference-semantic-conventions"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SubagentSpanTracker manages nested AGENT spans for sub-agent tasks.
type SubagentSpanTracker struct {
	tracer      trace.Tracer
	rootSpan    trace.Span
	toolTracker *ToolSpanTracker
	cfg         *TraceConfig
	mu          sync.Mutex
	agents      map[string]trace.Span
	toolToAgent map[string]string
}

// NewSubagentSpanTracker creates a new subagent tracker.
func NewSubagentSpanTracker(tracer trace.Tracer, rootSpan trace.Span, toolTracker *ToolSpanTracker, cfg *TraceConfig) *SubagentSpanTracker {
	return &SubagentSpanTracker{
		tracer:      tracer,
		rootSpan:    rootSpan,
		toolTracker: toolTracker,
		cfg:         cfg,
		agents:      make(map[string]trace.Span),
		toolToAgent: make(map[string]string),
	}
}

// GetOrCreate creates or returns a nested AGENT span for a subagent.
func (s *SubagentSpanTracker) GetOrCreate(toolUseID, agentID, agentType, toolName string) trace.Span {
	s.mu.Lock()
	defer s.mu.Unlock()

	if span, ok := s.agents[agentID]; ok {
		return span
	}

	parentCtx := trace.ContextWithSpan(context.TODO(), s.rootSpan)
	if toolSpan, ok := s.toolTracker.GetInFlightSpan(toolUseID); ok {
		parentCtx = trace.ContextWithSpan(context.TODO(), toolSpan)
	}

	spanName := fmt.Sprintf("ClaudeAgentSDK.%s", toolName)
	_, span := s.tracer.Start(parentCtx, spanName,
		trace.WithAttributes(
			attribute.String(semconv.OpenInferenceSpanKind, semconv.SpanKindAgent),
			attribute.String(semconv.AgentName, agentID),
		),
	)
	span = wrapSpan(span, s.cfg)

	s.agents[agentID] = span
	s.toolToAgent[toolUseID] = agentID
	return span
}

// GetByToolUseID returns the subagent span for a given tool use ID, or nil.
func (s *SubagentSpanTracker) GetByToolUseID(toolUseID string) trace.Span {
	s.mu.Lock()
	defer s.mu.Unlock()
	agentID := s.toolToAgent[toolUseID]
	if agentID == "" {
		return nil
	}
	return s.agents[agentID]
}

// ProcessMessage routes messages to the correct subagent span.
func (s *SubagentSpanTracker) ProcessMessage(msg claude.Message) {
	switch m := msg.(type) {
	case *claude.TaskNotificationMessage:
		s.handleTaskNotification(m)
	case *claude.TaskStartedMessage:
		// Subagent span should already exist from hook detection.
	case *claude.TaskProgressMessage:
		// No action needed for span lifecycle.
	}
}

// handleTaskNotification ends a subagent span when the task finishes.
func (s *SubagentSpanTracker) handleTaskNotification(msg *claude.TaskNotificationMessage) {
	s.mu.Lock()
	agentID := s.toolToAgent[msg.ToolUseID]
	if agentID == "" {
		for aid := range s.agents {
			agentID = aid
			break
		}
	}

	span, ok := s.agents[agentID]
	if ok {
		delete(s.agents, agentID)
		delete(s.toolToAgent, msg.ToolUseID)
	}
	s.mu.Unlock()

	if !ok {
		return
	}

	switch msg.Status {
	case claude.TaskStatusCompleted:
		span.SetStatus(codes.Ok, "")
	case claude.TaskStatusFailed:
		span.SetStatus(codes.Error, "subagent task failed")
	case claude.TaskStatusStopped:
		span.SetStatus(codes.Error, "subagent task stopped")
	}
	span.End()
}

// EndAll ends all in-flight subagent spans as abandoned.
func (s *SubagentSpanTracker) EndAll() {
	s.mu.Lock()
	agents := make(map[string]trace.Span, len(s.agents))
	for k, v := range s.agents {
		agents[k] = v
	}
	s.agents = make(map[string]trace.Span)
	s.toolToAgent = make(map[string]string)
	s.mu.Unlock()

	for id, span := range agents {
		span.SetAttributes(attribute.String("error.type", "subagent_span_abandoned"))
		span.SetStatus(codes.Error, fmt.Sprintf("subagent span abandoned: %s", id))
		span.End()
	}
}
