package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// invokeControlRequest calls handleInboundControlRequest with the given envelope
// and returns the JSON that was written to the transport (the wire-format response).
// This mirrors Python's: await query._handle_control_request(request);
// response_data = json.loads(transport.written_messages[-1])
func invokeControlRequest(t *testing.T, q *queryProto, envelope map[string]any) map[string]any {
	t.Helper()

	// Redirect the transport's stdin write to a pipe so we can capture it.
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// Set up a minimal cliTransport that writes to outW.
	if q.transport == nil {
		stdinDummyR, stdinDummyW, _ := os.Pipe()
		_ = stdinDummyR.Close()
		tr := &cliTransport{
			opts:          &ClaudeAgentOptions{},
			maxBufferSize: defaultMaxBufferSize,
		}
		tr.stdin = outW
		_ = stdinDummyW
		q.transport = tr
	} else {
		q.transport.stdin = outW
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		q.handleInboundControlRequest(context.Background(), envelope)
		_ = outW.Close()
	}()

	written, _ := io.ReadAll(outR)
	<-done

	line := strings.TrimSpace(string(written))
	if line == "" {
		t.Fatal("no response written to transport")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(line), &out); err != nil {
		t.Fatalf("failed to parse response JSON: %v\nraw: %s", err, line)
	}
	return out
}

// makeQueueProtoWithTransport builds a queryProto with a minimal fake transport
// for write capture, and a bufio.Scanner backed by an empty reader.
func makeQueryProtoWithHooks(t *testing.T, callbacks map[string]HookCallback) *queryProto {
	t.Helper()
	pr, pw := io.Pipe()
	go func() { _ = pw.Close() }()
	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdout = bufio.NewScanner(pr)
	tr.stdout.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	q := newQueryProto(tr, &ClaudeAgentOptions{})
	for id, cb := range callbacks {
		q.hookCallbacks[id] = cb
	}
	return q
}

// makeFakeTransport creates a cliTransport backed by a fake stdin/stdout pair
// so query protocol tests don't need a real subprocess.
// We use a pipe-based approach: write JSON lines to stdout, read from the transport.

// TestHandleCanUseTool_Allow verifies Allow sends behavior+updatedInput.
func TestHandleCanUseTool_Allow(t *testing.T) {
	updated := map[string]any{"command": "echo safe"}
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			return &PermissionResultAllow{UpdatedInput: updated}, nil
		},
	}
	q := &queryProto{opts: opts}
	req := map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "rm -rf /"},
	}
	resp, err := q.handleCanUseTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp["behavior"] != "allow" {
		t.Errorf("expected behavior=allow, got %v", resp["behavior"])
	}
	if ui, ok := resp["updatedInput"].(map[string]any); !ok || ui["command"] != "echo safe" {
		t.Errorf("wrong updatedInput: %v", resp["updatedInput"])
	}
	if _, hasInterrupt := resp["interrupt"]; hasInterrupt {
		t.Error("interrupt should not appear in Allow response")
	}
}

// TestHandleCanUseTool_AllowUpdatedPermissions checks updatedPermissions is sent.
func TestHandleCanUseTool_AllowUpdatedPermissions(t *testing.T) {
	perms := []PermissionUpdate{{Type: PermissionUpdateSetMode, Mode: PermissionModeAcceptEdits}}
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			return &PermissionResultAllow{UpdatedPermissions: perms}, nil
		},
	}
	q := &queryProto{opts: opts}
	resp, err := q.handleCanUseTool(context.Background(), map[string]any{"tool_name": "Edit"})
	if err != nil {
		t.Fatal(err)
	}
	if resp["updatedPermissions"] == nil {
		t.Error("updatedPermissions should be present")
	}
}

// TestHandleCanUseTool_Deny_InterruptTrue verifies interrupt propagates.
func TestHandleCanUseTool_Deny_InterruptTrue(t *testing.T) {
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			return &PermissionResultDeny{Message: "denied", Interrupt: true}, nil
		},
	}
	q := &queryProto{opts: opts}
	resp, err := q.handleCanUseTool(context.Background(), map[string]any{"tool_name": "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if resp["behavior"] != "deny" {
		t.Errorf("expected behavior=deny, got %v", resp["behavior"])
	}
	if resp["interrupt"] != true {
		t.Errorf("expected interrupt=true, got %v", resp["interrupt"])
	}
}

// TestHandleCanUseTool_Deny_InterruptFalse verifies interrupt=false when not set.
func TestHandleCanUseTool_Deny_InterruptFalse(t *testing.T) {
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			return &PermissionResultDeny{Message: "no"}, nil
		},
	}
	q := &queryProto{opts: opts}
	resp, err := q.handleCanUseTool(context.Background(), map[string]any{"tool_name": "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if resp["interrupt"] != false {
		t.Errorf("expected interrupt=false, got %v", resp["interrupt"])
	}
}

