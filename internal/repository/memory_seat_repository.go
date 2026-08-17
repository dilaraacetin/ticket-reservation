package repository

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"ticket-reservation/internal/domain"
)

// MemorySeatRepository keeps seats in a map.
type MemorySeatRepository struct {
	mu    sync.RWMutex
	seats map[string]*domain.Seat
}

// NewMemorySeatRepository returns a store seeded with the given seats.
func NewMemorySeatRepository(seats ...*domain.Seat) *MemorySeatRepository {
	r := &MemorySeatRepository{seats: make(map[string]*domain.Seat, len(seats))}
	for _, seat := range seats {
		stored := *seat
		r.seats[seatKey(seat.EventID, seat.ID)] = &stored
	}

	return r
}

func seatKey(eventID, seatID string) string {
	return eventID + "/" + seatID
}

// GetSeat returns a copy of the stored seat. Handing out the stored pointer
// would let callers mutate the store without going through UpdateSeat, which no
// real database would ever allow.
func (r *MemorySeatRepository) GetSeat(_ context.Context, eventID, seatID string) (*domain.Seat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stored, ok := r.seats[seatKey(eventID, seatID)]
	if !ok {
		return nil, ErrSeatNotFound
	}

	seat := *stored

	return &seat, nil
}

// UpdateSeat runs mutate against the stored seat while holding the write lock,
// so no other caller can read or write that seat in between.
func (r *MemorySeatRepository) UpdateSeat(
	_ context.Context,
	eventID, seatID string,
	mutate func(*domain.Seat) error,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := seatKey(eventID, seatID)

	stored, ok := r.seats[key]
	if !ok {
		return ErrSeatNotFound
	}

	seat := *stored
	if err := mutate(&seat); err != nil {
		return err
	}

	r.seats[key] = &seat

	return nil
}

// UpdateSeatByHoldID scans for the seat carrying holdID and updates it under the
// write lock.
func (r *MemorySeatRepository) UpdateSeatByHoldID(
	_ context.Context,
	holdID string,
	mutate func(*domain.Seat) error,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for key, stored := range r.seats {
		if stored.HoldID != holdID {
			continue
		}

		seat := *stored
		if err := mutate(&seat); err != nil {
			return err
		}

		r.seats[key] = &seat

		return nil
	}

	return ErrHoldNotFound
}

// ExpireHolds frees every seat whose hold has run out by now.
func (r *MemorySeatRepository) ExpireHolds(_ context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	expired := 0

	for key, stored := range r.seats {
		seat := *stored
		if !seat.ExpireHold(now) {
			continue
		}

		r.seats[key] = &seat
		expired++
	}

	return expired, nil
}

// ListSeats returns copies of every seat of an event, ordered by row and number
// so that callers get a stable seat map rather than Go's random map order.
func (r *MemorySeatRepository) ListSeats(_ context.Context, eventID string) ([]*domain.Seat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seats := make([]*domain.Seat, 0, len(r.seats))
	for _, stored := range r.seats {
		if stored.EventID != eventID {
			continue
		}

		seat := *stored
		seats = append(seats, &seat)
	}

	slices.SortFunc(seats, func(a, b *domain.Seat) int {
		if row := strings.Compare(a.Row, b.Row); row != 0 {
			return row
		}

		return a.Number - b.Number
	})

	return seats, nil
}
