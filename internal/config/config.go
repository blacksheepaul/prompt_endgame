package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
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
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found or error loading it, using system environment variables")
	} else {
		log.Println("Loaded configuration from .env file")
	}

	return &Config{
		Server: ServerConfig{
			Addr:         getEnv("SERVER_ADDR", ":8080"),
			ReadTimeout:  getDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout: getDurationEnv("SERVER_WRITE_TIMEOUT", 0), // No timeout for SSE
		},
		Provider: ProviderConfig{
			Type:       getEnv("PROVIDER_TYPE", "mock"),
			TokenDelay: getDurationEnv("PROVIDER_TOKEN_DELAY", 50*time.Millisecond),
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

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil {
			log.Printf("Invalid duration format for %s: %s, using default", key, value)
			return defaultValue
		}
		return duration
	}
	return defaultValue
}