// TestHandleCanUseTool_NilCallbackAllows verifies default allow when CanUseTool is nil.
func TestHandleCanUseTool_NilCallbackAllows(t *testing.T) {
	q := &queryProto{opts: &ClaudeAgentOptions{}}
	resp, err := q.handleCanUseTool(context.Background(), map[string]any{"tool_name": "Bash"})
	if err != nil {
		t.Fatal(err)
	}
	if resp["behavior"] != "allow" {
		t.Errorf("expected default allow, got %v", resp["behavior"])
	}
}

// TestHandleCanUseTool_SignalPropagated verifies Signal is read from req.
func TestHandleCanUseTool_SignalPropagated(t *testing.T) {
	var capturedCtx ToolPermissionContext
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			capturedCtx = permCtx
			return &PermissionResultAllow{}, nil
		},
	}
	q := &queryProto{opts: opts}
	req := map[string]any{
		"tool_name": "Bash",
		"signal":    "stop_sequence",
	}
	_, err := q.handleCanUseTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if capturedCtx.Signal != "stop_sequence" {
		t.Errorf("expected Signal=%q, got %v", "stop_sequence", capturedCtx.Signal)
	}
}

// TestHandleHookCallback_BasicFlow tests callback lookup by ID and 3-param call.
func TestHandleHookCallback_BasicFlow(t *testing.T) {
	var calledInput map[string]any
	var calledToolUseID string

	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		hookCallbacks: map[string]HookCallback{},
		hookTimeouts:  map[string]float64{},
	}
	q.hookCallbacks["hook_0"] = func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		calledInput = input
		calledToolUseID = toolUseID
		return map[string]any{"continue_": true}, nil
	}

	req := map[string]any{
		"callback_id": "hook_0",
		"input":       map[string]any{"tool_name": "Bash"},
		"tool_use_id": "toolu_xyz",
	}
	resp, err := q.handleHookCallback(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if calledToolUseID != "toolu_xyz" {
		t.Errorf("toolUseID not passed: %q", calledToolUseID)
	}
	if calledInput["tool_name"] != "Bash" {
		t.Errorf("input not passed: %v", calledInput)
	}
	// continue_ should be converted to continue.
	if resp["continue"] == nil {
		t.Error("continue_ → continue conversion missing")
	}
	if _, hasOld := resp["continue_"]; hasOld {
		t.Error("continue_ should have been removed")
	}
}

// TestHandleHookCallback_UnknownIDReturnsEmpty verifies unknown callback_id is a no-op.
func TestHandleHookCallback_UnknownIDReturnsEmpty(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		hookCallbacks: map[string]HookCallback{},
		hookTimeouts:  map[string]float64{},
	}
	resp, err := q.handleHookCallback(context.Background(), map[string]any{"callback_id": "hook_99"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty map, got %v", resp)
	}
}

// TestConvertHookOutput_Translations checks async_→async and continue_→continue.
func TestConvertHookOutput_Translations(t *testing.T) {
	in := map[string]any{
		"async_":    true,
		"continue_": true,
		"decision":  "block",
	}
	out := convertHookOutput(in)
	if out["async"] != true {
		t.Error("async_ → async conversion failed")
	}
	if out["continue"] != true {
		t.Error("continue_ → continue conversion failed")
	}
	if out["decision"] != "block" {
		t.Error("decision should pass through unchanged")
	}
	if _, has := out["async_"]; has {
		t.Error("async_ should be removed")
	}
}

// TestHandleMCPMessage_Initialize verifies JSONRPC initialize response.
func TestHandleMCPMessage_Initialize(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"calc": &fakeMCPServer{}},
	}
	req := map[string]any{
		"server_name": "calc",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "initialize",
			"params":  map[string]any{},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result := mcp["result"].(map[string]any)
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("wrong protocolVersion: %v", result["protocolVersion"])
	}
}

// TestHandleMCPMessage_ToolsList verifies tools/list response.
func TestHandleMCPMessage_ToolsList(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"calc": &fakeMCPServer{}},
	}
	req := map[string]any{
		"server_name": "calc",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(2),
			"method":  "tools/list",
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result := mcp["result"].(map[string]any)
	tools, ok := result["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Errorf("expected non-empty tools list, got %v", result["tools"])
	}
}

// TestHandleMCPMessage_NotificationsInitialized verifies JSON-RPC response.
func TestHandleMCPMessage_NotificationsInitialized(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"calc": &fakeMCPServer{}},
	}
	req := map[string]any{
		"server_name": "calc",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"method":  "notifications/initialized",
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	if mcp["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", mcp["jsonrpc"])
	}
	if mcp["result"] == nil {
		t.Error("expected result field in mcp_response")
	}
}

