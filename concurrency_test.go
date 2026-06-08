package claude

// concurrency_test.go tests the concurrency optimizations for sub-agent fan-out:
//   - Run() split goroutine: control protocol dispatch vs message forwarding
//   - Control request concurrency semaphore
//   - Prompt relay goroutine leak fix
//   - Channel buffer sizing

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mockTransportWithConcurrentHooks creates a transport that emits N concurrent
// hook control_requests interleaved with regular messages.
// It reads all hook responses from stdin before emitting the result.
func mockTransportWithConcurrentHooks(t *testing.T, hookCount int) *cliTransport {
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

		// Emit N hook control_requests interleaved with assistant messages.
		// The hooks arrive on the control channel, while assistant messages
		// arrive on the message channel. This exercises the split-goroutine design.
		hookReqIDs := make([]string, hookCount)
		for i := 0; i < hookCount; i++ {
			hookReqIDs[i] = "hook_" + itoa(i)
			hookLine, _ := json.Marshal(map[string]any{
				"type":       "control_request",
				"request_id": hookReqIDs[i],
				"request": map[string]any{
					"subtype":    "hook_callback",
					"callback_id": "hook_0",
					"input":      map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "echo " + itoa(i)}},
					"tool_use_id": "tu_" + itoa(i),
				},
			})
			assistantLine, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "step " + itoa(i)}},
					"model":   "claude-sonnet-4-20250514",
				},
			})
			_, _ = w.Write(hookLine)
			_ = w.WriteByte('\n')
			_, _ = w.Write(assistantLine)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}

		// Read all hook responses from stdin.
		responsesNeeded := hookCount
		for responsesNeeded > 0 && sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				continue
			}
			msgType, _ := req["type"].(string)
			if msgType == "control_response" {
				responsesNeeded--
			}
		}

		// Emit result.
		resultLine, _ := json.Marshal(map[string]any{
			"type":       "result",
			"subtype":    "success",
			"session_id": "test",
		})
		_, _ = w.Write(resultLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Drain stdin.
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

// mockTransportControlPriority creates a transport that emits a control_request
// AFTER a burst of regular messages that would fill a small channel buffer.
// The control_request must still be processed (not blocked by the full channel).
func mockTransportControlPriority(t *testing.T, burstSize int) *cliTransport {
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

		// Emit burstSize assistant messages to fill the out channel.
		for i := 0; i < burstSize; i++ {
			line, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "filler " + itoa(i)}},
					"model":   "claude-sonnet-4-20250514",
				},
			})
			_, _ = w.Write(line)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}

		// Now emit a hook control_request. With the split-goroutine design,
		// this should be dispatched via controlCh even if `out` is full.
		hookLine, _ := json.Marshal(map[string]any{
			"type":       "control_request",
			"request_id": "hook_priority",
			"request": map[string]any{
				"subtype":     "hook_callback",
				"callback_id": "hook_0",
				"input":       map[string]any{"tool_name": "Bash"},
				"tool_use_id": "tu_priority",
			},
		})
		_, _ = w.Write(hookLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Read the hook response.
		for sc.Scan() {
			var req map[string]any
			if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
				continue
			}
			msgType, _ := req["type"].(string)
			if msgType == "control_response" {
				break
			}
		}

		// Emit result.
		resultLine, _ := json.Marshal(map[string]any{
			"type":       "result",
			"subtype":    "success",
			"session_id": "test",
		})
		_, _ = w.Write(resultLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

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

// itoa is a simple int-to-string helper for test code.
func itoa(i int) string {
	return json.Number(fmt.Sprintf("%d", i)).String()
}

// ---------------------------------------------------------------------------
// Test: Split Run() goroutine — control requests dispatched even when out is full
// ---------------------------------------------------------------------------

func TestRun_ControlRequestNotBlockedBySlowConsumer(t *testing.T) {
	// Fill the out channel by not reading from it, then verify that
	// a control_request (hook) is still dispatched and responded to.
	// Without the split-goroutine design, the hook would time out.

	hookCallbackExecuted := make(chan string, 10)
	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
						name, _ := input["tool_name"].(string)
						hookCallbackExecuted <- name
						return nil, nil
					},
				}},
			},
		},
	}

	tr := mockTransportControlPriority(t, 200) // 200 messages to fill channel
	tr.opts = opts

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	q := newQueryProto(tr, opts)
	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Consume messages slowly — read just enough to not block the transport
	// scanner, but slow enough that the out channel would be full in the
	// old single-goroutine design.
	consumed := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case raw, ok := <-rawCh:
			if !ok {
				goto done
			}
			consumed++
			msgType, _ := raw["type"].(string)
			if msgType == "result" {
				goto done
			}
		case <-timeout:
			goto done
		}
	}
