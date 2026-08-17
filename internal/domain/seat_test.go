package domain

import (
	"errors"
	"testing"
	"time"
)

const (
	testEventID = "event-1"
	testSeatID  = "A1"
	testHoldID  = "hold-1"
	testUser    = "user-1"
	otherUser   = "user-2"
	holdTTL     = 5 * time.Minute
)

func baseTime() time.Time {
	return time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
}

func availableSeat() *Seat {
	return NewSeat(testEventID, testSeatID, "A", 1)
}

func heldSeat(userID string, expiresAt time.Time) *Seat {
	s := availableSeat()
	s.Status = StatusHeld
	s.HoldID = testHoldID
	s.HeldBy = userID
	s.HoldCreatedAt = expiresAt.Add(-holdTTL)
	s.HoldExpiresAt = expiresAt

	return s
}

func reservedSeat(userID string) *Seat {
	s := availableSeat()
	s.Status = StatusReserved
	s.ReservedBy = userID

	return s
}

func assertSeatState(t *testing.T, got *Seat, want Seat) {
	t.Helper()

	if got.Status != want.Status {
		t.Errorf("Status = %v, want %v", got.Status, want.Status)
	}
	if got.HoldID != want.HoldID {
		t.Errorf("HoldID = %q, want %q", got.HoldID, want.HoldID)
	}
	if got.HeldBy != want.HeldBy {
		t.Errorf("HeldBy = %q, want %q", got.HeldBy, want.HeldBy)
	}
	if !got.HoldCreatedAt.Equal(want.HoldCreatedAt) {
		t.Errorf("HoldCreatedAt = %v, want %v", got.HoldCreatedAt, want.HoldCreatedAt)
	}
	if !got.HoldExpiresAt.Equal(want.HoldExpiresAt) {
		t.Errorf("HoldExpiresAt = %v, want %v", got.HoldExpiresAt, want.HoldExpiresAt)
	}
	if got.ReservedBy != want.ReservedBy {
		t.Errorf("ReservedBy = %q, want %q", got.ReservedBy, want.ReservedBy)
	}
}

func TestNewSeat(t *testing.T) {
	s := NewSeat(testEventID, testSeatID, "A", 1)

	if s.EventID != testEventID {
		t.Errorf("EventID = %q, want %q", s.EventID, testEventID)
	}
	if s.ID != testSeatID {
		t.Errorf("ID = %q, want %q", s.ID, testSeatID)
	}
	if s.Row != "A" || s.Number != 1 {
		t.Errorf("Row/Number = %q/%d, want A/1", s.Row, s.Number)
	}
	if s.Status != StatusAvailable {
		t.Errorf("Status = %v, want %v", s.Status, StatusAvailable)
	}
}

func TestSeat_Hold(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name     string
		seat     *Seat
		holdID   string
		userID   string
		duration time.Duration
		wantErr  error
	}{
		{
			name:     "available seat can be held",
			seat:     availableSeat(),
			holdID:   testHoldID,
			userID:   testUser,
			duration: holdTTL,
			wantErr:  nil,
		},
		{
			name:     "seat held by another user cannot be held",
			seat:     heldSeat(otherUser, now.Add(time.Minute)),
			holdID:   testHoldID,
			userID:   testUser,
			duration: holdTTL,
			wantErr:  ErrSeatNotAvailable,
		},
		{
			name:     "live hold cannot be re-held by its own owner",
			seat:     heldSeat(testUser, now.Add(time.Minute)),
			holdID:   "hold-2",
			userID:   testUser,
			duration: holdTTL,
			wantErr:  ErrSeatNotAvailable,
		},
		{
			name:     "expired hold can be taken over by another user",
			seat:     heldSeat(otherUser, now.Add(-time.Second)),
			holdID:   "hold-2",
			userID:   testUser,
			duration: holdTTL,
			wantErr:  nil,
		},
		{
			name:     "hold expiring exactly now can be taken over",
			seat:     heldSeat(otherUser, now),
			holdID:   "hold-2",
			userID:   testUser,
			duration: holdTTL,
			wantErr:  nil,
		},
		{
			name:     "reserved seat cannot be held",
			seat:     reservedSeat(otherUser),
			holdID:   testHoldID,
			userID:   testUser,
			duration: holdTTL,
			wantErr:  ErrSeatNotAvailable,
		},
		{
			name:     "empty hold id is rejected",
			seat:     availableSeat(),
			holdID:   "",
			userID:   testUser,
			duration: holdTTL,
			wantErr:  ErrEmptyHoldID,
		},
		{
			name:     "empty user id is rejected",
			seat:     availableSeat(),
			holdID:   testHoldID,
			userID:   "",
			duration: holdTTL,
			wantErr:  ErrEmptyUserID,
		},
		{
			name:     "zero duration is rejected",
			seat:     availableSeat(),
			holdID:   testHoldID,
			userID:   testUser,
			duration: 0,
			wantErr:  ErrInvalidHoldDuration,
		},
		{
			name:     "negative duration is rejected",
			seat:     availableSeat(),
			holdID:   testHoldID,
			userID:   testUser,
			duration: -time.Minute,
			wantErr:  ErrInvalidHoldDuration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.seat.Hold(tt.holdID, tt.userID, tt.duration, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Hold() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			assertSeatState(t, tt.seat, Seat{
				Status:        StatusHeld,
				HoldID:        tt.holdID,
				HeldBy:        tt.userID,
				HoldCreatedAt: now,
				HoldExpiresAt: now.Add(tt.duration),
			})
		})
	}
}