// TestHandleMCPMessage_UnknownMethod returns -32601.
func TestHandleMCPMessage_UnknownMethod(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"calc": &fakeMCPServer{}},
	}
	req := map[string]any{
		"server_name": "calc",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(3),
			"method":  "tools/unknown",
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	errObj, ok := mcp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error field, got %v", mcp)
	}
	code, _ := errObj["code"].(int)
	if code != -32601 {
		t.Errorf("expected code -32601, got %d", code)
	}
}

// TestInitialize_RegistersHookCallbacks verifies hookCallbacks are populated.
func TestInitialize_RegistersHookCallbacks(t *testing.T) {
	var cb1Called, cb2Called bool
	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{
					Hooks: []HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							cb1Called = true
							return nil, nil
						},
					},
				},
				{
					Hooks: []HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							cb2Called = true
							return nil, nil
						},
					},
				},
			},
		},
	}

	// Build the hooks map as Initialize() does, then invoke each callback.
	q := newQueryProto(nil, opts)
	// Manually register without real transport.
	for _, matchers := range opts.Hooks {
		for _, matcher := range matchers {
			for _, cb := range matcher.Hooks {
				id := fmt.Sprintf("hook_%d", q.counter.Add(1)-1)
				q.hookCallbacks[id] = cb
				q.hookTimeouts[id] = matcher.Timeout
			}
		}
	}

	for id, cb := range q.hookCallbacks {
		_, _ = cb(context.Background(), nil, "")
		_ = id
	}
	if !cb1Called || !cb2Called {
		t.Errorf("not all hook callbacks were called: cb1=%v cb2=%v", cb1Called, cb2Called)
	}
}

// fakeMCPServer is a minimal SdkMcpServer for testing.
type fakeMCPServer struct{}

func (f *fakeMCPServer) Name() string    { return "fake" }
func (f *fakeMCPServer) Version() string { return "1.0.0" }
func (f *fakeMCPServer) ListTools(ctx context.Context) ([]MCPTool, error) {
	return []MCPTool{{Name: "add", Description: "Add two numbers", InputSchema: map[string]any{}}}, nil
}
func (f *fakeMCPServer) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{Content: []map[string]any{{"type": "text", "text": "42"}}}, nil
}

// -----------------------------------------------------------------------
// Extended MCP integration tests (mirrors test_sdk_mcp_integration.py)
// -----------------------------------------------------------------------

// annotatedMCPServer provides tools with ToolAnnotations set.
type annotatedMCPServer struct{}

func (s *annotatedMCPServer) Name() string    { return "annotated" }
func (s *annotatedMCPServer) Version() string { return "1.0.0" }
func (s *annotatedMCPServer) ListTools(ctx context.Context) ([]MCPTool, error) {
	readOnly := true
	destructive := true
	idempotent := true
	openWorld := true
	return []MCPTool{
		{
			Name:        "read_data",
			Description: "Read data",
			InputSchema: map[string]any{},
			Annotations: &ToolAnnotations{ReadOnlyHint: &readOnly},
		},
		{
			Name:        "delete_item",
			Description: "Delete an item",
			InputSchema: map[string]any{},
			Annotations: &ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: &idempotent},
		},
		{
			Name:        "search",
			Description: "Search",
			InputSchema: map[string]any{},
			Annotations: &ToolAnnotations{OpenWorldHint: &openWorld},
		},
		{
			Name:        "no_annotations",
			Description: "No annotations",
			InputSchema: map[string]any{},
		},
	}, nil
}
func (s *annotatedMCPServer) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{Content: []map[string]any{{"type": "text", "text": "ok"}}}, nil
}

// errorMCPServer always returns an error from CallTool.
type errorMCPServer struct{}

func (s *errorMCPServer) Name() string    { return "error-server" }
func (s *errorMCPServer) Version() string { return "1.0.0" }
func (s *errorMCPServer) ListTools(ctx context.Context) ([]MCPTool, error) {
	return []MCPTool{{Name: "fail", Description: "Always fails", InputSchema: map[string]any{}}}, nil
}
func (s *errorMCPServer) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{}, fmt.Errorf("expected error from tool")
}

// imageMCPServer returns image content from CallTool.
type imageMCPServer struct{}

func (s *imageMCPServer) Name() string    { return "image-server" }
func (s *imageMCPServer) Version() string { return "1.0.0" }
func (s *imageMCPServer) ListTools(ctx context.Context) ([]MCPTool, error) {
	return []MCPTool{{Name: "chart", Description: "Generate chart", InputSchema: map[string]any{}}}, nil
}
func (s *imageMCPServer) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{
		Content: []map[string]any{
			{"type": "text", "text": "Generated chart"},
			{"type": "image", "data": "base64data==", "mimeType": "image/png"},
		},
	}, nil
}

