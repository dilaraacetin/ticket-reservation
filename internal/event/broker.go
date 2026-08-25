package event

import (
	"log/slog"
	"sync"
)

const subscriberBuffer = 16

// Broker fans notices out to whoever is watching.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[*Subscriber]struct{}
	logger      *slog.Logger
	dropped     int64
}

// Subscriber is one watcher.
type Subscriber struct {
	Events  chan Event
	eventID string
	userID  string
}

// NewBroker returns a broker with nobody watching.
func NewBroker(logger *slog.Logger) *Broker {
	return &Broker{
		subscribers: make(map[*Subscriber]struct{}),
		logger:      logger,
	}
}

// Subscribe registers a watcher of one event, on behalf of one user.
func (b *Broker) Subscribe(eventID, userID string) *Subscriber {
	subscriber := &Subscriber{
		Events:  make(chan Event, subscriberBuffer),
		eventID: eventID,
		userID:  userID,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers[subscriber] = struct{}{}

	return subscriber
}

// Unsubscribe removes a watcher and closes its channel.
func (b *Broker) Unsubscribe(subscriber *Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, watching := b.subscribers[subscriber]; !watching {
		return
	}

	delete(b.subscribers, subscriber)
	close(subscriber.Events)
}

// Publish delivers a notice to everyone it concerns and reports how many got it.
func (b *Broker) Publish(event Event) int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	delivered := 0

	for subscriber := range b.subscribers {
		if !subscriber.wants(event) {
			continue
		}

		select {
		case subscriber.Events <- event:
			delivered++
		default:
			b.dropped++

			b.logger.Warn("dropped a notice for a subscriber that is not keeping up",
				"kind", event.Kind,
				"eventId", event.EventID,
			)
		}
	}

	return delivered
}

// wants reports whether a notice concerns this watcher.
func (s *Subscriber) wants(event Event) bool {
	if s.eventID != event.EventID {
		return false
	}

	return event.IsForEveryone() || event.UserID == s.userID
}

// Subscribers reports how many watchers there are, for tests and for a metric
// later.
func (b *Broker) Subscribers() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.subscribers)
}

// Dropped reports how many notices were thrown away for slow subscribers. A
// number that climbs is the signal that the buffer or the reader is wrong.
func (b *Broker) Dropped() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.dropped
}

// Close removes every watcher, so that a shutdown does not leave readers blocked
// on a channel that will never receive again.
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for subscriber := range b.subscribers {
		delete(b.subscribers, subscriber)
		close(subscriber.Events)
	}
}

// Publisher is the slice of the broker the service layer needs. Declared for the
// consumer, so the service never learns about subscriptions or transports.
type Publisher interface {
	Publish(event Event) int
}

var _ Publisher = (*Broker)(nil)

// Discard is a publisher that throws everything away, for the paths that have no
// broker wired in.
type Discard struct{}

// Publish accepts a notice and forgets it.
func (Discard) Publish(Event) int { return 0 }
