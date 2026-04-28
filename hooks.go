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

// -----------------------------------------------------------------------
// Strongly-typed hook input types
// -----------------------------------------------------------------------

// BaseHookInput is the common fields for all hook inputs.
type BaseHookInput struct {
	SessionID       string         `json:"session_id"`
	TranscriptPath  string         `json:"transcript_path"`
	CWD             string         `json:"cwd"`
	PermissionMode  string         `json:"permission_mode,omitempty"`
}

// SubagentContextMixin provides agent context for tool lifecycle hooks.
type SubagentContextMixin struct {
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// PreToolUseHookInput is the input for PreToolUse hooks.
type PreToolUseHookInput struct {
	BaseHookInput
	SubagentContextMixin
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	ToolUseID     string         `json:"tool_use_id"`
}

// PostToolUseHookInput is the input for PostToolUse hooks.
type PostToolUseHookInput struct {
	BaseHookInput
	SubagentContextMixin
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	ToolResponse  any            `json:"tool_response"`
	ToolUseID     string         `json:"tool_use_id"`
}

// PostToolUseFailureHookInput is the input for PostToolUseFailure hooks.
type PostToolUseFailureHookInput struct {
	BaseHookInput
	SubagentContextMixin
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	ToolUseID     string         `json:"tool_use_id"`
	Error         string         `json:"error"`
	IsInterrupt   *bool          `json:"is_interrupt,omitempty"`
}

// UserPromptSubmitHookInput is the input for UserPromptSubmit hooks.
type UserPromptSubmitHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	Prompt        string `json:"prompt"`
}

// StopHookInput is the input for Stop hooks.
type StopHookInput struct {
	BaseHookInput
	HookEventName  string `json:"hook_event_name"`
	StopHookActive bool   `json:"stop_hook_active"`
}

// SubagentStopHookInput is the input for SubagentStop hooks.
type SubagentStopHookInput struct {
	BaseHookInput
	HookEventName       string `json:"hook_event_name"`
	StopHookActive      bool   `json:"stop_hook_active"`
	AgentID             string `json:"agent_id"`
	AgentTranscriptPath string `json:"agent_transcript_path"`
	AgentType           string `json:"agent_type"`
}

// PreCompactHookInput is the input for PreCompact hooks.
type PreCompactHookInput struct {
	BaseHookInput
	HookEventName     string `json:"hook_event_name"`
	Trigger           string `json:"trigger"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
}

// NotificationHookInput is the input for Notification hooks.
type NotificationHookInput struct {
	BaseHookInput
	HookEventName    string `json:"hook_event_name"`
	Message          string `json:"message"`
	Title            string `json:"title,omitempty"`
	NotificationType string `json:"notification_type"`
}

// SubagentStartHookInput is the input for SubagentStart hooks.
type SubagentStartHookInput struct {
	BaseHookInput
	HookEventName string `json:"hook_event_name"`
	AgentID       string `json:"agent_id"`
	AgentType     string `json:"agent_type"`
}

// PermissionRequestHookInput is the input for PermissionRequest hooks.
type PermissionRequestHookInput struct {
	BaseHookInput
	SubagentContextMixin
	HookEventName        string         `json:"hook_event_name"`
	ToolName             string         `json:"tool_name"`
	ToolInput            map[string]any `json:"tool_input"`
	PermissionSuggestions []any          `json:"permission_suggestions,omitempty"`
}

// -----------------------------------------------------------------------
// Strongly-typed hook output types
// -----------------------------------------------------------------------

// SyncHookJSONOutput is the output for synchronous hook callbacks.
type SyncHookJSONOutput struct {
	Continue          *bool  `json:"continue,omitempty"`
	SuppressOutput    *bool  `json:"suppressOutput,omitempty"`
	StopReason        string `json:"stopReason,omitempty"`
	Decision          string `json:"decision,omitempty"`
	SystemMessage     string `json:"systemMessage,omitempty"`
	Reason            string `json:"reason,omitempty"`
	HookSpecificOutput any   `json:"hookSpecificOutput,omitempty"`
}

// AsyncHookJSONOutput is the output for asynchronous hook callbacks.
type AsyncHookJSONOutput struct {
	Async        bool    `json:"async"`
	AsyncTimeout float64 `json:"asyncTimeout,omitempty"`
}

// PreToolUseHookSpecificOutput is the hook-specific output for PreToolUse.
type PreToolUseHookSpecificOutput struct {
	HookEventName              string         `json:"hookEventName"`
	PermissionDecision         string         `json:"permissionDecision,omitempty"`
	PermissionDecisionReason   string         `json:"permissionDecisionReason,omitempty"`
	UpdatedInput               map[string]any `json:"updatedInput,omitempty"`
	AdditionalContext          string         `json:"additionalContext,omitempty"`
}

// PostToolUseHookSpecificOutput is the hook-specific output for PostToolUse.
type PostToolUseHookSpecificOutput struct {
	HookEventName       string         `json:"hookEventName"`
	AdditionalContext   string         `json:"additionalContext,omitempty"`
	UpdatedMCPToolOutput map[string]any `json:"updatedMCPToolOutput,omitempty"`
}

// PostToolUseFailureHookSpecificOutput is the hook-specific output for PostToolUseFailure.
type PostToolUseFailureHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// UserPromptSubmitHookSpecificOutput is the hook-specific output for UserPromptSubmit.
type UserPromptSubmitHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// NotificationHookSpecificOutput is the hook-specific output for Notification.
type NotificationHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// SubagentStartHookSpecificOutput is the hook-specific output for SubagentStart.
type SubagentStartHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// PermissionRequestHookSpecificOutput is the hook-specific output for PermissionRequest.
type PermissionRequestHookSpecificOutput struct {
	HookEventName string         `json:"hookEventName"`
	Decision      map[string]any `json:"decision,omitempty"`
}
