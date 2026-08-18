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
// test failures instead of as bare integers. It is also the stored form, which
// is why the words matter: a database column holds these exact strings.
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

// ParseSeatStatus turns the stored form back into a status. It exists for the
// edges of the system, where a status arrives as text from a database column or
// a JSON body and cannot be trusted to be one of the three.
func ParseSeatStatus(value string) (SeatStatus, error) {
	for _, status := range []SeatStatus{StatusAvailable, StatusHeld, StatusReserved} {
		if status.String() == value {
			return status, nil
		}
	}

	return 0, fmt.Errorf("%w: %q", ErrUnknownSeatStatus, value)
}
