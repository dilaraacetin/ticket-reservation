package service

import (
	"errors"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/event"
	"ticket-reservation/internal/repository"
)

func newInventorySetup(t *testing.T) (*InventoryService, *ReservationService, *repository.MemoryEventRepository) {
	t.Helper()

	var (
		clock  = newFakeClock(testTime())
		seats  = repository.NewMemorySeatRepository(newTestSeat())
		events = repository.NewMemoryEventRepository(testEvent()).WithSeats(seats)
	)

	inventory := NewInventoryService(InventoryConfig{
		Events: events,
		Seats:  seats,
		Clock:  clock,
		NewID:  NewRandomID,
	})

	reservations := NewReservationService(Config{
		Seats:   seats,
		Events:  events,
		Clock:   clock,
		NewID:   NewRandomID,
		HoldTTL: DefaultHoldTTL,
	})

	return inventory, reservations, events
}

// The point of withdrawing an event: it stops selling.
// keepDetails is the edit that changes none of the descriptive fields.
func keepDetails(current domain.EventDetails) domain.EventDetails { return current }

func TestInventory_CancelledEventStopsSelling(t *testing.T) {
	inventory, reservations, _ := newInventorySetup(t)

	if _, err := inventory.CancelEvent(t.Context(), testEventID); err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}

	_, err := reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser)
	if !errors.Is(err, domain.ErrEventCancelled) {
		t.Errorf("HoldSeat() on a cancelled event = %v, want %v", err, domain.ErrEventCancelled)
	}
}

// Deliberately not symmetrical with holding. Somebody already holding a seat was
// part way through a purchase when the event was pulled, and letting them finish
// is the kinder answer.
func TestInventory_AHoldMadeBeforeCancellationCanStillBeConfirmed(t *testing.T) {
	inventory, reservations, _ := newInventorySetup(t)

	hold, err := reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser)
	if err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	if _, err := inventory.CancelEvent(t.Context(), testEventID); err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}

	if _, err := reservations.ConfirmReservation(t.Context(), hold.ID, testUser); err != nil {
		t.Errorf("ConfirmReservation() after cancellation = %v, want it to go through", err)
	}
}

// The event stays in the catalogue, because somebody holding a ticket to it has
// to be able to see what happened.
func TestInventory_ACancelledEventIsStillThere(t *testing.T) {
	inventory, reservations, _ := newInventorySetup(t)

	cancelled, err := inventory.CancelEvent(t.Context(), testEventID)
	if err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}
	if !cancelled.IsCancelled() {
		t.Error("the returned event does not report itself as cancelled")
	}

	events, err := reservations.Events(t.Context())
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 1 || !events[0].IsCancelled() {
		t.Errorf("catalogue = %v, want the cancelled event still listed", events)
	}
}

func TestInventory_CancellingTwiceIsRefused(t *testing.T) {
	inventory, _, _ := newInventorySetup(t)

	if _, err := inventory.CancelEvent(t.Context(), testEventID); err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}

	_, err := inventory.CancelEvent(t.Context(), testEventID)
	if !errors.Is(err, domain.ErrEventAlreadyCancelled) {
		t.Errorf("second CancelEvent() = %v, want %v", err, domain.ErrEventAlreadyCancelled)
	}
}

// A field left out is left alone, so fixing one does not send the others back
// stale.
func TestInventory_UpdateChangesOnlyWhatWasGiven(t *testing.T) {
	inventory, _, _ := newInventorySetup(t)

	before := testEvent()

	updated, err := inventory.UpdateEvent(t.Context(), testEventID, "", "Cemal Resit Rey", time.Time{}, keepDetails)
	if err != nil {
		t.Fatalf("UpdateEvent() error = %v", err)
	}

	if updated.Venue != "Cemal Resit Rey" {
		t.Errorf("Venue = %q, want it changed", updated.Venue)
	}
	if updated.Name != before.Name {
		t.Errorf("Name = %q, want it left as %q", updated.Name, before.Name)
	}
	if !updated.StartsAt.Equal(before.StartsAt) {
		t.Errorf("StartsAt = %s, want it left as %s", updated.StartsAt, before.StartsAt)
	}
}

// A cancelled event is a record of something that is not happening. Editing it
// would quietly turn it into a different one.
func TestInventory_ACancelledEventIsNotEditable(t *testing.T) {
	inventory, _, _ := newInventorySetup(t)

	if _, err := inventory.CancelEvent(t.Context(), testEventID); err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}

	_, err := inventory.UpdateEvent(t.Context(), testEventID, "Something Else", "", time.Time{}, keepDetails)
	if !errors.Is(err, domain.ErrEventAlreadyCancelled) {
		t.Errorf("UpdateEvent() on a cancelled event = %v, want %v", err, domain.ErrEventAlreadyCancelled)
	}
}

// Removing a mistake is one thing; removing something people have tickets to is
// another, and cancelling is what that is for.
func TestInventory_DeleteRefusesWhileASeatIsSpokenFor(t *testing.T) {
	inventory, reservations, _ := newInventorySetup(t)

	if _, err := reservations.HoldSeat(t.Context(), testEventID, testSeatID, testUser); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	if err := inventory.DeleteEvent(t.Context(), testEventID); !errors.Is(err, repository.ErrEventInUse) {
		t.Errorf("DeleteEvent() with a held seat = %v, want %v", err, repository.ErrEventInUse)
	}

	// Once the seat is given back, the mistake can go.
	holds, err := reservations.SeatMap(t.Context(), testEventID)
	if err != nil {
		t.Fatalf("SeatMap() error = %v", err)
	}

	if err := reservations.ReleaseSeat(t.Context(), holds[0].HoldID, testUser); err != nil {
		t.Fatalf("ReleaseSeat() error = %v", err)
	}

	if err := inventory.DeleteEvent(t.Context(), testEventID); err != nil {
		t.Errorf("DeleteEvent() with nothing held = %v, want it to go through", err)
	}
}

