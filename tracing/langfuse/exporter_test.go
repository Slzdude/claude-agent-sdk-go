package langfuse

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestLangfuseConfig_WithDefaults(t *testing.T) {
	t.Run("uses env vars when config is empty", func(t *testing.T) {
		os.Setenv("LANGFUSE_PUBLIC_KEY", "pk-env")
		os.Setenv("LANGFUSE_SECRET_KEY", "sk-env")
		os.Setenv("LANGFUSE_HOST", "https://custom.langfuse.com")
		os.Setenv("OTEL_SERVICE_NAME", "my-service")
		defer os.Unsetenv("LANGFUSE_PUBLIC_KEY")
		defer os.Unsetenv("LANGFUSE_SECRET_KEY")
		defer os.Unsetenv("LANGFUSE_HOST")
		defer os.Unsetenv("OTEL_SERVICE_NAME")

		cfg := LangfuseConfig{}.WithDefaults()
		if cfg.PublicKey != "pk-env" {
			t.Errorf("PublicKey = %q, want %q", cfg.PublicKey, "pk-env")
		}
		if cfg.SecretKey != "sk-env" {
			t.Errorf("SecretKey = %q, want %q", cfg.SecretKey, "sk-env")
		}
		if cfg.Host != "https://custom.langfuse.com" {
			t.Errorf("Host = %q, want %q", cfg.Host, "https://custom.langfuse.com")
		}
		if cfg.ServiceName != "my-service" {
			t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "my-service")
		}
	})

	t.Run("explicit config takes precedence", func(t *testing.T) {
		os.Setenv("LANGFUSE_PUBLIC_KEY", "pk-env")
		defer os.Unsetenv("LANGFUSE_PUBLIC_KEY")

		cfg := LangfuseConfig{PublicKey: "pk-explicit"}.WithDefaults()
		if cfg.PublicKey != "pk-explicit" {
			t.Errorf("PublicKey = %q, want %q", cfg.PublicKey, "pk-explicit")
		}
	})

	t.Run("default host when no env var", func(t *testing.T) {
		os.Unsetenv("LANGFUSE_HOST")
		cfg := LangfuseConfig{}.WithDefaults()
		if cfg.Host != DefaultHost {
			t.Errorf("Host = %q, want %q", cfg.Host, DefaultHost)
		}
	})
}

func TestLangfuseConfig_Validate(t *testing.T) {
	t.Run("missing public key", func(t *testing.T) {
		cfg := LangfuseConfig{SecretKey: "sk"}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for missing PublicKey")
		}
	})

	t.Run("missing secret key", func(t *testing.T) {
		cfg := LangfuseConfig{PublicKey: "pk"}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for missing SecretKey")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := LangfuseConfig{PublicKey: "pk", SecretKey: "sk"}
		err := cfg.Validate()
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBasicAuth(t *testing.T) {
	auth := basicAuth("pk-lf-123", "sk-lf-456")
	decoded, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		t.Fatalf("failed to decode basic auth: %v", err)
	}
	if string(decoded) != "pk-lf-123:sk-lf-456" {
		t.Errorf("basic auth = %q, want %q", string(decoded), "pk-lf-123:sk-lf-456")
	}
}
