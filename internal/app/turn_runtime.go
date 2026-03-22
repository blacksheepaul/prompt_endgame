package app

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/adapter/metrics"
	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
	"go.uber.org/zap"
)

// TurnRuntime orchestrates turn execution and agent streaming
type TurnRuntime struct {
	llmProvider port.LLMProvider
	eventSink   port.EventSink
	roomRepo    port.RoomRepository
	sceneryRepo port.SceneryRepository
	logger      *zap.Logger

	mu      sync.RWMutex
	cancels map[domain.RoomID]context.CancelFunc
}

// NewTurnRuntime creates a new turn runtime
func NewTurnRuntime(
	llmProvider port.LLMProvider,
	eventSink port.EventSink,
	roomRepo port.RoomRepository,
	sceneryRepo port.SceneryRepository,
	logger *zap.Logger,
) *TurnRuntime {
	return &TurnRuntime{
		llmProvider: llmProvider,
		eventSink:   eventSink,
		roomRepo:    roomRepo,
		sceneryRepo: sceneryRepo,
		logger:      logger,
		cancels:     make(map[domain.RoomID]context.CancelFunc),
	}
}

// ExecuteTurn runs the turn execution for all agents
func (r *TurnRuntime) ExecuteTurn(ctx context.Context, roomID domain.RoomID, turn *domain.Turn) {
	startTime := time.Now()
	r.logger.Info("Starting turn",
		zap.String("turn_id", string(turn.ID)),
		zap.String("room_id", string(roomID)),
	)
	metrics.ActiveTurns.Inc()
	metrics.Goroutines.Set(float64(runtime.NumGoroutine()))
	defer func() {
		metrics.ActiveTurns.Dec()
		r.logger.Info("Completed turn",
			zap.String("turn_id", string(turn.ID)),
			zap.String("room_id", string(roomID)),
			zap.Duration("duration", time.Since(startTime)),
		)
		metrics.Goroutines.Set(float64(runtime.NumGoroutine()))
	}()

	ctx, cancel := context.WithCancel(ctx)

	r.mu.Lock()
	r.cancels[roomID] = cancel
	r.mu.Unlock()

	turn.State = domain.TurnStateStreaming

	defer func() {
		r.mu.Lock()
		delete(r.cancels, roomID)
		r.mu.Unlock()
		cancel()

		// Determine turn final state based on context
		if ctx.Err() != nil && turn.State == domain.TurnStateStreaming {
			turn.State = domain.TurnStateCancelled
		}

		duration := time.Since(startTime).Seconds()
		metrics.TurnDuration.Observe(duration)
		metrics.TurnTotal.WithLabelValues(string(turn.State)).Inc()

		// Room always returns to Idle after turn ends
		r.roomRepo.Update(context.Background(), roomID, func(room *domain.Room) error {
			if room.CurrentTurn != nil {
				room.CurrentTurn.State = turn.State
			}
			room.State = domain.RoomStateIdle
			room.UpdatedAt = time.Now()
			return nil
		})
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

	r.logger.Info("Found agents in scenery",
		zap.Int("agent_count", len(scenery.Agents)),
	)

	// Execute for each agent sequentially (can be parallelized later)
	for _, agent := range scenery.Agents {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.logger.Info("Streaming agent",
			zap.String("agent_id", agent.ID),
			zap.String("room_id", string(roomID)),
		)
		r.streamAgent(ctx, roomID, turn, agent, startTime)
		r.logger.Info("Finished agent",
			zap.String("agent_id", agent.ID),
			zap.String("room_id", string(roomID)),
		)
	}

	// avoid sending completion event if context was cancelled
	select {
	case <-ctx.Done():
		return
	default:
	}

	// Mark turn as completed
	r.completeTurn(ctx, roomID, turn)
}

func (r *TurnRuntime) streamAgent(ctx context.Context, roomID domain.RoomID, turn *domain.Turn, agent port.Agent, turnStartTime time.Time) {
	prompt := turn.UserInput // simplified; could include history
	r.logger.Info("Starting stream for agent",
		zap.String("agent_id", agent.ID),
		zap.String("prompt", prompt),
	)

	var content string
	tokenCount := 0
	var firstTokenTime time.Time
	streamStartTime := time.Now()

	// Record metrics and store response when streaming completes
	defer func() {
		if tokenCount > 0 {
			streamDuration := time.Since(streamStartTime).Seconds()
			if streamDuration > 0 {
				metrics.TokensPerSecond.Observe(float64(tokenCount) / streamDuration)
			}
		}

		r.logger.Info("Completed agent",
			zap.String("agent_id", agent.ID),
			zap.Int("token_count", tokenCount),
			zap.Duration("duration", time.Since(streamStartTime)),
		)

		turn.Responses = append(turn.Responses, domain.Response{
			AgentID: agent.ID,
			Content: content,
		})
	}()

	for token := range r.llmProvider.StreamCompletion(ctx, agent.ID, prompt) {
		if ctx.Err() != nil {
			r.logger.Info("Context cancelled for agent",
				zap.String("agent_id", agent.ID),
				zap.Int("token_count", tokenCount),
			)
			return
		}

		if token.Error != nil {
			metrics.ProviderErrors.WithLabelValues(classifyProviderError(token.Error)).Inc()
			r.emitError(ctx, roomID, turn.ID, "stream_error", token.Error.Error())
			return
		}

		if token.Done {
			return
		}

		// Track first token for TTFT
		if tokenCount == 0 {
			firstTokenTime = time.Now()
			metrics.TimeToFirstToken.Observe(firstTokenTime.Sub(turnStartTime).Seconds())
		}

		content += token.Token
		tokenCount++
		metrics.TotalTokens.Inc()

		// Emit token event
		event := domain.NewEvent(domain.EventTokenReceived, roomID, turn.ID, domain.TokenPayload{
			AgentID: agent.ID,
			Token:   token.Token,
		})
		r.eventSink.Append(ctx, event)
	}
}

func (r *TurnRuntime) completeTurn(ctx context.Context, roomID domain.RoomID, turn *domain.Turn) {
	turn.State = domain.TurnStateDone

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

// classifyProviderError maps a provider error to a canonical label used in ProviderErrors metric.
// Labels: timeout, connection_refused, 429, parse_error, cancelled, stream_error
func classifyProviderError(err error) string {
	if err == nil {
		return "stream_error"
	}
	// context.Canceled / context.DeadlineExceeded
	if err == context.Canceled || err == context.DeadlineExceeded {
		return "cancelled"
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "timeout", "deadline exceeded", "context deadline"):
		return "timeout"
	case containsAny(msg, "connection refused", "no such host", "connection reset"):
		return "connection_refused"
	case containsAny(msg, "429", "too many requests"):
		return "429"
	case containsAny(msg, "parse", "unmarshal", "json"):
		return "parse_error"
	case containsAny(msg, "context canceled", "context cancelled"):
		return "cancelled"
	default:
		return "stream_error"
	}
}

func containsAny(s string, subs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
