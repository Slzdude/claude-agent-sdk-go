package claude

// mock_transport_test.go provides cross-platform in-memory mock transports for
// tests. This mirrors the Python SDK's approach of mocking the transport layer
// entirely (unittest.mock.patch on SubprocessCLITransport) instead of spawning
// real subprocess shell scripts, which fail on Windows because .sh files cannot
// be directly executed via exec.Command.
//
// Each helper constructs a *cliTransport with in-memory io.Pipe connections for
// stdin/stdout instead of a real subprocess, then starts a goroutine to drive
// the mock protocol.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// mockTransportLines creates a *cliTransport backed by in-memory pipes that
// emits the provided JSON lines as if they were written by a CLI subprocess.
// Stdin writes from the transport are silently discarded.
//
// Use for tests that call q.Run() directly without q.Initialize().
func mockTransportLines(t *testing.T, lines ...string) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		// Drain stdin in the background so transport writes never block.
		go io.Copy(io.Discard, stdinR) //nolint:errcheck
		w := bufio.NewWriter(outW)
		for _, l := range lines {
			if _, err := w.WriteString(l + "\n"); err != nil {
				return
			}
		}
		_ = w.Flush()
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

// mockTransportWithInit creates a *cliTransport backed by in-memory pipes that:
//  1. reads the initialize control_request from "stdin",
//  2. writes back a success control_response with matching request_id,
//  3. emits the provided lines,
//  4. drains remaining stdin so writes from the SDK never block.
//
// This is the Go equivalent of Python's mocked SubprocessCLITransport that
// returns pre-configured lines.
func mockTransportWithInit(t *testing.T, lines ...string) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)

		// Read and respond to the initialize control_request.
		if sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err == nil {
				reqID, _ := req["request_id"].(string)
				resp := map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"request_id": reqID,
						"subtype":    "success",
						"response": map[string]any{
							"commands":     []any{},
							"output_style": "default",
						},
					},
				}
				b, _ := json.Marshal(resp)
				w := bufio.NewWriter(outW)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Emit provided lines.
		w := bufio.NewWriter(outW)
		for _, l := range lines {
			if _, err := w.WriteString(l + "\n"); err != nil {
				break
			}
		}
		_ = w.Flush()
		// outW is closed by defer; drain stdin in the background so that any
		// writes from the transport (e.g. Query) don't block on a full pipe.
		go func() {
			for sc.Scan() {
			}
			_ = stdinR.Close()
		}()
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

// mockTransportWithInitAndControl creates a *cliTransport that:
//  1. responds to the initialize control_request,
//  2. emits the provided lines,
//  3. reads one additional control_request from stdin and responds with
//     responseSubtype (e.g. "success").
//
// Use for streaming client tests that call control methods (Interrupt,
// SetModel, SetPermissionMode, etc.) after connecting.
func mockTransportWithInitAndControl(t *testing.T, responseSubtype string, lines ...string) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)
		w := bufio.NewWriter(outW)

		// Handle initialize.
		if sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err == nil {
				reqID, _ := req["request_id"].(string)
				resp := map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"request_id": reqID,
						"subtype":    "success",
						"response": map[string]any{
							"commands":     []any{},
							"output_style": "default",
						},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Emit provided lines.
		for _, l := range lines {
			if _, err := w.WriteString(l + "\n"); err != nil {
				return
			}
		}
		_ = w.Flush()

		// Read stdin lines until we find a control_request (skip user messages).
		for sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				continue
			}
			msgType, _ := req["type"].(string)
			if msgType != "control_request" {
				continue // skip user messages etc.
			}
			reqID, _ := req["request_id"].(string)
			resp := map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"request_id": reqID,
					"subtype":    responseSubtype,
					"response":   map[string]any{},
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			_ = w.WriteByte('\n')
			_ = w.Flush()
			break
		}
		// Drain remaining stdin.
		for sc.Scan() {
		}
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

