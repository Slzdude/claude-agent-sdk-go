package claude

import (
	"context"
	"encoding/json"
	"time"
)

// processQuery is the core orchestrator: creates a transport, connects,
// initialises the session, optionally sends a user message, and returns
// a channel of typed Messages.
//
// Exactly one of prompt or promptCh must be set:
//   - prompt: one-shot single-message query
//   - promptCh: streaming multi-message query (used when CanUseTool is set)
//
// The returned channel is closed when the subprocess exits or ctx is cancelled.
func processQuery(
	ctx context.Context,
	prompt string,
	promptCh <-chan map[string]any,
	opts *ClaudeAgentOptions,
	_ any, // reserved for future use
) (<-chan Message, error) {
	// Validate mutual exclusions.
	if opts.CanUseTool != nil && opts.PermissionPromptToolName != "" {
		return nil, &CLIConnectionError{
			Message: "CanUseTool and PermissionPromptToolName are mutually exclusive",
		}
	}

	configuredOpts := *opts
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

	// Collect SDK MCP servers.
	sdkServers := map[string]SdkMcpServer{}
	for name, cfg := range configuredOpts.MCPServers {
		if s, ok := cfg.(*MCPSdkServerConfig); ok && s.Instance != nil {
			sdkServers[name] = s.Instance
		}
	}

	// Collect agent definitions.
	var agentsMap map[string]map[string]any
	if len(configuredOpts.Agents) > 0 {
		agentsMap = make(map[string]map[string]any, len(configuredOpts.Agents))
		for name, def := range configuredOpts.Agents {
			b, _ := json.Marshal(def)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			agentsMap[name] = m
		}
	}

	q := newQueryProto(t, &configuredOpts)
	q.SetSDKMCPServers(sdkServers)
	q.SetAgents(agentsMap)

	// Start the read loop BEFORE sending the initialize request.
	// This mirrors Python SDK's `await query.start()` then `await query.initialize()`.
	// Without this, Initialize() sends a control_request but no goroutine is reading
	// the response, causing a 30-second timeout.
	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		_ = t.close()
		return nil, err
	}

	// Send user input.
	if prompt != "" {
		if err := q.SendUserMessage(ctx, prompt); err != nil {
			_ = t.close()
			return nil, err
		}
	}
	// If a promptCh is provided the caller is responsible for sending messages.
	// We start a goroutine to relay them.
	if promptCh != nil {
		go func() {
			for raw := range promptCh {
				if err := q.SendRawMessage(ctx, raw); err != nil {
					return
				}
			}
			_ = t.closeStdin()
		}()
	} else {
		// For single-message queries: if hooks or SDK MCP servers are active,
		// defer stdin close until the first result is received (matching Python SDK).
		// This ensures the protocol stays open for inbound control requests.
		if len(sdkServers) > 0 || len(configuredOpts.Hooks) > 0 {
			go func() {
				q.WaitForFirstResult(ctx, 60*time.Second)
				_ = t.closeStdin()
			}()
		} else if prompt != "" {
			_ = t.closeStdin()
		}
	}
	out := make(chan Message, 64)

	go func() {
		defer close(out)
		defer func() { _ = t.close() }()

		for raw := range rawCh {
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

	return out, nil
}
