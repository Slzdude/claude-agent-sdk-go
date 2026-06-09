// Example: Using a custom OTel collector (e.g., Jaeger, Zipkin, Grafana Tempo).
//
// The SDK is backend-agnostic — it accepts any trace.TracerProvider.
// This example shows how to set up a generic OTLP exporter.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func main() {
	ctx := context.Background()

	// 1. Create your exporter (Jaeger, Tempo, etc.)
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("localhost:4318"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}

	// 2. Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("my-claude-app"),
			semconv.ServiceVersion("1.0.0"),
		)),
	)
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	// 3. Pass TracerProvider to the SDK — tracing is automatic
	msgs, err := claude.Query(ctx, "Say hello", &claude.ClaudeAgentOptions{
		PermissionMode: claude.PermissionModeBypassPermissions,
		TracerProvider: tp,
	})
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	for msg := range msgs {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Response:", text.Text)
				}
			}
		}
	}
}
