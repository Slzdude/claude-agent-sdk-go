// Example: Langfuse tracing for Claude Agent SDK Go.
//
// The SDK is backend-agnostic — it accepts any trace.TracerProvider.
// This example shows how to set up a Langfuse OTLP exporter and pass it
// to the SDK. The same pattern works for any OTel-compatible backend.
//
// Prerequisites:
//
//	export LANGFUSE_PUBLIC_KEY="pk-lf-..."
//	export LANGFUSE_SECRET_KEY="sk-lf-..."
//	export LANGFUSE_HOST="https://cloud.langfuse.com"  # or self-hosted
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// setupLangfuse creates a TracerProvider configured for Langfuse OTLP.
func setupLangfuse(ctx context.Context) (*sdktrace.TracerProvider, error) {
	pk := os.Getenv("LANGFUSE_PUBLIC_KEY")
	sk := os.Getenv("LANGFUSE_SECRET_KEY")
	host := os.Getenv("LANGFUSE_HOST")
	if host == "" {
		host = "https://cloud.langfuse.com"
	}
	sn := os.Getenv("OTEL_SERVICE_NAME")
	if sn == "" {
		sn = "my-claude-app"
	}

	parsed, err := url.Parse(strings.TrimRight(host, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid LANGFUSE_HOST: %w", err)
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(parsed.Host),
		otlptracehttp.WithURLPath("/api/public/otel/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(pk+":"+sk)),
		}),
	)
	if err != nil {
		return nil, err
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(sn)))
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	), nil
}

func main() {
	ctx := context.Background()

	tp, err := setupLangfuse(ctx)
	if err != nil {
		log.Fatalf("Failed to setup Langfuse: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	// Inject metadata into context — appears on every span in Langfuse
	ctx = tracing.WithSession(ctx, "my-session-id")
	ctx = tracing.WithUser(ctx, "user-123")
	ctx = tracing.WithMetadata(ctx, `{"env":"production","version":"1.0"}`)
	ctx = tracing.WithTags(ctx, "production", "v1")

	// Option A: Use TracedQuery (decorator pattern)
	msgs, err := tracing.TracedQuery(ctx,
		"What is 2+2? Reply with just the number.",
		&claude.ClaudeAgentOptions{
			PermissionMode: claude.PermissionModeBypassPermissions,
		},
		tracing.WithTracerProvider(tp),
	)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	// Option B (equivalent): Set TracerProvider directly on options
	// msgs, err := claude.Query(ctx, "...", &claude.ClaudeAgentOptions{
	//     TracerProvider: tp,
	// })

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Response:", text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("Done! Session: %s, Cost: $%.4f\n", m.SessionID, *m.TotalCostUSD)
			}
		}
	}
}
