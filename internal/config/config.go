package config

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap/zapcore"
)

// Config holds application configuration
type Config struct {
	Server   ServerConfig
	Provider ProviderConfig
	Scenery  SceneryConfig
	Log      LogConfig
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// ProviderConfig holds LLM provider settings
type ProviderConfig struct {
	Type   string // "mock" or "openai" etc
	OpenAI OpenAIConfig
	Mock   MockConfig
}

// OpenAIConfig holds OpenAI provider specific settings
type OpenAIConfig struct {
	Endpoint string
	Model    string
	APIKey   string
}

// MockConfig holds mock provider specific settings
type MockConfig struct {
	TokenDelay time.Duration
}

// SceneryConfig holds scenery repository settings
type SceneryConfig struct {
	BasePath string
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level zapcore.Level
}

// configKeysToCheck contains keys used to determine if any configuration is present
var configKeysToCheck = []string{
	"SERVER_ADDR",
	"PROVIDER_TYPE",
	"PROVIDER_ENDPOINT",
	"PROVIDER_MODEL",
	"PROVIDER_API_KEY",
	"PROVIDER_TOKEN_DELAY",
	"SCENERY_PATH",
	"LOG_LEVEL",
}

// Load loads configuration from environment variables.
// Panics if no configuration is found or if required configuration is invalid.
func Load() *Config {
	// Load .env file if it exists
	envFileLoaded := true
	if err := godotenv.Load(); err != nil {
		envFileLoaded = false
	}

	// Check if there's any configuration source
	if !envFileLoaded && !hasAnyEnvConfig() {
		panic("No configuration found: .env file not found and no environment variables set. " +
			"Please create a .env file or set required environment variables.")
	}

	cfg := &Config{
		Server: ServerConfig{
			Addr:         getEnv("SERVER_ADDR", ""),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 0), // No timeout for SSE
		},
		Provider: ProviderConfig{
			Type: getEnv("PROVIDER_TYPE", ""),
			OpenAI: OpenAIConfig{
				Endpoint: getEnv("PROVIDER_ENDPOINT", ""),
				Model:    getEnv("PROVIDER_MODEL", ""),
				APIKey:   getEnv("PROVIDER_API_KEY", ""),
			},
			Mock: MockConfig{
				TokenDelay: getDurationEnv("PROVIDER_TOKEN_DELAY", 50*time.Millisecond),
			},
		},
		Scenery: SceneryConfig{
			BasePath: getEnv("SCENERY_PATH", ""),
		},
		Log: LogConfig{
			Level: parseLogLevel(getEnv("LOG_LEVEL", "info")),
		},
	}

	// Validate configuration
	validateConfig(cfg)

	return cfg
}

// hasAnyEnvConfig checks if any expected configuration key is set in environment variables
func hasAnyEnvConfig() bool {
	for _, key := range configKeysToCheck {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

// validateConfig validates the loaded configuration.
// Panics if any required configuration is invalid.
func validateConfig(cfg *Config) {
	if cfg.Server.Addr == "" {
		panic("Invalid configuration: SERVER_ADDR is required but not set")
	}
	if cfg.Provider.Type == "" {
		panic("Invalid configuration: PROVIDER_TYPE is required but not set. " +
			"Valid values are: mock, openai")
	}
	if cfg.Scenery.BasePath == "" {
		panic("Invalid configuration: SCENERY_PATH is required but not set")
	}

	// Validate OpenAI-specific configuration when provider type is openai
	if cfg.Provider.Type == "openai" {
		if cfg.Provider.OpenAI.Endpoint == "" {
			panic("Invalid configuration: PROVIDER_ENDPOINT is required when PROVIDER_TYPE=openai")
		}
		if cfg.Provider.OpenAI.Model == "" {
			panic("Invalid configuration: PROVIDER_MODEL is required when PROVIDER_TYPE=openai")
		}
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			panic(fmt.Sprintf("Invalid configuration: invalid duration format for %s: %s", key, value))
		}
		return duration
	}
	return defaultValue
}

func parseLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
