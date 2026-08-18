package domain

import (
	"errors"
	"testing"
)

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

func TestParseSeatStatus(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    SeatStatus
		wantErr bool
	}{
		{"available", "available", StatusAvailable, false},
		{"held", "held", StatusHeld, false},
		{"reserved", "reserved", StatusReserved, false},
		{"unknown word", "booked", 0, true},
		{"empty", "", 0, true},
		{"wrong case", "Available", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSeatStatus(tt.value)
			if tt.wantErr {
				if !errors.Is(err, ErrUnknownSeatStatus) {
					t.Fatalf("ParseSeatStatus() error = %v, want %v", err, ErrUnknownSeatStatus)
				}

				return
			}
			if err != nil {
				t.Fatalf("ParseSeatStatus() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseSeatStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeatStatus_RoundTrip(t *testing.T) {
	for status := StatusAvailable; status <= StatusReserved; status++ {
		got, err := ParseSeatStatus(status.String())
		if err != nil {
			t.Errorf("ParseSeatStatus(%q) error = %v", status.String(), err)

			continue
		}
		if got != status {
			t.Errorf("round trip of %v gave %v", status, got)
		}
	}
}
