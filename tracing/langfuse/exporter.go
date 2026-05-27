package langfuse

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// SetupLangfuse creates a TracerProvider configured to export to Langfuse via OTLP.
// Call Shutdown on the returned TracerProvider before exiting.
func SetupLangfuse(ctx context.Context, cfg LangfuseConfig) (*sdktrace.TracerProvider, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	host := strings.TrimRight(cfg.Host, "/")

	// Parse the host URL to extract scheme and host
	parsed, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("langfuse: invalid host URL %q: %w", host, err)
	}

	// otlptracehttp.WithEndpoint expects host:port (no scheme)
	endpoint := parsed.Host

	// WithURLPath sets the COMPLETE path (not a prefix).
	// Langfuse OTLP endpoint: /api/public/otel/v1/traces
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithURLPath(OTELPath + "/v1/traces"),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization": "Basic " + basicAuth(cfg.PublicKey, cfg.SecretKey),
		}),
	}

	// Use HTTP if Insecure or scheme is http
	if cfg.Insecure || parsed.Scheme == "http" {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	// Merge additional headers
	for k, v := range cfg.Headers {
		opts = append(opts, otlptracehttp.WithHeaders(map[string]string{k: v}))
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("langfuse: failed to create OTLP exporter: %w", err)
	}

	// Build resource
	resOpts := []resource.Option{
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	}
	if cfg.ServiceVersion != "" {
		resOpts = append(resOpts, resource.WithAttributes(semconv.ServiceVersion(cfg.ServiceVersion)))
	}

	res, err := resource.New(ctx, resOpts...)
	if err != nil {
		return nil, fmt.Errorf("langfuse: failed to create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	return tp, nil
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