func TestSeat_Hold_RejectedAttemptLeavesSeatUntouched(t *testing.T) {
	now := baseTime()
	expiresAt := now.Add(time.Minute)
	seat := heldSeat(otherUser, expiresAt)

	if err := seat.Hold("hold-2", testUser, holdTTL, now); !errors.Is(err, ErrSeatNotAvailable) {
		t.Fatalf("Hold() error = %v, want %v", err, ErrSeatNotAvailable)
	}

	assertSeatState(t, seat, Seat{
		Status:        StatusHeld,
		HoldID:        testHoldID,
		HeldBy:        otherUser,
		HoldCreatedAt: expiresAt.Add(-holdTTL),
		HoldExpiresAt: expiresAt,
	})
}

func TestSeat_Confirm(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name    string
		seat    *Seat
		userID  string
		wantErr error
	}{
		{
			name:    "owner confirms a live hold",
			seat:    heldSeat(testUser, now.Add(time.Minute)),
			userID:  testUser,
			wantErr: nil,
		},
		{
			name:    "hold with one nanosecond left can still be confirmed",
			seat:    heldSeat(testUser, now.Add(time.Nanosecond)),
			userID:  testUser,
			wantErr: nil,
		},
		{
			name:    "hold expiring exactly now cannot be confirmed",
			seat:    heldSeat(testUser, now),
			userID:  testUser,
			wantErr: ErrHoldExpired,
		},
		{
			name:    "owner of an expired hold gets ErrHoldExpired",
			seat:    heldSeat(testUser, now.Add(-time.Second)),
			userID:  testUser,
			wantErr: ErrHoldExpired,
		},
		{
			name:    "another user cannot confirm a live hold",
			seat:    heldSeat(otherUser, now.Add(time.Minute)),
			userID:  testUser,
			wantErr: ErrNotHoldOwner,
		},
		{
			// Ownership is checked before expiry, so a stranger learns nothing
			// about whether the hold was still alive.
			name:    "another user confirming an expired hold gets ErrNotHoldOwner",
			seat:    heldSeat(otherUser, now.Add(-time.Second)),
			userID:  testUser,
			wantErr: ErrNotHoldOwner,
		},
		{
			name:    "available seat cannot be confirmed",
			seat:    availableSeat(),
			userID:  testUser,
			wantErr: ErrSeatNotHeld,
		},
		{
			name:    "reserved seat cannot be confirmed again",
			seat:    reservedSeat(testUser),
			userID:  testUser,
			wantErr: ErrSeatNotHeld,
		},
		{
			name:    "empty user id is rejected",
			seat:    heldSeat(testUser, now.Add(time.Minute)),
			userID:  "",
			wantErr: ErrEmptyUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.seat.Confirm(tt.userID, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Confirm() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			// A confirmed seat keeps no trace of the hold it came from.
			assertSeatState(t, tt.seat, Seat{
				Status:     StatusReserved,
				ReservedBy: tt.userID,
			})
		})
	}
}

