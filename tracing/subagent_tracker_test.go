package tracing

import (
	"context"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing/semconv"
	"go.opentelemetry.io/otel/codes"
)

func TestSubagentSpanTracker_GetOrCreate(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	toolTracker := NewToolSpanTracker(tracer, rootSpan, nil)
	subagentTracker := NewSubagentSpanTracker(tracer, rootSpan, toolTracker, nil)

	span := subagentTracker.GetOrCreate("tool_task_1", "agent_abc", "task", "Task")
	if span == nil {
		t.Fatal("expected non-nil span")
	}

	span2 := subagentTracker.GetOrCreate("tool_task_1", "agent_abc", "task", "Task")
	if span != span2 {
		t.Error("expected same span for same agentID")
	}

	subagentTracker.EndAll()
	rootSpan.End()

	spans := exporter.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "ClaudeAgentSDK.Task" {
			found = true
			attrs := attrMap(s.Attributes)
			if attrs[string(semconv.AgentName)] != "agent_abc" {
				t.Errorf("agent name = %q, want agent_abc", attrs[string(semconv.AgentName)])
			}
			if attrs[string(semconv.SpanKindKey)] != "AGENT" {
				t.Errorf("span kind = %q, want AGENT", attrs[string(semconv.SpanKindKey)])
			}
		}
	}
	if !found {
		t.Error("expected ClaudeAgentSDK.Task span")
	}
}

func TestSubagentSpanTracker_GetByToolUseID(t *testing.T) {
	tp, _ := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	toolTracker := NewToolSpanTracker(tracer, rootSpan, nil)
	subagentTracker := NewSubagentSpanTracker(tracer, rootSpan, toolTracker, nil)

	subagentTracker.GetOrCreate("tool_task_1", "agent_1", "task", "Task")

	span := subagentTracker.GetByToolUseID("tool_task_1")
	if span == nil {
		t.Error("expected non-nil span for tool_task_1")
	}

	span2 := subagentTracker.GetByToolUseID("nonexistent")
	if span2 != nil {
		t.Error("expected nil span for nonexistent tool use ID")
	}
}

func TestSubagentSpanTracker_TaskNotification(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	toolTracker := NewToolSpanTracker(tracer, rootSpan, nil)
	subagentTracker := NewSubagentSpanTracker(tracer, rootSpan, toolTracker, nil)

	subagentTracker.GetOrCreate("tool_task_1", "agent_1", "task", "Task")

	subagentTracker.ProcessMessage(&claude.TaskNotificationMessage{
		TaskID:    "task_1",
		Status:    claude.TaskStatusCompleted,
		ToolUseID: "tool_task_1",
	})

	rootSpan.End()

	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "ClaudeAgentSDK.Task" {
			if s.Status.Code != codes.Ok {
				t.Errorf("subagent span status = %v, want OK", s.Status.Code)
			}
		}
	}
}

func TestSubagentSpanTracker_MultipleSubagents(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	toolTracker := NewToolSpanTracker(tracer, rootSpan, nil)
	subagentTracker := NewSubagentSpanTracker(tracer, rootSpan, toolTracker, nil)

	subagentTracker.GetOrCreate("tool_a", "agent_1", "task", "Task")
	subagentTracker.GetOrCreate("tool_b", "agent_2", "task", "Task")

	subagentTracker.ProcessMessage(&claude.TaskNotificationMessage{
		TaskID:    "task_1",
		Status:    claude.TaskStatusCompleted,
		ToolUseID: "tool_a",
	})
	subagentTracker.ProcessMessage(&claude.TaskNotificationMessage{
		TaskID:    "task_2",
		Status:    claude.TaskStatusFailed,
		ToolUseID: "tool_b",
	})

	rootSpan.End()

	spans := exporter.GetSpans()
	taskSpans := 0
	for _, s := range spans {
		if s.Name == "ClaudeAgentSDK.Task" {
			taskSpans++
		}
	}
	if taskSpans != 2 {
		t.Errorf("expected 2 Task spans, got %d", taskSpans)
	}
}

func TestSubagentSpanTracker_EndAllAbandoned(t *testing.T) {
	tp, exporter := setupTestTracer()
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	_, rootSpan := tracer.Start(context.Background(), "root")

	toolTracker := NewToolSpanTracker(tracer, rootSpan, nil)
	subagentTracker := NewSubagentSpanTracker(tracer, rootSpan, toolTracker, nil)

	subagentTracker.GetOrCreate("tool_a", "agent_1", "task", "Task")
	subagentTracker.GetOrCreate("tool_b", "agent_2", "task", "Task")

	subagentTracker.EndAll()
	rootSpan.End()

	spans := exporter.GetSpans()
	for _, s := range spans {
		if s.Name == "ClaudeAgentSDK.Task" {
			if s.Status.Code != codes.Error {
				t.Errorf("abandoned span %q should have ERROR status", s.Name)
			}
			attrs := attrMap(s.Attributes)
			if attrs[string(semconv.ErrorType)] != semconv.ErrorTypeSubagentSpanAbandoned {
				t.Errorf("expected abandoned error type, got %q", attrs[string(semconv.ErrorType)])
			}
		}
	}
}
