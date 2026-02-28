package app

import (
	"context"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// RoomService handles room-related business logic
type RoomService struct {
	roomRepo    port.RoomRepository
	eventSink   port.EventSink
	sceneryRepo port.SceneryRepository
	turnRuntime *TurnRuntime
}

// NewRoomService creates a new room service
func NewRoomService(
	roomRepo port.RoomRepository,
	eventSink port.EventSink,
	sceneryRepo port.SceneryRepository,
	turnRuntime *TurnRuntime,
) *RoomService {
	return &RoomService{
		roomRepo:    roomRepo,
		eventSink:   eventSink,
		sceneryRepo: sceneryRepo,
		turnRuntime: turnRuntime,
	}
}

// CreateRoom creates a new conversation room
func (s *RoomService) CreateRoom(ctx context.Context, sceneryID string) (*domain.Room, error) {
	// Validate scenery exists
	if sceneryID == "" {
		sceneryID = "default"
	}
	_, err := s.sceneryRepo.Get(ctx, sceneryID)
	if err != nil {
		return nil, ErrInvalidScenery
	}

	room := domain.NewRoom(sceneryID)
	if err := s.roomRepo.Save(ctx, room); err != nil {
		return nil, err
	}

	// Emit room created event
	event := domain.NewEvent(domain.EventRoomCreated, room.ID, "", nil)
	s.eventSink.Append(ctx, event)

	return room, nil
}

// GetRoom retrieves a room by ID
func (s *RoomService) GetRoom(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	return s.roomRepo.Get(ctx, id)
}

// ListRooms returns a snapshot of all rooms
func (s *RoomService) ListRooms(ctx context.Context) ([]domain.Room, error) {
	return s.roomRepo.List(ctx)
}

// SubmitAnswer processes a user answer and triggers agent responses
func (s *RoomService) SubmitAnswer(ctx context.Context, roomID domain.RoomID, userInput string) (*domain.Turn, error) {
	var turn *domain.Turn

	// Thread-safe update within lock
	err := s.roomRepo.Update(ctx, roomID, func(r *domain.Room) error {
		if !r.CanStartTurn() {
			return ErrRoomBusy
		}

		// Calculate round number
		round := 1
		if r.CurrentTurn != nil {
			round = r.CurrentTurn.Round + 1
		}

		turn = domain.NewTurn(roomID, round, userInput)
		r.CurrentTurn = turn
		r.State = domain.RoomStateStreaming
		r.UpdatedAt = time.Now()
		return nil
	})

	if err != nil {
		switch err {
		case ErrRoomBusy:
			return nil, ErrRoomBusy
		case domain.ErrRoomNotFound:
			return nil, domain.ErrRoomNotFound
		default:
			return nil, err
		}
	}

	// Emit turn started event
	event := domain.NewEvent(domain.EventTurnStarted, roomID, turn.ID, map[string]string{
		"user_input": userInput,
	})
	s.eventSink.Append(ctx, event)

	// Start streaming in background - pass roomID, not room pointer
	go s.turnRuntime.ExecuteTurn(context.Background(), roomID, turn)

	return turn, nil
}

// CancelTurn cancels the current streaming turn
func (s *RoomService) CancelTurn(ctx context.Context, roomID domain.RoomID) error {
	var turnID domain.TurnID

	// Cancel via runtime first
	s.turnRuntime.Cancel(roomID)

	// Thread-safe update within lock
	err := s.roomRepo.Update(ctx, roomID, func(room *domain.Room) error {
		if !room.IsStreaming() {
			return ErrNoActiveTurn
		}

		room.State = domain.RoomStateCancelled
		room.UpdatedAt = time.Now()

		if room.CurrentTurn != nil {
			turnID = room.CurrentTurn.ID
		}
		return nil
	})

	if err != nil {
		switch err {
		case ErrNoActiveTurn:
			return err
		case domain.ErrRoomNotFound:
			return domain.ErrRoomNotFound
		default:
			return err
		}
	}

	// Emit cancelled event
	event := domain.NewEvent(domain.EventTurnCancelled, roomID, turnID, nil)
	s.eventSink.Append(ctx, event)

	return nil
}
