package service

import (
	"errors"
	"testing"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// newGatedSetup wires the real rule, so these tests are about what a deployment
// actually does rather than about a fake.
func newGatedSetup(t *testing.T) (
	*ReservationService,
	*WaitingListService,
	*repository.MemoryUserRepository,
) {
	t.Helper()

	var (
		clock = newFakeClock(testTime())
		users = repository.NewMemoryUserRepository()
		seats = repository.NewMemorySeatRepository(newTestSeat())
		store = repository.NewMemoryEventRepository(testEvent())
		gate  = NewVerifiedOnly(users)
	)

	reservations := NewReservationService(Config{
		Seats:   seats,
		Events:  store,
		Clock:   clock,
		NewID:   NewRandomID,
		HoldTTL: DefaultHoldTTL,
		Gate:    gate,
	})

	waiting := NewWaitingListService(WaitingListConfig{
		Waiting: repository.NewMemoryWaitingListRepository(),
		Events:  store,
		Clock:   clock,
		NewID:   NewRandomID,
		Gate:    gate,
	})

	return reservations, waiting, users
}

func gatedUser(t *testing.T, users *repository.MemoryUserRepository, id, email string, verified bool) {
	t.Helper()

	user := domain.NewUser(id, email, "$argon2id$fake", testTime())
	if err := users.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if verified {
		if err := users.MarkEmailVerified(t.Context(), id, email, testTime()); err != nil {
			t.Fatalf("MarkEmailVerified() error = %v", err)
		}
	}
}

// A ticket sold to an address that may not exist is the thing this stops.
func TestGate_AnUnconfirmedAccountCannotTakeASeat(t *testing.T) {
	reservations, waiting, users := newGatedSetup(t)
	gatedUser(t, users, testUser, "unconfirmed@example.com", false)

	_, err := reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser)
	if !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Errorf("HoldSeat() = %v, want %v", err, domain.ErrEmailNotVerified)
	}

	// The same rule, because a waiter who cannot be given a seat would sit at
	// the front of the queue being refused.
	_, err = waiting.Join(t.Context(), testEventID, testUser)
	if !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Errorf("Join() = %v, want %v", err, domain.ErrEmailNotVerified)
	}
}

func TestGate_AConfirmedAccountIsUnaffected(t *testing.T) {
	reservations, waiting, users := newGatedSetup(t)
	gatedUser(t, users, testUser, "confirmed@example.com", true)

	hold, err := reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser)
	if err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	if _, err := reservations.ConfirmReservation(t.Context(), hold.ID, testUser); err != nil {
		t.Errorf("ConfirmReservation() error = %v", err)
	}

	gatedUser(t, users, otherUser, "also-confirmed@example.com", true)

	if _, err := waiting.Join(t.Context(), testEventID, otherUser); err != nil {
		t.Errorf("Join() error = %v", err)
	}
}

// Checked on confirming as well as on holding, because a hold made before the
// rule applied would otherwise still turn into a sale.
func TestGate_AnUnconfirmedAccountCannotConfirmAnOlderHold(t *testing.T) {
	reservations, _, users := newGatedSetup(t)

	// The hold is made while the rule would allow it, which is what a hold from
	// before the rule looks like.
	ungated := NewReservationService(Config{
		Seats:   repository.NewMemorySeatRepository(newTestSeat()),
		Events:  repository.NewMemoryEventRepository(testEvent()),
		Clock:   newFakeClock(testTime()),
		NewID:   func() string { return "hold-from-before" },
		HoldTTL: DefaultHoldTTL,
	})

	if _, err := ungated.HoldSeat(t.Context(), testEventID, testSeatID, testUser); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	gatedUser(t, users, testUser, "unconfirmed@example.com", false)

	_, err := reservations.ConfirmReservation(t.Context(), "hold-from-before", testUser)
	if !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Errorf("ConfirmReservation() = %v, want %v", err, domain.ErrEmailNotVerified)
	}
}

// An account that is not there at all is refused rather than let through, which
// is the direction a missing record has to fail in.
func TestGate_AnAccountThatIsNotThereIsRefused(t *testing.T) {
	reservations, _, _ := newGatedSetup(t)

	_, err := reservations.HoldSeat(t.Context(), testEventID, testSeatID, "nobody")
	if err == nil {
		t.Fatal("HoldSeat() for an account that does not exist went through")
	}
	if !errors.Is(err, repository.ErrUserNotFound) {
		t.Errorf("HoldSeat() = %v, want it to report the missing account", err)
	}
}
