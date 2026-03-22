package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

// setupRequiredEnv sets the minimum required environment variables for testing
func setupRequiredEnv(t *testing.T) {
	t.Helper()
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "mock")
	os.Setenv("SCENERY_PATH", "./sceneries")
}

// clearAllEnv clears all configuration environment variables
func clearAllEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"SERVER_ADDR", "PROVIDER_TYPE", "PROVIDER_ENDPOINT",
		"PROVIDER_MODEL", "PROVIDER_API_KEY", "PROVIDER_TOKEN_DELAY",
		"SCENERY_PATH", "LOG_LEVEL", "SERVER_READ_TIMEOUT", "SERVER_WRITE_TIMEOUT",
	}
	for _, key := range keys {
		os.Unsetenv(key)
	}
}

func TestLoad_NoConfigPanic(t *testing.T) {
	// Clear all environment variables
	clearAllEnv(t)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when no configuration is found")
		} else if !strings.Contains(r.(string), "No configuration found") {
			t.Errorf("Expected panic message about no configuration, got: %v", r)
		}
	}()

	Load()
}

func TestLoad_InvalidProviderTypePanic(t *testing.T) {
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "") // Empty provider type
	os.Setenv("SCENERY_PATH", "./sceneries")

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when PROVIDER_TYPE is empty")
		} else if !strings.Contains(r.(string), "PROVIDER_TYPE is required") {
			t.Errorf("Expected panic message about PROVIDER_TYPE, got: %v", r)
		}
	}()

	Load()
}

func TestLoad_InvalidServerAddrPanic(t *testing.T) {
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", "") // Empty server addr
	os.Setenv("PROVIDER_TYPE", "mock")
	os.Setenv("SCENERY_PATH", "./sceneries")

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when SERVER_ADDR is empty")
		} else if !strings.Contains(r.(string), "SERVER_ADDR is required") {
			t.Errorf("Expected panic message about SERVER_ADDR, got: %v", r)
		}
	}()

	Load()
}

func TestLoad_InvalidSceneryPathPanic(t *testing.T) {
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "mock")
	os.Setenv("SCENERY_PATH", "") // Empty scenery path

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when SCENERY_PATH is empty")
		} else if !strings.Contains(r.(string), "SCENERY_PATH is required") {
			t.Errorf("Expected panic message about SCENERY_PATH, got: %v", r)
		}
	}()

	Load()
}

func TestLoad_DefaultValues(t *testing.T) {
	// Clear environment variables but set required ones
	clearAllEnv(t)
	setupRequiredEnv(t)
	defer clearAllEnv(t)

	cfg := Load()

	if cfg.Server.Addr != ":8080" {
		t.Errorf("Expected server addr :8080, got %s", cfg.Server.Addr)
	}

	if cfg.Provider.Type != "mock" {
		t.Errorf("Expected provider type 'mock', got %s", cfg.Provider.Type)
	}

	if cfg.Scenery.BasePath != "./sceneries" {
		t.Errorf("Expected scenery path './sceneries', got %s", cfg.Scenery.BasePath)
	}

	if cfg.Log.Level != zapcore.InfoLevel {
		t.Errorf("Expected default log level Info, got %v", cfg.Log.Level)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	// Set environment variables
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":9090")
	os.Setenv("PROVIDER_TYPE", "openai")
	os.Setenv("PROVIDER_ENDPOINT", "http://localhost:10181/v1")
	os.Setenv("PROVIDER_MODEL", "gpt-4")
	os.Setenv("PROVIDER_TOKEN_DELAY", "100ms")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("SCENERY_PATH", "./test-sceneries")

	// Ensure cleanup
	defer clearAllEnv(t)

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
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "openai")
	os.Setenv("PROVIDER_ENDPOINT", "http://localhost:10181/v1")
	os.Setenv("PROVIDER_MODEL", "gpt-4")
	os.Setenv("PROVIDER_API_KEY", "test-api-key")
	os.Setenv("SCENERY_PATH", "./sceneries")

	defer clearAllEnv(t)

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
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "mock")
	os.Setenv("PROVIDER_TOKEN_DELAY", "200ms")
	os.Setenv("SCENERY_PATH", "./sceneries")

	defer clearAllEnv(t)

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
			clearAllEnv(t)
			os.Setenv("SERVER_ADDR", ":8080")
			os.Setenv("PROVIDER_TYPE", "mock")
			os.Setenv("SCENERY_PATH", "./sceneries")
			os.Setenv("PROVIDER_TOKEN_DELAY", tt.envValue)
			defer clearAllEnv(t)

			cfg := Load()
			if cfg.Provider.Mock.TokenDelay != tt.expected {
				t.Errorf("Expected mock token delay %v, got %v", tt.expected, cfg.Provider.Mock.TokenDelay)
			}
		})
	}
}

func TestLoad_InvalidDurationPanic(t *testing.T) {
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "mock")
	os.Setenv("SCENERY_PATH", "./sceneries")
	os.Setenv("PROVIDER_TOKEN_DELAY", "invalid")
	defer clearAllEnv(t)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when PROVIDER_TOKEN_DELAY is invalid")
		} else if !strings.Contains(r.(string), "invalid duration format") {
			t.Errorf("Expected panic message about invalid duration format, got: %v", r)
		}
	}()

	Load()
}

func TestLoad_OpenAIWithoutEndpointPanic(t *testing.T) {
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "openai")
	os.Setenv("PROVIDER_MODEL", "gpt-4")
	os.Setenv("SCENERY_PATH", "./sceneries")
	// PROVIDER_ENDPOINT is not set

	defer clearAllEnv(t)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when PROVIDER_ENDPOINT is not set for openai provider")
		} else if !strings.Contains(r.(string), "PROVIDER_ENDPOINT is required") {
			t.Errorf("Expected panic message about PROVIDER_ENDPOINT, got: %v", r)
		}
	}()

	Load()
}

func TestLoad_OpenAIWithoutModelPanic(t *testing.T) {
	clearAllEnv(t)
	os.Setenv("SERVER_ADDR", ":8080")
	os.Setenv("PROVIDER_TYPE", "openai")
	os.Setenv("PROVIDER_ENDPOINT", "http://localhost:10181/v1")
	os.Setenv("SCENERY_PATH", "./sceneries")
	// PROVIDER_MODEL is not set

	defer clearAllEnv(t)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic when PROVIDER_MODEL is not set for openai provider")
		} else if !strings.Contains(r.(string), "PROVIDER_MODEL is required") {
			t.Errorf("Expected panic message about PROVIDER_MODEL, got: %v", r)
		}
	}()

	Load()
}
