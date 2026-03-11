package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// queryProto manages the bidirectional control protocol between the SDK and CLI.
type queryProto struct {
	transport *cliTransport
	opts      *ClaudeAgentOptions

	sdkMCPServers map[string]SdkMcpServer
	agents        map[string]map[string]any

	// hookCallbacks maps hookCallbackId ("hook_0", "hook_1", ...) to HookCallback.
	// Populated by Initialize() from opts.Hooks.
	hookCallbacks map[string]HookCallback
	// hookTimeouts maps hookCallbackId to a timeout in seconds (0 = no timeout).
	hookTimeouts map[string]float64
	// counter is used to derive hookCallbackIds.
	counter atomic.Uint64

	// initResult stores the server information received in the initialize response.
	initResult map[string]any

	// firstResultCh is closed the first time a "result" message is received.
	firstResultOnce sync.Once
	firstResultCh   chan struct{}

	// pending maps request_id to response channel for SDK-initiated control requests.
	pendingMu sync.Mutex
	pending   map[string]chan controlResult
}

type controlResult struct {
	data map[string]any
	err  error
}

func newQueryProto(t *cliTransport, opts *ClaudeAgentOptions) *queryProto {
	return &queryProto{
		transport:     t,
		opts:          opts,
		hookCallbacks: make(map[string]HookCallback),
		hookTimeouts:  make(map[string]float64),
		firstResultCh: make(chan struct{}),
		pending:       make(map[string]chan controlResult),
	}
}

func (q *queryProto) SetSDKMCPServers(servers map[string]SdkMcpServer) {
	q.sdkMCPServers = servers
}

func (q *queryProto) SetAgents(agents map[string]map[string]any) {
	q.agents = agents
}

// GetInitResult returns the server information received in the initialize response.
func (q *queryProto) GetInitResult() map[string]any {
	return q.initResult
}

// WaitForFirstResult blocks until the first "result" message is received, the
// timeout elapses, or ctx is cancelled.
func (q *queryProto) WaitForFirstResult(ctx context.Context, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-q.firstResultCh:
	case <-timer.C:
	case <-ctx.Done():
	}
}

// Initialize sends the SDK initialize control_request and waits for the response.
//
// Wire format sent:
//
//	{"type":"control_request","request_id":"<id>","request":{
//	  "subtype":"initialize",
//	  "hooks":{"PreToolUse":[{"matcher":"Bash","hookCallbackIds":["hook_0"],"timeout":null}]},
//	  "agents":{...}
//	}}
func (q *queryProto) Initialize(ctx context.Context) (map[string]any, error) {
	reqPayload := map[string]any{
		"subtype": "initialize",
	}

	if len(q.opts.Hooks) > 0 {
		hooksMap := make(map[string][]map[string]any, len(q.opts.Hooks))
		for event, matchers := range q.opts.Hooks {
			hookConfigs := make([]map[string]any, 0, len(matchers))
			for _, matcher := range matchers {
				ids := make([]string, 0, len(matcher.Hooks))
				for _, cb := range matcher.Hooks {
					id := fmt.Sprintf("hook_%d", q.counter.Add(1)-1)
					q.hookCallbacks[id] = cb
					q.hookTimeouts[id] = matcher.Timeout
					ids = append(ids, id)
				}
				cfg := map[string]any{
					"hookCallbackIds": ids,
				}
				// Always include "matcher" key; set to null when no matcher pattern.
				if matcher.Matcher != nil {
					cfg["matcher"] = *matcher.Matcher
				} else {
					cfg["matcher"] = nil
				}
				// Only include "timeout" when non-zero, matching Python SDK wire format.
				if matcher.Timeout > 0 {
					cfg["timeout"] = matcher.Timeout
				}
				hookConfigs = append(hookConfigs, cfg)
			}
			hooksMap[string(event)] = hookConfigs
		}
		reqPayload["hooks"] = hooksMap
	}

	if len(q.agents) > 0 {
		reqPayload["agents"] = q.agents
	}

	resp, err := q.SendControlRequest(ctx, reqPayload, 30*time.Second)
	if err != nil {
		return nil, &CLIConnectionError{Message: "failed to initialize SDK", Cause: err}
	}
	q.initResult = resp
	return resp, nil
}

// SendUserMessage sends a user turn to the CLI.
// Content is a plain string matching the Python SDK wire format.
func (q *queryProto) SendUserMessage(ctx context.Context, prompt string) error {
	msg := map[string]any{
		"type":               "user",
		"session_id":         "",
		"parent_tool_use_id": nil,
		"message": map[string]any{
			"role":    "user",
			"content": prompt,
		},
	}
	return q.writeJSON(ctx, msg)
}

// SendRawMessage sends a pre-built user message to the CLI.
func (q *queryProto) SendRawMessage(ctx context.Context, raw map[string]any) error {
	return q.writeJSON(ctx, raw)
}

