package domain

import (
	"fmt"
	"time"
)

// MaxSeatsPerRequest bounds how many seats one call may create. Without it a
// single mistyped number turns into millions of rows.
const MaxSeatsPerRequest = 2000

// NewEvent returns an event ready to be stored.
//
// StartsAt is not required to be in the future: a venue loading last season's
// archive has a reason to store one that is past, and whether a started event
// still sells is already a service decision.
func NewEvent(id, name, venue string, startsAt time.Time, details EventDetails) (*Event, error) {
	if id == "" {
		return nil, ErrEmptyEventID
	}
	if name == "" {
		return nil, fmt.Errorf("%w: the name is empty", ErrInvalidEvent)
	}
	if venue == "" {
		return nil, fmt.Errorf("%w: the venue is empty", ErrInvalidEvent)
	}
	if startsAt.IsZero() {
		return nil, fmt.Errorf("%w: it has no start time", ErrInvalidEvent)
	}

	details = details.WithDefaults()
	if err := details.Validate(); err != nil {
		return nil, err
	}

	return &Event{
		ID:    id,
		Name:  name,
		Venue: venue,
		// Stored to the second. Sub-second precision on a concert start time is
		// noise that only makes two timestamps compare unequal.
		StartsAt: startsAt.UTC().Truncate(time.Second),
		Details:  details,
	}, nil
}

// NewSeatBlock builds the seats of a row block: the given rows, each numbered
// from one to perRow. Seat ids are the row label and the number, so A1 reads the
// same in a URL as it does on a ticket.
func NewSeatBlock(eventID string, rows []string, perRow int) ([]*Seat, error) {
	if eventID == "" {
		return nil, ErrEmptyEventID
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: no rows were given", ErrInvalidSeatMap)
	}
	if perRow < 1 {
		return nil, fmt.Errorf("%w: a row needs at least one seat, got %d", ErrInvalidSeatMap, perRow)
	}

	// Divided rather than multiplied. len(rows) * perRow overflows for a large
	// enough perRow, and an overflowed product is a small number that passes the
	// check and then panics in make below. perRow is known to be at least one by
	// here, so the division is safe.
	if perRow > MaxSeatsPerRequest || len(rows) > MaxSeatsPerRequest/perRow {
		return nil, fmt.Errorf("%w: %d rows of %d is more than the %d seats allowed in one request",
			ErrInvalidSeatMap, len(rows), perRow, MaxSeatsPerRequest)
	}

	seen := make(map[string]bool, len(rows))
	seats := make([]*Seat, 0, len(rows)*perRow)

	for _, row := range rows {
		if row == "" {
			return nil, fmt.Errorf("%w: a row label is empty", ErrInvalidSeatMap)
		}

		// Two rows called A would produce two seats called A1, and the second
		// would be silently dropped by the store's conflict handling.
		if seen[row] {
			return nil, fmt.Errorf("%w: row %q is listed twice", ErrInvalidSeatMap, row)
		}

		seen[row] = true

		for number := 1; number <= perRow; number++ {
			seats = append(seats, NewSeat(eventID, fmt.Sprintf("%s%d", row, number), row, number))
		}
	}

	return seats, nil
}
