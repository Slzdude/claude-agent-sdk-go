// Package langfuse provides Langfuse OTLP exporter configuration and setup.
package langfuse

import (
	"fmt"
	"os"
)

// Default values.
const (
	DefaultHost        = "https://cloud.langfuse.com"
	DefaultServiceName = "claude-agent-app"
	OTELPath           = "/api/public/otel"
)

// LangfuseConfig configures the Langfuse OTLP exporter.
type LangfuseConfig struct {
	// PublicKey is the Langfuse public key (required).
	PublicKey string
	// SecretKey is the Langfuse secret key (required).
	SecretKey string
	// Host is the Langfuse instance URL (default: https://cloud.langfuse.com).
	Host string
	// ServiceName for the OTel resource (default: claude-agent-app).
	ServiceName string
	// ServiceVersion for the OTel resource.
	ServiceVersion string
	// Headers are additional OTLP HTTP headers.
	Headers map[string]string
	// Insecure disables TLS (for local development).
	Insecure bool
}

// WithDefaults fills empty fields from environment variables and defaults.
func (c LangfuseConfig) WithDefaults() LangfuseConfig {
	if c.PublicKey == "" {
		c.PublicKey = os.Getenv("LANGFUSE_PUBLIC_KEY")
	}
	if c.SecretKey == "" {
		c.SecretKey = os.Getenv("LANGFUSE_SECRET_KEY")
	}
	if c.Host == "" {
		if h := os.Getenv("LANGFUSE_HOST"); h != "" {
			c.Host = h
		} else {
			c.Host = DefaultHost
		}
	}
	if c.ServiceName == "" {
		if s := os.Getenv("OTEL_SERVICE_NAME"); s != "" {
			c.ServiceName = s
		} else {
			c.ServiceName = DefaultServiceName
		}
	}
	return c
}

// Validate checks that required fields are present.
func (c LangfuseConfig) Validate() error {
	if c.PublicKey == "" {
		return fmt.Errorf("langfuse: PublicKey is required (set directly or via LANGFUSE_PUBLIC_KEY)")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("langfuse: SecretKey is required (set directly or via LANGFUSE_SECRET_KEY)")
	}
	return nil
}
