package claude

import "context"

// PermissionMode controls how Claude handles permission requests.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModeAuto              PermissionMode = "auto"
)

// SdkBeta names feature flags passed to the --betas CLI arg.
type SdkBeta string

const (
	SdkBetaContext1M SdkBeta = "context-1m-2025-08-07"
)

// SettingSource selects which settings files the CLI loads.
type SettingSource string

const (
	SettingSourceUser    SettingSource = "user"
	SettingSourceProject SettingSource = "project"
	SettingSourceLocal   SettingSource = "local"
)

// EffortLevel sets the reasoning depth for extended thinking.
type EffortLevel string

const (
	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
	EffortXHigh  EffortLevel = "xhigh" // Opus 4.7 only; falls back to "high" on other models
	EffortMax    EffortLevel = "max"
)

// AgentDefinition describes a custom sub-agent.
type AgentDefinition struct {
	Description     string   `json:"description"`
	Prompt          string   `json:"prompt"`
	Tools           []string `json:"tools,omitempty"`
	DisallowedTools []string `json:"disallowedTools,omitempty"`
	Model           string   `json:"model,omitempty"` // "sonnet", "opus", "haiku", "inherit", or full model ID
	Skills          []string `json:"skills,omitempty"`
	Memory          string   `json:"memory,omitempty"` // "user", "project", "local"
	// MCPServers is a list of server name strings or inline {name: config} dicts.
	// Matches Python's list[str | dict[str, Any]].
	MCPServers     []any           `json:"mcpServers,omitempty"`
	InitialPrompt  string          `json:"initialPrompt,omitempty"`
	MaxTurns       *int            `json:"maxTurns,omitempty"`
	Background     *bool           `json:"background,omitempty"`
	Effort         EffortLevel     `json:"effort,omitempty"`
	PermissionMode *PermissionMode `json:"permissionMode,omitempty"`
}

// -----------------------------------------------------------------------
// Permission types
// -----------------------------------------------------------------------

// PermissionUpdateType enumerates permission mutation operations.
type PermissionUpdateType string

const (
	PermissionUpdateAddRules          PermissionUpdateType = "addRules"
	PermissionUpdateReplaceRules      PermissionUpdateType = "replaceRules"
	PermissionUpdateRemoveRules       PermissionUpdateType = "removeRules"
	PermissionUpdateSetMode           PermissionUpdateType = "setMode"
	PermissionUpdateAddDirectories    PermissionUpdateType = "addDirectories"
	PermissionUpdateRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionUpdateDestination selects the settings tier to modify.
type PermissionUpdateDestination string

const (
	PermissionDestUserSettings    PermissionUpdateDestination = "userSettings"
	PermissionDestProjectSettings PermissionUpdateDestination = "projectSettings"
	PermissionDestLocalSettings   PermissionUpdateDestination = "localSettings"
	PermissionDestSession         PermissionUpdateDestination = "session"
)

// PermissionBehavior is the decision for a permission rule.
type PermissionBehavior string

const (
	PermissionBehaviorAllow PermissionBehavior = "allow"
	PermissionBehaviorDeny  PermissionBehavior = "deny"
	PermissionBehaviorAsk   PermissionBehavior = "ask"
)

// PermissionRuleValue is a single permission rule.
type PermissionRuleValue struct {
	ToolName    string  `json:"toolName"`
	RuleContent *string `json:"ruleContent,omitempty"`
}

// PermissionUpdate describes a mutation to the permission system.
type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Rules       []PermissionRuleValue       `json:"rules,omitempty"`
	Behavior    PermissionBehavior          `json:"behavior,omitempty"`
	Mode        PermissionMode              `json:"mode,omitempty"`
	Directories []string                    `json:"directories,omitempty"`
	Destination PermissionUpdateDestination `json:"destination,omitempty"`
}

// ToolPermissionContext carries contextual data passed to CanUseTool callbacks.
type ToolPermissionContext struct {
	Signal      any
	Suggestions []PermissionUpdate
	ToolUseID   string // Unique identifier for this specific tool call
	AgentID     string // Sub-agent ID if running within a sub-agent context
	// BlockedPath is the file path that triggered the permission request.
	BlockedPath string `json:"blocked_path,omitempty"`
	// DecisionReason explains why this permission request was triggered.
	DecisionReason string `json:"decision_reason,omitempty"`
	// Title is the full permission prompt sentence (e.g. "Claude wants to read foo.txt").
	Title string `json:"title,omitempty"`
	// DisplayName is a short noun phrase for the tool action (e.g. "Read file").
	DisplayName string `json:"display_name,omitempty"`
	// Description is a human-readable subtitle for the permission UI.
	Description string `json:"description,omitempty"`
}

// PermissionResult is implemented by PermissionResultAllow and PermissionResultDeny.
type PermissionResult interface {
	permissionResult()
}

