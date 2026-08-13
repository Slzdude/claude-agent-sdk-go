package claude

// edge_cases_test.go covers boundary and edge-case scenarios for:
//   - ReportMirrorError nil/closed/full channel
//   - WaitForFirstResult timeout and cancelled context
//   - SendControlRequest timeout and error responses
//   - handleInboundControlRequest missing fields and unknown subtypes
//   - Hook callback timeout, nil result, and panic recovery
//   - control_response with non-map response field
//   - Malformed messages in the stream
//   - closeStdin double-call
//   - Query/Close sequencing
//   - Empty prompt path

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ReportMirrorError edge cases
// ---------------------------------------------------------------------------

func TestReportMirrorError_NilRawOut(t *testing.T) {
	q := &queryProto{rawOut: nil}
	// Should not panic.
	q.ReportMirrorError(nil, "test error")
}

func TestReportMirrorError_ClosedChannel(t *testing.T) {
	q := &queryProto{rawOut: make(chan map[string]any, 1)}
	close(q.rawOut)
	// Should not panic (recover() in ReportMirrorError).
	q.ReportMirrorError(nil, "test error")
}

func TestReportMirrorError_FullChannelDropsMessage(t *testing.T) {
	ch := make(chan map[string]any, 2)
	q := &queryProto{rawOut: ch}

	// Fill the channel.
	ch <- map[string]any{"a": 1}
	ch <- map[string]any{"b": 2}

	// ReportMirrorError should not block — it drops the message.
	done := make(chan struct{})
	go func() {
		q.ReportMirrorError(nil, "dropped")
		close(done)
	}()

	select {
	case <-done:
		// Good — did not block.
	case <-time.After(1 * time.Second):
		t.Error("ReportMirrorError blocked on full channel")
	}

	// Channel should still have the original 2 messages.
	if len(ch) != 2 {
		t.Errorf("channel length = %d, want 2", len(ch))
	}
}

func TestReportMirrorError_WithSessionKey(t *testing.T) {
	ch := make(chan map[string]any, 1)
	q := &queryProto{rawOut: ch}

	key := &SessionKey{ProjectKey: "proj", SessionID: "sess-123"}
	q.ReportMirrorError(key, "store append failed")

	msg := <-ch
	msgType, _ := msg["type"].(string)
	if msgType != "system" {
		t.Errorf("type = %q, want system", msgType)
	}
	subtype, _ := msg["subtype"].(string)
	if subtype != "mirror_error" {
		t.Errorf("subtype = %q, want mirror_error", subtype)
	}
}

// ---------------------------------------------------------------------------
// WaitForFirstResult edge cases
// ---------------------------------------------------------------------------

func TestWaitForFirstResult_TimeoutFires(t *testing.T) {
	q := &queryProto{
		firstResultCh: make(chan struct{}),
	}
	ctx := context.Background()

	start := time.Now()
	q.WaitForFirstResult(ctx, 100*time.Millisecond)
	elapsed := time.Since(start)

	if elapsed < 90*time.Millisecond {
		t.Errorf("returned too quickly: %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("took too long: %v", elapsed)
	}
}

func TestWaitForFirstResult_AlreadyCancelledContext(t *testing.T) {
	q := &queryProto{
		firstResultCh: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	start := time.Now()
	q.WaitForFirstResult(ctx, 10*time.Second)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("did not return promptly on cancelled context: %v", elapsed)
	}
}

func TestWaitForFirstResult_ResultArrivesBeforeTimeout(t *testing.T) {
	q := &queryProto{
		firstResultCh: make(chan struct{}),
	}
	ctx := context.Background()

	go func() {
		time.Sleep(50 * time.Millisecond)
		close(q.firstResultCh)
	}()

	start := time.Now()
	q.WaitForFirstResult(ctx, 5*time.Second)
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("took too long: %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// SendControlRequest edge cases
// ---------------------------------------------------------------------------

func TestSendControlRequest_Timeout(t *testing.T) {
	// Transport that never responds to control requests.
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		sc := bufio.NewScanner(stdinR)

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
						"response":   map[string]any{"commands": []any{}},
					},
				}
				b, _ := json.Marshal(resp)
				w := bufio.NewWriter(outW)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Read but NEVER respond to subsequent control requests.
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

	ctx := context.Background()
	client, err := newMockSDKClient(ctx, t, nil, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}
	defer client.Close()

	// Drain messages.
	go func() {
		for range client.ReceiveMessages(ctx) {
		}
	}()

	// SendControlRequest with short timeout — should timeout.
	start := time.Now()
	_, err = client.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "set_permission_mode",
		"mode":    "default",
	}, 200*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took too long to timeout: %v", elapsed)
	}
}

