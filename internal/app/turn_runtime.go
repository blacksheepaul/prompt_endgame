package app

import (
	"context"
	"sync"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// TurnRuntime orchestrates turn execution and agent streaming
type TurnRuntime struct {
	llmProvider port.LLMProvider
	eventSink   port.EventSink
	roomRepo    port.RoomRepository
	sceneryRepo port.SceneryRepository

	mu      sync.RWMutex
	cancels map[domain.RoomID]context.CancelFunc
}

// NewTurnRuntime creates a new turn runtime
func NewTurnRuntime(
	llmProvider port.LLMProvider,
	eventSink port.EventSink,
	roomRepo port.RoomRepository,
	sceneryRepo port.SceneryRepository,
) *TurnRuntime {
	return &TurnRuntime{
		llmProvider: llmProvider,
		eventSink:   eventSink,
		roomRepo:    roomRepo,
		sceneryRepo: sceneryRepo,
		cancels:     make(map[domain.RoomID]context.CancelFunc),
	}
}

// ExecuteTurn runs the turn execution for all agents
func (r *TurnRuntime) ExecuteTurn(ctx context.Context, roomID domain.RoomID, turn *domain.Turn) {
	ctx, cancel := context.WithCancel(ctx)

	r.mu.Lock()
	r.cancels[roomID] = cancel
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.cancels, roomID)
		r.mu.Unlock()
		cancel()
	}()

	// Get room to retrieve scenery ID
	room, err := r.roomRepo.Get(ctx, roomID)
	if err != nil {
		r.emitError(ctx, roomID, turn.ID, "room_not_found", err.Error())
		return
	}

	// Get scenery for agents
	scenery, err := r.sceneryRepo.Get(ctx, room.SceneryID)
	if err != nil {
		r.emitError(ctx, room.ID, turn.ID, "scenery_not_found", err.Error())
		return
	}

	// Execute for each agent sequentially (can be parallelized later)
	for _, agent := range scenery.Agents {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.streamAgent(ctx, roomID, turn, agent)
	}

	// Mark turn as completed
	r.completeTurn(ctx, roomID, turn)
}

func (r *TurnRuntime) streamAgent(ctx context.Context, roomID domain.RoomID, turn *domain.Turn, agent port.Agent) {
	prompt := turn.UserInput // simplified; could include history

	tokenCh := r.llmProvider.StreamCompletion(ctx, agent.ID, prompt)

	var content string
	for token := range tokenCh {
		if token.Error != nil {
			r.emitError(ctx, roomID, turn.ID, "stream_error", token.Error.Error())
			return
		}

		if token.Done {
			break
		}

		content += token.Token

		// Emit token event
		event := domain.NewEvent(domain.EventTokenReceived, roomID, turn.ID, domain.TokenPayload{
			AgentID: agent.ID,
			Token:   token.Token,
		})
		r.eventSink.Append(ctx, event)
	}

	// Store response
	turn.Responses = append(turn.Responses, domain.Response{
		AgentID: agent.ID,
		Content: content,
	})
}

func (r *TurnRuntime) completeTurn(ctx context.Context, roomID domain.RoomID, turn *domain.Turn) {
	// Use Update for thread safety
	r.roomRepo.Update(ctx, roomID, func(r *domain.Room) error {
		r.State = domain.RoomStateDone
		r.UpdatedAt = time.Now()
		return nil
	})

	event := domain.NewEvent(domain.EventTurnCompleted, roomID, turn.ID, nil)
	r.eventSink.Append(ctx, event)
}

func (r *TurnRuntime) emitError(ctx context.Context, roomID domain.RoomID, turnID domain.TurnID, code, message string) {
	event := domain.NewEvent(domain.EventError, roomID, turnID, domain.ErrorPayload{
		Code:    code,
		Message: message,
	})
	r.eventSink.Append(ctx, event)
}

// Cancel cancels a running turn for the given room
func (r *TurnRuntime) Cancel(roomID domain.RoomID) {
	r.mu.RLock()
	cancel, ok := r.cancels[roomID]
	r.mu.RUnlock()

	if ok {
		cancel()
	}
}