// PermissionResultAllow grants permission, optionally modifying the tool input.
type PermissionResultAllow struct {
	UpdatedInput       map[string]any     `json:"updatedInput,omitempty"`
	UpdatedPermissions []PermissionUpdate `json:"updatedPermissions,omitempty"`
}

func (r *PermissionResultAllow) permissionResult() {}

// PermissionResultDeny rejects permission with an optional message.
type PermissionResultDeny struct {
	Message   string `json:"message,omitempty"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

func (r *PermissionResultDeny) permissionResult() {}

// CanUseTool is the callback type for tool permission decisions.
type CanUseTool func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error)

// -----------------------------------------------------------------------
// Content blocks
// -----------------------------------------------------------------------

// ContentBlock is implemented by all content block types.
type ContentBlock interface {
	contentBlockType() string
}

// TextBlock contains a plain-text response segment.
type TextBlock struct {
	Text string `json:"text"`
}

func (b *TextBlock) contentBlockType() string { return "text" }

// ThinkingBlock contains an extended-thinking segment.
type ThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

func (b *ThinkingBlock) contentBlockType() string { return "thinking" }

// ToolUseBlock represents a tool invocation by the model.
type ToolUseBlock struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (b *ToolUseBlock) contentBlockType() string { return "tool_use" }

// ToolResultBlock carries the result of a tool invocation.
type ToolResultBlock struct {
	ToolUseID string `json:"tool_use_id"`
	// Content may be a string or []map[string]any (MCP-style content array).
	Content any   `json:"content,omitempty"`
	IsError *bool `json:"is_error,omitempty"`
}

func (b *ToolResultBlock) contentBlockType() string { return "tool_result" }

// ServerToolName identifies server-side tools executed by the API.
type ServerToolName string

const (
	ServerToolAdvisor            ServerToolName = "advisor"
	ServerToolWebSearch          ServerToolName = "web_search"
	ServerToolWebFetch           ServerToolName = "web_fetch"
	ServerToolCodeExecution      ServerToolName = "code_execution"
	ServerToolBashCodeExecution  ServerToolName = "bash_code_execution"
	ServerToolTextEditorCodeExec ServerToolName = "text_editor_code_execution"
	ServerToolSearchToolRegex    ServerToolName = "tool_search_tool_regex"
	ServerToolSearchToolBM25     ServerToolName = "tool_search_tool_bm25"
)

// ServerToolUseBlock represents a server-side tool invocation (e.g. advisor, web_search).
type ServerToolUseBlock struct {
	ID    string         `json:"id"`
	Name  ServerToolName `json:"name"`
	Input map[string]any `json:"input"`
}

func (b *ServerToolUseBlock) contentBlockType() string { return "server_tool_use" }

// ServerToolResultBlock carries the result of a server-side tool call.
type ServerToolResultBlock struct {
	ToolUseID string         `json:"tool_use_id"`
	Content   map[string]any `json:"content"`
}

func (b *ServerToolResultBlock) contentBlockType() string { return "advisor_tool_result" }

// -----------------------------------------------------------------------
// Message types
// -----------------------------------------------------------------------

// Message is implemented by all message types emitted by the SDK.
type Message interface {
	messageType() string
}

// UserMessage is a user-role message.
type UserMessage struct {
	// Content is either a plain string or a slice of ContentBlock.
	Content         any            `json:"content"`
	UUID            string         `json:"uuid,omitempty"`
	ParentToolUseID string         `json:"parent_tool_use_id,omitempty"`
	ToolUseResult   map[string]any `json:"tool_use_result,omitempty"`
}

func (m *UserMessage) messageType() string { return "user" }

// AssistantMessageErrorType enumerates assistant-level errors.
type AssistantMessageErrorType string

const (
	AssistantErrorAuthFailed AssistantMessageErrorType = "authentication_failed"
	AssistantErrorBilling    AssistantMessageErrorType = "billing_error"
	AssistantErrorRateLimit  AssistantMessageErrorType = "rate_limit"
	AssistantErrorInvalidReq AssistantMessageErrorType = "invalid_request"
	AssistantErrorServer     AssistantMessageErrorType = "server_error"
	AssistantErrorUnknown    AssistantMessageErrorType = "unknown"
)

// AssistantMessage is a model-role message.
type AssistantMessage struct {
	Content         []ContentBlock            `json:"content"`
	Model           string                    `json:"model"`
	ParentToolUseID string                    `json:"parent_tool_use_id,omitempty"`
	Error           AssistantMessageErrorType `json:"error,omitempty"`
	Usage           map[string]any            `json:"usage,omitempty"`
	MessageID       string                    `json:"message_id,omitempty"`
	StopReason      string                    `json:"stop_reason,omitempty"`
	SessionID       string                    `json:"session_id,omitempty"`
	UUID            string                    `json:"uuid,omitempty"`
}

func (m *AssistantMessage) messageType() string { return "assistant" }

// SystemMessage is a system-level metadata message.
type SystemMessage struct {
	Subtype string         `json:"subtype"`
	Data    map[string]any `json:"data"`
}

func (m *SystemMessage) messageType() string { return "system" }

// TaskUsage reports resource consumption for a task.
type TaskUsage struct {
	TotalTokens int `json:"total_tokens"`
	ToolUses    int `json:"tool_uses"`
	DurationMs  int `json:"duration_ms"`
}

// TaskStartedMessage is emitted when a sub-agent task begins.
type TaskStartedMessage struct {
	SystemMessage
	TaskID      string `json:"task_id"`
	Description string `json:"description"`
	UUID        string `json:"uuid"`
	SessionID   string `json:"session_id"`
	ToolUseID   string `json:"tool_use_id,omitempty"`
	TaskType    string `json:"task_type,omitempty"`
}

// TaskProgressMessage is emitted periodically while a task runs.
type TaskProgressMessage struct {
	SystemMessage
	TaskID       string    `json:"task_id"`
	Description  string    `json:"description"`
	Usage        TaskUsage `json:"usage"`
	UUID         string    `json:"uuid"`
	SessionID    string    `json:"session_id"`
	ToolUseID    string    `json:"tool_use_id,omitempty"`
	LastToolName string    `json:"last_tool_name,omitempty"`
}

// TaskNotificationStatus enumerates terminal task states.
type TaskNotificationStatus string

const (
	TaskStatusCompleted TaskNotificationStatus = "completed"
	TaskStatusFailed    TaskNotificationStatus = "failed"
	TaskStatusStopped   TaskNotificationStatus = "stopped"
)

// TaskNotificationMessage is emitted when a task finishes.
type TaskNotificationMessage struct {
	SystemMessage
	TaskID     string                 `json:"task_id"`
	Status     TaskNotificationStatus `json:"status"`
	OutputFile string                 `json:"output_file"`
	Summary    string                 `json:"summary"`
	UUID       string                 `json:"uuid"`
	SessionID  string                 `json:"session_id"`
	ToolUseID  string                 `json:"tool_use_id,omitempty"`
	Usage      *TaskUsage             `json:"usage,omitempty"`
}

// MirrorErrorMessage is emitted when a SessionStore.append call fails.
type MirrorErrorMessage struct {
	SystemMessage
	Key   *SessionKey `json:"key,omitempty"`
	Error string      `json:"error,omitempty"`
}

func (m *MirrorErrorMessage) messageType() string { return "mirror_error" }

// DeferredToolUse represents a tool use that was deferred by a PreToolUse hook
// returning permissionDecision "defer". The run stops and the result message
// carries the deferred tool call so the caller can inspect it.
type DeferredToolUse struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// HookEventMessage is emitted by the CLI when include_hook_events is enabled.
// It carries hook lifecycle events (PreToolUse, PostToolUse, Stop, etc.)
// as system messages with subtype "hook_started" or "hook_response".
type HookEventMessage struct {
	SystemMessage
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id,omitempty"`
	UUID          string `json:"uuid,omitempty"`
}

