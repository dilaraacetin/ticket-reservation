package service

import (
	"context"
	"log/slog"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/event"
	"ticket-reservation/internal/repository"
)

// InventoryService stocks the service with events and their seats. Separate from
// ReservationService because creating inventory and selling it are different
// jobs with different callers: one is an administrator at a desk, the other is
// everybody at once.
type InventoryService struct {
	events        repository.EventRepository
	seats         repository.SeatRepository
	waiting       repository.WaitingListRepository
	notifications repository.NotificationRepository
	deliveries    repository.DeliveryRepository
	channels      []domain.DeliveryChannel
	publisher     event.Publisher
	clock         Clock
	newID         func() string
	logger        *slog.Logger
}

// InventoryConfig carries the service's dependencies.
type InventoryConfig struct {
	Events repository.EventRepository
	Seats  repository.SeatRepository

	// Waiting and Notifications are what makes withdrawing an event tell the
	// people it affects. Optional: without them the event is still withdrawn, it
	// is just withdrawn quietly.
	Waiting       repository.WaitingListRepository
	Notifications repository.NotificationRepository

	// Deliveries and Channels are how a notice leaves the application. Which
	// channels exist is a deployment's business, not this service's, so it is
	// told rather than working it out.
	Deliveries repository.DeliveryRepository
	Channels   []domain.DeliveryChannel

	Publisher event.Publisher
	Clock     Clock
	NewID     func() string
	Logger    *slog.Logger
}

func NewInventoryService(cfg InventoryConfig) *InventoryService {
	if cfg.Publisher == nil {
		cfg.Publisher = event.Discard{}
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}

	return &InventoryService{
		events:        cfg.Events,
		seats:         cfg.Seats,
		waiting:       cfg.Waiting,
		notifications: cfg.Notifications,
		deliveries:    cfg.Deliveries,
		channels:      cfg.Channels,
		publisher:     cfg.Publisher,
		clock:         cfg.Clock,
		newID:         cfg.NewID,
		logger:        cfg.Logger,
	}
}

// CreateEvent stores a new event and returns it with the id it was given. The id
// is generated rather than taken from the caller, so two people stocking the
// service at once cannot collide over a name they both thought was free.
func (s *InventoryService) CreateEvent(
	ctx context.Context,
	name, venue string,
	startsAt time.Time,
	details domain.EventDetails,
) (*domain.Event, error) {
	event, err := domain.NewEvent(s.newID(), name, venue, startsAt, details)
	if err != nil {
		return nil, err
	}

	if err := s.events.CreateEvent(ctx, event); err != nil {
		return nil, err
	}

	return event, nil
}

