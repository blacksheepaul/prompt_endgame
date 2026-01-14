package config

import (
	"os"
	"time"
)

// Config holds application configuration
type Config struct {
	Server   ServerConfig
	Provider ProviderConfig
	Scenery  SceneryConfig
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// ProviderConfig holds LLM provider settings
type ProviderConfig struct {
	Type       string // "mock" or "openai" etc
	TokenDelay time.Duration
}

// SceneryConfig holds scenery repository settings
type SceneryConfig struct {
	BasePath string
}

// Load loads configuration from environment variables with defaults
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Addr:         getEnv("SERVER_ADDR", ":8080"),
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 0, // No timeout for SSE
		},
		Provider: ProviderConfig{
			Type:       getEnv("PROVIDER_TYPE", "mock"),
			TokenDelay: 50 * time.Millisecond,
		},
		Scenery: SceneryConfig{
			BasePath: getEnv("SCENERY_PATH", "./sceneries"),
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