// mockTransportHanging creates a *cliTransport that responds to the initialize
// control_request and then blocks reading stdin until it is closed.
//
// Use for tests that verify Close() / context-cancellation unblocks
// ReceiveResponse.  When closeStdin() or close() is called on the transport,
// the mock goroutine receives EOF on the pipe reader and exits, which in turn
// closes the stdout writer, causing readMessages to terminate.
func mockTransportHanging(t *testing.T) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)

		// Respond to initialize.
		if sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err == nil {
				reqID, _ := req["request_id"].(string)
				resp := map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"request_id": reqID,
						"subtype":    "success",
						"response": map[string]any{
							"commands":     []any{},
							"output_style": "default",
						},
					},
				}
				b, _ := json.Marshal(resp)
				w := bufio.NewWriter(outW)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Block until stdin is closed (by client.Close() or closeStdin()).
		// When the write end (tr.stdin) is closed, this loop exits and the
		// deferred outW.Close() propagates the termination signal.
		for sc.Scan() {
		}
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

// mockTransportWithMCP creates a *cliTransport that simulates the full MCP
// control-request round-trip used by TestQuery_MCPControlRequestsHandled and
// TestQuery_AsyncIterableMCPControlRequests:
//  1. responds to initialize,
//  2. emits two mcp_message control_requests for serverName,
//  3. reads and discards two SDK responses,
//  4. emits an assistant + result message.
func mockTransportWithMCP(t *testing.T, serverName string) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	mcpReq1, _ := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": "mcp_init_1",
		"request": map[string]any{
			"subtype":     "mcp_message",
			"server_name": serverName,
			"message":     map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		},
	})
	mcpReq2, _ := json.Marshal(map[string]any{
		"type":       "control_request",
		"request_id": "mcp_init_2",
		"request": map[string]any{
			"subtype":     "mcp_message",
			"server_name": serverName,
			"message":     map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": map[string]any{}},
		},
	})
	assistantLine, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "text", "text": "Hello!"}},
			"model":   "claude-opus-4-1-20250805",
		},
	})
	resultLine, _ := json.Marshal(map[string]any{
		"type":            "result",
		"subtype":         "success",
		"duration_ms":     100,
		"duration_api_ms": 80,
		"is_error":        false,
		"num_turns":       1,
		"session_id":      "test",
		"total_cost_usd":  0.001,
	})

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)
		w := bufio.NewWriter(outW)

		// Handle initialize.
		if sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err == nil {
				reqID, _ := req["request_id"].(string)
				resp := map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"request_id": reqID,
						"subtype":    "success",
						"response": map[string]any{
							"commands":     []any{},
							"output_style": "default",
						},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Emit MCP control_request 1, read SDK response.
		_, _ = w.Write(mcpReq1)
		_ = w.WriteByte('\n')
		_ = w.Flush()
		sc.Scan() // read SDK's mcp response

		// Emit MCP control_request 2, read SDK response.
		_, _ = w.Write(mcpReq2)
		_ = w.WriteByte('\n')
		_ = w.Flush()
		sc.Scan() // read SDK's mcp response

		// Emit final messages.
		_, _ = w.Write(assistantLine)
		_ = w.WriteByte('\n')
		_, _ = w.Write(resultLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Drain remaining stdin.
		for sc.Scan() {
		}
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}

// newMockSDKClient constructs a *ClaudeSDKClient directly from a pre-built
// in-memory transport, performing the same setup steps as NewClaudeSDKClient
// but without spawning a subprocess.
//
// This is the Go equivalent of Python's unittest.mock.patch on
// SubprocessCLITransport: the mock transport drives output in-process.
func newMockSDKClient(ctx context.Context, t *testing.T, opts *ClaudeAgentOptions, tr *cliTransport) (*ClaudeSDKClient, error) {
	t.Helper()
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	tr.opts = opts

	servers := map[string]SdkMcpServer{}
	for name, cfg := range opts.MCPServers {
		if s, ok := cfg.(*MCPSdkServerConfig); ok {
			servers[name] = s.Instance
		}
	}

	q := newQueryProto(tr, opts)
	q.SetSDKMCPServers(servers)

	client := &ClaudeSDKClient{
		opts:      opts,
		transport: tr,
		proto:     q,
	}
	client.msgCh = q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		_ = tr.close()
		return nil, err
	}
	return client, nil
}