// newNotifyingSetup wires the parts a cancellation has to reach: the people
// holding seats, the people queued for one, and the live stream.
func newNotifyingSetup(t *testing.T) (
	*InventoryService,
	*ReservationService,
	*repository.MemoryNotificationRepository,
	*repository.MemoryWaitingListRepository,
	*recordingPublisher,
) {
	t.Helper()

	var (
		clock     = newFakeClock(testTime())
		publisher = &recordingPublisher{}
		seats     = repository.NewMemorySeatRepository(
			domain.NewSeat(testEventID, "A1", "A", 1),
			domain.NewSeat(testEventID, "A2", "A", 2),
			domain.NewSeat(testEventID, "A3", "A", 3),
		)
		events        = repository.NewMemoryEventRepository(testEvent()).WithSeats(seats)
		waiting       = repository.NewMemoryWaitingListRepository()
		notifications = repository.NewMemoryNotificationRepository()
	)

	inventory := NewInventoryService(InventoryConfig{
		Events:        events,
		Seats:         seats,
		Waiting:       waiting,
		Notifications: notifications,
		Publisher:     publisher,
		Clock:         clock,
		NewID:         NewRandomID,
		Logger:        discardLogger(),
	})

	reservations := NewReservationService(Config{
		Seats:   seats,
		Events:  events,
		Clock:   clock,
		NewID:   NewRandomID,
		HoldTTL: DefaultHoldTTL,
	})

	return inventory, reservations, notifications, waiting, publisher
}

// Everybody with a stake in the event is told: whoever holds a seat, whoever
// owns one, and whoever is still queued for one.
func TestInventory_CancellingTellsEveryoneAffected(t *testing.T) {
	inventory, reservations, notifications, waiting, publisher := newNotifyingSetup(t)

	// A1 held by one person, A2 confirmed by another, A3 untouched.
	if _, err := reservations.HoldSeat(t.Context(), testEventID, "A1", "holder"); err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}

	owner, err := reservations.HoldSeat(t.Context(), testEventID, "A2", "owner")
	if err != nil {
		t.Fatalf("HoldSeat() error = %v", err)
	}
	if _, err := reservations.ConfirmReservation(t.Context(), owner.ID, "owner"); err != nil {
		t.Fatalf("ConfirmReservation() error = %v", err)
	}

	entry, err := domain.NewWaitingEntry("wait-1", testEventID, "waiter", testTime())
	if err != nil {
		t.Fatalf("NewWaitingEntry() error = %v", err)
	}
	if _, _, err := waiting.Join(t.Context(), entry); err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	if _, err := inventory.CancelEvent(t.Context(), testEventID); err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}

	for _, who := range []string{"holder", "owner", "waiter"} {
		notices, err := notifications.ListForUser(t.Context(), who)
		if err != nil {
			t.Fatalf("ListForUser(%s) error = %v", who, err)
		}
		if len(notices) != 1 {
			t.Errorf("%s has %d notices, want 1", who, len(notices))

			continue
		}

		// The event's name is carried in, because the event may be gone by the
		// time anyone reads this.
		if notices[0].EventName != testEvent().Name {
			t.Errorf("%s was told about %q", who, notices[0].EventName)
		}
		if notices[0].Kind != domain.NotifyEventCancelled {
			t.Errorf("%s got kind %q", who, notices[0].Kind)
		}
	}

	// Nobody held A3, so nobody is told about it.
	if notices, _ := notifications.ListForUser(t.Context(), "stranger"); len(notices) != 0 {
		t.Errorf("somebody with no stake was told: %v", notices)
	}

	// And anyone with the page open hears it at once.
	var announced bool
	for _, published := range publisher.seen() {
		if published.Kind == event.EventCancelled && published.EventID == testEventID {
			announced = true
		}
	}

	if !announced {
		t.Error("nothing went down the live stream, so an open page would not know")
	}
}

// A queue for something that is not happening keeps people waiting for a turn
// that cannot come.
func TestInventory_CancellingEmptiesTheQueue(t *testing.T) {
	inventory, _, _, waiting, _ := newNotifyingSetup(t)

	entry, err := domain.NewWaitingEntry("wait-1", testEventID, "waiter", testTime())
	if err != nil {
		t.Fatalf("NewWaitingEntry() error = %v", err)
	}
	if _, _, err := waiting.Join(t.Context(), entry); err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	if _, err := inventory.CancelEvent(t.Context(), testEventID); err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}

	if _, err := waiting.Position(t.Context(), testEventID, "waiter"); !errors.Is(err, repository.ErrNotWaiting) {
		t.Errorf("Position() after cancellation = %v, want %v", err, repository.ErrNotWaiting)
	}
}

// The event is already withdrawn by the time the telling happens. Reporting a
// failure would say the cancellation did not work when it did.
func TestInventory_CancellingSucceedsEvenIfNobodyCanBeTold(t *testing.T) {
	clock := newFakeClock(testTime())
	seats := repository.NewMemorySeatRepository(newTestSeat())

	inventory := NewInventoryService(InventoryConfig{
		Events: repository.NewMemoryEventRepository(testEvent()).WithSeats(seats),
		Seats:  seats,
		// No notification store at all, which is the in-memory deployment that
		// has nowhere to keep them.
		Clock:  clock,
		NewID:  NewRandomID,
		Logger: discardLogger(),
	})

	cancelled, err := inventory.CancelEvent(t.Context(), testEventID)
	if err != nil {
		t.Fatalf("CancelEvent() error = %v", err)
	}
	if !cancelled.IsCancelled() {
		t.Error("the event was not withdrawn")
	}
}