func (m *HookEventMessage) messageType() string { return "hook_event" }

// SessionStoreFlushMode controls when transcript mirror entries are flushed.
type SessionStoreFlushMode string

const (
	FlushModeBatched SessionStoreFlushMode = "batched" // Coalesce and flush per turn or threshold
	FlushModeEager   SessionStoreFlushMode = "eager"   // Flush after every frame
)

// ResultMessage is the final message from a query.
type ResultMessage struct {
	Subtype           string         `json:"subtype"`
	DurationMs        int            `json:"duration_ms"`
	DurationAPIMs     int            `json:"duration_api_ms"`
	IsError           bool           `json:"is_error"`
	NumTurns          int            `json:"num_turns"`
	SessionID         string         `json:"session_id"`
	StopReason        string         `json:"stop_reason,omitempty"`
	TotalCostUSD      *float64       `json:"total_cost_usd,omitempty"`
	Usage             map[string]any `json:"usage,omitempty"`
	Result            string         `json:"result,omitempty"`
	StructuredOutput  any            `json:"structured_output,omitempty"`
	ModelUsage        map[string]any `json:"model_usage,omitempty"`
	PermissionDenials []any          `json:"permission_denials,omitempty"`
	DeferredToolUse   *DeferredToolUse `json:"deferred_tool_use,omitempty"`
	Errors            []string       `json:"errors,omitempty"`
	// APIErrorStatus is the HTTP status code (e.g. 429, 500, 529) of the
	// failing API call when IsError is true and Subtype is "success".
	APIErrorStatus    *int           `json:"api_error_status,omitempty"`
	UUID              string         `json:"uuid,omitempty"`
}

func (m *ResultMessage) messageType() string { return "result" }

// StreamEvent carries a partial streaming event (Anthropic API wire format).
type StreamEvent struct {
	UUID            string         `json:"uuid"`
	SessionID       string         `json:"session_id"`
	Event           map[string]any `json:"event"`
	ParentToolUseID string         `json:"parent_tool_use_id,omitempty"`
}

