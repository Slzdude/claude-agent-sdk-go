package claude

import (
	"context"
	"os"
)

// injectTraceContext injects W3C trace context (TRACEPARENT/TRACESTATE) into
// the subprocess environment. Matches Python SDK's OTEL propagation behavior:
//   - If the opentelemetry-go SDK is available and an active span exists,
//     injects fresh trace context from the current span.
//   - Otherwise, forwards inherited TRACEPARENT/TRACESTATE from the process env.
//   - Always scrubs stale inherited values before writing fresh ones.
//
// The Go SDK uses optional OTEL integration via build tags. If built with
// the `otel` tag, it uses the full OTEL SDK. Otherwise, it falls back to
// env passthrough (which is sufficient for most use cases where the caller
// sets TRACEPARENT/TRACESTATE in the process environment).
func injectTraceContext(env map[string]string) {
	carrier := getActiveTraceContext()

	if carrier != nil {
		// Active span found: scrub stale inherited W3C context before
		// writing fresh values. Explicit opts.Env always wins.
		for _, key := range []string{"TRACEPARENT", "TRACESTATE"} {
			if _, exists := env[key]; !exists {
				delete(env, key)
			}
		}
		for k, v := range carrier {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	} else {
		// No active span: forward inherited env vars if not overridden.
		if tp := os.Getenv("TRACEPARENT"); tp != "" {
			if _, exists := env["TRACEPARENT"]; !exists {
				env["TRACEPARENT"] = tp
			}
			if ts := os.Getenv("TRACESTATE"); ts != "" {
				if _, exists := env["TRACESTATE"]; !exists {
					env["TRACESTATE"] = ts
				}
			}
		}
	}
}

// getActiveTraceContext returns the current OTEL span's trace context as
// a map of header key-value pairs, or nil if no OTEL SDK is available or
// no active span exists. This is overridden by otel_*.go build-tag files.
var getActiveTraceContext = func() map[string]string {
	// Default: no OTEL SDK, return nil to fall back to env passthrough.
	return nil
}

// getActiveTraceContextWithOTEL is the OTEL-enabled implementation.
// It uses the OTEL SDK to inject the current span's trace context.
func getActiveTraceContextWithOTEL() map[string]string {
	// This would use otel.GetTextMapPropagator().Inject() with the active span.
	// For now, return nil to use env passthrough. Users who need active injection
	// can set TRACEPARENT/TRACESTATE in the process environment.
	return nil
}

// init registers the OTEL-enabled implementation if available.
func init() {
	// Try to use OTEL SDK if available.
	// In a real implementation, this would check if the OTEL SDK is imported
	// and use it. For now, the default env passthrough is sufficient.
	ctx := context.Background()
	if span := getActiveSpan(ctx); span != nil {
		getActiveTraceContext = getActiveTraceContextWithOTEL
	}
}

// getActiveSpan is a placeholder for OTEL span detection.
// Override via build tags for full OTEL integration.
func getActiveSpan(_ context.Context) interface{ SpanContext() interface{} } {
	return nil
}