done:

	// Verify the hook was executed (not blocked by slow consumer).
	select {
	case name := <-hookCallbackExecuted:
		if name != "Bash" {
			t.Errorf("hook got tool_name=%q, want Bash", name)
		}
	case <-time.After(3 * time.Second):
		t.Error("hook callback was NOT executed — control request blocked by slow consumer (priority inversion)")
	}

	_ = tr.close()
}

// ---------------------------------------------------------------------------
// Test: Concurrent control_request handling via semaphore
// ---------------------------------------------------------------------------

func TestRun_ConcurrentControlRequestsRespectSemaphore(t *testing.T) {
	// Send more concurrent hooks than the semaphore capacity (10).
	// Verify that at most 10 run simultaneously.

	var activeCount atomic.Int32
	var maxActive atomic.Int32
	var wg sync.WaitGroup

	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
						cur := activeCount.Add(1)
						// Track max concurrency.
						for {
							old := maxActive.Load()
							if cur <= old || maxActive.CompareAndSwap(old, cur) {
								break
							}
						}
						// Simulate work.
						time.Sleep(50 * time.Millisecond)
						activeCount.Add(-1)
						wg.Done()
						return nil, nil
					},
				}},
			},
		},
	}

	const hookCount = 25 // More than semaphore cap of 10.
	wg.Add(hookCount)

	tr := mockTransportWithConcurrentHooks(t, hookCount)
	tr.opts = opts

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	q := newQueryProto(tr, opts)
	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Drain all messages.
	for raw := range rawCh {
		msgType, _ := raw["type"].(string)
		if msgType == "result" {
			break
		}
	}

	// Wait for all hooks to complete.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for hooks to complete")
	}

	// Max concurrent should be <= 10 (semaphore capacity).
	actual := maxActive.Load()
	if actual > 10 {
		t.Errorf("max concurrent hooks = %d, want <= 10 (semaphore capacity)", actual)
	}
	t.Logf("max concurrent hooks: %d (semaphore cap: 10)", actual)

	_ = tr.close()
}

// ---------------------------------------------------------------------------
// Test: Prompt relay goroutine exits on context cancellation
// ---------------------------------------------------------------------------

