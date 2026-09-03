package claude

import "context"

// MCP protocol versions.
// See https://modelcontextprotocol.io/specification
const (
	MCPProtocolVersion20241105 = "2024-11-05"
	MCPProtocolVersion20250326 = "2025-03-26"
	MCPProtocolVersion20250618 = "2025-06-18"
	MCPProtocolVersion20251125 = "2025-11-25"
)

// MCPDefaultNegotiatedVersion is the fallback version returned when the
// client's requested version is not in the supported set.
// Matches Python SDK's DEFAULT_NEGOTIATED_VERSION.
const MCPDefaultNegotiatedVersion = MCPProtocolVersion20250326

// MCPSupportedProtocolVersions lists all protocol versions this SDK supports.
var MCPSupportedProtocolVersions = []string{
	MCPProtocolVersion20241105,
	MCPProtocolVersion20250326,
	MCPProtocolVersion20250618,
	MCPProtocolVersion20251125,
}

// negotiateProtocolVersion returns the protocol version to use in the
// initialize response. If the client's version is supported, it is echoed
// back; otherwise MCPDefaultNegotiatedVersion is returned.
// Matches Python SDK's version negotiation logic.
func negotiateProtocolVersion(clientVersion string) string {
	for _, v := range MCPSupportedProtocolVersions {
		if v == clientVersion {
			return clientVersion
		}
	}
	return MCPDefaultNegotiatedVersion
}

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
	Content           []map[string]any `json:"content"`
	StructuredContent map[string]any   `json:"structuredContent,omitempty"` // MCP 2025-03-26+
	IsError           bool             `json:"isError,omitempty"`
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
	// ListResources returns the list of resources. Return nil to indicate no resources.
	ListResources(ctx context.Context) ([]MCPResource, error)
	// ReadResource reads a resource by URI. Return error for unsupported URIs.
	ReadResource(ctx context.Context, uri string) (MCPResourceContent, error)
	// ListPrompts returns the list of prompts. Return nil to indicate no prompts.
	ListPrompts(ctx context.Context) ([]MCPPrompt, error)
	// GetPrompt gets a prompt by name with arguments. Return error for unknown prompts.
	GetPrompt(ctx context.Context, name string, arguments map[string]any) (MCPPromptResult, error)
}

// MCPTool describes a single tool exposed by an MCP server.
type MCPTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	InputSchema  map[string]any   `json:"inputSchema"`
	OutputSchema map[string]any   `json:"outputSchema,omitempty"` // MCP 2025-03-26+
	Annotations  *ToolAnnotations `json:"annotations,omitempty"`
	Meta         map[string]any   `json:"_meta,omitempty"`
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

// MCPResource describes a resource exposed by an MCP server.
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceContent is the content of a resource read from an MCP server.
type MCPResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// MCPPrompt describes a prompt exposed by an MCP server.
type MCPPrompt struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Arguments   []MCPPromptArg `json:"arguments,omitempty"`
}

// MCPPromptArg describes an argument to an MCP prompt.
type MCPPromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// MCPPromptResult is the result of getting a prompt from an MCP server.
type MCPPromptResult struct {
	Description string                   `json:"description,omitempty"`
	Messages    []MCPPromptResultMessage `json:"messages"`
}

// MCPPromptResultMessage is a single message in a prompt result.
// Content can be a string or a structured object per MCP spec.
type MCPPromptResultMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or map[string]any
}

// MCPAudioContent represents audio content in MCP responses.
// Added in MCP 2025-03-26.
type MCPAudioContent struct {
	Type     string `json:"type"`     // "audio"
	Data     string `json:"data"`     // base64 encoded audio
	MimeType string `json:"mimeType"` // e.g. "audio/wav", "audio/mp3"
}

// MCPResourceLink represents a link to a resource in tool/prompt results.
// Added in MCP 2025-03-26.
type MCPResourceLink struct {
	Type        string `json:"type"`        // "resource_link"
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// MCPResourceTemplate represents a resource template with a URI pattern.
// Added in MCP 2025-03-26.
type MCPResourceTemplate struct {
	URITemplate string `json:"uriTemplate"` // RFC 6570 URI template
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
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