// TestHandleMCPMessage_AnnotationsInListTools verifies that ToolAnnotations
// are included in the JSONRPC tools/list response.
func TestHandleMCPMessage_AnnotationsInListTools(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"annotated": &annotatedMCPServer{}},
	}
	req := map[string]any{
		"server_name": "annotated",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/list",
			"params":  map[string]any{},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result, ok := mcp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T: %v", mcp["result"], mcp["result"])
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools array, got %T", result["tools"])
	}
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	// Build name→tool map. Tools are serialized via JSON round-trip in
	// handleMCPMessage, so annotations appear as map[string]any.
	byName := map[string]map[string]any{}
	for _, ti := range tools {
		tm := ti.(map[string]any)
		byName[tm["name"].(string)] = tm
	}

	// read_data should have readOnlyHint=true
	if ann, ok := byName["read_data"]["annotations"].(map[string]any); !ok || ann["readOnlyHint"] != true {
		t.Errorf("read_data annotations wrong: %v", byName["read_data"]["annotations"])
	}
	// delete_item should have destructiveHint + idempotentHint
	if ann, ok := byName["delete_item"]["annotations"].(map[string]any); !ok || ann["destructiveHint"] != true || ann["idempotentHint"] != true {
		t.Errorf("delete_item annotations wrong: %v", byName["delete_item"]["annotations"])
	}
	// search should have openWorldHint
	if ann, ok := byName["search"]["annotations"].(map[string]any); !ok || ann["openWorldHint"] != true {
		t.Errorf("search annotations wrong: %v", byName["search"]["annotations"])
	}
	// no_annotations should have nil/missing annotations
	if ann := byName["no_annotations"]["annotations"]; ann != nil {
		t.Errorf("expected no annotations, got: %v", ann)
	}
}

// TestHandleMCPMessage_CallToolErrorHandling verifies that CallTool errors are
// reported as JSONRPC error responses.
func TestHandleMCPMessage_CallToolErrorHandling(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"error-server": &errorMCPServer{}},
	}
	req := map[string]any{
		"server_name": "error-server",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/call",
			"params":  map[string]any{"name": "fail", "arguments": map[string]any{}},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	// Should have an error field in the JSONRPC response, not a result.
	if _, hasErr := mcp["error"]; !hasErr {
		t.Errorf("expected error in mcp_response, got: %v", mcp)
	}
}

// TestHandleMCPMessage_ImageContent verifies that image content is included
// in the tools/call JSONRPC response.
func TestHandleMCPMessage_ImageContent(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"image-server": &imageMCPServer{}},
	}
	req := map[string]any{
		"server_name": "image-server",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/call",
			"params":  map[string]any{"name": "chart", "arguments": map[string]any{}},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result, ok := mcp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result map, got %T: %v", mcp["result"], mcp["result"])
	}
	content, ok := result["content"].([]any)
	if !ok {
		t.Fatalf("expected content array, got %T", result["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content items, got %d", len(content))
	}
	text := content[0].(map[string]any)
	if text["type"] != "text" {
		t.Errorf("expected text type, got %v", text["type"])
	}
	image := content[1].(map[string]any)
	if image["type"] != "image" || image["data"] != "base64data==" || image["mimeType"] != "image/png" {
		t.Errorf("unexpected image content: %v", image)
	}
}

// TestHandleMCPMessage_MultipleServers verifies that different server names
// are routed to the correct SdkMcpServer.
func TestHandleMCPMessage_MultipleServers(t *testing.T) {
	q := &queryProto{
		opts: &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{
			"calc":   &fakeMCPServer{},
			"images": &imageMCPServer{},
		},
	}

	// Call calc server
	calcReq := map[string]any{
		"server_name": "calc",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/call",
			"params":  map[string]any{"name": "add", "arguments": map[string]any{"a": 1, "b": 2}},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), calcReq)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result := mcp["result"].(map[string]any)
	content := result["content"].([]any)
	if content[0].(map[string]any)["text"] != "42" {
		t.Errorf("unexpected calc result: %v", content)
	}

	// Call images server
	imgReq := map[string]any{
		"server_name": "images",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(2),
			"method":  "tools/call",
			"params":  map[string]any{"name": "chart", "arguments": map[string]any{}},
		},
	}
	resp2, err := q.handleMCPMessage(context.Background(), imgReq)
	if err != nil {
		t.Fatal(err)
	}
	mcp2 := resp2["mcp_response"].(map[string]any)
	result2 := mcp2["result"].(map[string]any)
	content2 := result2["content"].([]any)
	if len(content2) != 2 {
		t.Errorf("expected 2 content items from image server, got %d", len(content2))
	}
}

// -----------------------------------------------------------------------
// Hook output field tests (mirrors test_tool_callbacks.py)
// -----------------------------------------------------------------------