func (m *StreamEvent) messageType() string { return "stream_event" }

// -----------------------------------------------------------------------
// Rate limit types
// -----------------------------------------------------------------------

// RateLimitStatus is the current rate limit state.
type RateLimitStatus string

const (
	RateLimitAllowed        RateLimitStatus = "allowed"
	RateLimitAllowedWarning RateLimitStatus = "allowed_warning"
	RateLimitRejected       RateLimitStatus = "rejected"
)

// RateLimitType identifies which rate limit window applies.
type RateLimitType string

const (
	RateLimitTypeFiveHour       RateLimitType = "five_hour"
	RateLimitTypeSevenDay       RateLimitType = "seven_day"
	RateLimitTypeSevenDayOpus   RateLimitType = "seven_day_opus"
	RateLimitTypeSevenDaySonnet RateLimitType = "seven_day_sonnet"
	RateLimitTypeOverage        RateLimitType = "overage"
)

// RateLimitInfo carries rate limit status details.
type RateLimitInfo struct {
	Status                RateLimitStatus  `json:"status"`
	ResetsAt              *int64           `json:"resetsAt,omitempty"`
	RateLimitType         *RateLimitType   `json:"rateLimitType,omitempty"`
	Utilization           *float64         `json:"utilization,omitempty"`
	OverageStatus         *RateLimitStatus `json:"overageStatus,omitempty"`
	OverageResetsAt       *int64           `json:"overageResetsAt,omitempty"`
	OverageDisabledReason *string          `json:"overageDisabledReason,omitempty"`
	Raw                   map[string]any   `json:"raw,omitempty"`
}

// RateLimitEvent is emitted when rate limit status changes.
type RateLimitEvent struct {
	RateLimitInfo RateLimitInfo `json:"rate_limit_info"`
	UUID          string        `json:"uuid"`
	SessionID     string        `json:"session_id"`
}

func (m *RateLimitEvent) messageType() string { return "rate_limit_event" }

// -----------------------------------------------------------------------
// Context usage types
// -----------------------------------------------------------------------

// ContextUsageCategory describes a single context usage category.
type ContextUsageCategory struct {
	Name       string `json:"name"`
	Tokens     int    `json:"tokens"`
	Color      string `json:"color"`
	IsDeferred *bool  `json:"isDeferred,omitempty"`
}

// ContextUsageResponse describes the current context window usage.
type ContextUsageResponse struct {
	Categories           []ContextUsageCategory `json:"categories"`
	TotalTokens          int                    `json:"totalTokens"`
	MaxTokens            int                    `json:"maxTokens"`
	RawMaxTokens         int                    `json:"rawMaxTokens"`
	Percentage           float64                `json:"percentage"`
	Model                string                 `json:"model"`
	IsAutoCompactEnabled bool                   `json:"isAutoCompactEnabled"`
	MemoryFiles          []map[string]any       `json:"memoryFiles"`
	MCPTools             []map[string]any       `json:"mcpTools"`
	Agents               []map[string]any       `json:"agents"`
	GridRows             [][]map[string]any     `json:"gridRows"`
	AutoCompactThreshold *int                   `json:"autoCompactThreshold,omitempty"`
	DeferredBuiltinTools []map[string]any       `json:"deferredBuiltinTools,omitempty"`
	SystemTools          []map[string]any       `json:"systemTools,omitempty"`
	SystemPromptSections []map[string]any       `json:"systemPromptSections,omitempty"`
	SlashCommands        map[string]any         `json:"slashCommands,omitempty"`
	Skills               map[string]any         `json:"skills,omitempty"`
	MessageBreakdown     map[string]any         `json:"messageBreakdown,omitempty"`
	APIUsage             map[string]any         `json:"apiUsage,omitempty"`
}

// -----------------------------------------------------------------------
// Thinking and Sandbox config
// -----------------------------------------------------------------------

// ThinkingDisplay controls whether thinking text is returned summarized or omitted.
type ThinkingDisplay string

const (
	ThinkingDisplaySummarized ThinkingDisplay = "summarized"
	ThinkingDisplayOmitted    ThinkingDisplay = "omitted"
)

// ThinkingConfig controls extended-thinking behaviour.
type ThinkingConfig interface {
	thinkingType() string
}

// ThinkingAdaptive enables adaptive thinking (SDK chooses budget automatically).
type ThinkingAdaptive struct {
	Display ThinkingDisplay `json:"display,omitempty"` // "summarized" or "omitted"
}

func (t *ThinkingAdaptive) thinkingType() string { return "adaptive" }

// ThinkingEnabled enables thinking with an explicit token budget.
type ThinkingEnabled struct {
	BudgetTokens int             `json:"budget_tokens"`
	Display      ThinkingDisplay `json:"display,omitempty"` // "summarized" or "omitted"
}

func (t *ThinkingEnabled) thinkingType() string { return "enabled" }

// ThinkingDisabled disables extended thinking.
type ThinkingDisabled struct{}

