// Package claude is the Go SDK for Claude Code CLI.
//
// # Quick start
//
//	msgs, err := claude.Query(ctx, "Say hello", nil)
//	for msg := range msgs {
//	    // handle msg
//	}
//
// # Streaming client (multi-turn)
//
//	client, err := claude.NewClaudeSDKClient(ctx, nil)
//	if err != nil { ... }
//	defer client.Close()
//
//	if err := client.Query(ctx, "Hello"); err != nil { ... }
//	for msg := range client.ReceiveMessages(ctx) { ... }
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// -----------------------------------------------------------------------
// One-shot Query
// -----------------------------------------------------------------------

// Query runs a one-shot query against the Claude CLI and returns a read-only
// channel of typed messages.  The channel is closed after the final
// ResultMessage or when ctx is cancelled.
//
// opts may be nil (defaults apply).
func Query(ctx context.Context, prompt string, opts *ClaudeAgentOptions) (<-chan Message, error) {
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	return processQuery(ctx, prompt, nil, opts, nil)
}

// QueryStream is like Query but accepts a channel of raw user messages for
// multi-message input (required when using CanUseTool).
func QueryStream(ctx context.Context, promptCh <-chan map[string]any, opts *ClaudeAgentOptions) (<-chan Message, error) {
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	return processQuery(ctx, "", promptCh, opts, nil)
}

// -----------------------------------------------------------------------
// ClaudeSDKClient — streaming bidirectional client
// -----------------------------------------------------------------------

// ClaudeSDKClient maintains an open subprocess and supports multi-turn
// conversations with runtime control (interrupt, set model, MCP status, etc.).
//
// Use NewClaudeSDKClient to construct; call Close when done.
type ClaudeSDKClient struct {
	opts *ClaudeAgentOptions

	mu        sync.Mutex
	transport *cliTransport
	proto     *queryProto
	msgCh     <-chan map[string]any
	closed    bool
}

// NewClaudeSDKClient creates a new streaming client.  The underlying CLI
// subprocess is started and the initialize handshake runs before returning.
//
// opts may be nil (defaults apply).
func NewClaudeSDKClient(ctx context.Context, opts *ClaudeAgentOptions) (*ClaudeSDKClient, error) {
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}

	configuredOpts := *opts
	if opts.CanUseTool != nil && opts.PermissionPromptToolName != "" {
		return nil, &CLIConnectionError{
			Message: "CanUseTool and PermissionPromptToolName are mutually exclusive",
		}
	}
	if opts.CanUseTool != nil {
		configuredOpts.PermissionPromptToolName = "stdio"
	}

	t, err := newCLITransport(&configuredOpts)
	if err != nil {
		return nil, err
	}
	if err := t.connect(ctx); err != nil {
		return nil, err
	}

	// SDK MCP servers
	servers := map[string]SdkMcpServer{}
	for name, cfg := range configuredOpts.MCPServers {
		if s, ok := cfg.(*MCPSdkServerConfig); ok {
			servers[name] = s.Instance
		}
	}

	// Agents
	var agentsMap map[string]map[string]any
	if len(configuredOpts.Agents) > 0 {
		agentsMap = make(map[string]map[string]any, len(configuredOpts.Agents))
		for name, def := range configuredOpts.Agents {
			b, _ := json.Marshal(def)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			for k, v := range m {
				if v == nil || v == "" {
					delete(m, k)
				}
			}
			agentsMap[name] = m
		}
	}

	q := newQueryProto(t, &configuredOpts)
	q.SetSDKMCPServers(servers)
	q.SetAgents(agentsMap)
	if sp, ok := configuredOpts.SystemPrompt.(*SystemPromptPreset); ok && sp.ExcludeDynamicSections != nil {
		q.SetExcludeDynamicSections(sp.ExcludeDynamicSections)
	}

	client := &ClaudeSDKClient{
		opts:      &configuredOpts,
		transport: t,
		proto:     q,
	}

	// Start the read loop BEFORE sending the initialize request.
	// This mirrors Python SDK's `await query.start()` then `await query.initialize()`.
	// Without this, Initialize() sends a control_request but no goroutine is reading
	// the response, causing a 30-second timeout.
	client.msgCh = q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		_ = t.close()
		return nil, err
	}

	return client, nil
}

// checkConnected returns a CLIConnectionError if the client has not been
// initialised via NewClaudeSDKClient (proto == nil) or has already been
// closed.  All runtime methods on ClaudeSDKClient call this guard.
func (c *ClaudeSDKClient) checkConnected() error {
	if c.proto == nil {
		return &CLIConnectionError{Message: "Not connected"}
	}
	return nil
}

// Close terminates the CLI subprocess and releases resources.
func (c *ClaudeSDKClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.transport == nil {
		return nil
	}
	return c.transport.close()
}

// Query sends a user prompt to the CLI.  Call ReceiveMessages to read the
// response.  For async-iterable prompts use QueryStream instead.
func (c *ClaudeSDKClient) Query(ctx context.Context, prompt string) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	return c.proto.SendUserMessage(ctx, prompt)
}