// UpdateEvent changes the parts of an event that are safe to change. An empty
// name, venue or start time means "leave this alone".
//
// details is a function rather than a value because the caller decides what to
// keep from what is stored, and what is stored can only be read inside the
// locked read-decide-write that UpdateEvent already is.
func (s *InventoryService) UpdateEvent(
	ctx context.Context,
	eventID, name, venue string,
	startsAt time.Time,
	details func(domain.EventDetails) domain.EventDetails,
) (*domain.Event, error) {
	var updated *domain.Event

	err := s.events.UpdateEvent(ctx, eventID, func(event *domain.Event) error {
		if err := event.Update(name, venue, startsAt, details(event.Details)); err != nil {
			return err
		}

		result := *event
		updated = &result

		return nil
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// CancelEvent withdraws an event from sale without removing it, so that the
// people holding tickets to it can still see what happened.
func (s *InventoryService) CancelEvent(ctx context.Context, eventID string) (*domain.Event, error) {
	var cancelled *domain.Event

	now := s.clock.Now()

	err := s.events.UpdateEvent(ctx, eventID, func(event *domain.Event) error {
		if err := event.Cancel(now); err != nil {
			return err
		}

		result := *event
		cancelled = &result

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.announceCancellation(ctx, cancelled)

	return cancelled, nil
}

// announceCancellation tells the people a withdrawal affects.
//
// Failures are logged rather than returned. The event is already withdrawn by
// the time this runs, and reporting an error would tell the administrator their
// cancellation did not happen when it did. A notice that could not be written is
// a problem for whoever reads the logs, not a reason to pretend the event is
// still on sale.
func (s *InventoryService) announceCancellation(ctx context.Context, cancelled *domain.Event) {
	// Public, for anyone who has the event open right now. The stored notices
	// below are for everybody else.
	s.publisher.Publish(event.Event{
		Kind:    event.EventCancelled,
		EventID: cancelled.ID,
		At:      s.clock.Now(),
	})

	if s.notifications == nil {
		return
	}

	notices, err := s.cancellationNotices(ctx, cancelled)
	if err != nil {
		s.logger.ErrorContext(ctx, "could not work out who to tell about a cancellation",
			"err", err,
			"eventId", cancelled.ID,
		)

		return
	}

	if len(notices) == 0 {
		return
	}

	if err := s.notifications.Notify(ctx, notices...); err != nil {
		s.logger.ErrorContext(ctx, "a cancellation was not announced",
			"err", err,
			"eventId", cancelled.ID,
			"people", len(notices),
		)

		return
	}

	s.logger.InfoContext(ctx, "announced a cancellation",
		"eventId", cancelled.ID,
		"people", len(notices),
	)

	s.queueDeliveries(ctx, notices)
}

// queueDeliveries puts each notice in the outbox for every channel this
// deployment has.
//
// Queued rather than sent here: reaching somebody else's mail server inside the
// request that cancelled the event would make the cancellation as slow and as
// unreliable as that server. By this point the notices are written down, and
// getting them out is a separate job that may take several tries.
func (s *InventoryService) queueDeliveries(ctx context.Context, notices []*domain.Notification) {
	if s.deliveries == nil || len(s.channels) == 0 {
		return
	}

	deliveries := make([]*domain.Delivery, 0, len(notices)*len(s.channels))
	now := s.clock.Now()

	for _, notice := range notices {
		for _, channel := range s.channels {
			delivery, err := domain.NewDelivery(s.newID(), notice.ID, notice.UserID, channel, now)
			if err != nil {
				s.logger.ErrorContext(ctx, "could not queue a delivery", "err", err, "channel", channel)

				continue
			}

			deliveries = append(deliveries, delivery)
		}
	}

	if err := s.deliveries.Enqueue(ctx, deliveries...); err != nil {
		// The notices are stored, so people will still find them in the
		// application. What failed is reaching them outside it.
		s.logger.ErrorContext(ctx, "notices were stored but not queued for sending",
			"err", err,
			"deliveries", len(deliveries),
		)
	}
}

// cancellationNotices works out who has a stake in the event: everyone holding
// or owning a seat, and everyone still in the queue for one.
func (s *InventoryService) cancellationNotices(
	ctx context.Context,
	cancelled *domain.Event,
) ([]*domain.Notification, error) {
	now := s.clock.Now()
	notices := make([]*domain.Notification, 0)

	seats, err := s.seats.ListSeats(ctx, cancelled.ID)
	if err != nil {
		return nil, err
	}

	for _, seat := range seats {
		// Whoever has a claim on the seat, if anyone does. A seat is held by
		// somebody or reserved by somebody, never both.
		holder := seat.HeldBy
		if holder == "" {
			holder = seat.ReservedBy
		}

		if holder == "" {
			continue
		}

		notice, err := domain.NewNotification(
			s.newID(), holder, domain.NotifyEventCancelled,
			cancelled.ID, cancelled.Name, seat.ID, now)
		if err != nil {
			return nil, err
		}

		notices = append(notices, notice)
	}

	if s.waiting == nil {
		return notices, nil
	}

	// The queue goes with the event. Leaving it would keep people waiting for a
	// turn that cannot come.
	waiting, err := s.waiting.Drain(ctx, cancelled.ID)
	if err != nil {
		return nil, err
	}

	for _, entry := range waiting {
		notice, err := domain.NewNotification(
			s.newID(), entry.UserID, domain.NotifyEventCancelled,
			cancelled.ID, cancelled.Name, "", now)
		if err != nil {
			return nil, err
		}

		notices = append(notices, notice)
	}

	return notices, nil
}

// DeleteEvent removes an event outright, and only while nobody holds or owns one
// of its seats. Withdrawing is what the other case is for.
func (s *InventoryService) DeleteEvent(ctx context.Context, eventID string) error {
	return s.events.DeleteEvent(ctx, eventID)
}

// AddSeats adds a block of rows to an event and reports how many seats it built.
//
// The event is looked up first so that a mistyped id is reported as missing. The
// database's foreign key would catch it too, but the in-memory store has no such
// thing, and the two have to answer the same way.
func (s *InventoryService) AddSeats(
	ctx context.Context,
	eventID string,
	rows []string,
	perRow int,
) (int, error) {
	if _, err := s.events.GetEvent(ctx, eventID); err != nil {
		return 0, err
	}

	seats, err := domain.NewSeatBlock(eventID, rows, perRow)
	if err != nil {
		return 0, err
	}

	// What was asked for and what was new are different numbers: seats that are
	// already there are left alone, and reporting them as created would tell an
	// administrator a row block had been added when nothing happened.
	return s.seats.CreateSeats(ctx, seats...)
}