func TestSeat_Release(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name    string
		seat    *Seat
		userID  string
		wantErr error
	}{
		{
			name:    "owner releases a live hold",
			seat:    heldSeat(testUser, now.Add(time.Minute)),
			userID:  testUser,
			wantErr: nil,
		},
		{
			name:    "owner may release an already expired hold",
			seat:    heldSeat(testUser, now.Add(-time.Second)),
			userID:  testUser,
			wantErr: nil,
		},
		{
			name:    "another user cannot release the hold",
			seat:    heldSeat(otherUser, now.Add(time.Minute)),
			userID:  testUser,
			wantErr: ErrNotHoldOwner,
		},
		{
			name:    "available seat cannot be released",
			seat:    availableSeat(),
			userID:  testUser,
			wantErr: ErrSeatNotHeld,
		},
		{
			name:    "reserved seat cannot be released",
			seat:    reservedSeat(testUser),
			userID:  testUser,
			wantErr: ErrSeatNotHeld,
		},
		{
			name:    "empty user id is rejected",
			seat:    heldSeat(testUser, now.Add(time.Minute)),
			userID:  "",
			wantErr: ErrEmptyUserID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.seat.Release(tt.userID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Release() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			assertSeatState(t, tt.seat, Seat{Status: StatusAvailable})
		})
	}
}

func TestSeat_ExpireHold(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name       string
		seat       *Seat
		want       bool
		wantStatus SeatStatus
	}{
		{
			name:       "expired hold is dropped",
			seat:       heldSeat(testUser, now.Add(-time.Second)),
			want:       true,
			wantStatus: StatusAvailable,
		},
		{
			name:       "hold expiring exactly now is dropped",
			seat:       heldSeat(testUser, now),
			want:       true,
			wantStatus: StatusAvailable,
		},
		{
			name:       "live hold is kept",
			seat:       heldSeat(testUser, now.Add(time.Minute)),
			want:       false,
			wantStatus: StatusHeld,
		},
		{
			name:       "available seat is untouched",
			seat:       availableSeat(),
			want:       false,
			wantStatus: StatusAvailable,
		},
		{
			name:       "reserved seat is untouched",
			seat:       reservedSeat(testUser),
			want:       false,
			wantStatus: StatusReserved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seat.ExpireHold(now); got != tt.want {
				t.Fatalf("ExpireHold() = %v, want %v", got, tt.want)
			}
			if tt.seat.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", tt.seat.Status, tt.wantStatus)
			}
			if tt.want && tt.seat.HeldBy != "" {
				t.Errorf("HeldBy = %q, want empty after expiry", tt.seat.HeldBy)
			}
		})
	}
}

