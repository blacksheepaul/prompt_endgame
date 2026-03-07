package config

import (
	"os"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

func TestLoad_DefaultValues(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("SERVER_ADDR")
	os.Unsetenv("PROVIDER_TYPE")
	os.Unsetenv("PROVIDER_ENDPOINT")
	os.Unsetenv("PROVIDER_MODEL")
	os.Unsetenv("PROVIDER_API_KEY")

	cfg := Load()

	if cfg.Server.Addr != ":8080" {
		t.Errorf("Expected default server addr :8080, got %s", cfg.Server.Addr)
	}

	if cfg.Provider.Type != "mock" {
		t.Errorf("Expected default provider type 'mock', got %s", cfg.Provider.Type)
	}

	if cfg.Log.Level != zapcore.InfoLevel {
		t.Errorf("Expected default log level Info, got %v", cfg.Log.Level)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	// Set environment variables
	os.Setenv("SERVER_ADDR", ":9090")
	os.Setenv("PROVIDER_TYPE", "openai")
	os.Setenv("PROVIDER_TOKEN_DELAY", "100ms")
	os.Setenv("LOG_LEVEL", "debug")

	// Ensure cleanup
	defer func() {
		os.Unsetenv("SERVER_ADDR")
		os.Unsetenv("PROVIDER_TYPE")
		os.Unsetenv("PROVIDER_TOKEN_DELAY")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg := Load()

	if cfg.Server.Addr != ":9090" {
		t.Errorf("Expected server addr :9090, got %s", cfg.Server.Addr)
	}

	if cfg.Provider.Type != "openai" {
		t.Errorf("Expected provider type 'openai', got %s", cfg.Provider.Type)
	}

	if cfg.Log.Level != zapcore.DebugLevel {
		t.Errorf("Expected log level Debug, got %v", cfg.Log.Level)
	}
}

func TestLoad_OpenAIConfig(t *testing.T) {
	// Set OpenAI specific environment variables
	os.Setenv("PROVIDER_TYPE", "openai")
	os.Setenv("PROVIDER_ENDPOINT", "http://localhost:10181/v1")
	os.Setenv("PROVIDER_MODEL", "gpt-4")
	os.Setenv("PROVIDER_API_KEY", "test-api-key")

	defer func() {
		os.Unsetenv("PROVIDER_TYPE")
		os.Unsetenv("PROVIDER_ENDPOINT")
		os.Unsetenv("PROVIDER_MODEL")
		os.Unsetenv("PROVIDER_API_KEY")
	}()

	cfg := Load()

	if cfg.Provider.Type != "openai" {
		t.Errorf("Expected provider type 'openai', got %s", cfg.Provider.Type)
	}

	if cfg.Provider.OpenAI.Endpoint != "http://localhost:10181/v1" {
		t.Errorf("Expected OpenAI endpoint 'http://localhost:10181/v1', got %s", cfg.Provider.OpenAI.Endpoint)
	}

	if cfg.Provider.OpenAI.Model != "gpt-4" {
		t.Errorf("Expected OpenAI model 'gpt-4', got %s", cfg.Provider.OpenAI.Model)
	}

	if cfg.Provider.OpenAI.APIKey != "test-api-key" {
		t.Errorf("Expected OpenAI API key 'test-api-key', got %s", cfg.Provider.OpenAI.APIKey)
	}
}

func TestLoad_MockConfig(t *testing.T) {
	// Set mock provider specific environment variables
	os.Setenv("PROVIDER_TYPE", "mock")
	os.Setenv("PROVIDER_TOKEN_DELAY", "200ms")

	defer func() {
		os.Unsetenv("PROVIDER_TYPE")
		os.Unsetenv("PROVIDER_TOKEN_DELAY")
	}()

	cfg := Load()

	if cfg.Provider.Type != "mock" {
		t.Errorf("Expected provider type 'mock', got %s", cfg.Provider.Type)
	}

	if cfg.Provider.Mock.TokenDelay != 200*time.Millisecond {
		t.Errorf("Expected mock token delay 200ms, got %v", cfg.Provider.Mock.TokenDelay)
	}
}

func TestLoad_DurationParsing(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected time.Duration
	}{
		{"milliseconds", "100ms", 100 * time.Millisecond},
		{"seconds", "5s", 5 * time.Second},
		{"minutes", "2m", 2 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("PROVIDER_TOKEN_DELAY", tt.envValue)
			defer os.Unsetenv("PROVIDER_TOKEN_DELAY")

			cfg := Load()
			if cfg.Provider.Mock.TokenDelay != tt.expected {
				t.Errorf("Expected mock token delay %v, got %v", tt.expected, cfg.Provider.Mock.TokenDelay)
			}
		})
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	os.Setenv("PROVIDER_TOKEN_DELAY", "invalid")
	defer os.Unsetenv("PROVIDER_TOKEN_DELAY")

	// Should use default value when parsing fails
	cfg := Load()
	if cfg.Provider.Mock.TokenDelay != 50*time.Millisecond {
		t.Errorf("Expected default mock token delay 50ms for invalid input, got %v", cfg.Provider.Mock.TokenDelay)
	}
}