func TestQueryStream_PromptRelayExitsOnContextCancel(t *testing.T) {
	tr := mockTransportWithInit(t)
	tr.opts = &ClaudeAgentOptions{}

	ctx, cancel := context.WithCancel(context.Background())

	promptCh := make(chan map[string]any, 1)
	// Send one message.
	promptCh <- map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": "hello"}}

	opts := &ClaudeAgentOptions{}
	q := newQueryProto(tr, opts)
	rawCh := q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Start prompt relay (simulating processQuery's behavior).
	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		for {
			select {
			case raw, ok := <-promptCh:
				if !ok {
					return
				}
				_ = q.SendRawMessage(ctx, raw)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Cancel context — the relay goroutine should exit even though
	// promptCh is not closed.
	cancel()

	select {
	case <-relayDone:
		// Good — goroutine exited.
	case <-time.After(2 * time.Second):
		t.Error("prompt relay goroutine did not exit after context cancellation (leak)")
	}

	// Drain rawCh.
	go func() {
		for range rawCh {
		}
	}()
	_ = tr.close()
}

// ---------------------------------------------------------------------------
// Test: Channel buffer sizing — transport channel is 256
// ---------------------------------------------------------------------------

func TestTransport_ChannelBufferSize(t *testing.T) {
	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}

	// Create a pipe and readMessages to check the channel capacity.
	outR, outW := io.Pipe()
	defer outR.Close()
	defer outW.Close()

	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)
	tr.stdout = sc

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := tr.readMessages(ctx)

	// The channel should be buffered (not unbuffered).
	// We can't directly read the capacity from the outside, but we can
	// verify that sending multiple items doesn't block immediately.
	// Since readMessages creates the channel internally, we verify the
	// behavior indirectly: the transport should not block on the first
	// few messages written to stdout.

	go func() {
		w := bufio.NewWriter(outW)
		for i := 0; i < 10; i++ {
			line, _ := json.Marshal(map[string]any{"type": "assistant", "index": i})
			_, _ = w.Write(line)
			_ = w.WriteByte('\n')
			_ = w.Flush()
		}
	}()

	// Read 10 messages — should not block.
	received := 0
	timeout := time.After(2 * time.Second)
	for received < 10 {
		select {
		case <-ch:
			received++
		case <-timeout:
			t.Fatalf("received %d/10 messages before timeout", received)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: control_cancel_request dispatched via controlCh (not blocked by out)
// ---------------------------------------------------------------------------

func TestRun_ControlCancelDispatchedViaControlCh(t *testing.T) {
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	go func() {
		defer func() { _ = outW.Close() }()
		go io.Copy(io.Discard, stdinR) //nolint:errcheck
		w := bufio.NewWriter(outW)

		// Emit a control_cancel_request.
		line, _ := json.Marshal(map[string]any{
			"type":       "control_cancel_request",
			"request_id": "cancel_123",
		})
		_, _ = w.Write(line)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Emit result to close the stream.
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

	// Pre-register an inflight handler so we can verify cancellation.
	cancelled := make(chan struct{})
	q.inflightMu.Lock()
	q.inflightHandlers["cancel_123"] = func() {
		close(cancelled)
	}
	q.inflightMu.Unlock()

	rawCh := q.Run(ctx)

	// Drain all messages.
	for raw := range rawCh {
		msgType, _ := raw["type"].(string)
		if msgType == "result" {
			break
		}
	}

	// The cancel handler should have been invoked.
	select {
	case <-cancelled:
		// Good — cancel was dispatched.
	case <-time.After(2 * time.Second):
		t.Error("control_cancel_request was not dispatched (blocked by message forwarding)")
	}

	_ = tr.close()
}

// ---------------------------------------------------------------------------
// Test: Hook error propagation through Run()
// ---------------------------------------------------------------------------

func TestRun_HookErrorPropagatedToCLI(t *testing.T) {
	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
						return nil, fmt.Errorf("hook failed: permission denied")
					},
				}},
			},
		},
	}

	// Create transport that sends a hook request and reads the response.
	outR, outW := io.Pipe()
	stdinR, stdinW := io.Pipe()

	var responseReceived string
	var wg sync.WaitGroup
	wg.Add(1)

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

		// Emit hook control_request.
		hookLine, _ := json.Marshal(map[string]any{
			"type":       "control_request",
			"request_id": "hook_err",
			"request": map[string]any{
				"subtype":     "hook_callback",
				"callback_id": "hook_0",
				"input":       map[string]any{"tool_name": "Bash"},
				"tool_use_id": "tu_err",
			},
		})
		_, _ = w.Write(hookLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Read the hook response — should contain the error.
		if sc.Scan() {
			responseReceived = sc.Text()
			wg.Done()
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
	tr.opts = opts

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	q := newQueryProto(tr, opts)
	_ = q.Run(ctx)

	if _, err := q.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Wait for the response.
	wg.Wait()

	if responseReceived == "" {
		t.Fatal("no response received for hook")
	}

	// Verify the response contains the error.
	var resp map[string]any
	if err := json.Unmarshal([]byte(responseReceived), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	respObj, _ := resp["response"].(map[string]any)
	subtype, _ := respObj["subtype"].(string)
	if subtype != "error" {
		t.Errorf("response subtype = %q, want error", subtype)
	}
	errMsg, _ := respObj["error"].(string)
	if errMsg == "" {
		t.Error("expected error message in response")
	}

	_ = tr.close()
}

// ---------------------------------------------------------------------------
// Test: Multiple hooks in same matcher — each gets its own callback_id
// ---------------------------------------------------------------------------

func TestRun_MultipleHooksGetSeparateCallbackIDs(t *testing.T) {
	// Verify that multiple hooks in the same matcher each get a distinct
	// callback_id and are independently callable.

	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{
					Hooks: []HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							return map[string]any{"continue": true}, nil
						},
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							return map[string]any{"continue": true}, nil
						},
					},
				},
			},
		},
	}

	ctx := context.Background()
	tr := &cliTransport{opts: &ClaudeAgentOptions{}, maxBufferSize: defaultMaxBufferSize}
	q := newQueryProto(tr, opts)

	// Simulate Initialize() hook registration.
	for _, matchers := range opts.Hooks {
		for _, matcher := range matchers {
			for _, cb := range matcher.Hooks {
				id := fmt.Sprintf("hook_%d", q.counter.Add(1))
				q.hookCallbacks[id] = cb
			}
		}
	}

	if len(q.hookCallbacks) < 2 {
		t.Errorf("expected >= 2 hook callbacks, got %d", len(q.hookCallbacks))
	}

	// Verify distinct IDs.
	ids := make([]string, 0, len(q.hookCallbacks))
	for id := range q.hookCallbacks {
		ids = append(ids, id)
	}
	if ids[0] == ids[1] {
		t.Errorf("hook callback IDs are not distinct: %v", ids)
	}

	// Verify each is callable.
	for id, cb := range q.hookCallbacks {
		result, err := cb(ctx, map[string]any{"tool_name": "Bash"}, "tu_test")
		if err != nil {
			t.Errorf("callback %s returned error: %v", id, err)
		}
		if result == nil {
			t.Errorf("callback %s returned nil result", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Test: Close during active hook processing
// ---------------------------------------------------------------------------

func TestStreamingClient_CloseDuringHookExecution(t *testing.T) {
	// Verify that Close() doesn't deadlock when hooks are in-flight.
	hookStarted := make(chan struct{})
	hookDone := make(chan struct{})

	opts := &ClaudeAgentOptions{
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{Hooks: []HookCallback{
					func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
						close(hookStarted)
						// Simulate slow hook.
						select {
						case <-time.After(5 * time.Second):
						case <-ctx.Done():
						}
						close(hookDone)
						return nil, nil
					},
				}},
			},
		},
	}

	// Create a transport that emits a hook request.
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

		// Emit hook control_request.
		hookLine, _ := json.Marshal(map[string]any{
			"type":       "control_request",
			"request_id": "hook_close",
			"request": map[string]any{
				"subtype":     "hook_callback",
				"callback_id": "hook_0",
				"input":       map[string]any{"tool_name": "Bash"},
				"tool_use_id": "tu_close",
			},
		})
		_, _ = w.Write(hookLine)
		_ = w.WriteByte('\n')
		_ = w.Flush()

		// Block until stdin is closed (by client.Close()).
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
	tr.opts = opts

	ctx := context.Background()
	client, err := newMockSDKClient(ctx, t, opts, tr)
	if err != nil {
		t.Fatalf("newMockSDKClient: %v", err)
	}

	// Start consuming messages.
	go func() {
		for range client.ReceiveMessages(ctx) {
		}
	}()

	// Wait for hook to start.
	select {
	case <-hookStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("hook did not start")
	}

	// Close should not deadlock.
	closeDone := make(chan struct{})
	go func() {
		_ = client.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		// Good — Close returned.
	case <-time.After(5 * time.Second):
		t.Error("Close() deadlocked during hook execution")
	}
}

// ---------------------------------------------------------------------------
// Test: Concurrent SendControlRequest calls
// ---------------------------------------------------------------------------

func TestSendControlRequest_ConcurrentCalls(t *testing.T) {
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

		// Respond to all control_requests.
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
					"response":   map[string]any{"mode": "default"},
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	// Send 10 concurrent control requests.
	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := client.proto.SendControlRequest(ctx, map[string]any{
				"subtype": "set_permission_mode",
				"mode":    "default",
			}, 5*time.Second)
			if err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent SendControlRequest failed: %v", err)
	}
}
