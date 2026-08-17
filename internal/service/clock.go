package service

import "time"

// Clock is the service layer's only source of the current time. Injecting it
// lets tests jump five minutes forward instead of sleeping, which is what keeps
// the expiry tests fast and repeatable.
type Clock interface {
	Now() time.Time
}

// SystemClock is the real clock, used everywhere outside tests.
type SystemClock struct{}

// Now returns the current wall clock time.
func (SystemClock) Now() time.Time {
	return time.Now()
}