func (t *ThinkingDisabled) thinkingType() string { return "disabled" }

// SandboxNetworkConfig is the network configuration for sandbox.
type SandboxNetworkConfig struct {
	// AllowedDomains is a list of domain names that sandboxed processes can access.
	AllowedDomains []string `json:"allowedDomains,omitempty"`
	// DeniedDomains is a list of domains that are always blocked, even if matched by AllowedDomains.
	DeniedDomains []string `json:"deniedDomains,omitempty"`
	// AllowManagedDomainsOnly when true in managed settings, only managed-settings AllowedDomains are respected.
	AllowManagedDomainsOnly bool `json:"allowManagedDomainsOnly,omitempty"`
	// AllowUnixSockets is a list of Unix socket paths accessible in sandbox (e.g., SSH agents).
	AllowUnixSockets []string `json:"allowUnixSockets,omitempty"`
	// AllowAllUnixSockets allows all Unix sockets (less secure).
	AllowAllUnixSockets bool `json:"allowAllUnixSockets,omitempty"`
	// AllowLocalBinding allows binding to localhost ports (macOS only).
	AllowLocalBinding bool `json:"allowLocalBinding,omitempty"`
	// AllowMachLookup is macOS only: XPC/Mach service names to allow (supports trailing wildcard).
	AllowMachLookup []string `json:"allowMachLookup,omitempty"`
	// HTTPProxyPort is the HTTP proxy port if bringing your own proxy.
	HTTPProxyPort int `json:"httpProxyPort,omitempty"`
	// SOCKSProxyPort is the SOCKS5 proxy port if bringing your own proxy.
	SOCKSProxyPort int `json:"socksProxyPort,omitempty"`
}

// SandboxIgnoreViolations specifies violations to ignore in sandbox.
type SandboxIgnoreViolations struct {
	// File is a list of file paths for which violations should be ignored.
	File []string `json:"file,omitempty"`
	// Network is a list of network hosts for which violations should be ignored.
	Network []string `json:"network,omitempty"`
}

// SandboxSettings configures process sandboxing for Bash tool commands.
//
// Important: Filesystem and network restrictions are configured via permission
// rules (Read/Edit for filesystem, WebFetch for network), not via these
// sandbox settings — sandbox settings control sandbox behavior (enabled, auto-allow, etc.).
//
// See https://docs.anthropic.com/en/docs/claude-code/settings#sandbox-settings.
type SandboxSettings struct {
	// Enabled enables bash sandboxing (macOS/Linux only). Default: false.
	Enabled bool `json:"enabled,omitempty"`
	// AutoAllowBashIfSandboxed auto-approves bash commands when sandboxed. Default: true.
	AutoAllowBashIfSandboxed *bool `json:"autoAllowBashIfSandboxed,omitempty"`
	// ExcludedCommands lists commands that should run outside the sandbox (e.g., ["git", "docker"]).
	ExcludedCommands []string `json:"excludedCommands,omitempty"`
	// AllowUnsandboxedCommands allows commands to bypass sandbox via dangerouslyDisableSandbox.
	// When false, all commands must run sandboxed (or be in ExcludedCommands). Default: true.
	AllowUnsandboxedCommands *bool `json:"allowUnsandboxedCommands,omitempty"`
	// Network is the network configuration for sandbox.
	Network *SandboxNetworkConfig `json:"network,omitempty"`
	// IgnoreViolations specifies violations to ignore.
	IgnoreViolations *SandboxIgnoreViolations `json:"ignoreViolations,omitempty"`
	// EnableWeakerNestedSandbox enables weaker sandbox for unprivileged Docker environments
	// (Linux only). Reduces security. Default: false.
	EnableWeakerNestedSandbox bool `json:"enableWeakerNestedSandbox,omitempty"`
}

// SystemPromptPreset selects a built-in system prompt.
type SystemPromptPreset struct {
	Type                   string `json:"type"`   // "preset"
	Preset                 string `json:"preset"` // "claude_code"
	Append                 string `json:"append,omitempty"`
	ExcludeDynamicSections *bool  `json:"excludeDynamicSections,omitempty"`
}

// SystemPromptFile loads the system prompt from a file.
type SystemPromptFile struct {
	Type string `json:"type"` // "file"
	Path string `json:"path"`
}

// TaskBudget sets an API-side token budget for the task.
type TaskBudget struct {
	Total int `json:"total"`
}

// ToolsPreset selects a built-in tool set.
type ToolsPreset struct {
	Type   string `json:"type"`   // "preset"
	Preset string `json:"preset"` // "claude_code"
}

// SdkPluginConfig describes a local plugin directory.
type SdkPluginConfig struct {
	Type string `json:"type"` // "local"
	Path string `json:"path"`
}

// OutputFormat describes the structured output format.
// Example: {"type": "json_schema", "schema": {...}}
type OutputFormat map[string]any

// -----------------------------------------------------------------------
// ClaudeAgentOptions
// -----------------------------------------------------------------------