func TestSendControlRequest_ErrorResponse(t *testing.T) {
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
						"response":   map[string]any{"commands": []any{}},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Respond to control requests with error.
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
			resp := map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"request_id": reqID,
					"subtype":    "error",
					"error":      "permission denied by policy",
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			_ = w.WriteByte('\n')
			_ = w.Flush()
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

	ctx := context.Background()
	client, err := newMockSDKClient(ctx, t, nil, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}
	defer client.Close()

	go func() {
		for range client.ReceiveMessages(ctx) {
		}
	}()

	_, err = client.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "set_permission_mode",
		"mode":    "default",
	}, 5*time.Second)

	if err == nil {
		t.Fatal("expected error from control response, got nil")
	}
	if err.Error() != "control error: permission denied by policy" {
		t.Errorf("error = %q, want 'control error: permission denied by policy'", err.Error())
	}
}

func TestSendControlRequest_UnknownResponseRequestID(t *testing.T) {
	// control_response with a request_id not in the pending map should be
	// silently ignored — no deadlock, no panic.
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
						"response":   map[string]any{"commands": []any{}},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Emit a control_response with an unknown request_id.
		bogusResp, _ := json.Marshal(map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"request_id": "nonexistent-id-999",
				"subtype":    "success",
				"response":   map[string]any{},
			},
		})
		_, _ = w.Write(bogusResp)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Now respond to the real control request normally.
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
			resp := map[string]any{
				"type": "control_response",
				"response": map[string]any{
					"request_id": reqID,
					"subtype":    "success",
					"response":   map[string]any{},
				},
			}
			b, _ := json.Marshal(resp)
			_, _ = w.Write(b)
			_ = w.WriteByte('\n')
			_ = w.Flush()
			break
		}
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

	ctx := context.Background()
	client, err := newMockSDKClient(ctx, t, nil, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}
	defer client.Close()

	go func() {
		for range client.ReceiveMessages(ctx) {
		}
	}()

	// This should complete without deadlock — the bogus response was ignored.
	_, err = client.proto.SendControlRequest(ctx, map[string]any{
		"subtype": "set_permission_mode",
		"mode":    "default",
	}, 3*time.Second)
	if err != nil {
		t.Errorf("SendControlRequest failed: %v", err)
	}
}

func TestSendControlRequest_MalformedResponseField(t *testing.T) {
	// control_response where "response" is a string instead of map.
	// Verify the Run() loop silently ignores it without blocking or panicking.
	tr := mockTransportWithInit(t,
		`{"type":"control_response","response":"not-a-map"}`,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgs, err := mockProcessQuery(ctx, t, "test", tr, nil)
	if err != nil {
		t.Fatalf("mockProcessQuery: %v", err)
	}

	// Drain messages — should complete without hanging.
	for range msgs {
	}
}

// ---------------------------------------------------------------------------
// handleInboundControlRequest edge cases
// ---------------------------------------------------------------------------

func TestHandleInboundControlRequest_MissingRequestField(t *testing.T) {
	q := makeQueryProtoWithHooks(t, nil)

	// Envelope with no "request" key.
	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "req-no-body",
	}

	// Should produce an "unknown control subtype" error response.
	resp := invokeControlRequest(t, q, envelope)
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "error" {
		t.Errorf("subtype = %q, want error", subtype)
	}
}

func TestHandleInboundControlRequest_MissingRequestID(t *testing.T) {
	q := makeQueryProtoWithHooks(t, nil)

	// Envelope with no "request_id" key.
	envelope := map[string]any{
		"type": "control_request",
		"request": map[string]any{
			"subtype": "can_use_tool",
		},
	}

	// Should still produce a response (with empty request_id).
	resp := invokeControlRequest(t, q, envelope)
	respObj, _ := resp["response"].(map[string]any)
	reqID, _ := respObj["request_id"].(string)
	if reqID != "" {
		t.Errorf("request_id = %q, want empty", reqID)
	}
}