// TestHandleHookCallback_OutputFields mirrors test_hook_output_fields:
// verifies that all SyncHookJSONOutput fields (continue_, suppressOutput,
// stopReason, decision, systemMessage, reason, hookSpecificOutput) are
// passed through the full wire path correctly.
func TestHandleHookCallback_OutputFields(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_fields": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"continue_":      true,
				"suppressOutput": false,
				"stopReason":     "Test stop reason",
				"decision":       "block",
				"systemMessage":  "Test system message",
				"reason":         "Test reason for blocking",
				"hookSpecificOutput": map[string]any{
					"hookEventName":            "PreToolUse",
					"permissionDecision":       "deny",
					"permissionDecisionReason": "Security policy violation",
					"updatedInput":             map[string]any{"modified": "input"},
				},
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-comprehensive",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_fields",
			"input":       map[string]any{"test": "data"},
			"tool_use_id": "tool-456",
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	if inner["subtype"] != "success" {
		t.Fatalf("expected success, got: %v", inner["subtype"])
	}
	result := inner["response"].(map[string]any)

	if result["continue"] != true {
		t.Errorf("continue_ should be converted to continue=true, got %v", result["continue"])
	}
	if _, has := result["continue_"]; has {
		t.Error("continue_ should not appear in wire output")
	}
	if result["suppressOutput"] != false {
		t.Errorf("suppressOutput should be false, got %v", result["suppressOutput"])
	}
	if result["stopReason"] != "Test stop reason" {
		t.Errorf("stopReason mismatch: %v", result["stopReason"])
	}
	if result["decision"] != "block" {
		t.Errorf("decision mismatch: %v", result["decision"])
	}
	if result["systemMessage"] != "Test system message" {
		t.Errorf("systemMessage mismatch: %v", result["systemMessage"])
	}
	if result["reason"] != "Test reason for blocking" {
		t.Errorf("reason mismatch: %v", result["reason"])
	}
	hookOut, ok := result["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("hookSpecificOutput missing or wrong type: %v", result["hookSpecificOutput"])
	}
	if hookOut["hookEventName"] != "PreToolUse" {
		t.Errorf("hookEventName mismatch: %v", hookOut["hookEventName"])
	}
	if hookOut["permissionDecision"] != "deny" {
		t.Errorf("permissionDecision mismatch: %v", hookOut["permissionDecision"])
	}
	if hookOut["permissionDecisionReason"] != "Security policy violation" {
		t.Errorf("permissionDecisionReason mismatch")
	}
	if _, hasUpdated := hookOut["updatedInput"]; !hasUpdated {
		t.Error("updatedInput missing from hookSpecificOutput")
	}
}

// TestHandleHookCallback_AsyncFieldConversion mirrors test_async_hook_output:
// verifies async_ → async field conversion through the full wire path.
func TestHandleHookCallback_AsyncFieldConversion(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_async": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"async_":       true,
				"asyncTimeout": float64(5000),
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-async",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_async",
			"input":       map[string]any{"test": "async_data"},
			"tool_use_id": nil,
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	if inner["subtype"] != "success" {
		t.Fatalf("expected success subtype, got %v", inner["subtype"])
	}
	result := inner["response"].(map[string]any)

	if result["async"] != true {
		t.Errorf("async_ should convert to async=true, got %v", result["async"])
	}
	if _, has := result["async_"]; has {
		t.Error("async_ should not appear in wire output")
	}
	if result["asyncTimeout"] == nil {
		t.Error("asyncTimeout missing from response")
	}
}

// TestHandleHookCallback_BothFieldConversions mirrors test_field_name_conversion:
// verifies both async_→async AND continue_→continue conversions together.
func TestHandleHookCallback_BothFieldConversions(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_conv": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"async_":        true,
				"asyncTimeout":  float64(10000),
				"continue_":     false,
				"stopReason":    "Testing field conversion",
				"systemMessage": "Fields should be converted",
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-conversion",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_conv",
			"input":       map[string]any{"test": "data"},
			"tool_use_id": nil,
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	result := inner["response"].(map[string]any)

	if result["async"] != true {
		t.Errorf("async_ → async failed: %v", result["async"])
	}
	if _, has := result["async_"]; has {
		t.Error("async_ should be removed from output")
	}
	if result["continue"] != false {
		t.Errorf("continue_ → continue failed: %v", result["continue"])
	}
	if _, has := result["continue_"]; has {
		t.Error("continue_ should be removed from output")
	}
	if result["asyncTimeout"] == nil {
		t.Error("asyncTimeout should be preserved")
	}
	if result["stopReason"] != "Testing field conversion" {
		t.Errorf("stopReason mismatch: %v", result["stopReason"])
	}
	if result["systemMessage"] != "Fields should be converted" {
		t.Errorf("systemMessage mismatch: %v", result["systemMessage"])
	}
}

