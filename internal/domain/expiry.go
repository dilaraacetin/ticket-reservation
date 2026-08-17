package domain

import "time"

// reached reports whether instant has arrived by now, counting the boundary
// instant itself as arrived. Every deadline in the package asks this one
// function, which is what makes the hold-expires-as-the-owner-confirms race
// resolve the same way every time.
func reached(instant, now time.Time) bool {
	return !now.Before(instant)
}
