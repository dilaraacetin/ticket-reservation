package domain

import "fmt"

// SeatStatus is the lifecycle state of a seat. The zero value is
// StatusAvailable, so a seat is available without anyone having to initialise
// the field.
type SeatStatus int

// The states a seat can be in. StatusReserved is terminal in this stage.
const (
	StatusAvailable SeatStatus = iota
	StatusHeld
	StatusReserved
)

// String implements fmt.Stringer so that statuses read as words in logs and
// test failures instead of as bare integers.
func (s SeatStatus) String() string {
	switch s {
	case StatusAvailable:
		return "available"
	case StatusHeld:
		return "held"
	case StatusReserved:
		return "reserved"
	default:
		return fmt.Sprintf("SeatStatus(%d)", int(s))
	}
}

// IsValid reports whether s is one of the defined statuses. It is for the edges
// of the system, where a status arrives as a number from JSON or a database
// column.
func (s SeatStatus) IsValid() bool {
	return s >= StatusAvailable && s <= StatusReserved
}