// TestClaudeAgentOptions_WithCallbacks mirrors test_options_with_callbacks:
// verifies that ClaudeAgentOptions accepts CanUseTool and Hooks fields.
func TestClaudeAgentOptions_WithCallbacks(t *testing.T) {
	myCb := func(ctx context.Context, toolName string, input map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
		return &PermissionResultAllow{}, nil
	}
	myHook := func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		return map[string]any{}, nil
	}
	matcherStr := "Bash"

	opts := &ClaudeAgentOptions{
		CanUseTool: myCb,
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{Matcher: &matcherStr, Hooks: []HookCallback{myHook}},
			},
		},
	}

	if opts.CanUseTool == nil {
		t.Error("CanUseTool should be set")
	}
	matchers, ok := opts.Hooks[HookEventPreToolUse]
	if !ok || len(matchers) != 1 {
		t.Errorf("expected 1 PreToolUse matcher, got %d", len(matchers))
	}
	if len(matchers[0].Hooks) != 1 {
		t.Errorf("expected 1 hook in matcher, got %d", len(matchers[0].Hooks))
	}
}

// TestHandleHookCallback_NotificationEvent mirrors test_notification_hook_callback.
func TestHandleHookCallback_NotificationEvent(t *testing.T) {
	var capturedInput map[string]any
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_notification": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			capturedInput = input
			return map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":     "Notification",
					"additionalContext": "Notification processed",
				},
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-notification",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_notification",
			"input": map[string]any{
				"session_id":        "sess-1",
				"transcript_path":   "/tmp/t",
				"cwd":               "/home",
				"hook_event_name":   "Notification",
				"message":           "Task completed",
				"notification_type": "info",
			},
			"tool_use_id": nil,
		},
	}

	resp := invokeControlRequest(t, q, envelope)

	if capturedInput["hook_event_name"] != "Notification" {
		t.Errorf("hook_event_name mismatch: %v", capturedInput["hook_event_name"])
	}
	if capturedInput["message"] != "Task completed" {
		t.Errorf("message mismatch: %v", capturedInput["message"])
	}

	inner := resp["response"].(map[string]any)
	result := inner["response"].(map[string]any)
	hookOut := result["hookSpecificOutput"].(map[string]any)
	if hookOut["hookEventName"] != "Notification" {
		t.Errorf("hookEventName mismatch: %v", hookOut["hookEventName"])
	}
	if hookOut["additionalContext"] != "Notification processed" {
		t.Errorf("additionalContext mismatch: %v", hookOut["additionalContext"])
	}
}

// TestHandleHookCallback_PermissionRequestEvent mirrors test_permission_request_hook_callback.
func TestHandleHookCallback_PermissionRequestEvent(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_perm_req": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName": "PermissionRequest",
					"decision":      map[string]any{"type": "allow"},
				},
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-perm-req",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_perm_req",
			"input": map[string]any{
				"session_id":      "sess-1",
				"hook_event_name": "PermissionRequest",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
			},
			"tool_use_id": nil,
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	result := inner["response"].(map[string]any)
	hookOut := result["hookSpecificOutput"].(map[string]any)
	if hookOut["hookEventName"] != "PermissionRequest" {
		t.Errorf("hookEventName mismatch: %v", hookOut["hookEventName"])
	}
	decision, ok := hookOut["decision"].(map[string]any)
	if !ok || decision["type"] != "allow" {
		t.Errorf("decision mismatch: %v", hookOut["decision"])
	}
}

// TestHandleHookCallback_SubagentStartEvent mirrors test_subagent_start_hook_callback.
func TestHandleHookCallback_SubagentStartEvent(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_subagent": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":     "SubagentStart",
					"additionalContext": "Subagent approved",
				},
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-subagent-start",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_subagent",
			"input": map[string]any{
				"session_id":      "sess-1",
				"hook_event_name": "SubagentStart",
				"agent_id":        "agent-42",
				"agent_type":      "researcher",
			},
			"tool_use_id": nil,
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	result := inner["response"].(map[string]any)
	hookOut := result["hookSpecificOutput"].(map[string]any)
	if hookOut["hookEventName"] != "SubagentStart" {
		t.Errorf("hookEventName mismatch: %v", hookOut["hookEventName"])
	}
	if hookOut["additionalContext"] != "Subagent approved" {
		t.Errorf("additionalContext mismatch: %v", hookOut["additionalContext"])
	}
}

// TestHandleHookCallback_PostToolUseUpdatedMCPOutput mirrors test_post_tool_use_hook_with_updated_mcp_output.
func TestHandleHookCallback_PostToolUseUpdatedMCPOutput(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_post_tool": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":        "PostToolUse",
					"updatedMCPToolOutput": map[string]any{"result": "modified output"},
				},
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-post-tool-mcp",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_post_tool",
			"input": map[string]any{
				"session_id":      "sess-1",
				"hook_event_name": "PostToolUse",
				"tool_name":       "mcp_tool",
				"tool_input":      map[string]any{},
				"tool_response":   "original output",
				"tool_use_id":     "tu-123",
			},
			"tool_use_id": "tu-123",
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	result := inner["response"].(map[string]any)
	hookOut := result["hookSpecificOutput"].(map[string]any)
	updatedOutput, ok := hookOut["updatedMCPToolOutput"].(map[string]any)
	if !ok {
		t.Fatalf("updatedMCPToolOutput missing or wrong type: %v", hookOut["updatedMCPToolOutput"])
	}
	if updatedOutput["result"] != "modified output" {
		t.Errorf("updatedMCPToolOutput.result mismatch: %v", updatedOutput["result"])
	}
}

