package claude

import "context"

// MCPServerConfig is implemented by all MCP server configuration types.
type MCPServerConfig interface {
	mcpServerType() string
}

// MCPStdioServerConfig configures an external MCP server launched as a subprocess.
type MCPStdioServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func (c *MCPStdioServerConfig) mcpServerType() string { return "stdio" }

// MCPSSEServerConfig configures a remote MCP server using Server-Sent Events.
type MCPSSEServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (c *MCPSSEServerConfig) mcpServerType() string { return "sse" }

// MCPHTTPServerConfig configures a remote MCP server using HTTP.
type MCPHTTPServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (c *MCPHTTPServerConfig) mcpServerType() string { return "http" }

// MCPSdkServerConfig configures an in-process SDK MCP server.
type MCPSdkServerConfig struct {
	Name     string
	Instance SdkMcpServer
}

func (c *MCPSdkServerConfig) mcpServerType() string { return "sdk" }

// ToolResult is returned by an SdkMcpServer tool call.
type ToolResult struct {
	Content []map[string]any `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

// SdkMcpServer is an in-process MCP server that the SDK bridges to the CLI.
type SdkMcpServer interface {
	// Name returns the server name (used for --mcp-config and routing).
	Name() string
	// Version returns the server version string.
	Version() string
	// ListTools returns the list of tools provided by this server.
	ListTools(ctx context.Context) ([]MCPTool, error)
	// CallTool executes a named tool with the given arguments.
	CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error)
}

// MCPTool describes a single tool exposed by an MCP server.
type MCPTool struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	InputSchema map[string]any   `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
	Meta        map[string]any   `json:"_meta,omitempty"`
}

// ToolAnnotations provides optional hints about a tool's behaviour.
// Used for SDK MCP server tool definitions (wire format: readOnlyHint, etc.)
type ToolAnnotations struct {
	ReadOnlyHint       *bool `json:"readOnlyHint,omitempty"`
	DestructiveHint    *bool `json:"destructiveHint,omitempty"`
	IdempotentHint     *bool `json:"idempotentHint,omitempty"`
	OpenWorldHint      *bool `json:"openWorldHint,omitempty"`
	MaxResultSizeChars *int  `json:"maxResultSizeChars,omitempty"`
}

// McpServerConnectionStatus enumerates MCP server connection states.
type McpServerConnectionStatus string

const (
	McpStatusConnected McpServerConnectionStatus = "connected"
	McpStatusPending   McpServerConnectionStatus = "pending"
	McpStatusFailed    McpServerConnectionStatus = "failed"
	McpStatusNeedsAuth McpServerConnectionStatus = "needs-auth"
	McpStatusDisabled  McpServerConnectionStatus = "disabled"
)

// McpServerInfo describes an MCP server's identity.
type McpServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// McpToolAnnotations is the wire format for tool annotations in MCP status responses.
// Note: field names differ from ToolAnnotations (no "Hint" suffix).
type McpToolAnnotations struct {
	ReadOnly    *bool `json:"readOnly,omitempty"`
	Destructive *bool `json:"destructive,omitempty"`
	OpenWorld   *bool `json:"openWorld,omitempty"`
}

// McpToolInfo describes a single tool in an MCP server status response.
type McpToolInfo struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Annotations *McpToolAnnotations `json:"annotations,omitempty"`
}

// McpSdkServerConfigStatus describes an SDK MCP server in status responses.
type McpSdkServerConfigStatus struct {
	Type string `json:"type"` // "sdk"
	Name string `json:"name"`
}

// McpClaudeAIProxyServerConfig describes a Claude AI proxy server in status responses.
type McpClaudeAIProxyServerConfig struct {
	Type string `json:"type"` // "claudeai-proxy"
	URL  string `json:"url,omitempty"`
	ID   string `json:"id,omitempty"`
}

// McpServerStatus represents the connection status of an MCP server.
type McpServerStatus struct {
	Name       string                    `json:"name"`
	Status     McpServerConnectionStatus `json:"status"`
	ServerInfo *McpServerInfo            `json:"serverInfo,omitempty"`
	Config     map[string]any            `json:"config,omitempty"`
	Error      string                    `json:"error,omitempty"`
	Scope      string                    `json:"scope,omitempty"`
	Tools      []McpToolInfo             `json:"tools,omitempty"`
}

// McpStatusResponse is returned by ClaudeSDKClient.GetMcpStatus.
type McpStatusResponse struct {
	MCPServers []McpServerStatus `json:"mcpServers"`
}
