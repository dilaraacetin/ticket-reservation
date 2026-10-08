package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/event"
	"ticket-reservation/internal/repository"
)

const waitingUser = "user-3"

type handoffSetup struct {
	reservations *ReservationService
	handoff      *Handoff
	waiting      *repository.MemoryWaitingListRepository
	published    *recordingPublisher
	seats        *repository.MemorySeatRepository
}

func newHandoffSetup(t *testing.T) *handoffSetup {
	t.Helper()

	var (
		clock     = newFakeClock(testTime())
		publisher = &recordingPublisher{}
		seats     = repository.NewMemorySeatRepository(newTestSeat())
		waiting   = repository.NewMemoryWaitingListRepository()
	)

	reservations := NewReservationService(Config{
		Seats:     seats,
		Events:    repository.NewMemoryEventRepository(testEvent()),
		Clock:     clock,
		NewID:     NewRandomID,
		HoldTTL:   DefaultHoldTTL,
		Publisher: publisher,
	})

	handoff := NewHandoff(HandoffConfig{
		Waiting:   waiting,
		Holder:    reservations,
		Publisher: publisher,
		Clock:     clock,
		Logger:    discardLogger(),
		Workers:   2,
	})

	reservations.WithOfferer(handoff)

	return &handoffSetup{
		reservations: reservations,
		handoff:      handoff,
		waiting:      waiting,
		published:    publisher,
		seats:        seats,
	}
}

// joinQueue puts a user in the queue and fails the test if that does not work.
func (s *handoffSetup) joinQueue(t *testing.T, userID string, at time.Time) {
	t.Helper()

	entry, err := domain.NewWaitingEntry("wait-"+userID, testEventID, userID, at)
	if err != nil {
		t.Fatalf("NewWaitingEntry() error = %v", err)
	}

	if _, _, err := s.waiting.Join(t.Context(), entry); err != nil {
		t.Fatalf("Join() error = %v", err)
	}
}

// waitFor polls until cond holds or the deadline passes, for the parts of this
// flow that happen on another goroutine.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func turnCame(events []event.Event) (event.Event, bool) {
	for _, e := range events {
		if e.Kind == event.TurnCame {
			return e, true
		}
	}

	return event.Event{}, false
}

// The whole point of the feature, through the real channel and worker pool:
// releasing a seat hands it to whoever has waited longest.
func TestHandoff_GivesAFreedSeatToTheNextWaiter(t *testing.T) {
	setup := newHandoffSetup(t)

	go func() { _ = setup.handoff.Run(t.Context()) }()

	hold, err := setup.reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser)
	if err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	setup.joinQueue(t, waitingUser, testTime())

	if err := setup.reservations.ReleaseSeat(t.Context(), hold.ID, testUser); err != nil {
		t.Fatalf("ReleaseSeat() error = %v", err)
	}

	waitFor(t, "the waiter's turn", func() bool {
		_, ok := turnCame(setup.published.seen())

		return ok
	})

	// The seat is held for the waiter, not merely free for anyone to take.
	seat, err := setup.seats.GetSeat(t.Context(), testEventID, testSeatID)
	if err != nil {
		t.Fatalf("GetSeat() error = %v", err)
	}

	if seat.Status != domain.StatusHeld {
		t.Errorf("seat status = %v, want %v", seat.Status, domain.StatusHeld)
	}
	if seat.HeldBy != waitingUser {
		t.Errorf("seat held by %q, want %q", seat.HeldBy, waitingUser)
	}

	// The notice goes to one person. Telling every watcher that somebody's turn
	// came would be both noise and a leak.
	notice, _ := turnCame(setup.published.seen())
	if notice.UserID != waitingUser {
		t.Errorf("turn_came went to %q, want %q", notice.UserID, waitingUser)
	}
	if notice.IsForEveryone() {
		t.Error("turn_came was published to every watcher")
	}
	if notice.HoldID == "" {
		t.Error("turn_came carries no hold id, so the client cannot confirm it")
	}

	// Without this the client can confirm the hold but cannot show how long it
	// has, which is the one thing that makes a hold feel temporary.
	if notice.ExpiresAt.IsZero() {
		t.Error("turn_came carries no expiry")
	}

	// Their turn came, so they are no longer waiting for one.
	if _, err := setup.waiting.Position(t.Context(), testEventID, waitingUser); !errors.Is(err, repository.ErrNotWaiting) {
		t.Errorf("Position() after the turn came = %v, want %v", err, repository.ErrNotWaiting)
	}
}

// A seat can be taken between coming free and being offered. The waiter did
// nothing wrong, so they keep the place they had.
func TestHandoff_KeepsTheWaitersPlaceWhenTheSeatIsGone(t *testing.T) {
	setup := newHandoffSetup(t)

	setup.joinQueue(t, waitingUser, testTime())
	setup.joinQueue(t, otherUser, testTime().Add(time.Second))

	// Somebody else walks up and takes it before the offer is worked through.
	if _, err := setup.reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	setup.handoff.hand(t.Context(), domain.SeatRef{EventID: testEventID, SeatID: testSeatID})

	position, err := setup.waiting.Position(t.Context(), testEventID, waitingUser)
	if err != nil {
		t.Fatalf("Position() error = %v", err)
	}
	if position != 1 {
		t.Errorf("Position() = %d after a failed offer, want 1", position)
	}

	if _, ok := turnCame(setup.published.seen()); ok {
		t.Error("a turn_came was published although no hold was made")
	}
}

