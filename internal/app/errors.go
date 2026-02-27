package app

import "errors"

var (
	ErrRoomBusy       = errors.New("room is busy")
	ErrNoActiveTurn   = errors.New("no active turn to cancel")
	ErrInvalidScenery = errors.New("invalid scenery")
)
