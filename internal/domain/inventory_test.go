package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseRole(t *testing.T) {
	for _, value := range []string{"customer", "admin"} {
		role, err := ParseRole(value)
		if err != nil {
			t.Errorf("ParseRole(%q) error = %v", value, err)
		}
		if role.String() != value {
			t.Errorf("ParseRole(%q) = %q", value, role)
		}
	}

	// A stored value nobody has written a check for must fail rather than be
	// read as a role that happens to grant something.
	for _, value := range []string{"", "superuser", "Admin", "admn"} {
		if _, err := ParseRole(value); !errors.Is(err, ErrUnknownRole) {
			t.Errorf("ParseRole(%q) error = %v, want %v", value, err, ErrUnknownRole)
		}
	}
}

// Nobody becomes an administrator by signing up.
func TestDefaultRoleIsNotAdmin(t *testing.T) {
	if DefaultRole.IsAdmin() {
		t.Errorf("DefaultRole = %q, which is an administrator", DefaultRole)
	}
}

func TestNewEvent(t *testing.T) {
	startsAt := time.Date(2026, 11, 14, 20, 30, 0, 123456789, time.UTC)

	details := EventDetails{
		City:        "Istanbul",
		Category:    CategoryConcert,
		ImageURL:    "https://images.seathold.test/radiohead.jpg",
		Description: "The band's first Istanbul date since 2017, playing In Rainbows in full.",
		Rules:       "Doors open at 19:00. No professional cameras. Under 16s must come with an adult.",
	}

	event, err := NewEvent("event-radiohead", "Radiohead", "Volkswagen Arena", startsAt, details)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	if event.Details != details {
		t.Errorf("Details = %+v, want %+v", event.Details, details)
	}

	// Sub-second precision on a concert start time is noise that only makes two
	// timestamps compare unequal.
	if event.StartsAt.Nanosecond() != 0 {
		t.Errorf("StartsAt = %s, want it truncated to the second", event.StartsAt)
	}
	if !event.StartsAt.Equal(startsAt.Truncate(time.Second)) {
		t.Errorf("StartsAt = %s, want %s", event.StartsAt, startsAt.Truncate(time.Second))
	}
}

func TestNewEvent_RejectsIncompleteEvents(t *testing.T) {
	startsAt := time.Date(2026, 11, 14, 20, 30, 0, 0, time.UTC)

	ok := EventDetails{City: "Istanbul", Category: CategoryConcert}

	tests := []struct {
		name     string
		id       string
		event    string
		venue    string
		startsAt time.Time
		details  EventDetails
		wants    error
	}{
		{"no id", "", "Radiohead", "Volkswagen Arena", startsAt, ok, ErrEmptyEventID},
		{"no name", "event-1", "", "Volkswagen Arena", startsAt, ok, ErrInvalidEvent},
		{"no venue", "event-1", "Radiohead", "", startsAt, ok, ErrInvalidEvent},
		{"no start time", "event-1", "Radiohead", "Volkswagen Arena", time.Time{}, ok, ErrInvalidEvent},
		{"no city", "event-1", "Radiohead", "Volkswagen Arena", startsAt, EventDetails{}, ErrInvalidEvent},
		{
			"an image address that is not a web address",
			"event-1", "Radiohead", "Volkswagen Arena", startsAt,
			EventDetails{City: "Istanbul", ImageURL: "javascript:alert(1)"},
			ErrInvalidEvent,
		},
		{
			"an image address with no host",
			"event-1", "Radiohead", "Volkswagen Arena", startsAt,
			EventDetails{City: "Istanbul", ImageURL: "https:///poster.jpg"},
			ErrInvalidEvent,
		},
		{
			"a description longer than the limit",
			"event-1", "Radiohead", "Volkswagen Arena", startsAt,
			EventDetails{City: "Istanbul", Description: strings.Repeat("a", MaxEventTextLength+1)},
			ErrInvalidEvent,
		},
		{
			"a category nothing will ever match",
			"event-1", "Radiohead", "Volkswagen Arena", startsAt,
			EventDetails{City: "Istanbul", Category: "stand up"},
			ErrInvalidEvent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewEvent(tt.id, tt.event, tt.venue, tt.startsAt, tt.details); !errors.Is(err, tt.wants) {
				t.Errorf("NewEvent() error = %v, want %v", err, tt.wants)
			}
		})
	}
}

// An event nobody classified is still an event, so the category falls back
// rather than refusing the whole thing.
func TestNewEventFallsBackToTheDefaultCategory(t *testing.T) {
	startsAt := time.Date(2026, 11, 14, 20, 30, 0, 0, time.UTC)

	event, err := NewEvent("event-1", "Radiohead", "Volkswagen Arena", startsAt,
		EventDetails{City: "Istanbul"})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	if event.Details.Category != DefaultCategory {
		t.Errorf("Category = %q, want %q", event.Details.Category, DefaultCategory)
	}
}

func TestNewSeatBlock(t *testing.T) {
	seats, err := NewSeatBlock("event-radiohead", []string{"A", "B"}, 3)
	if err != nil {
		t.Fatalf("NewSeatBlock() error = %v", err)
	}

	if len(seats) != 6 {
		t.Fatalf("built %d seats, want 6", len(seats))
	}

	// The id is what ends up in a URL and on a ticket, so it has to read the
	// same in both.
	want := []string{"A1", "A2", "A3", "B1", "B2", "B3"}
	for i, seat := range seats {
		if seat.ID != want[i] {
			t.Errorf("seat %d is %q, want %q", i, seat.ID, want[i])
		}
		if seat.Status != StatusAvailable {
			t.Errorf("seat %s is %v, want %v", seat.ID, seat.Status, StatusAvailable)
		}
		if seat.EventID != "event-radiohead" {
			t.Errorf("seat %s belongs to %q", seat.ID, seat.EventID)
		}
	}
}

func TestNewSeatBlock_RejectsBadMaps(t *testing.T) {
	tests := []struct {
		name    string
		eventID string
		rows    []string
		perRow  int
		wants   error
	}{
		{"no event", "", []string{"A"}, 1, ErrEmptyEventID},
		{"no rows", "event-1", nil, 10, ErrInvalidSeatMap},
		{"empty row label", "event-1", []string{"A", ""}, 10, ErrInvalidSeatMap},
		{"zero seats per row", "event-1", []string{"A"}, 0, ErrInvalidSeatMap},
		{"negative seats per row", "event-1", []string{"A"}, -5, ErrInvalidSeatMap},
		// Two rows called A would produce two seats called A1, and the store
		// would silently keep one of them.
		{"the same row twice", "event-1", []string{"A", "B", "A"}, 10, ErrInvalidSeatMap},
		// One mistyped number should not become millions of rows.
		{"more seats than allowed", "event-1", []string{"A", "B"}, MaxSeatsPerRequest, ErrInvalidSeatMap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewSeatBlock(tt.eventID, tt.rows, tt.perRow); !errors.Is(err, tt.wants) {
				t.Errorf("NewSeatBlock() error = %v, want %v", err, tt.wants)
			}
		})
	}
}