// ClaudeAgentOptions configures a query or streaming client session.
// Zero values are safe defaults.
type ClaudeAgentOptions struct {
	// Tools specifies the base set of available built-in tools.
	//
	//   - []string — Specific tool names (e.g. ["Bash", "Read", "Edit"]).
	//   - []string{} (empty slice) — Disable all built-in tools.
	//   - *ToolsPreset — Use all default Claude Code tools.
	//
	// To restrict which tools the model may call without being prompted, use
	// AllowedTools instead.
	Tools any // nil | []string | *ToolsPreset

	// AllowedTools lists tool names that are auto-allowed without prompting
	// for permission. These tools execute automatically without asking the
	// user for approval. To restrict which tools are available at all, use
	// Tools.
	AllowedTools []string

	// DisallowedTools lists tool names that are disallowed. These tools are
	// removed from the model's context and cannot be used, even if they
	// would otherwise be allowed.
	DisallowedTools []string

	// SystemPrompt configures the system prompt.
	//
	//   - string — Use a custom system prompt.
	//   - *SystemPromptPreset — Use Claude Code's default system prompt.
	//     Set Append to add instructions after the default prompt.
	//   - *SystemPromptFile — Load the system prompt from a file.
	SystemPrompt any // nil | string | *SystemPromptPreset | *SystemPromptFile

	// MCPServers maps server names to their MCP (Model Context Protocol)
	// server configurations. Values may be *MCPStdioServerConfig,
	// *MCPSSEServerConfig, *MCPHTTPServerConfig, or *MCPSdkServerConfig.
	MCPServers map[string]MCPServerConfig

	// MCPConfigPath is a file-system path to a JSON MCP config file. When
	// set it is passed as-is to --mcp-config, taking precedence over
	// MCPServers.
	MCPConfigPath string

	// PermissionMode controls how permission prompts are handled for the session.
	//
	//   - PermissionModeDefault — Standard behavior; prompts for dangerous operations.
	//   - PermissionModeAcceptEdits — Auto-accept file edit operations.
	//   - PermissionModeBypassPermissions — Bypass all permission checks.
	//   - PermissionModePlan — Planning mode, no execution of tools.
	//   - PermissionModeDontAsk — Don't prompt; deny if not pre-approved.
	//   - PermissionModeAuto — Automatic permission handling.
	PermissionMode PermissionMode

	// ContinueConversation resumes the most recent conversation in the
	// current directory instead of starting a new one. Mutually exclusive
	// with Resume.
	ContinueConversation bool

	// Resume specifies a session ID to resume. Loads the conversation
	// history from the specified session.
	Resume string

	// SessionID specifies a custom session ID for the conversation instead
	// of an auto-generated one. Must be a valid UUID. Cannot be used with
	// ContinueConversation or Resume unless ForkSession is also set.
	SessionID string

	// MaxTurns limits the number of conversation turns before the query
	// stops. A turn consists of a user message and assistant response.
	// Zero means unlimited.
	MaxTurns int

	// MaxBudgetUSD stops the session when the cost exceeds this threshold
	// (in USD), returning an error_max_budget_usd result.
	MaxBudgetUSD *float64

	// Model selects the Claude model. Defaults to the CLI default model.
	// Examples: "claude-sonnet-4-5", "claude-opus-4-5".
	Model string

	// FallbackModel is the model to use if the primary model fails or is
	// unavailable.
	FallbackModel string

	// Betas enables SDK beta features. Currently supported:
	//
	//   - SdkBetaContext1M ("context-1m-2025-08-07") — Enable 1M token
	//     context window (Sonnet 4/4.5 only).
	//
	// See https://docs.anthropic.com/en/api/beta-headers.
	Betas []SdkBeta

	// PermissionPromptToolName overrides the MCP tool name used for
	// permission prompts. When set, permission requests are routed through
	// this MCP tool instead of the default handler. Mutually exclusive
	// with CanUseTool.
	PermissionPromptToolName string

	// CWD sets the working directory for the CLI subprocess. Defaults to
	// the process cwd.
	CWD string

	// CLIPath overrides the path to the Claude Code CLI executable. Uses
	// the bundled executable if not specified.
	CLIPath string

	// Settings is a path to an additional settings JSON file to load.
	// These are loaded into the "flag settings" layer, which has the
	// highest priority among user-controlled settings. Equivalent to the
	// --settings CLI flag.
	Settings string

	// AddDirs adds directories to the allowed-read list. Paths should be
	// absolute.
	AddDirs []string

	// Env merges extra environment variables into the subprocess
	// environment. SDK consumers can identify their app/library in the
	// User-Agent header by setting CLAUDE_AGENT_SDK_CLIENT_APP
	// (e.g. "my-app/1.0.0").
	Env map[string]string

	// ExtraArgs passes additional --flag [value] pairs to the CLI. Keys
	// are argument names (without --), values are argument values. A nil
	// value means a boolean flag (no value argument).
	ExtraArgs map[string]*string

	// MaxBufferSize limits the maximum bytes to buffer when reading the CLI
	// subprocess stdout. Zero means default (1 MB).
	MaxBufferSize int

	// Stderr is called for each line of stderr output from the CLI
	// subprocess. Useful for debugging and logging.
	Stderr func(line string)

	// CanUseTool is called for each tool-use permission request to
	// determine if it should be allowed, denied, or prompt the user.
	// Requires an async-iterable (channel) prompt; mutually exclusive
	// with PermissionPromptToolName.
	CanUseTool CanUseTool

	// Hooks registers event callbacks keyed by HookEvent. Hooks can
	// modify behavior, add context, or implement custom logic.
	// See https://docs.anthropic.com/en/docs/claude-code/hooks.
	Hooks map[HookEvent][]HookMatcher

	// User is an optional user identifier associated with the session
	// (Linux/macOS only).
	User string

	// IncludePartialMessages enables partial/streaming message events in
	// the output. When true, StreamEvent messages are emitted during
	// streaming.
	IncludePartialMessages bool

	// ForkSession forks the session on resume instead of continuing it.
	// Use with Resume.
	ForkSession bool

	// Agents programmatically defines custom sub-agents invokable via the
	// Agent tool. Keys are agent names, values are agent definitions.
	Agents map[string]AgentDefinition

	// SettingSources controls which filesystem settings the CLI loads.
	//
	//   - SettingSourceUser — Global user settings (~/.claude/settings.json).
	//   - SettingSourceProject — Project settings (.claude/settings.json).
	//   - SettingSourceLocal — Local settings (.claude/settings.local.json).
	//
	// When nil, all sources are loaded (matches CLI defaults). Pass an
	// empty slice to disable filesystem settings (SDK isolation mode).
	// Must include "project" to load CLAUDE.md files.
	SettingSources []SettingSource

	// Sandbox configures process sandboxing for Bash command execution.
	// When enabled, commands execute in a sandboxed environment. Filesystem
	// and network restrictions are configured via permission rules, not via
	// these sandbox settings — sandbox settings control sandbox behavior
	// (enabled, auto-allow, etc.).
	//
	// See https://docs.anthropic.com/en/docs/claude-code/settings#sandbox-settings.
	Sandbox *SandboxSettings

	// Plugins loads local plugin directories for this session. Plugins
	// provide custom commands, agents, skills, and hooks that extend
	// Claude Code's capabilities.
	Plugins []SdkPluginConfig

	// MaxThinkingTokens is the maximum tokens the model may use for its
	// thinking/reasoning process.
	//
	// Deprecated: Use Thinking instead. On newer models, this is treated
	// as on/off (0 = disabled, any other value = adaptive). For explicit
	// control, use ThinkingAdaptive or ThinkingEnabled.
	MaxThinkingTokens *int

	// Thinking controls Claude's thinking/reasoning behavior. Takes
	// precedence over MaxThinkingTokens.
	//
	//   - *ThinkingAdaptive — Claude decides when and how much to think
	//     (Opus 4.6+). Default for models that support it.
	//   - *ThinkingEnabled — Fixed thinking token budget (older models).
	//   - *ThinkingDisabled — No extended thinking.
	//
	// See https://docs.anthropic.com/en/docs/build-with-claude/adaptive-thinking.
	Thinking ThinkingConfig

	// Effort controls how much effort Claude puts into its response.
	// Works with adaptive thinking to guide thinking depth.
	//
	//   - EffortLow — Minimal thinking, fastest responses.
	//   - EffortMedium — Moderate thinking.
	//   - EffortHigh — Deep reasoning (default).
	//   - EffortMax — Maximum effort.
	//
	// See https://docs.anthropic.com/en/docs/build-with-claude/effort.
	Effort EffortLevel

	// OutputFormat specifies structured output format. When set, the agent
	// returns structured data matching the schema. Example:
	// OutputFormat{"type": "json_schema", "schema": map[string]any{...}}.
	OutputFormat OutputFormat

	// EnableFileCheckpointing enables file checkpointing to track file
	// changes during the session. When enabled, files can be rewound to
	// their state at any user message using ClaudeSDKClient.RewindFiles().
	EnableFileCheckpointing bool

	// TaskBudget sets an API-side task budget in tokens. When set, the
	// model is made aware of its remaining token budget so it can pace
	// tool use and wrap up before the limit. Sent as output_config.task_budget
	// with the task-budgets-2026-03-13 beta header.
	TaskBudget *TaskBudget

	// Skills enables specific skills for the main session. This is the
	// single place to turn skills on; you do not need to add "Skill" to
	// AllowedTools or set SettingSources yourself — the SDK does both
	// when this is set.
	//
	//   - nil (default): no SDK auto-configuration. The CLI's own defaults
	//     still apply, so this is NOT "skills off" — to suppress every
	//     skill from the listing, use []string{}.
	//   - "all": enable every discovered skill.
	//   - []string: enable only the listed skills. Names match the
	//     SKILL.md name / directory name, or plugin:skill for
	//     plugin-qualified skills.
	//
	// This is a context filter, not a sandbox: unlisted skills are hidden
	// from the model's listing and rejected by the Skill tool, but their
	// files remain on disk and are reachable via Read/Bash. Do not store
	// secrets in skill files.
	Skills any // nil | "all" | []string

	// SessionStore mirrors session transcripts to an external store. When
	// set, every transcript line written locally is also passed to
	// SessionStore.Append(), and Resume can materialize from the store
	// when the local file is absent.
	SessionStore SessionStore

	// LoadTimeoutMs is the timeout for each SessionStore.Load() /
	// ListSubkeys() call during resume materialization, in milliseconds.
	// If the adapter doesn't settle within this window the query fails
	// with a clear error instead of hanging forever. Zero means default
	// (60000ms); use a large value to effectively disable.
	LoadTimeoutMs int

	// IncludeHookEvents enables hook lifecycle events in the message stream.
	// When true, the CLI emits hook events (PreToolUse, PostToolUse, Stop,
	// etc.) as HookEventMessage objects.
	IncludeHookEvents bool

	// StrictMCPConfig when true only uses MCP servers passed via MCPServers,
	// ignoring all other MCP configurations the CLI would otherwise load.
	StrictMCPConfig bool

	// SessionStoreFlush controls when transcript mirror entries are flushed.
	// "batched" (default) coalesces and flushes per turn or threshold.
	// "eager" flushes after every frame for near-real-time delivery.
	SessionStoreFlush SessionStoreFlushMode
}