// TestHandleHookCallback_PreToolUseAdditionalContext mirrors test_pre_tool_use_hook_with_additional_context.
func TestHandleHookCallback_PreToolUseAdditionalContext(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"cb_pre_tool": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return map[string]any{
				"hookSpecificOutput": map[string]any{
					"hookEventName":      "PreToolUse",
					"permissionDecision": "allow",
					"additionalContext":  "Extra context for Claude",
				},
			}, nil
		},
	})

	envelope := map[string]any{
		"request_id": "test-pre-tool-ctx",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "cb_pre_tool",
			"input": map[string]any{
				"session_id":      "sess-1",
				"hook_event_name": "PreToolUse",
				"tool_name":       "Bash",
				"tool_input":      map[string]any{"command": "ls"},
				"tool_use_id":     "tu-456",
			},
			"tool_use_id": "tu-456",
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	inner := resp["response"].(map[string]any)
	result := inner["response"].(map[string]any)
	hookOut := result["hookSpecificOutput"].(map[string]any)
	if hookOut["additionalContext"] != "Extra context for Claude" {
		t.Errorf("additionalContext mismatch: %v", hookOut["additionalContext"])
	}
	if hookOut["permissionDecision"] != "allow" {
		t.Errorf("permissionDecision mismatch: %v", hookOut["permissionDecision"])
	}
}

// TestNewHookEventsRegisteredInHooksConfig mirrors test_new_hook_events_registered_in_hooks_config:
// verifies that Notification, SubagentStart, and PermissionRequest can be used as hook event keys.
func TestNewHookEventsRegisteredInHooksConfig(t *testing.T) {
	noopHook := func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		return map[string]any{}, nil
	}

	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventNotification:      {{Hooks: []HookCallback{noopHook}}},
			HookEventSubagentStart:     {{Hooks: []HookCallback{noopHook}}},
			HookEventPermissionRequest: {{Hooks: []HookCallback{noopHook}}},
		},
	}

	if _, ok := opts.Hooks[HookEventNotification]; !ok {
		t.Error("HookEventNotification should be a valid hook event key")
	}
	if _, ok := opts.Hooks[HookEventSubagentStart]; !ok {
		t.Error("HookEventSubagentStart should be a valid hook event key")
	}
	if _, ok := opts.Hooks[HookEventPermissionRequest]; !ok {
		t.Error("HookEventPermissionRequest should be a valid hook event key")
	}
	if len(opts.Hooks) != 3 {
		t.Errorf("expected 3 hook events, got %d", len(opts.Hooks))
	}
}

// -----------------------------------------------------------------------
// Tests for new features (Python SDK v0.1.49–v0.1.58)
// -----------------------------------------------------------------------

// TestHandleCanUseTool_WithToolUseIDAndAgentID verifies that tool_use_id
// and agent_id are extracted from permission requests and passed in ToolPermissionContext.
func TestHandleCanUseTool_WithToolUseIDAndAgentID(t *testing.T) {
	var capturedCtx ToolPermissionContext
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			capturedCtx = permCtx
			return &PermissionResultAllow{}, nil
		},
	}
	q := &queryProto{opts: opts}
	req := map[string]any{
		"subtype":     "can_use_tool",
		"tool_name":   "Bash",
		"input":       map[string]any{"command": "echo hi"},
		"tool_use_id": "toolu_123",
		"agent_id":    "agent_456",
	}
	_, err := q.handleCanUseTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if capturedCtx.ToolUseID != "toolu_123" {
		t.Errorf("wrong ToolUseID: %q", capturedCtx.ToolUseID)
	}
	if capturedCtx.AgentID != "agent_456" {
		t.Errorf("wrong AgentID: %q", capturedCtx.AgentID)
	}
}

// TestControlCancelRequest_Handling verifies that control_cancel_request
// cancels inflight request handlers via their CancelFunc.
func TestControlCancelRequest_Handling(t *testing.T) {
	q := &queryProto{
		opts:              &ClaudeAgentOptions{},
		hookCallbacks:     make(map[string]HookCallback),
		hookTimeouts:      make(map[string]float64),
		firstResultCh:     make(chan struct{}),
		pending:           make(map[string]chan controlResult),
		inflightHandlers:  make(map[string]context.CancelFunc),
	}

	// Register a cancel function for a fake inflight request.
	ctx, cancel := context.WithCancel(context.Background())
	q.inflightHandlers["req-1"] = cancel

	// Simulate receiving a control_cancel_request.
	cancelled := false
	go func() {
		<-ctx.Done()
		cancelled = true
	}()

	// Process the cancel request (same logic as in Run()).
	cancelID := "req-1"
	q.inflightMu.Lock()
	cb, ok := q.inflightHandlers[cancelID]
	delete(q.inflightHandlers, cancelID)
	q.inflightMu.Unlock()
	if ok {
		cb()
	}

	// Wait for cancellation to propagate.
	time.Sleep(50 * time.Millisecond)
	if !cancelled {
		t.Error("handler context was not cancelled")
	}

	// Verify handler was removed from inflight map.
	q.inflightMu.Lock()
	_, still := q.inflightHandlers["req-1"]
	q.inflightMu.Unlock()
	if still {
		t.Error("inflight handler should be removed after cancel")
	}
}

