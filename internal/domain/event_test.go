package domain

import (
	"testing"
	"time"
)

func TestEvent_HasStarted(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name     string
		startsAt time.Time
		want     bool
	}{
		{"starts in an hour", now.Add(time.Hour), false},
		{"starts one nanosecond from now", now.Add(time.Nanosecond), false},
		{"starts exactly now", now, true},
		{"started an hour ago", now.Add(-time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Event{ID: testEventID, Name: "Concert", StartsAt: tt.startsAt}

			if got := e.HasStarted(now); got != tt.want {
				t.Errorf("HasStarted() = %v, want %v", got, tt.want)
			}
		})
	}
}
