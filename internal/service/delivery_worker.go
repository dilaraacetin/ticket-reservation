package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/notify"
	"ticket-reservation/internal/repository"
)

const (
	// DefaultDeliveryInterval is how often the outbox is drained. Unhurried,
	// because a notice arriving a few seconds later costs nothing and a tight
	// loop against a mail server costs goodwill.
	DefaultDeliveryInterval = 5 * time.Second

	// DefaultDeliveryBatch is how many are taken at a time, which bounds how
	// long one pass can hold a worker.
	DefaultDeliveryBatch = 20
)

// EmailSender sends a notice to an address. Declared here for the consumer, so
// nothing in the mail package learns about outboxes.
type EmailSender interface {
	SendEmail(ctx context.Context, address string, notice notify.Notice) error
}

// PushSender pushes a notice to whatever a person has subscribed. It reports how
// many subscriptions it reached, so a person with none is not mistaken for a
// failure.
type PushSender interface {
	SendPush(ctx context.Context, userID string, notice notify.Notice) (int, error)
}

// DeliveryWorker drains the outbox.
//
// Sending happens here rather than in the request that caused it, because
// reaching somebody else's mail server inside a request means a slow server
// makes the request slow and an outage makes it fail. The notice is already
// written down by then; getting it out is a separate job that may take several
// tries.
type DeliveryWorker struct {
	deliveries    repository.DeliveryRepository
	notifications repository.NotificationRepository
	users         repository.UserRepository

	mailer EmailSender
	pusher PushSender

	clock    Clock
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

// DeliveryConfig carries the worker's dependencies. Mailer and Pusher are both
// optional: a deployment with neither still queues deliveries, it just has
// nowhere to take them, and that shows up as a pending count rather than as an
// error on every pass.
type DeliveryConfig struct {
	Deliveries    repository.DeliveryRepository
	Notifications repository.NotificationRepository
	Users         repository.UserRepository

	Mailer EmailSender
	Pusher PushSender

	Clock    Clock
	Logger   *slog.Logger
	Interval time.Duration
	Batch    int
}

func NewDeliveryWorker(cfg DeliveryConfig) *DeliveryWorker {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultDeliveryInterval
	}

	if cfg.Batch <= 0 {
		cfg.Batch = DefaultDeliveryBatch
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}

	return &DeliveryWorker{
		deliveries:    cfg.Deliveries,
		notifications: cfg.Notifications,
		users:         cfg.Users,
		mailer:        cfg.Mailer,
		pusher:        cfg.Pusher,
		clock:         cfg.Clock,
		logger:        cfg.Logger,
		interval:      cfg.Interval,
		batch:         cfg.Batch,
	}
}

// Run drains the outbox on every tick until ctx is cancelled. It blocks, so
// callers start it in its own goroutine.
func (w *DeliveryWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.logger.InfoContext(ctx, "delivery worker started",
		"interval", w.interval,
		"email", w.mailer != nil,
		"push", w.pusher != nil,
	)

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "delivery worker stopped")

			return nil
		case <-ticker.C:
			w.Drain(ctx)
		}
	}
}

// Drain works through one batch and reports how many were sent. A failure is
// recorded against the delivery rather than returned, because one address that
// bounces must not stop the rest of the batch.
func (w *DeliveryWorker) Drain(ctx context.Context) int {
	claimed, err := w.deliveries.TakeDue(ctx, w.clock.Now(), w.batch)
	if err != nil {
		w.logger.ErrorContext(ctx, "claiming deliveries failed", "err", err)

		return 0
	}

	sent := 0

	for _, delivery := range claimed {
		if w.deliver(ctx, delivery) {
			sent++
		}
	}

	return sent
}

// deliver makes one attempt and writes down what happened.
func (w *DeliveryWorker) deliver(ctx context.Context, delivery *domain.Delivery) bool {
	err := w.attempt(ctx, delivery)
	now := w.clock.Now()

	switch {
	case errors.Is(err, errAddressNotVerified):
		// Waiting will not change this: an address nobody has verified is not
		// going to verify itself between now and the next attempt. Recorded and
		// dropped rather than retried five times and logged as a failure.
		delivery.GiveUp(err.Error(), now)

		w.logger.DebugContext(ctx, "a notice was not emailed",
			"reason", "the address is not verified",
			"userId", delivery.UserID,
		)
	case err != nil:
		delivery.Failed(err.Error(), now)

		level := slog.LevelWarn
		if !delivery.GaveUpAt.IsZero() {
			// Out of attempts. Somebody was not told, which is worth more than a
			// warning that scrolls past.
			level = slog.LevelError
		}

		w.logger.Log(ctx, level, "a notice could not be delivered",
			"err", err,
			"channel", delivery.Channel.String(),
			"userId", delivery.UserID,
			"attempts", delivery.Attempts,
			"gaveUp", !delivery.GaveUpAt.IsZero(),
		)
	default:
		delivery.Sent(now)
	}

	if err := w.deliveries.Save(ctx, delivery); err != nil {
		// The attempt happened whatever this says. Not recording it means the
		// lease runs out and it is tried again, which for a notice is the better
		// of the two mistakes.
		w.logger.ErrorContext(ctx, "recording a delivery failed", "err", err, "id", delivery.ID)
	}

	return err == nil
}

// attempt does the sending itself.
func (w *DeliveryWorker) attempt(ctx context.Context, delivery *domain.Delivery) error {
	notification, err := w.notifications.Get(ctx, delivery.NotificationID)
	if err != nil {
		return err
	}

	notice := notify.Render(notification)

	switch delivery.Channel {
	case domain.ChannelEmail:
		if w.mailer == nil {
			return notify.ErrMailNotConfigured
		}

		user, err := w.users.GetUserByID(ctx, delivery.UserID)
		if err != nil {
			return err
		}

		// Nobody has shown this address belongs to them. Sending to it would
		// make this service a way to post mail to strangers: anybody can
		// register with somebody else's address.
		if !user.EmailVerified() {
			return errAddressNotVerified
		}

		// Looked up now rather than copied into the outbox when it was queued:
		// somebody who has changed their address should be reached at the new
		// one, and one copy of an address is better than two.
		return w.mailer.SendEmail(ctx, user.Email, notice)

	case domain.ChannelPush:
		if w.pusher == nil {
			return errPushNotConfigured
		}

		reached, err := w.pusher.SendPush(ctx, delivery.UserID, notice)
		if err != nil {
			return err
		}

		if reached == 0 {
			// Nothing to push to. Counted as done rather than retried for ever:
			// somebody who has never allowed notifications is not a failure.
			w.logger.DebugContext(ctx, "nothing subscribed to push to", "userId", delivery.UserID)
		}

		return nil
	}

	return errors.New("no sender for channel " + delivery.Channel.String())
}

var (
	errPushNotConfigured = errors.New("push notifications are not configured")

	errAddressNotVerified = errors.New("the address has not been verified")
)