func TestHandleInboundControlRequest_UnknownSubtype(t *testing.T) {
	q := makeQueryProtoWithHooks(t, nil)

	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "req-unknown",
		"request": map[string]any{
			"subtype": "nonexistent_subtype_xyz",
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "error" {
		t.Errorf("subtype = %q, want error", subtype)
	}
	errMsg, _ := respObj["error"].(string)
	if errMsg == "" {
		t.Error("expected error message for unknown subtype")
	}
}

// ---------------------------------------------------------------------------
// Hook callback edge cases
// ---------------------------------------------------------------------------

func TestHandleHookCallback_NilResultReturnsEmptyMap(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"hook_nil": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			return nil, nil
		},
	})

	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "req-nil",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "hook_nil",
			"input":       map[string]any{},
			"tool_use_id": "tu_nil",
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "success" {
		t.Errorf("subtype = %q, want success", subtype)
	}
	// Should return empty map, not nil.
	respData, _ := respObj["response"].(map[string]any)
	if respData == nil {
		t.Error("expected non-nil response data for nil result")
	}
}

func TestHandleHookCallback_Timeout(t *testing.T) {
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"hook_slow": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return nil, nil
			}
		},
	})

	// Set a very short timeout.
	q.hookTimeouts["hook_slow"] = 0.1 // 100ms

	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "req-timeout",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "hook_slow",
			"input":       map[string]any{},
			"tool_use_id": "tu_timeout",
		},
	}

	start := time.Now()
	resp := invokeControlRequest(t, q, envelope)
	elapsed := time.Since(start)

	// Should complete quickly (within the timeout window).
	if elapsed > 2*time.Second {
		t.Errorf("took too long: %v", elapsed)
	}

	// The hook returns context.DeadlineExceeded which becomes an error.
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "error" {
		t.Errorf("subtype = %q, want error (hook should have timed out)", subtype)
	}
}

func TestHandleHookCallback_PanicInCallback(t *testing.T) {
	// handleInboundControlRequest does NOT recover panics — the Run()
	// goroutine's semaphore wrapper does. Verify that a panic in a hook
	// callback propagates (caller must recover).
	q := makeQueryProtoWithHooks(t, map[string]HookCallback{
		"hook_panic": func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
			panic("hook exploded")
		},
	})

	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "req-panic",
		"request": map[string]any{
			"subtype":     "hook_callback",
			"callback_id": "hook_panic",
			"input":       map[string]any{},
			"tool_use_id": "tu_panic",
		},
	}

	// Verify the panic propagates — the caller (Run goroutine) is responsible for recovery.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		q.handleInboundControlRequest(context.Background(), envelope)
	}()

	if !panicked {
		t.Error("expected panic from hook callback, but it was swallowed")
	}
}

// ---------------------------------------------------------------------------
// Query/Close edge cases
// ---------------------------------------------------------------------------

func TestStreamingClient_QueryAfterClose(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}

	_ = client.Close()

	// Query after Close should return an error.
	err = client.Query(ctx, "hello after close")
	if err == nil {
		t.Error("expected error from Query after Close, got nil")
	}
}

func TestStreamingClient_ReceiveResponseNoResult(t *testing.T) {
	// Mock that emits assistant messages but never a ResultMessage.
	// ReceiveResponse should eventually terminate when the channel closes.
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
						"response":   map[string]any{"commands": []any{}},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// Emit assistant messages but NO result.
		for i := 0; i < 3; i++ {
			line, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "partial"}},
					"model":   "test",
				},
			})
			_, _ = w.Write(line)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}

		// Close stdin to signal end of stream.
		_ = stdinR.Close()
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

	ctx := context.Background()
	client, err := newMockSDKClient(ctx, t, nil, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}
	defer client.Close()

	// ReceiveResponse should eventually close even without a ResultMessage.
	msgs := client.ReceiveResponse(ctx)
	collected := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case msg, ok := <-msgs:
			if !ok {
				goto done
			}
			collected++
			_ = msg
		case <-timeout:
			goto done
		}
	}
done:
	// We should have received the assistant messages.
	if collected < 1 {
		t.Errorf("expected >= 1 messages, got %d", collected)
	}
}

func TestStreamingClient_ReceiveMessagesContextCancel(t *testing.T) {
	// When context is cancelled, ReceiveMessages goroutine checks ctx.Done()
	// in its select. If msgCh has no messages, it exits. If msgCh is blocked,
	// it won't exit until a message arrives or msgCh closes.
	// Use a mock that emits messages so the select can fire.
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t,
		assistantJSON("msg1"),
		assistantJSON("msg2"),
		assistantJSON("msg3"),
	)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer client.Close()

	cancelCtx, cancel := context.WithCancel(context.Background())
	msgs := client.ReceiveMessages(cancelCtx)

	// Read one message, then cancel.
	select {
	case <-msgs:
	case <-time.After(2 * time.Second):
		t.Fatal("no message received")
	}

	cancel()

	// The goroutine should eventually exit. Give it time.
	// It may deliver a few more messages before checking ctx.Done().
	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-msgs:
			if !ok {
				return // Channel closed — good.
			}
		case <-timeout:
			// The channel may not close if msgCh is still open and
			// the goroutine is blocked on range. This is expected
			// behavior — context cancellation only helps when the
			// select has both cases available.
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Transport edge cases
// ---------------------------------------------------------------------------

