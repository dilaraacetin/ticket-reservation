package service

import (
	"context"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// WaitingListService lets a user take a place in an event's queue and give it
// up again.
type WaitingListService struct {
	waiting repository.WaitingListRepository
	events  repository.EventRepository
	clock   Clock
	newID   func() string
	gate    SeatGate
}

// WaitingListConfig carries the service's dependencies, for the same reason
// Config does: two of these are a func and an interface.
type WaitingListConfig struct {
	Waiting repository.WaitingListRepository
	Events  repository.EventRepository
	Clock   Clock
	NewID   func() string

	// Gate decides who may queue for a seat. It has to be the same rule that
	// decides who may hold one: somebody who cannot be given a seat would
	// otherwise reach the front of the queue, be refused, and be put back —
	// blocking everybody behind them for good.
	Gate SeatGate
}

func NewWaitingListService(cfg WaitingListConfig) *WaitingListService {
	if cfg.Gate == nil {
		cfg.Gate = AllowEveryone{}
	}

	return &WaitingListService{
		waiting: cfg.Waiting,
		events:  cfg.Events,
		clock:   cfg.Clock,
		newID:   cfg.NewID,
		gate:    cfg.Gate,
	}
}

// Join puts the user in the queue and reports where they stand. The event is
// looked up first, so queueing for an event that does not exist is a not found
// rather than a place in a queue nobody will ever call.
func (s *WaitingListService) Join(ctx context.Context, eventID, userID string) (int, error) {
	if err := s.gate.MayTakeSeat(ctx, userID); err != nil {
		return 0, err
	}

	if _, err := s.events.GetEvent(ctx, eventID); err != nil {
		return 0, err
	}

	entry, err := domain.NewWaitingEntry(s.newID(), eventID, userID, s.clock.Now())
	if err != nil {
		return 0, err
	}

	// One call, not a join followed by a read: a handoff running in between could
	// give the caller a seat and take them off the queue, turning a join that
	// worked into "you are not waiting".
	_, position, err := s.waiting.Join(ctx, entry)

	return position, err
}

// Leave gives up the user's place.
func (s *WaitingListService) Leave(ctx context.Context, eventID, userID string) error {
	return s.waiting.Leave(ctx, eventID, userID)
}

// Position reports where the user stands, counting from one.
func (s *WaitingListService) Position(ctx context.Context, eventID, userID string) (int, error) {
	return s.waiting.Position(ctx, eventID, userID)
}
