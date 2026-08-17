package domain

import "testing"

func TestSeatStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status SeatStatus
		want   string
	}{
		{"available", StatusAvailable, "available"},
		{"held", StatusHeld, "held"},
		{"reserved", StatusReserved, "reserved"},
		{"unknown value falls back to a readable form", SeatStatus(42), "SeatStatus(42)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSeatStatus_ZeroValueIsAvailable(t *testing.T) {
	var status SeatStatus

	if status != StatusAvailable {
		t.Errorf("zero value = %v, want %v", status, StatusAvailable)
	}
}

func TestSeatStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status SeatStatus
		want   bool
	}{
		{"available", StatusAvailable, true},
		{"held", StatusHeld, true},
		{"reserved", StatusReserved, true},
		{"below range", SeatStatus(-1), false},
		{"above range", SeatStatus(3), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