func TestHandoff_DoesNothingWhenNobodyIsWaiting(t *testing.T) {
	setup := newHandoffSetup(t)

	setup.handoff.hand(t.Context(), domain.SeatRef{EventID: testEventID, SeatID: testSeatID})

	seat, err := setup.seats.GetSeat(t.Context(), testEventID, testSeatID)
	if err != nil {
		t.Fatalf("GetSeat() error = %v", err)
	}

	if seat.Status != domain.StatusAvailable {
		t.Errorf("seat status = %v, want it left alone as %v", seat.Status, domain.StatusAvailable)
	}
}

// An expired hold frees a seat without anyone asking, so the sweeper has to feed
// the queue the same way a release does.
func TestHandoff_SweptSeatsReachTheQueue(t *testing.T) {
	setup := newHandoffSetup(t)
	clock := newFakeClock(testTime())

	sweeper := NewHoldSweeper(setup.seats, clock, DefaultSweepInterval, discardLogger()).
		WithOfferer(setup.handoff)

	go func() { _ = setup.handoff.Run(t.Context()) }()

	if _, err := setup.reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	setup.joinQueue(t, waitingUser, testTime())

	clock.Advance(DefaultHoldTTL)
	sweeper.Sweep(t.Context())

	waitFor(t, "the swept seat to reach the queue", func() bool {
		seat, err := setup.seats.GetSeat(t.Context(), testEventID, testSeatID)

		return err == nil && seat.HeldBy == waitingUser
	})
}

// A counter that can never move is indistinguishable from a broken one, and a
// load test reporting zero drops proves nothing until this does.
//
// No workers are started, so nothing drains the queue: every offer past the
// buffer has to be counted and thrown away rather than block the caller who
// released the seat.
func TestHandoff_CountsWhatItHadToDrop(t *testing.T) {
	setup := newHandoffSetup(t)

	const offers = DefaultHandoffBuffer + 25

	for i := range offers {
		setup.handoff.Offer(domain.SeatRef{EventID: testEventID, SeatID: fmt.Sprintf("A%d", i)})
	}

	if got, want := setup.handoff.Dropped(), int64(offers-DefaultHandoffBuffer); got != want {
		t.Errorf("Dropped() = %d, want %d", got, want)
	}

	if got := setup.handoff.Waiting(); got != DefaultHandoffBuffer {
		t.Errorf("Waiting() = %d, want the buffer to be full at %d", got, DefaultHandoffBuffer)
	}
}

// A waiter who can never be given a seat must not be put back at the front of
// the queue, or everybody behind them waits for ever. Without this the handoff
// pops the same person on every freed seat, refuses them, and parks them again.
func TestHandoff_DropsAWaiterWhoCanNeverBeServed(t *testing.T) {
	var (
		clock = newFakeClock(testTime())
		users = repository.NewMemoryUserRepository()
		seats = repository.NewMemorySeatRepository(newTestSeat())
		store = repository.NewMemoryEventRepository(testEvent())
		queue = repository.NewMemoryWaitingListRepository()
	)

	// Confirmed, so they can be served; unconfirmed, so they cannot.
	for _, account := range []struct {
		id       string
		email    string
		verified bool
	}{
		{"blocker", "unconfirmed@example.com", false},
		{"next", "confirmed@example.com", true},
	} {
		user := domain.NewUser(account.id, account.email, "$argon2id$fake", testTime())
		if err := users.CreateUser(t.Context(), user); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		if account.verified {
			err := users.MarkEmailVerified(t.Context(), account.id, account.email, testTime())
			if err != nil {
				t.Fatalf("MarkEmailVerified() error = %v", err)
			}
		}
	}

	reservations := NewReservationService(Config{
		Seats:   seats,
		Events:  store,
		Clock:   clock,
		NewID:   NewRandomID,
		HoldTTL: DefaultHoldTTL,
		Gate:    NewVerifiedOnly(users),
	})

	handoff := NewHandoff(HandoffConfig{
		Waiting: queue,
		Holder:  reservations,
		Clock:   clock,
		Logger:  discardLogger(),
		Workers: 1,
	})

	// The blocker is first in the queue, which is the whole problem.
	for i, who := range []string{"blocker", "next"} {
		entry, err := domain.NewWaitingEntry("wait-"+who, testEventID, who, testTime().Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("NewWaitingEntry() error = %v", err)
		}

		if _, _, err := queue.Join(t.Context(), entry); err != nil {
			t.Fatalf("Join() error = %v", err)
		}
	}

	// A seat comes free. The blocker is popped, refused, and must not come back.
	handoff.hand(t.Context(), domain.SeatRef{EventID: testEventID, SeatID: testSeatID})

	if _, err := queue.Position(t.Context(), testEventID, "blocker"); !errors.Is(err, repository.ErrNotWaiting) {
		t.Error("the blocker was put back and will block the queue for ever")
	}

	// And the next person is now reachable.
	position, err := queue.Position(t.Context(), testEventID, "next")
	if err != nil {
		t.Fatalf("Position() error = %v", err)
	}
	if position != 1 {
		t.Errorf("the next waiter is at %d, want 1", position)
	}

	// The seat was not given to anybody, so it is still there to be offered.
	handoff.hand(t.Context(), domain.SeatRef{EventID: testEventID, SeatID: testSeatID})

	seat, err := seats.GetSeat(t.Context(), testEventID, testSeatID)
	if err != nil {
		t.Fatalf("GetSeat() error = %v", err)
	}
	if seat.HeldBy != "next" {
		t.Errorf("the seat is held by %q, want next", seat.HeldBy)
	}
}