// SendControlRequest sends a control request to the CLI and waits for its response.
//
// Wire format sent:
//
//	{"type":"control_request","request_id":"<id>","request":{...payload...}}
//
// Wire format received:
//
//	{"type":"control_response","response":{"subtype":"success","request_id":"<id>","response":{...}}}
//	{"type":"control_response","response":{"subtype":"error","request_id":"<id>","error":"msg"}}
func (q *queryProto) SendControlRequest(ctx context.Context, payload map[string]any, timeout time.Duration) (map[string]any, error) {
	reqID := newUUID()
	envelope := map[string]any{
		"type":       "control_request",
		"request_id": reqID,
		"request":    payload,
	}

	ch := make(chan controlResult, 1)
	q.pendingMu.Lock()
	q.pending[reqID] = ch
	q.pendingMu.Unlock()

	defer func() {
		q.pendingMu.Lock()
		delete(q.pending, reqID)
		q.pendingMu.Unlock()
	}()

	if err := q.writeJSON(ctx, envelope); err != nil {
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-ch:
		return result.data, result.err
	case <-timer.C:
		subtype, _ := payload["subtype"].(string)
		return nil, fmt.Errorf("control request %q timed out after %v", subtype, timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Run starts the main read loop. It reads raw messages from the transport,
// dispatches inbound control requests from the CLI, routes control responses to
// pending waiters, and forwards all other messages on the returned channel.
func (q *queryProto) Run(ctx context.Context) <-chan map[string]any {
	raws := q.transport.readMessages(ctx)
	out := make(chan map[string]any, 64)
	go func() {
		defer close(out)
		for raw := range raws {
			msgType := strVal(raw, "type")
			switch msgType {
			case "control_request":
				// Inbound control request from CLI → SDK.
				go q.handleInboundControlRequest(ctx, raw)

			case "control_cancel_request":
				// CLI is cancelling a pending inbound request — drop silently.

			case "control_response":
				// Response to an SDK-initiated control request.
				// Format: {"type":"control_response","response":{"subtype":"success","request_id":"...","response":{...}}}
				respEnv, ok := raw["response"].(map[string]any)
				if !ok {
					continue
				}
				reqID := strVal(respEnv, "request_id")
				q.pendingMu.Lock()
				ch, ok := q.pending[reqID]
				q.pendingMu.Unlock()
				if !ok {
					continue
				}
				subtype := strVal(respEnv, "subtype")
				if subtype == "error" {
					errMsg := strVal(respEnv, "error")
					ch <- controlResult{err: fmt.Errorf("control error: %s", errMsg)}
				} else {
					var data map[string]any
					if d, ok := respEnv["response"].(map[string]any); ok {
						data = d
					} else {
						data = map[string]any{}
					}
					ch <- controlResult{data: data}
				}

			case "result":
				// Signal stdin can be closed.
				q.firstResultOnce.Do(func() { close(q.firstResultCh) })
				select {
				case out <- raw:
				case <-ctx.Done():
					return
				}

			default:
				select {
				case out <- raw:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

// handleInboundControlRequest handles an inbound control_request from the CLI.
// Wire format: {"type":"control_request","request_id":"...","request":{"subtype":"...", ...}}
func (q *queryProto) handleInboundControlRequest(ctx context.Context, envelope map[string]any) {
	reqID := strVal(envelope, "request_id")
	req, _ := envelope["request"].(map[string]any)
	if req == nil {
		req = map[string]any{}
	}
	subtype := strVal(req, "subtype")

	var respData map[string]any
	var handlerErr error

	switch subtype {
	case "hook_callback":
		respData, handlerErr = q.handleHookCallback(ctx, req)
	case "can_use_tool":
		respData, handlerErr = q.handleCanUseTool(ctx, req)
	case "mcp_message":
		respData, handlerErr = q.handleMCPMessage(ctx, req)
	default:
		handlerErr = fmt.Errorf("unknown control subtype: %s", subtype)
	}

	inner := map[string]any{"request_id": reqID}
	if handlerErr != nil {
		inner["subtype"] = "error"
		inner["error"] = handlerErr.Error()
	} else {
		inner["subtype"] = "success"
		if respData == nil {
			respData = map[string]any{}
		}
		inner["response"] = respData
	}

	_ = q.writeJSON(ctx, map[string]any{"type": "control_response", "response": inner})
}

func (q *queryProto) handleHookCallback(ctx context.Context, req map[string]any) (map[string]any, error) {
	callbackID := strVal(req, "callback_id")
	cb, ok := q.hookCallbacks[callbackID]
	if !ok {
		return map[string]any{}, nil
	}

	var inputData map[string]any
	if d, ok := req["input"].(map[string]any); ok {
		inputData = d
	}
	toolUseID := strVal(req, "tool_use_id")

	cbCtx := ctx
	if timeout := q.hookTimeouts[callbackID]; timeout > 0 {
		var cancel context.CancelFunc
		cbCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout*float64(time.Second)))
		defer cancel()
	}

	result, err := cb(cbCtx, inputData, toolUseID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return convertHookOutput(result), nil
}

func (q *queryProto) handleCanUseTool(ctx context.Context, req map[string]any) (map[string]any, error) {
	if q.opts.CanUseTool == nil {
		return map[string]any{"behavior": "allow", "updatedInput": map[string]any{}}, nil
	}

	toolName := strVal(req, "tool_name")
	var toolInput map[string]any
	// CLI sends the field as "input" (matches Python SDK's SDKControlPermissionRequest.input).
	if d, ok := req["input"].(map[string]any); ok {
		toolInput = d
	}
	// permission_suggestions is a JSON array ([]any), not an object.
	var permCtx ToolPermissionContext
	if arr, ok := req["permission_suggestions"].([]any); ok {
		b, _ := json.Marshal(arr)
		_ = json.Unmarshal(b, &permCtx.Suggestions)
	}
	if sig, ok := req["signal"]; ok {
		permCtx.Signal = sig
	}

	result, err := q.opts.CanUseTool(ctx, toolName, toolInput, permCtx)
	if err != nil {
		return nil, err
	}

	resp := map[string]any{}
	switch r := result.(type) {
	case *PermissionResultAllow:
		resp["behavior"] = "allow"
		if r.UpdatedInput != nil {
			resp["updatedInput"] = r.UpdatedInput
		} else {
			// Fall back to original tool input (Python SDK: updated_input is None → send original_input)
			resp["updatedInput"] = toolInput
		}
		if len(r.UpdatedPermissions) > 0 {
			resp["updatedPermissions"] = r.UpdatedPermissions
		}
	case *PermissionResultDeny:
		resp["behavior"] = "deny"
		if r.Message != "" {
			resp["message"] = r.Message
		}
		resp["interrupt"] = r.Interrupt
	default:
		resp["behavior"] = "allow"
		resp["updatedInput"] = toolInput
	}
	return resp, nil
}

func (q *queryProto) handleMCPMessage(ctx context.Context, req map[string]any) (map[string]any, error) {
	serverName := strVal(req, "server_name")
	server, ok := q.sdkMCPServers[serverName]
	if !ok {
		return nil, fmt.Errorf("SDK MCP server %q not found", serverName)
	}

	// The JSONRPC message is nested inside req["message"].
	msgRaw, ok := req["message"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp_message missing 'message' field")
	}

	method := strVal(msgRaw, "method")
	msgID := msgRaw["id"]

	buildResponse := func(result map[string]any) map[string]any {
		return map[string]any{
			"mcp_response": map[string]any{
				"jsonrpc": "2.0",
				"id":      msgID,
				"result":  result,
			},
		}
	}
	buildError := func(code int, msg string) map[string]any {
		return map[string]any{
			"mcp_response": map[string]any{
				"jsonrpc": "2.0",
				"id":      msgID,
				"error": map[string]any{
					"code":    code,
					"message": msg,
				},
			},
		}
	}

	switch method {
	case "initialize":
		return buildResponse(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    serverName,
				"version": "1.0.0",
			},
		}), nil

	case "notifications/initialized":
		// Notification — no response body required.
		return map[string]any{"mcp_response": map[string]any{}}, nil

	case "tools/list":
		tools, err := server.ListTools(ctx)
		if err != nil {
			return buildError(-32603, err.Error()), nil
		}
		b, _ := json.Marshal(map[string]any{"tools": tools})
		var result map[string]any
		_ = json.Unmarshal(b, &result)
		return buildResponse(result), nil

	case "tools/call":
		var params map[string]any
		if p, ok := msgRaw["params"].(map[string]any); ok {
			params = p
		}
		toolName := strVal(params, "name")
		var toolArgs map[string]any
		if a, ok := params["arguments"].(map[string]any); ok {
			toolArgs = a
		}
		result, err := server.CallTool(ctx, toolName, toolArgs)
		if err != nil {
			return buildError(-32603, err.Error()), nil
		}
		b, _ := json.Marshal(result)
		var resultMap map[string]any
		_ = json.Unmarshal(b, &resultMap)
		return buildResponse(resultMap), nil

	default:
		return buildError(-32601, "method not found: "+method), nil
	}
}

func (q *queryProto) writeJSON(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("JSON marshal error: %w", err)
	}
	return q.transport.write(ctx, string(b))
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:4]) + "-" +
		hex.EncodeToString(b[4:6]) + "-" +
		hex.EncodeToString(b[6:8]) + "-" +
		hex.EncodeToString(b[8:10]) + "-" +
		hex.EncodeToString(b[10:])
}