// -----------------------------------------------------------------------
// Session Store types
// -----------------------------------------------------------------------

// SessionKey identifies a session transcript in a store.
type SessionKey struct {
	ProjectKey string `json:"project_key"`
	SessionID  string `json:"session_id"`
	Subpath    string `json:"subpath,omitempty"` // Omit for main transcript
}

// SessionStoreEntry represents one JSONL transcript line in a store.
type SessionStoreEntry struct {
	Type      string `json:"type"`
	UUID      string `json:"uuid,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	// Additional fields are opaque JSON.
	Extra map[string]any `json:"-"`
}

// SessionStoreListEntry is returned by SessionStore.ListSessions.
type SessionStoreListEntry struct {
	SessionID string `json:"session_id"`
	Mtime     int64  `json:"mtime"` // Unix epoch milliseconds
}

// SessionSummaryEntry is an incrementally-maintained session summary.
type SessionSummaryEntry struct {
	SessionID string         `json:"session_id"`
	Mtime     int64          `json:"mtime"`
	Data      map[string]any `json:"data"`
}

// SessionListSubkeysKey is the key argument to SessionStore.ListSubkeys.
type SessionListSubkeysKey struct {
	ProjectKey string `json:"project_key"`
	SessionID  string `json:"session_id"`
}

// SessionStore is an adapter for mirroring session transcripts to external storage.
type SessionStore interface {
	// Append mirrors a batch of transcript entries.
	Append(key SessionKey, entries []SessionStoreEntry) error
	// Load loads a full session for resume. Returns nil if not found.
	Load(key SessionKey) ([]SessionStoreEntry, error)
	// ListSessions lists sessions for a project key. Optional.
	ListSessions(projectKey string) ([]SessionStoreListEntry, error)
	// ListSessionSummaries returns summaries for all sessions. Optional.
	ListSessionSummaries(projectKey string) ([]SessionSummaryEntry, error)
	// Delete deletes a session. Optional.
	Delete(key SessionKey) error
	// ListSubkeys lists subpath keys under a session. Optional.
	ListSubkeys(projectKey, sessionID string) ([]string, error)
}

// -----------------------------------------------------------------------
// Session types
// -----------------------------------------------------------------------

// SDKSessionInfo describes a stored session.
type SDKSessionInfo struct {
	SessionID    string `json:"session_id"`
	Summary      string `json:"summary"`
	LastModified int64  `json:"last_modified"` // milliseconds since epoch
	FileSize     *int64 `json:"file_size,omitempty"`
	CustomTitle  string `json:"custom_title,omitempty"`
	FirstPrompt  string `json:"first_prompt,omitempty"`
	GitBranch    string `json:"git_branch,omitempty"`
	CWD          string `json:"cwd,omitempty"`
	Tag          string `json:"tag,omitempty"`
	CreatedAt    *int64 `json:"created_at,omitempty"` // milliseconds since epoch from first entry timestamp
}

// SessionMessage is a user or assistant message from a session transcript.
type SessionMessage struct {
	Type            string         `json:"type"` // "user" | "assistant"
	UUID            string         `json:"uuid"`
	SessionID       string         `json:"session_id"`
	Message         map[string]any `json:"message"`
	ParentToolUseID *string        `json:"parent_tool_use_id,omitempty"`
}
