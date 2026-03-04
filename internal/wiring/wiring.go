package wiring

import (
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/http"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/provider/mock"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/scenery/fs"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/store/inmem"
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
	// Create adapters
	roomRepo := inmem.NewRoomRepo()
	eventSink := inmem.NewEventSink()

	// Create LLM provider (mock for now)
	llmProvider := mock.NewProvider(cfg.Provider.TokenDelay)

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
