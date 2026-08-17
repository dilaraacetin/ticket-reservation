package domain

import (
	"testing"
	"time"
)

func TestHold_IsExpired(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"expiry in the future", now.Add(time.Minute), false},
		{"expiry one nanosecond away", now.Add(time.Nanosecond), false},
		{"expiry exactly now", now, true},
		{"expiry in the past", now.Add(-time.Second), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Hold{ID: testHoldID, ExpiresAt: tt.expiresAt}

			if got := h.IsExpired(now); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHold_RemainingTime(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name      string
		expiresAt time.Time
		want      time.Duration
	}{
		{"two minutes left", now.Add(2 * time.Minute), 2 * time.Minute},
		{"expiry exactly now", now, 0},
		{"already expired never goes negative", now.Add(-time.Hour), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Hold{ID: testHoldID, ExpiresAt: tt.expiresAt}

			if got := h.RemainingTime(now); got != tt.want {
				t.Errorf("RemainingTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
