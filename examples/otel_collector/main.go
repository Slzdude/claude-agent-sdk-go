// Example: Using a custom OTel collector (e.g., Jaeger, Zipkin, Grafana Tempo).
//
// This example shows how to use the tracing package with any OTel-compatible
// backend instead of Langfuse.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func main() {
	ctx := context.Background()

	// Setup custom OTLP exporter (e.g., to Jaeger or Grafana Tempo)
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("localhost:4318"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}

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

	otel.SetTracerProvider(tp)

	// Use TracedQuery with the custom provider
	opts := &claude.ClaudeAgentOptions{
		PermissionMode: claude.PermissionModeBypassPermissions,
	}

	msgs, err := tracing.TracedQuery(ctx, "Say hello", opts,
		tracing.WithTracerProvider(tp),
	)
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