// mockProcessQuery runs the processQuery logic using a pre-built in-memory
// transport in place of creating a subprocess from opts.CLIPath.
//
// Use this in tests instead of Query(ctx, prompt, opts) to avoid spawning a
// real subprocess (which requires a cross-platform executable).
func mockProcessQuery(ctx context.Context, t *testing.T, prompt string, tr *cliTransport, opts *ClaudeAgentOptions) (<-chan Message, error) {
	t.Helper()
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	configuredOpts := *opts

	sdkServers := map[string]SdkMcpServer{}
	for name, cfg := range configuredOpts.MCPServers {
		if s, ok := cfg.(*MCPSdkServerConfig); ok && s.Instance != nil {
			sdkServers[name] = s.Instance
		}
	}

	q := newQueryProto(tr, &configuredOpts)
	q.SetSDKMCPServers(sdkServers)

	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		_ = tr.close()
		return nil, err
	}

	if prompt != "" {
		if err := q.SendUserMessage(ctx, prompt); err != nil {
			_ = tr.close()
			return nil, err
		}
	}

	// Mirror processQuery stdin management.
	if len(sdkServers) > 0 || len(configuredOpts.Hooks) > 0 {
		go func() {
			q.WaitForFirstResult(ctx, 60*time.Second)
			_ = tr.closeStdin()
		}()
	} else if prompt != "" {
		_ = tr.closeStdin()
	}

	out := make(chan Message, 64)
	go func() {
		defer close(out)
		defer func() { _ = tr.close() }()
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

// mockQueryStream runs the QueryStream logic using a pre-built in-memory
// transport in place of creating a subprocess from opts.CLIPath.
func mockQueryStream(ctx context.Context, t *testing.T, promptCh <-chan map[string]any, tr *cliTransport, opts *ClaudeAgentOptions) (<-chan Message, error) {
	t.Helper()
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	configuredOpts := *opts

	sdkServers := map[string]SdkMcpServer{}
	for name, cfg := range configuredOpts.MCPServers {
		if s, ok := cfg.(*MCPSdkServerConfig); ok && s.Instance != nil {
			sdkServers[name] = s.Instance
		}
	}

	q := newQueryProto(tr, &configuredOpts)
	q.SetSDKMCPServers(sdkServers)

	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		_ = tr.close()
		return nil, err
	}

	// Relay promptCh messages; close stdin when the channel is drained.
	if promptCh != nil {
		go func() {
			for raw := range promptCh {
				if err := q.SendRawMessage(ctx, raw); err != nil {
					return
				}
			}
			_ = tr.closeStdin()
		}()
	}

	out := make(chan Message, 64)
	go func() {
		defer close(out)
		defer func() { _ = tr.close() }()
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

// mockTransportWithMcpStatus creates an in-memory transport that responds to
// initialize and then to an mcp_status control_request with the provided JSON payload.
func mockTransportWithMcpStatus(t *testing.T, mcpStatusJSON string) *cliTransport {
	t.Helper()
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)
		w := bufio.NewWriter(outW)

		// Handle initialize.
		if sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err == nil {
				reqID, _ := req["request_id"].(string)
				resp := map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"request_id": reqID,
						"subtype":    "success",
						"response": map[string]any{
							"commands":     []any{},
							"output_style": "default",
						},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Handle the mcp_status control_request.
		for sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				continue
			}
			msgType, _ := req["type"].(string)
			if msgType != "control_request" {
				continue
			}
			reqID, _ := req["request_id"].(string)
			var respPayload any
			_ = json.Unmarshal([]byte(mcpStatusJSON), &respPayload)
			resp := map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"request_id": reqID,
					"subtype":    "success",
					"response":   respPayload,
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			_ = w.WriteByte('\n')
			_ = w.Flush()
			break
		}
		// Drain remaining stdin.
		for sc.Scan() {
		}
	}()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc
	return tr
}
