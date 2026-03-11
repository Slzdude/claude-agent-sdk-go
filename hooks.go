package claude

import "context"

// HookEvent names.
type HookEvent string

const (
	HookEventPreToolUse         HookEvent = "PreToolUse"
	HookEventPostToolUse        HookEvent = "PostToolUse"
	HookEventPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookEventUserPromptSubmit   HookEvent = "UserPromptSubmit"
	HookEventStop               HookEvent = "Stop"
	HookEventSubagentStop       HookEvent = "SubagentStop"
	HookEventPreCompact         HookEvent = "PreCompact"
	HookEventNotification       HookEvent = "Notification"
	HookEventSubagentStart      HookEvent = "SubagentStart"
	HookEventPermissionRequest  HookEvent = "PermissionRequest"
)

// HookCallback is a function called for hook events.
// input is the raw event data forwarded from the CLI (map[string]any).
// toolUseID is the tool_use_id from the CLI request, or empty string when not applicable.
// Returns the hook output map (may be nil) or an error.
//
// Breaking change from v0.0.x: toolUseID is now a required third parameter,
// matching the Python SDK signature: cb(input, tool_use_id, context).
type HookCallback func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error)

// convertHookOutput converts Python-safe field names to CLI-expected names.
// Specifically: "async_" → "async", "continue_" → "continue".
func convertHookOutput(out map[string]any) map[string]any {
	if out == nil {
		return nil
	}
	result := make(map[string]any, len(out))
	for k, v := range out {
		switch k {
		case "async_":
			result["async"] = v
		case "continue_":
			result["continue"] = v
		default:
			result[k] = v
		}
	}
	return result
}

// HookMatcher associates a tool-name matcher with one or more callbacks.
// Matcher == nil matches all tools.
// Timeout is in seconds; 0 means use the CLI default.
type HookMatcher struct {
	Matcher *string
	Hooks   []HookCallback
	Timeout float64
}