// TestControlCancelRequest_UnknownID verifies that cancelling an unknown
// request ID is a no-op (no panic).
func TestControlCancelRequest_UnknownID(t *testing.T) {
	q := &queryProto{
		opts:              &ClaudeAgentOptions{},
		inflightHandlers:  make(map[string]context.CancelFunc),
	}

	// Should not panic.
	q.inflightMu.Lock()
	cb, ok := q.inflightHandlers["unknown-req"]
	delete(q.inflightHandlers, "unknown-req")
	q.inflightMu.Unlock()
	if ok {
		cb()
	}
}

// TestMCPServer_MetaForwarding verifies that maxResultSizeChars is forwarded via _meta.
func TestMCPServer_MetaForwarding(t *testing.T) {
	maxSize := 100000
	server := &metaMCPServer{maxResultSizeChars: &maxSize}
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"meta-test": server},
	}
	req := map[string]any{
		"server_name": "meta-test",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/list",
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result := mcp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}
	tool := tools[0].(map[string]any)
	meta, ok := tool["_meta"].(map[string]any)
	if !ok {
		t.Fatal("expected _meta on tool")
	}
	if meta["anthropic/maxResultSizeChars"] != float64(100000) {
		t.Errorf("wrong maxResultSizeChars in _meta: %v", meta["anthropic/maxResultSizeChars"])
	}
}

type metaMCPServer struct {
	maxResultSizeChars *int
}

func (s *metaMCPServer) Name() string    { return "meta-test" }
func (s *metaMCPServer) Version() string { return "1.0.0" }
func (s *metaMCPServer) ListTools(ctx context.Context) ([]MCPTool, error) {
	return []MCPTool{
		{
			Name:        "big_result",
			Description: "Returns a large result",
			InputSchema: map[string]any{},
			Annotations: &ToolAnnotations{MaxResultSizeChars: s.maxResultSizeChars},
		},
	}, nil
}
func (s *metaMCPServer) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{Content: []map[string]any{{"type": "text", "text": "big"}}}, nil
}

// TestMCPContentTypes_ResourceLink verifies that resource_link content is passed through.
func TestMCPContentTypes_ResourceLink(t *testing.T) {
	server := &contentTypeMCPServer{
		content: []map[string]any{
			{"type": "resource_link", "name": "My Doc", "uri": "file:///doc.txt", "description": "A doc"},
		},
	}
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"ct-test": server},
	}
	req := map[string]any{
		"server_name": "ct-test",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/call",
			"params":  map[string]any{"name": "test", "arguments": map[string]any{}},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result := mcp["result"].(map[string]any)
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
	item := content[0].(map[string]any)
	if item["type"] != "resource_link" {
		t.Errorf("expected resource_link type, got %v", item["type"])
	}
}

// TestMCPContentTypes_EmbeddedResource verifies that embedded resource content is passed through.
func TestMCPContentTypes_EmbeddedResource(t *testing.T) {
	server := &contentTypeMCPServer{
		content: []map[string]any{
			{"type": "resource", "resource": map[string]any{"text": "embedded text", "uri": "file:///r.txt"}},
		},
	}
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		sdkMCPServers: map[string]SdkMcpServer{"ct-test": server},
	}
	req := map[string]any{
		"server_name": "ct-test",
		"message": map[string]any{
			"jsonrpc": "2.0",
			"id":      float64(1),
			"method":  "tools/call",
			"params":  map[string]any{"name": "test", "arguments": map[string]any{}},
		},
	}
	resp, err := q.handleMCPMessage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	mcp := resp["mcp_response"].(map[string]any)
	result := mcp["result"].(map[string]any)
	content := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
}

type contentTypeMCPServer struct {
	content []map[string]any
}

func (s *contentTypeMCPServer) Name() string    { return "ct-test" }
func (s *contentTypeMCPServer) Version() string { return "1.0.0" }
func (s *contentTypeMCPServer) ListTools(ctx context.Context) ([]MCPTool, error) {
	return []MCPTool{{Name: "test", InputSchema: map[string]any{}}}, nil
}
func (s *contentTypeMCPServer) CallTool(ctx context.Context, name string, arguments map[string]any) (ToolResult, error) {
	return ToolResult{Content: s.content}, nil
}
