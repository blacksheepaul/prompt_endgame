package wiring

import (
	"context"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/adapter/http"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/provider/mock"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/provider/openai"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/scenery/fs"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/store/inmem"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/store/sqlite"
	"github.com/blacksheepaul/prompt_endgame/internal/app"
	"github.com/blacksheepaul/prompt_endgame/internal/config"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
	"go.uber.org/zap"
)

// Container holds all wired dependencies
type Container struct {
	Config      *config.Config
	Logger      *zap.Logger
	RoomRepo    port.RoomRepository
	EventSink   port.EventSink
	LLMProvider port.LLMProvider
	SceneryRepo port.SceneryRepository
	RoomService *app.RoomService
	TurnRuntime *app.TurnRuntime
	HTTPServer  *http.Server
}

// Wire creates and wires all dependencies
func Wire(cfg *config.Config, logger *zap.Logger) *Container {
	// Create storage adapters based on configuration
	var roomRepo port.RoomRepository
	var eventSink port.EventSink

	switch cfg.Store.Type {
	case "sqlite":
		logger.Info("Using SQLite storage",
			zap.String("path", cfg.Store.SQLite.Path),
			zap.Bool("offload_enabled", cfg.Store.SQLite.OffloadEnabled),
		)

		// Open database
		db, err := sqlite.Open(cfg.Store.SQLite.Path)
		if err != nil {
			logger.Fatal("Failed to open SQLite database", zap.Error(err))
		}

		// Create repositories
		sqliteRepo := sqlite.NewRoomRepo(db, cfg.Store.SQLite)
		roomRepo = sqliteRepo
		eventSink = sqlite.NewEventSink(db)

		// Start background offloader if enabled
		if cfg.Store.SQLite.OffloadEnabled {
			checkInterval := cfg.Store.SQLite.IdleTimeout / 2
			if checkInterval < 30*time.Second {
				checkInterval = 30 * time.Second
			}
			sqliteRepo.StartOffloader(context.Background(), checkInterval)
			logger.Info("Started room offloader",
				zap.Duration("check_interval", checkInterval),
				zap.Duration("idle_timeout", cfg.Store.SQLite.IdleTimeout),
			)
		}

	case "memory", "":
		logger.Info("Using in-memory storage")
		roomRepo = inmem.NewRoomRepo()
		eventSink = inmem.NewEventSink()

	default:
		logger.Warn("Unknown store type, defaulting to memory", zap.String("type", cfg.Store.Type))
		roomRepo = inmem.NewRoomRepo()
		eventSink = inmem.NewEventSink()
	}

	// Create LLM provider based on configuration
	var llmProvider port.LLMProvider
	switch cfg.Provider.Type {
	case "openai":
		llmProvider = openai.NewProvider(openai.Config{
			Endpoint:   cfg.Provider.OpenAI.Endpoint,
			Model:      cfg.Provider.OpenAI.Model,
			APIKey:     cfg.Provider.OpenAI.APIKey,
			Timeout:    0, // Will use default
			MaxRetries: 0, // Will use default
		})
	case "mock", "":
		// Default to mock provider
		llmProvider = mock.NewProvider(cfg.Provider.Mock.TokenDelay)
	default:
		// Unknown provider type, default to mock
		llmProvider = mock.NewProvider(cfg.Provider.Mock.TokenDelay)
	}

	// Create scenery repo with default scenery
	sceneryRepo := fs.NewRepo(cfg.Scenery.BasePath, true)

	// Create turn runtime
	turnRuntime := app.NewTurnRuntime(
		llmProvider,
		eventSink,
		roomRepo,
		sceneryRepo,
		logger,
	)

	// Create room service
	roomService := app.NewRoomService(
		roomRepo,
		eventSink,
		sceneryRepo,
		turnRuntime,
		logger,
	)

	// Create HTTP server
	httpServer := http.NewServer(cfg.Server.Addr, roomService, eventSink, logger)

	return &Container{
		Config:      cfg,
		Logger:      logger,
		RoomRepo:    roomRepo,
		EventSink:   eventSink,
		LLMProvider: llmProvider,
		SceneryRepo: sceneryRepo,
		RoomService: roomService,
		TurnRuntime: turnRuntime,
		HTTPServer:  httpServer,
	}
}
