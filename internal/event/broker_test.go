package event

import (
	"log/slog"
	"sync"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func seatChanged(eventID, seatID string) Event {
	return Event{Kind: SeatChanged, EventID: eventID, SeatID: seatID, At: time.Now()}
}

// receive waits briefly for one notice, so a test fails with a message rather
// than hanging when nothing arrives.
func receive(t *testing.T, subscriber *Subscriber) (Event, bool) {
	t.Helper()

	select {
	case event, open := <-subscriber.Events:
		return event, open
	case <-time.After(time.Second):
		t.Fatal("no notice arrived within a second")

		return Event{}, false
	}
}

func TestBroker_DeliversToEveryWatcherOfAnEvent(t *testing.T) {
	broker := NewBroker(discardLogger())

	first := broker.Subscribe("event-1", "dilara")
	second := broker.Subscribe("event-1", "mehmet")

	if got := broker.Publish(seatChanged("event-1", "A1")); got != 2 {
		t.Errorf("delivered to %d watchers, want 2", got)
	}

	for _, subscriber := range []*Subscriber{first, second} {
		event, _ := receive(t, subscriber)
		if event.SeatID != "A1" {
			t.Errorf("seat = %q, want A1", event.SeatID)
		}
	}
}

// A watcher of one event must not hear about another.
func TestBroker_KeepsEventsApart(t *testing.T) {
	broker := NewBroker(discardLogger())

	watcher := broker.Subscribe("event-1", "dilara")

	if got := broker.Publish(seatChanged("event-2", "A1")); got != 0 {
		t.Errorf("delivered to %d watchers, want 0", got)
	}

	select {
	case event := <-watcher.Events:
		t.Errorf("a notice for another event arrived: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

// A notice addressed at one person goes only to them, which is what makes the
// waiting list's "your turn" private.
func TestBroker_DeliversAddressedNoticesToOnePerson(t *testing.T) {
	broker := NewBroker(discardLogger())

	mine := broker.Subscribe("event-1", "dilara")
	theirs := broker.Subscribe("event-1", "mehmet")

	turn := Event{Kind: TurnCame, EventID: "event-1", SeatID: "A1", UserID: "dilara", At: time.Now()}

	if got := broker.Publish(turn); got != 1 {
		t.Errorf("delivered to %d watchers, want 1", got)
	}

	if event, _ := receive(t, mine); event.Kind != TurnCame {
		t.Errorf("kind = %q, want %q", event.Kind, TurnCame)
	}

	select {
	case event := <-theirs.Events:
		t.Errorf("somebody else's notice arrived: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

// The channel is closed by the broker, so a reader ranging over it stops rather
// than blocking for ever.
func TestBroker_UnsubscribeClosesTheChannel(t *testing.T) {
	broker := NewBroker(discardLogger())

	subscriber := broker.Subscribe("event-1", "dilara")
	broker.Unsubscribe(subscriber)

	if _, open := <-subscriber.Events; open {
		t.Error("the channel is still open after unsubscribing")
	}

	if got := broker.Subscribers(); got != 0 {
		t.Errorf("subscribers = %d, want 0", got)
	}

	// Unsubscribing twice must not close a closed channel, which would panic.
	broker.Unsubscribe(subscriber)
}

// The point of the non blocking send: one browser that stops reading must not
// hold up the request that published the notice.
func TestBroker_DropsForSubscribersThatFallBehind(t *testing.T) {
	broker := NewBroker(discardLogger())

	slow := broker.Subscribe("event-1", "dilara")

	// Fill the buffer without reading any of it.
	for range subscriberBuffer {
		broker.Publish(seatChanged("event-1", "A1"))
	}

	done := make(chan int, 1)
	go func() {
		done <- broker.Publish(seatChanged("event-1", "A2"))
	}()

	select {
	case delivered := <-done:
		if delivered != 0 {
			t.Errorf("delivered = %d, want 0 once the buffer is full", delivered)
		}
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a subscriber that stopped reading")
	}

	if broker.Dropped() == 0 {
		t.Error("nothing was counted as dropped")
	}

	// The watcher is still subscribed and still has its earlier notices.
	if got := len(slow.Events); got != subscriberBuffer {
		t.Errorf("buffered = %d, want %d", got, subscriberBuffer)
	}
}

func TestBroker_CloseReleasesEveryWatcher(t *testing.T) {
	broker := NewBroker(discardLogger())

	subscribers := []*Subscriber{
		broker.Subscribe("event-1", "dilara"),
		broker.Subscribe("event-1", "mehmet"),
		broker.Subscribe("event-2", "ayse"),
	}

	broker.Close()

	for i, subscriber := range subscribers {
		if _, open := <-subscriber.Events; open {
			t.Errorf("subscriber %d was left with an open channel", i)
		}
	}

	if got := broker.Subscribers(); got != 0 {
		t.Errorf("subscribers = %d, want 0", got)
	}
}

// Subscribing, publishing and unsubscribing all happen on request goroutines at
// once, so the bookkeeping has to hold up under that.
func TestBroker_IsSafeUnderConcurrentUse(t *testing.T) {
	const watchers = 40

	broker := NewBroker(discardLogger())

	var wg sync.WaitGroup

	for i := range watchers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			subscriber := broker.Subscribe("event-1", "user")

			// Drain, so this watcher is never the slow one.
			go func() {
				for range subscriber.Events { //nolint:revive // draining on purpose
				}
			}()

			broker.Publish(seatChanged("event-1", "A1"))

			if i%2 == 0 {
				broker.Unsubscribe(subscriber)
			}
		}()
	}

	wg.Wait()
	broker.Close()

	if got := broker.Subscribers(); got != 0 {
		t.Errorf("subscribers = %d, want 0 after closing", got)
	}
}