// ReceiveMessages returns a read-only channel of parsed messages.
// The channel delivers all messages including ResultMessage and is NOT
// closed between turns — call it once and iterate.  It is closed when the
// subprocess exits.  Returns a closed channel immediately if not connected.
func (c *ClaudeSDKClient) ReceiveMessages(ctx context.Context) <-chan Message {
	out := make(chan Message, 64)
	if c.checkConnected() != nil {
		close(out)
		return out
	}
	go func() {
		defer close(out)
		for raw := range c.msgCh {
			msg, err := parseMessage(raw)
			if err != nil || msg == nil {
				continue
			}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// -----------------------------------------------------------------------
// Runtime control methods (streaming mode only)
// -----------------------------------------------------------------------

// SetPermissionMode changes the active permission mode at runtime.
func (c *ClaudeSDKClient) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "set_permission_mode",
		"mode":    string(mode),
	}, 10*time.Second)
	return err
}

// SetModel changes the active model at runtime.
// Pass nil to reset the model to the server's default (sends JSON null).
func (c *ClaudeSDKClient) SetModel(ctx context.Context, model *string) error {
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "set_model",
		"model":   model,
	}, 10*time.Second)
	return err
}

// Interrupt sends an interrupt signal to the current task.
func (c *ClaudeSDKClient) Interrupt(ctx context.Context) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "interrupt",
	}, 10*time.Second)
	return err
}

// StopTask requests that the CLI stop the named task.
// taskID must be the UUID of the task to stop.
func (c *ClaudeSDKClient) StopTask(ctx context.Context, taskID string) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "stop_task",
		"task_id": taskID,
	}, 10*time.Second)
	return err
}

// GetMcpStatus returns the connection status of all configured MCP servers.
func (c *ClaudeSDKClient) GetMcpStatus(ctx context.Context) (*McpStatusResponse, error) {
	if err := c.checkConnected(); err != nil {
		return nil, err
	}
	resp, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "mcp_status",
	}, 15*time.Second)
	if err != nil {
		return nil, err
	}

	b, _ := json.Marshal(resp)
	var status McpStatusResponse
	if err := json.Unmarshal(b, &status); err != nil {
		return nil, fmt.Errorf("failed to parse MCP status response: %w", err)
	}
	return &status, nil
}

// GetContextUsage returns a breakdown of current context window usage by category.
func (c *ClaudeSDKClient) GetContextUsage(ctx context.Context) (*ContextUsageResponse, error) {
	if err := c.checkConnected(); err != nil {
		return nil, err
	}
	resp, err := c.proto.GetContextUsage(ctx)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(resp)
	var usage ContextUsageResponse
	if err := json.Unmarshal(b, &usage); err != nil {
		return nil, fmt.Errorf("failed to parse context usage response: %w", err)
	}
	return &usage, nil
}

// ReconnectMcpServer requests that the CLI reconnect to the named MCP server.
func (c *ClaudeSDKClient) ReconnectMcpServer(ctx context.Context, serverName string) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype":    "mcp_reconnect",
		"serverName": serverName,
	}, 30*time.Second)
	return err
}

// ToggleMcpServer enables or disables the named MCP server at runtime.
func (c *ClaudeSDKClient) ToggleMcpServer(ctx context.Context, serverName string, enabled bool) error {
	if err := c.checkConnected(); err != nil {
		return err
	}
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype":    "mcp_toggle",
		"serverName": serverName,
		"enabled":    enabled,
	}, 10*time.Second)
	return err
}

// RewindFiles rewinds tracked files to their state at the given checkpoint
// (UserMessage UUID).  Requires EnableFileCheckpointing to have been set.
func (c *ClaudeSDKClient) RewindFiles(ctx context.Context, userMessageID string) error {
	_, err := c.proto.SendControlRequest(ctx, map[string]any{
		"subtype":         "rewind_files",
		"user_message_id": userMessageID,
	}, 15*time.Second)
	return err
}

// GetServerInfo returns the server information received during initialization.
// Returns nil if Initialize has not completed.
func (c *ClaudeSDKClient) GetServerInfo() map[string]any {
	return c.proto.GetInitResult()
}

// ReceiveResponse returns a channel that delivers messages for the current
// query turn and closes after the ResultMessage is received.
// This differs from ReceiveMessages which delivers all messages indefinitely.
// Returns a closed channel immediately if not connected.
func (c *ClaudeSDKClient) ReceiveResponse(ctx context.Context) <-chan Message {
	out := make(chan Message, 64)
	if c.checkConnected() != nil {
		close(out)
		return out
	}
	go func() {
		defer close(out)
		for raw := range c.msgCh {
			msg, err := parseMessage(raw)
			if err != nil || msg == nil {
				continue
			}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
			if _, ok := msg.(*ResultMessage); ok {
				return
			}
		}
	}()
	return out
}
