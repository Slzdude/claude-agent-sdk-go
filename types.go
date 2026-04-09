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
	EffortMax    EffortLevel = "max"
)

// AgentDefinition describes a custom sub-agent.
type AgentDefinition struct {
	Description     string                  `json:"description"`
	Prompt          string                  `json:"prompt"`
	Tools           []string                `json:"tools,omitempty"`
	DisallowedTools []string                `json:"disallowedTools,omitempty"`
	Model           string                  `json:"model,omitempty"` // "sonnet", "opus", "haiku", "inherit", or full model ID
	Skills          []string                `json:"skills,omitempty"`
	Memory          string                  `json:"memory,omitempty"` // "user", "project", "local"
	MCPServers      []map[string]any        `json:"mcpServers,omitempty"`
	InitialPrompt   string                  `json:"initialPrompt,omitempty"`
	MaxTurns        *int                    `json:"maxTurns,omitempty"`
	Background      *bool                   `json:"background,omitempty"`
	Effort          any                     `json:"effort,omitempty"` // string or int
	PermissionMode  *PermissionMode         `json:"permissionMode,omitempty"`
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

// ResultMessage is the final message from a query.
type ResultMessage struct {
	Subtype          string         `json:"subtype"`
	DurationMs       int            `json:"duration_ms"`
	DurationAPIMs    int            `json:"duration_api_ms"`
	IsError          bool           `json:"is_error"`
	NumTurns         int            `json:"num_turns"`
	SessionID        string         `json:"session_id"`
	StopReason       string         `json:"stop_reason,omitempty"`
	TotalCostUSD     *float64       `json:"total_cost_usd,omitempty"`
	Usage            map[string]any `json:"usage,omitempty"`
	Result           string         `json:"result,omitempty"`
	StructuredOutput any            `json:"structured_output,omitempty"`
	ModelUsage       map[string]any `json:"model_usage,omitempty"`
	PermissionDenials []any         `json:"permission_denials,omitempty"`
	Errors           []string       `json:"errors,omitempty"`
	UUID             string         `json:"uuid,omitempty"`
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
	RateLimitTypeFiveHour     RateLimitType = "five_hour"
	RateLimitTypeSevenDay     RateLimitType = "seven_day"
	RateLimitTypeSevenDayOpus RateLimitType = "seven_day_opus"
	RateLimitTypeSevenDaySonnet RateLimitType = "seven_day_sonnet"
	RateLimitTypeOverage      RateLimitType = "overage"
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
	Categories            []ContextUsageCategory `json:"categories"`
	TotalTokens           int                    `json:"totalTokens"`
	MaxTokens             int                    `json:"maxTokens"`
	RawMaxTokens          int                    `json:"rawMaxTokens"`
	Percentage            float64                `json:"percentage"`
	Model                 string                 `json:"model"`
	IsAutoCompactEnabled  bool                   `json:"isAutoCompactEnabled"`
	MemoryFiles           []map[string]any       `json:"memoryFiles"`
	MCPTools              []map[string]any       `json:"mcpTools"`
	Agents                []map[string]any       `json:"agents"`
	GridRows              [][]map[string]any     `json:"gridRows"`
	AutoCompactThreshold  *int                   `json:"autoCompactThreshold,omitempty"`
	DeferredBuiltinTools  []map[string]any       `json:"deferredBuiltinTools,omitempty"`
	SystemTools           []map[string]any       `json:"systemTools,omitempty"`
	SystemPromptSections  []map[string]any       `json:"systemPromptSections,omitempty"`
	SlashCommands         map[string]any         `json:"slashCommands,omitempty"`
	Skills                map[string]any         `json:"skills,omitempty"`
	MessageBreakdown      map[string]any         `json:"messageBreakdown,omitempty"`
	APIUsage              map[string]any         `json:"apiUsage,omitempty"`
}

// -----------------------------------------------------------------------
// Thinking and Sandbox config
// -----------------------------------------------------------------------

// ThinkingConfig controls extended-thinking behaviour.
type ThinkingConfig interface {
	thinkingType() string
}

// ThinkingAdaptive enables adaptive thinking (SDK chooses budget automatically).
type ThinkingAdaptive struct{}

func (t *ThinkingAdaptive) thinkingType() string { return "adaptive" }

// ThinkingEnabled enables thinking with an explicit token budget.
type ThinkingEnabled struct {
	BudgetTokens int `json:"budget_tokens"`
}

func (t *ThinkingEnabled) thinkingType() string { return "enabled" }

// ThinkingDisabled disables extended thinking.
type ThinkingDisabled struct{}

func (t *ThinkingDisabled) thinkingType() string { return "disabled" }

// SandboxSettings configures process sandboxing for Bash tool commands.
type SandboxSettings map[string]any

// SystemPromptPreset selects a built-in system prompt.
type SystemPromptPreset struct {
	Type                    string `json:"type"`   // "preset"
	Preset                  string `json:"preset"` // "claude_code"
	Append                  string `json:"append,omitempty"`
	ExcludeDynamicSections  *bool  `json:"excludeDynamicSections,omitempty"`
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
	// Tools specifies the base tool set: nil = default, []string = explicit list,
	// *ToolsPreset = preset.
	Tools any // nil | []string | *ToolsPreset

	// AllowedTools is the list of additional tools to allow.
	AllowedTools []string

	// DisallowedTools is the list of tools to disallow.
	DisallowedTools []string

	// SystemPrompt is either a plain string, *SystemPromptPreset, or *SystemPromptFile.
	SystemPrompt any // nil | string | *SystemPromptPreset | *SystemPromptFile

	// MCPServers maps server names to their config.
	// Values may be *MCPStdioServerConfig, *MCPSSEServerConfig,
	// *MCPHTTPServerConfig, or *MCPSdkServerConfig.
	MCPServers map[string]MCPServerConfig

	// MCPConfigPath is a file-system path to a JSON MCP config file.
	// When set it is passed as-is to --mcp-config, taking precedence over MCPServers.
	MCPConfigPath string

	// PermissionMode controls how permission prompts are handled.
	PermissionMode PermissionMode

	// ContinueConversation resumes the most recent session.
	ContinueConversation bool

	// Resume specifies a session ID to resume.
	Resume string

	// SessionID specifies a custom session ID.
	SessionID string

	// MaxTurns limits the number of conversation turns (0 = unlimited).
	MaxTurns int

	// MaxBudgetUSD stops the session when the cost exceeds this threshold.
	MaxBudgetUSD *float64

	// Model selects the Claude model.
	Model string

	// FallbackModel is used when the primary model is unavailable.
	FallbackModel string

	// Betas enables SDK beta features.
	Betas []SdkBeta

	// PermissionPromptToolName overrides the permission prompt tool name.
	// Mutually exclusive with CanUseTool.
	PermissionPromptToolName string

	// CWD sets the working directory for the CLI subprocess.
	CWD string

	// CLIPath overrides the path to the claude binary.
	CLIPath string

	// Settings is a JSON string or file path for the settings file.
	Settings string

	// AddDirs adds directories to the allowed-read list.
	AddDirs []string

	// Env merges extra environment variables into the subprocess environment.
	Env map[string]string

	// ExtraArgs passes additional --flag [value] pairs to the CLI.
	// A nil value means a boolean flag (no value argument).
	ExtraArgs map[string]*string

	// MaxBufferSize limits the internal read buffer (0 = default 1 MB).
	MaxBufferSize int

	// Stderr is called for each line of stderr output from the CLI.
	Stderr func(line string)

	// CanUseTool is called for each tool-use permission request.
	// Requires an async-iterable (channel) prompt; mutually exclusive
	// with PermissionPromptToolName.
	CanUseTool CanUseTool

	// Hooks registers event callbacks keyed by HookEvent.
	Hooks map[HookEvent][]HookMatcher

	// User is the OS user to run the CLI subprocess as (Linux/macOS only).
	User string

	// IncludePartialMessages enables StreamEvent messages during streaming.
	IncludePartialMessages bool

	// ForkSession forks the session on resume instead of continuing it.
	ForkSession bool

	// Agents defines additional sub-agents passed via the initialize request.
	Agents map[string]AgentDefinition

	// SettingSources selects which settings tiers are loaded by the CLI.
	SettingSources []SettingSource

	// Sandbox configures process sandboxing merged into the settings JSON.
	Sandbox SandboxSettings

	// Plugins lists local plugin directories.
	Plugins []SdkPluginConfig

	// MaxThinkingTokens is deprecated; use Thinking instead.
	MaxThinkingTokens *int

	// Thinking controls extended-thinking behaviour.
	// Takes precedence over MaxThinkingTokens.
	Thinking ThinkingConfig

	// Effort sets the reasoning depth.
	Effort EffortLevel

	// OutputFormat specifies structured output format (e.g. JSON Schema).
	OutputFormat OutputFormat

	// EnableFileCheckpointing enables file rewind functionality.
	EnableFileCheckpointing bool

	// TaskBudget sets an API-side token budget for the task.
	TaskBudget *TaskBudget
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