func TestSeat_CanHold(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name string
		seat *Seat
		want bool
	}{
		{"available seat", availableSeat(), true},
		{"live hold", heldSeat(testUser, now.Add(time.Minute)), false},
		{"hold expiring exactly now", heldSeat(testUser, now), true},
		{"expired hold", heldSeat(testUser, now.Add(-time.Second)), true},
		{"reserved seat", reservedSeat(testUser), false},
		{"unknown status", &Seat{Status: SeatStatus(42)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seat.CanHold(now); got != tt.want {
				t.Errorf("CanHold() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeat_IsHoldExpired(t *testing.T) {
	now := baseTime()

	tests := []struct {
		name string
		seat *Seat
		want bool
	}{
		{"live hold", heldSeat(testUser, now.Add(time.Minute)), false},
		{"expired hold", heldSeat(testUser, now.Add(-time.Second)), true},
		{"hold expiring exactly now", heldSeat(testUser, now), true},
		{
			// Only a held seat can have an expired hold; stale timestamps on a
			// seat in any other status must not be read as one.
			name: "available seat with a stale expiry timestamp",
			seat: &Seat{Status: StatusAvailable, HoldExpiresAt: now.Add(-time.Hour)},
			want: false,
		},
		{
			name: "reserved seat with a stale expiry timestamp",
			seat: &Seat{Status: StatusReserved, HoldExpiresAt: now.Add(-time.Hour)},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.seat.IsHoldExpired(now); got != tt.want {
				t.Errorf("IsHoldExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeat_CurrentHold(t *testing.T) {
	now := baseTime()
	expiresAt := now.Add(time.Minute)

	t.Run("returns the live hold", func(t *testing.T) {
		seat := heldSeat(testUser, expiresAt)

		got := seat.CurrentHold(now)
		if got == nil {
			t.Fatal("CurrentHold() = nil, want a hold")
		}
		if got.ID != testHoldID || got.UserID != testUser {
			t.Errorf("hold = %+v, want id %q held by %q", got, testHoldID, testUser)
		}
		if got.SeatID != testSeatID || got.EventID != testEventID {
			t.Errorf("hold = %+v, want seat %q of event %q", got, testSeatID, testEventID)
		}
		if !got.ExpiresAt.Equal(expiresAt) {
			t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, expiresAt)
		}
		if !got.CreatedAt.Equal(seat.HoldCreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, seat.HoldCreatedAt)
		}
	})

	t.Run("mutating the returned hold does not affect the seat", func(t *testing.T) {
		seat := heldSeat(testUser, expiresAt)

		got := seat.CurrentHold(now)
		got.UserID = otherUser

		if seat.HeldBy != testUser {
			t.Errorf("HeldBy = %q, want %q", seat.HeldBy, testUser)
		}
	})

	t.Run("returns nil when there is no live hold", func(t *testing.T) {
		tests := []struct {
			name string
			seat *Seat
		}{
			{"available seat", availableSeat()},
			{"expired hold", heldSeat(testUser, now.Add(-time.Second))},
			{"reserved seat", reservedSeat(testUser)},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := tt.seat.CurrentHold(now); got != nil {
					t.Errorf("CurrentHold() = %+v, want nil", got)
				}
			})
		}
	})
}

// TestSeat_Lifecycle walks the happy path and the two ways it can end, which is
// the sequence the service layer will drive in stage 2.
func TestSeat_Lifecycle(t *testing.T) {
	now := baseTime()

	t.Run("hold then confirm locks the seat for good", func(t *testing.T) {
		seat := availableSeat()

		if err := seat.Hold(testHoldID, testUser, holdTTL, now); err != nil {
			t.Fatalf("Hold() error = %v", err)
		}
		if err := seat.Confirm(testUser, now.Add(time.Minute)); err != nil {
			t.Fatalf("Confirm() error = %v", err)
		}

		later := now.Add(time.Hour)
		if err := seat.Hold("hold-2", otherUser, holdTTL, later); !errors.Is(err, ErrSeatNotAvailable) {
			t.Errorf("Hold() after confirm error = %v, want %v", err, ErrSeatNotAvailable)
		}
	})

	t.Run("hold then release frees the seat for the next user", func(t *testing.T) {
		seat := availableSeat()

		if err := seat.Hold(testHoldID, testUser, holdTTL, now); err != nil {
			t.Fatalf("Hold() error = %v", err)
		}
		if err := seat.Release(testUser); err != nil {
			t.Fatalf("Release() error = %v", err)
		}
		if err := seat.Hold("hold-2", otherUser, holdTTL, now); err != nil {
			t.Errorf("Hold() after release error = %v, want nil", err)
		}
	})

	t.Run("a hold that runs out frees the seat without anyone acting", func(t *testing.T) {
		seat := availableSeat()

		if err := seat.Hold(testHoldID, testUser, holdTTL, now); err != nil {
			t.Fatalf("Hold() error = %v", err)
		}

		afterExpiry := now.Add(holdTTL)
		if err := seat.Hold("hold-2", otherUser, holdTTL, afterExpiry); err != nil {
			t.Errorf("Hold() after expiry error = %v, want nil", err)
		}
		if err := seat.Confirm(testUser, afterExpiry); !errors.Is(err, ErrNotHoldOwner) {
			t.Errorf("Confirm() by the previous owner error = %v, want %v", err, ErrNotHoldOwner)
		}
	})
}
