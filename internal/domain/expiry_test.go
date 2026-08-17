package domain

import (
	"testing"
	"time"
)

func TestReached(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name    string
		instant time.Time
		want    bool
	}{
		{"an hour away", now.Add(time.Hour), false},
		{"one nanosecond away", now.Add(time.Nanosecond), false},
		{"exactly now counts as reached", now, true},
		{"one nanosecond ago", now.Add(-time.Nanosecond), true},
		{"an hour ago", now.Add(-time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := reached(tt.instant, now); got != tt.want {
				t.Errorf("reached() = %v, want %v", got, tt.want)
			}
		})
	}
}