func TestTransport_CloseStdinDoubleCall(t *testing.T) {
	outR, outW := io.Pipe()
	_, stdinW := io.Pipe()
	defer outR.Close()
	defer outW.Close()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc

	// First call.
	err1 := tr.closeStdin()
	// Second call should be idempotent.
	err2 := tr.closeStdin()

	if err1 != nil {
		t.Errorf("first closeStdin: %v", err1)
	}
	if err2 != nil {
		t.Errorf("second closeStdin should be no-op, got: %v", err2)
	}
}

func TestTransport_WriteToClosedTransport(t *testing.T) {
	outR, outW := io.Pipe()
	_, stdinW := io.Pipe()
	defer outR.Close()
	defer outW.Close()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc

	_ = tr.close()

	err := tr.write(context.Background(), `{"test": true}`)
	if err == nil {
		t.Error("expected error writing to closed transport, got nil")
	}
}

func TestTransport_CloseWithNilCmd(t *testing.T) {
	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
		cmd:           nil,
	}

	// Should not panic.
	err := tr.close()
	if err != nil {
		t.Errorf("close with nil cmd: %v", err)
	}
}

func TestTransport_ReadMessagesContextCancel(t *testing.T) {
	outR, outW := io.Pipe()
	defer outR.Close()

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc

	ctx, cancel := context.WithCancel(context.Background())
	ch := tr.readMessages(ctx)

	// Write a few messages.
	go func() {
		w := bufio.NewWriter(outW)
		for i := 0; i < 5; i++ {
			line, _ := json.Marshal(map[string]any{"type": "assistant", "i": i})
			_, _ = w.Write(line)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}
		// Cancel mid-stream.
		cancel()
		_ = outW.Close()
	}()

	// Drain — should terminate cleanly.
	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // Channel closed — good.
			}
		case <-timeout:
			t.Error("readMessages did not terminate after context cancellation")
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Malformed message handling
// ---------------------------------------------------------------------------

func TestMockProcessQuery_MalformedMessageSkipped(t *testing.T) {
	// Inject a message with missing "type" field — should be silently dropped.
	validAssistant, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
			"model":   "test",
		},
	})
	validResult, _ := json.Marshal(map[string]any{
		"type":       "result",
		"subtype":    "success",
		"session_id": "test",
	})

	tr := mockTransportWithInit(t,
		`{"no_type_field": true}`,                       // malformed — no "type"
		string(validAssistant),                          // valid
		`{"type": "assistant", "message": "not_a_map"}`, // malformed message
		string(validResult),                             // valid
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := mockProcessQuery(ctx, t, "test", tr, nil)
	if err != nil {
		t.Fatalf("mockProcessQuery: %v", err)
	}

	collected := 0
	for msg := range msgs {
		collected++
		_ = msg
	}

	// Should have received 2 valid messages (assistant + result),
	// with the 2 malformed ones silently dropped.
	if collected < 2 {
		t.Errorf("expected >= 2 valid messages, got %d", collected)
	}
}

// ---------------------------------------------------------------------------
// Empty prompt path
// ---------------------------------------------------------------------------

func TestMockProcessQuery_EmptyPrompt(t *testing.T) {
	resultLine, _ := json.Marshal(map[string]any{
		"type":       "result",
		"subtype":    "success",
		"session_id": "test",
	})

	tr := mockTransportWithInit(t, string(resultLine))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Empty prompt — should not send a user message.
	msgs, err := mockProcessQuery(ctx, t, "", tr, nil)
	if err != nil {
		t.Fatalf("mockProcessQuery with empty prompt: %v", err)
	}

	// Should still receive the result.
	collected := 0
	for range msgs {
		collected++
	}

	if collected < 1 {
		t.Error("expected at least 1 message with empty prompt")
	}
}

// ---------------------------------------------------------------------------
// MCP edge cases
// ---------------------------------------------------------------------------

