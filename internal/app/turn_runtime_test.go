package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/adapter/metrics"
	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
)

// Mock provider that returns errors
type mockErrorProvider struct {
	err error
}

func (m *mockErrorProvider) StreamCompletion(ctx context.Context, agentID string, prompt string) <-chan port.StreamToken {
	ch := make(chan port.StreamToken, 1)
	ch <- port.StreamToken{Error: m.err}
	close(ch)
	return ch
}

// mockRoomRepo implements port.RoomRepository
type mockRoomRepo struct {
	room domain.Room
	err  error
}

func (m *mockRoomRepo) Save(ctx context.Context, room *domain.Room) error {
	return nil
}

func (m *mockRoomRepo) Get(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	if m.err != nil {
		return domain.Room{}, m.err
	}
	return m.room, nil
}

func (m *mockRoomRepo) Update(ctx context.Context, id domain.RoomID, fn func(*domain.Room) error) error {
	return fn(&m.room)
}

func (m *mockRoomRepo) Delete(ctx context.Context, id domain.RoomID) error {
	return nil
}

func (m *mockRoomRepo) List(ctx context.Context) ([]domain.Room, error) {
	return nil, nil
}

// mockEventSink implements port.EventSink
type mockEventSink struct {
	events []domain.Event
	offset domain.Offset
}

func (m *mockEventSink) Append(ctx context.Context, event domain.Event) (domain.Offset, error) {
	m.events = append(m.events, event)
	m.offset++
	return m.offset, nil
}

func (m *mockEventSink) ReadFromOffsetAndSubscribe(ctx context.Context, roomID domain.RoomID, offset domain.Offset) ([]domain.Event, <-chan domain.Event, func(), error) {
	return m.events, make(<-chan domain.Event), func() {}, nil
}

func (m *mockEventSink) Subscribe(ctx context.Context, roomID domain.RoomID) (<-chan domain.Event, func()) {
	return make(<-chan domain.Event), func() {}
}

// mockSceneryRepo implements port.SceneryRepository
type mockSceneryRepo struct {
	scenery *port.Scenery
}

func (m *mockSceneryRepo) Get(ctx context.Context, id string) (*port.Scenery, error) {
	return m.scenery, nil
}

func (m *mockSceneryRepo) List(ctx context.Context) ([]port.Scenery, error) {
	return nil, nil
}

func TestTurnRuntime_ProviderErrors(t *testing.T) {
	// Create runtime with error provider
	errProvider := &mockErrorProvider{err: errors.New("connection refused")}
	eventSink := &mockEventSink{}
	roomRepo := &mockRoomRepo{
		room: domain.Room{
			ID:        "test-room",
			SceneryID: "test-scenery",
			State:     domain.RoomStateIdle,
		},
	}
	sceneryRepo := &mockSceneryRepo{
		scenery: &port.Scenery{
			ID:     "test-scenery",
			Agents: []port.Agent{{ID: "agent-1", Name: "Test Agent"}},
		},
	}
	logger := zap.NewNop()

	runtime := NewTurnRuntime(errProvider, eventSink, roomRepo, sceneryRepo, logger)

	turn := &domain.Turn{
		ID:        "test-turn",
		UserInput: "test input",
		State:     domain.TurnStateStreaming,
	}

	// Reset metrics before test
	// Note: In real test, we might need to reset prometheus registry

	// Execute turn
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runtime.ExecuteTurn(ctx, "test-room", turn)

	// Wait a bit for async processing
	time.Sleep(100 * time.Millisecond)

	// Check that error was emitted
	hasError := false
	for _, event := range eventSink.events {
		if event.Type == domain.EventError {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("Expected error event to be emitted")
	}
}

func TestTurnRuntime_ProviderErrors_Count(t *testing.T) {
	// TDD: this test must fail until turn_runtime.go calls
	// metrics.ProviderErrors.WithLabelValues(...).Inc() in streamAgent's error branch.
	//
	// Error "timeout" → classifyProviderError → label "timeout"
	errProvider := &mockErrorProvider{err: errors.New("timeout")}
	eventSink := &mockEventSink{}
	roomRepo := &mockRoomRepo{
		room: domain.Room{
			ID:        "test-room-2",
			SceneryID: "test-scenery",
			State:     domain.RoomStateIdle,
		},
	}
	sceneryRepo := &mockSceneryRepo{
		scenery: &port.Scenery{
			ID:     "test-scenery",
			Agents: []port.Agent{{ID: "agent-1", Name: "Test Agent"}},
		},
	}
	logger := zap.NewNop()

	runtime := NewTurnRuntime(errProvider, eventSink, roomRepo, sceneryRepo, logger)

	turn := &domain.Turn{
		ID:        "test-turn-2",
		UserInput: "test input",
		State:     domain.TurnStateStreaming,
	}

	// Use relative assertion to avoid global counter pollution across test runs.
	// "timeout" error is classified as label "timeout" by classifyProviderError.
	initialCount := testutil.ToFloat64(metrics.ProviderErrors.WithLabelValues("timeout"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runtime.ExecuteTurn(ctx, "test-room-2", turn)

	finalCount := testutil.ToFloat64(metrics.ProviderErrors.WithLabelValues("timeout"))
	if finalCount <= initialCount {
		t.Errorf("Expected ProviderErrors{type=timeout} to increment: before=%.0f, after=%.0f", initialCount, finalCount)
	}
}

func TestTurnRuntime_StreamAgent_ErrorHandling(t *testing.T) {
	tests := []struct {
		name      string
		provider  port.LLMProvider
		wantError bool
	}{
		{
			name:      "connection refused error",
			provider:  &mockErrorProvider{err: errors.New("connection refused")},
			wantError: true,
		},
		{
			name:      "timeout error",
			provider:  &mockErrorProvider{err: errors.New("timeout")},
			wantError: true,
		},
		{
			name:      "context cancelled error",
			provider:  &mockErrorProvider{err: context.Canceled},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventSink := &mockEventSink{}
			roomRepo := &mockRoomRepo{
				room: domain.Room{
					ID:        "test-room",
					SceneryID: "test-scenery",
					State:     domain.RoomStateIdle,
				},
			}
			sceneryRepo := &mockSceneryRepo{
				scenery: &port.Scenery{
					ID:     "test-scenery",
					Agents: []port.Agent{{ID: "agent-1", Name: "Test Agent"}},
				},
			}
			logger := zap.NewNop()

			runtime := NewTurnRuntime(tt.provider, eventSink, roomRepo, sceneryRepo, logger)

			turn := &domain.Turn{
				ID:        "test-turn",
				UserInput: "test input",
				State:     domain.TurnStateStreaming,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			runtime.ExecuteTurn(ctx, "test-room", turn)
			time.Sleep(50 * time.Millisecond)

			// Check for error event
			if tt.wantError {
				hasError := false
				for _, event := range eventSink.events {
					if event.Type == domain.EventError {
						hasError = true
						break
					}
				}
				if !hasError {
					t.Error("Expected error event to be emitted")
				}
			}
		})
	}
}
