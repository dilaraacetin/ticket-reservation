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

// ListSeatsForUser returns copies of every seat the user holds or has reserved.
func (r *MemorySeatRepository) ListSeatsForUser(_ context.Context, userID string) ([]*domain.Seat, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seats := make([]*domain.Seat, 0)

	for _, stored := range r.seats {
		if stored.HeldBy != userID && stored.ReservedBy != userID {
			continue
		}

		seat := *stored
		seats = append(seats, &seat)
	}

	sortSeats(seats)

	return seats, nil
}

// ExpireHolds frees every seat whose hold has run out by now and returns which
// ones it freed.
func (r *MemorySeatRepository) ExpireHolds(_ context.Context, now time.Time) ([]domain.SeatRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var freed []domain.SeatRef

	for key, stored := range r.seats {
		seat := *stored
		if !seat.ExpireHold(now) {
			continue
		}

		r.seats[key] = &seat
		freed = append(freed, seat.Ref())
	}

	// Map order is random, so without this two runs over the same data would
	// report the same seats in a different order.
	slices.SortFunc(freed, func(a, b domain.SeatRef) int {
		if event := strings.Compare(a.EventID, b.EventID); event != 0 {
			return event
		}

		return strings.Compare(a.SeatID, b.SeatID)
	})

	return freed, nil
}

// CreateSeats adds seats, leaving alone any that are already there. Adding a row
// twice must not reset a seat that is already held or sold.
func (r *MemorySeatRepository) CreateSeats(_ context.Context, seats ...*domain.Seat) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	created := 0

	for _, seat := range seats {
		key := seatKey(seat.EventID, seat.ID)
		if _, exists := r.seats[key]; exists {
			continue
		}

		stored := *seat
		r.seats[key] = &stored
		created++
	}

	return created, nil
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

	sortSeats(seats)

	return seats, nil
}

// sortSeats puts seats in the order a seat map is read in, so callers get a
// stable answer rather than Go's random map order.
func sortSeats(seats []*domain.Seat) {
	slices.SortFunc(seats, func(a, b *domain.Seat) int {
		if event := strings.Compare(a.EventID, b.EventID); event != 0 {
			return event
		}

		if row := strings.Compare(a.Row, b.Row); row != 0 {
			return row
		}

		return a.Number - b.Number
	})
}