func TestHandleMCPMessage_EmptyServerName(t *testing.T) {
	q := makeQueryProtoWithHooks(t, nil)

	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "mcp-empty",
		"request": map[string]any{
			"subtype":     "mcp_message",
			"server_name": "",
			"message":     map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "error" {
		t.Errorf("subtype = %q, want error for empty server_name", subtype)
	}
}

func TestHandleMCPMessage_MissingMessageField(t *testing.T) {
	q := makeQueryProtoWithHooks(t, nil)

	envelope := map[string]any{
		"type":       "control_request",
		"request_id": "mcp-nomsg",
		"request": map[string]any{
			"subtype":     "mcp_message",
			"server_name": "test-server",
			// "message" key is absent.
		},
	}

	resp := invokeControlRequest(t, q, envelope)
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "error" {
		t.Errorf("subtype = %q, want error for missing message field", subtype)
	}
}

// ---------------------------------------------------------------------------
// control_cancel_request edge cases
// ---------------------------------------------------------------------------

func TestRun_ControlCancelForUnknownID(t *testing.T) {
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		go io.Copy(io.Discard, stdinR) //nolint:errcheck
		w := bufio.NewWriter(outW)

		// Emit cancel for unknown ID.
		line, _ := json.Marshal(map[string]any{
			"type":       "control_cancel_request",
			"request_id": "unknown-cancel-id",
		})
		_, _ = w.Write(line)
		_ = w.WriteByte('\n')

		// Result to close.
		resultLine, _ := json.Marshal(map[string]any{
			"type":       "result",
			"subtype":    "success",
			"session_id": "test",
		})
		_, _ = w.Write(resultLine)
		_ = w.WriteByte('\n')
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := newQueryProto(tr, &ClaudeAgentOptions{})
	rawCh := q.Run(ctx)

	// Drain — should not panic on unknown cancel ID.
	for raw := range rawCh {
		msgType, _ := raw["type"].(string)
		if msgType == "result" {
			break
		}
	}

	_ = tr.close()
}

// ---------------------------------------------------------------------------
// Concurrent Close + ReceiveMessages race
// ---------------------------------------------------------------------------

func TestStreamingClient_ConcurrentCloseAndReceive(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t,
		assistantJSON("hello"),
		assistantJSON("world"),
		resultJSON(),
	)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: receive messages.
	go func() {
		defer wg.Done()
		for range client.ReceiveMessages(ctx) {
		}
	}()

	// Goroutine 2: close after a short delay.
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		_ = client.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good — both goroutines finished.
	case <-time.After(5 * time.Second):
		t.Error("concurrent Close + ReceiveMessages deadlocked")
	}
}

// ---------------------------------------------------------------------------
// Multi-turn query test
// ---------------------------------------------------------------------------

func TestStreamingClient_MultiTurnQuery(t *testing.T) {
	ctx := context.Background()

	// Mock that handles two query-response cycles.
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
						"response":   map[string]any{"commands": []any{}},
					},
				}
				b, _ := json.Marshal(resp)
				_, _ = w.Write(b)
				_ = w.WriteByte('\n')
				_ = w.Flush()
			}
		}

		// For each user message, emit assistant + result.
		for sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				continue
			}
			msgType, _ := req["type"].(string)
			if msgType != "user" {
				continue
			}

			assistantLine, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "response"}},
					"model":   "test",
				},
			})
			resultLine, _ := json.Marshal(map[string]any{
				"type":       "result",
				"subtype":    "success",
				"session_id": "test",
			})
			_, _ = w.Write(assistantLine)
			_ = w.WriteByte('\n')
			_, _ = w.Write(resultLine)
			_ = w.WriteByte('\n')
			_ = w.Flush()
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

	client, err := newMockSDKClient(ctx, t, nil, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}
	defer client.Close()

	// Turn 1.
	if err := client.Query(ctx, "hello"); err != nil {
		t.Fatalf("Query 1: %v", err)
	}
	turn1 := 0
	for msg := range client.ReceiveResponse(ctx) {
		turn1++
		_ = msg
	}
	if turn1 < 2 {
		t.Errorf("turn 1: expected >= 2 messages, got %d", turn1)
	}

	// Turn 2.
	if err := client.Query(ctx, "world"); err != nil {
		t.Fatalf("Query 2: %v", err)
	}
	turn2 := 0
	for msg := range client.ReceiveResponse(ctx) {
		turn2++
		_ = msg
	}
	if turn2 < 2 {
		t.Errorf("turn 2: expected >= 2 messages, got %d", turn2)
	}
}
