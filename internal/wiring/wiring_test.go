package wiring

import (
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/adapter/provider/mock"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/provider/openai"
	"github.com/blacksheepaul/prompt_endgame/internal/config"
	"go.uber.org/zap"
)

func TestWire_MockProvider(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr: ":8080",
		},
		Provider: config.ProviderConfig{
			Type:       "mock",
			TokenDelay: 100 * time.Millisecond,
			Mock: config.MockConfig{
				TokenDelay: 100 * time.Millisecond,
			},
		},
		Scenery: config.SceneryConfig{
			BasePath: "./sceneries",
		},
		Log: config.LogConfig{
			Level: 0, // Info level
		},
	}

	logger := zap.NewNop()
	container := Wire(cfg, logger)

	if container == nil {
		t.Fatal("Wire() returned nil")
	}

	// Check that LLMProvider is mock type
	_, ok := container.LLMProvider.(*mock.Provider)
	if !ok {
		t.Errorf("Expected LLMProvider to be *mock.Provider, got %T", container.LLMProvider)
	}
}

func TestWire_OpenAIProvider(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr: ":8080",
		},
		Provider: config.ProviderConfig{
			Type: "openai",
			OpenAI: config.OpenAIConfig{
				Endpoint: "http://localhost:10181/v1",
				Model:    "gpt-4",
				APIKey:   "test-key",
			},
		},
		Scenery: config.SceneryConfig{
			BasePath: "./sceneries",
		},
		Log: config.LogConfig{
			Level: 0,
		},
	}

	logger := zap.NewNop()
	container := Wire(cfg, logger)

	if container == nil {
		t.Fatal("Wire() returned nil")
	}

	// Check that LLMProvider is openai type
	_, ok := container.LLMProvider.(*openai.Provider)
	if !ok {
		t.Errorf("Expected LLMProvider to be *openai.Provider, got %T", container.LLMProvider)
	}
}

func TestWire_OpenAIProvider_InvalidConfig(t *testing.T) {
	// Test that wiring panics when OpenAI config is incomplete
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr: ":8080",
		},
		Provider: config.ProviderConfig{
			Type: "openai",
			OpenAI: config.OpenAIConfig{
				Endpoint: "", // Empty endpoint should cause panic
				Model:    "gpt-4",
			},
		},
		Scenery: config.SceneryConfig{
			BasePath: "./sceneries",
		},
		Log: config.LogConfig{
			Level: 0,
		},
	}

	logger := zap.NewNop()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Wire() to panic with empty OpenAI endpoint")
		}
	}()

	Wire(cfg, logger)
}

func TestWire_DefaultsToMock(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr: ":8080",
		},
		Provider: config.ProviderConfig{
			Type:       "", // Empty type
			TokenDelay: 50 * time.Millisecond,
		},
		Scenery: config.SceneryConfig{
			BasePath: "./sceneries",
		},
		Log: config.LogConfig{
			Level: 0,
		},
	}

	logger := zap.NewNop()
	container := Wire(cfg, logger)

	if container == nil {
		t.Fatal("Wire() returned nil")
	}

	// When type is empty, should default to mock
	_, ok := container.LLMProvider.(*mock.Provider)
	if !ok {
		t.Errorf("Expected LLMProvider to default to *mock.Provider when type is empty, got %T", container.LLMProvider)
	}
}

func TestWire_ContainerDependencies(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr: ":8080",
		},
		Provider: config.ProviderConfig{
			Type:       "mock",
			TokenDelay: 50 * time.Millisecond,
		},
		Scenery: config.SceneryConfig{
			BasePath: "./sceneries",
		},
		Log: config.LogConfig{
			Level: 0,
		},
	}

	logger := zap.NewNop()
	container := Wire(cfg, logger)

	// Check all dependencies are wired
	if container.Config == nil {
		t.Error("Config is nil")
	}
	if container.Logger == nil {
		t.Error("Logger is nil")
	}
	if container.RoomRepo == nil {
		t.Error("RoomRepo is nil")
	}
	if container.EventSink == nil {
		t.Error("EventSink is nil")
	}
	if container.LLMProvider == nil {
		t.Error("LLMProvider is nil")
	}
	if container.SceneryRepo == nil {
		t.Error("SceneryRepo is nil")
	}
	if container.RoomService == nil {
		t.Error("RoomService is nil")
	}
	if container.TurnRuntime == nil {
		t.Error("TurnRuntime is nil")
	}
	if container.HTTPServer == nil {
		t.Error("HTTPServer is nil")
	}
}
