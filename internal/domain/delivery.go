package domain

import (
	"fmt"
	"time"
)

// DeliveryChannel is a way of reaching somebody outside the application.
type DeliveryChannel string

const (
	ChannelEmail DeliveryChannel = "email"
	ChannelPush  DeliveryChannel = "push"
)

// ParseDeliveryChannel turns a stored value back into a channel and refuses
// anything else, so a row nobody has written a sender for fails here rather
// than being picked up and silently dropped.
func ParseDeliveryChannel(value string) (DeliveryChannel, error) {
	switch channel := DeliveryChannel(value); channel {
	case ChannelEmail, ChannelPush:
		return channel, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownDeliveryChannel, value)
	}
}

func (c DeliveryChannel) String() string {
	return string(c)
}

// MaxDeliveryAttempts is how many times a delivery is tried before it is left
// alone. An address that has refused five times is not going to accept the
// sixth, and a queue that retries for ever is a queue that never drains.
const MaxDeliveryAttempts = 5

// Delivery is one notice owed to one person down one channel.
//
// Separate from the Notification itself because the two fail differently: the
// notice is written once and is then simply true, while sending it reaches
// somebody else's server and may have to be tried again tomorrow.
type Delivery struct {
	ID             string
	NotificationID string
	UserID         string
	Channel        DeliveryChannel

	Attempts  int
	LastError string

	CreatedAt time.Time

	// DueAt is when this may next be tried. Backing off rather than hammering a
	// server that has already said no.
	DueAt time.Time

	// SentAt is zero until it has gone.
	SentAt time.Time

	// GaveUpAt is zero unless the attempts ran out.
	GaveUpAt time.Time
}

// NewDelivery returns a delivery owed from now.
func NewDelivery(id, notificationID, userID string, channel DeliveryChannel, now time.Time) (*Delivery, error) {
	if id == "" || notificationID == "" {
		return nil, ErrEmptyNotificationID
	}
	if userID == "" {
		return nil, ErrEmptyUserID
	}

	if _, err := ParseDeliveryChannel(channel.String()); err != nil {
		return nil, err
	}

	return &Delivery{
		ID:             id,
		NotificationID: notificationID,
		UserID:         userID,
		Channel:        channel,
		CreatedAt:      now,
		DueAt:          now,
	}, nil
}

// IsDone reports whether anything more will be tried.
func (d *Delivery) IsDone() bool {
	return !d.SentAt.IsZero() || !d.GaveUpAt.IsZero()
}

// Failed records an attempt that did not work and says when to try again, or
// gives up once the attempts have run out.
//
// The wait doubles each time, which is the whole point: a server that is busy
// now is not helped by being asked again immediately, and one that is broken is
// not worth asking thirty times.
func (d *Delivery) Failed(reason string, now time.Time) {
	d.Attempts++
	d.LastError = reason

	if d.Attempts >= MaxDeliveryAttempts {
		d.GaveUpAt = now

		return
	}

	d.DueAt = now.Add(RetryBackoff(d.Attempts))
}

// GiveUp stops without trying again, for a reason that will not change by
// waiting. An address nobody has verified is not going to verify itself between
// now and the next attempt.
func (d *Delivery) GiveUp(reason string, now time.Time) {
	d.Attempts++
	d.LastError = reason
	d.GaveUpAt = now
}

// Sent records that it went.
func (d *Delivery) Sent(now time.Time) {
	d.SentAt = now
	d.LastError = ""
}

// BaseRetryDelay is how long the first retry waits.
const BaseRetryDelay = time.Minute

// RetryBackoff is how long to wait before attempt number n+1.
func RetryBackoff(attempts int) time.Duration {
	delay := BaseRetryDelay
	for range attempts - 1 {
		delay *= 2
	}

	return delay
}
