package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/event"
	"ticket-reservation/internal/repository"
)

const (
	// DefaultHandoffWorkers is how many freed seats are offered at once. A
	// worker spends most of its time waiting on the database, so this is set by
	// how fast seats come free rather than by how many cores there are.
	DefaultHandoffWorkers = 16

	// DefaultHandoffBuffer is how many freed seats may wait to be offered. It
	// exists so that releasing a seat never waits on the waiting list.
	//
	// Both numbers were guesses until a load test measured them: against
	// Postgres under contention, four workers and a buffer of 64 threw away a
	// third of the seats that came free.
	DefaultHandoffBuffer = 256

	// rejoinTimeout bounds putting a waiter back. Short, because it runs during
	// shutdown as well as during ordinary work.
	rejoinTimeout = 5 * time.Second
)

// SeatOfferer is told that a seat has just come free. Declared here for the
// consumer, so the sweeper and the reservation service can announce a freed seat
// without learning what happens to it.
type SeatOfferer interface {
	Offer(seat domain.SeatRef)
}

// seatHolder is the slice of the reservation service the handoff needs. Taking
// the whole service would let the handoff do things it has no business doing.
type seatHolder interface {
	HoldSeat(ctx context.Context, eventID, seatID, userID string) (*domain.Hold, error)
}

// Handoff gives a freed seat to whoever has waited longest for it. The hold it
// makes is an ordinary one, so a waiter's turn expires the same way any other
// hold does and the seat comes back round.
type Handoff struct {
	waiting   repository.WaitingListRepository
	holder    seatHolder
	publisher event.Publisher
	clock     Clock
	logger    *slog.Logger
	workers   int
	freed     chan domain.SeatRef
	dropped   atomic.Int64
}

// HandoffConfig carries the handoff's dependencies.
type HandoffConfig struct {
	Waiting   repository.WaitingListRepository
	Holder    seatHolder
	Publisher event.Publisher
	Clock     Clock
	Logger    *slog.Logger
	Workers   int
	Buffer    int
}

func NewHandoff(cfg HandoffConfig) *Handoff {
	if cfg.Publisher == nil {
		cfg.Publisher = event.Discard{}
	}

	if cfg.Workers < 1 {
		cfg.Workers = DefaultHandoffWorkers
	}

	if cfg.Buffer < 1 {
		cfg.Buffer = DefaultHandoffBuffer
	}

	return &Handoff{
		waiting:   cfg.Waiting,
		holder:    cfg.Holder,
		publisher: cfg.Publisher,
		clock:     cfg.Clock,
		logger:    cfg.Logger,
		workers:   cfg.Workers,
		freed:     make(chan domain.SeatRef, cfg.Buffer),
	}
}

// Offer hands a freed seat to the workers without waiting for them.
//
// A full buffer drops the seat rather than holding up whoever released it. The
// seat stays available to everyone either way, so a dropped offer costs the
// queue its turn at that seat, not the seat itself.
func (h *Handoff) Offer(seat domain.SeatRef) {
	select {
	case h.freed <- seat:
	default:
		h.dropped.Add(1)
		h.logger.Warn("the handoff queue is full, this seat was not offered",
			"eventId", seat.EventID,
			"seatId", seat.SeatID,
		)
	}
}

// Waiting reports how many freed seats are queued up to be offered.
func (h *Handoff) Waiting() int {
	return len(h.freed)
}

// Dropped reports how many freed seats were never offered because the queue was
// full. A number that climbs is the signal that the buffer or the worker count
// is wrong.
func (h *Handoff) Dropped() int64 {
	return h.dropped.Load()
}

// Run works through freed seats until ctx is cancelled. It blocks, so callers
// start it in its own goroutine.
func (h *Handoff) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	h.logger.InfoContext(ctx, "seat handoff started", "workers", h.workers)

	for range h.workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case seat := <-h.freed:
					h.hand(ctx, seat)
				}
			}
		}()
	}

	wg.Wait()
	h.logger.InfoContext(ctx, "seat handoff stopped")

	return nil
}

// hand offers one seat to the person at the front of its queue.
func (h *Handoff) hand(ctx context.Context, seat domain.SeatRef) {
	entry, err := h.waiting.TakeNext(ctx, seat.EventID)

	switch {
	case errors.Is(err, repository.ErrWaitingListEmpty):
		return
	case err != nil:
		h.logger.ErrorContext(ctx, "taking the next waiter failed",
			"err", err,
			"eventId", seat.EventID,
		)

		return
	}

	hold, err := h.holder.HoldSeat(ctx, seat.EventID, seat.SeatID, entry.UserID)
	if err != nil {
		// Some reasons will not change by waiting: an account that may not take
		// a seat will be refused for the next one too. Putting such a waiter
		// back would park them at the front of the queue for good and block
		// everybody behind them, so they are dropped and told why in the log.
		if errors.Is(err, domain.ErrEmailNotVerified) {
			h.logger.WarnContext(ctx, "a waiter was dropped from the queue",
				"reason", "the account may not take a seat",
				"err", err,
				"eventId", seat.EventID,
				"userId", entry.UserID,
			)

			return
		}

		// Somebody took the seat between it coming free and this offer. The
		// waiter goes back where they stood rather than to the back of the queue,
		// since none of this was their doing.
		//
		// On a context of its own, because ctx is what shutdown cancels: putting
		// the waiter back is the compensating half of a pop that already
		// happened, and skipping it because the process is closing would drop
		// somebody out of the queue for good.
		rejoinCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rejoinTimeout)
		defer cancel()

		if rejoinErr := h.waiting.Rejoin(rejoinCtx, entry); rejoinErr != nil {
			h.logger.ErrorContext(ctx, "a waiter lost their place after a failed offer",
				"err", rejoinErr,
				"eventId", seat.EventID,
				"userId", entry.UserID,
			)
		}

		return
	}

	h.publisher.Publish(event.Event{
		Kind:      event.TurnCame,
		EventID:   seat.EventID,
		SeatID:    seat.SeatID,
		HoldID:    hold.ID,
		ExpiresAt: hold.ExpiresAt,
		UserID:    entry.UserID,
		At:        h.clock.Now(),
	})
}

var _ SeatOfferer = (*Handoff)(nil)

// discardOfferer is what a service uses when no waiting list is wired in.
type discardOfferer struct{}

func (discardOfferer) Offer(domain.SeatRef) {}
