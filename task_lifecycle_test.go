package claude

import (
	"testing"
)

func TestTrackTaskLifecycle_AddAndDiscard(t *testing.T) {
	q := &queryProto{}

	// task_started adds to inflightTasks
	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-1",
		"task_type": "local_agent",
	})
	if !q.inflightTasks["task-1"] {
		t.Error("task-1 should be in inflightTasks")
	}

	// task_notification discards
	q.trackTaskLifecycle(map[string]any{
		"subtype": "task_notification",
		"task_id": "task-1",
	})
	if q.inflightTasks["task-1"] {
		t.Error("task-1 should be removed from inflightTasks")
	}
}

func TestTrackTaskLifecycle_NonDeferringTypeIgnored(t *testing.T) {
	q := &queryProto{}

	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-shell",
		"task_type": "background_shell",
	})
	if q.inflightTasks["task-shell"] {
		t.Error("background_shell should not be tracked")
	}
}

func TestTrackTaskLifecycle_TerminalTaskUpdated(t *testing.T) {
	q := &queryProto{}

	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-1",
		"task_type": "local_workflow",
	})
	if !q.inflightTasks["task-1"] {
		t.Fatal("task-1 should be in inflightTasks")
	}

	// task_updated with terminal status
	q.trackTaskLifecycle(map[string]any{
		"subtype": "task_updated",
		"task_id": "task-1",
		"patch":   map[string]any{"status": "completed"},
	})
	if q.inflightTasks["task-1"] {
		t.Error("task-1 should be removed after terminal task_updated")
	}
}

func TestTrackTaskLifecycle_NonTerminalTaskUpdatedIgnored(t *testing.T) {
	q := &queryProto{}

	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-1",
		"task_type": "local_agent",
	})

	q.trackTaskLifecycle(map[string]any{
		"subtype": "task_updated",
		"task_id": "task-1",
		"patch":   map[string]any{"status": "running"},
	})
	if !q.inflightTasks["task-1"] {
		t.Error("task-1 should still be in inflightTasks for non-terminal status")
	}
}

func TestDeferringTaskTypes(t *testing.T) {
	if !deferringTaskTypes["local_agent"] {
		t.Error("local_agent should be deferring")
	}
	if !deferringTaskTypes["local_workflow"] {
		t.Error("local_workflow should be deferring")
	}
	if deferringTaskTypes["background_shell"] {
		t.Error("background_shell should not be deferring")
	}
}
