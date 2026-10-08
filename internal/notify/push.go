package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"ticket-reservation/internal/domain"
)

// ErrPushNotConfigured means no VAPID keys were given, so there is nothing to
// sign a push with.
var ErrPushNotConfigured = errors.New("push notifications are not configured")

// pushTimeout bounds one push. A push service that accepts the connection and
// then says nothing must not hold a worker.
const pushTimeout = 10 * time.Second

// Subscriptions is the slice of the store the pusher needs: who to push to, and
// how to forget somebody the push service says is gone.
type Subscriptions interface {
	ListForUser(ctx context.Context, userID string) ([]*domain.PushSubscription, error)
	Unsubscribe(ctx context.Context, endpoint string) error
}

// Pusher sends notices to browsers over Web Push.
//
// The encryption is a library's job rather than this project's: the payload is
// sealed to each browser's own keys with ECDH, HKDF and AES-GCM, and hand
// rolling that is how a message ends up readable by the service carrying it.
type Pusher struct {
	subscriptions Subscriptions
	publicKey     string
	privateKey    string
	subject       string
}

// PusherConfig carries the VAPID identity a push is signed with.
type PusherConfig struct {
	// PublicKey and PrivateKey are the VAPID pair. Empty means push is off.
	PublicKey  string
	PrivateKey string

	// Subject is a mailto: or https: URL saying who is sending, which the push
	// services use to reach somebody when something is wrong.
	Subject string
}

// NewPusher returns a pusher, or nil when no keys are configured. A nil pusher
// is how push stays off by default rather than failing at startup.
func NewPusher(subscriptions Subscriptions, cfg PusherConfig) (*Pusher, error) {
	if cfg.PublicKey == "" && cfg.PrivateKey == "" {
		return nil, nil
	}

	if cfg.PublicKey == "" || cfg.PrivateKey == "" {
		return nil, fmt.Errorf("%w: both VAPID keys are needed, not one", ErrPushNotConfigured)
	}

	if cfg.Subject == "" {
		return nil, fmt.Errorf("%w: a VAPID subject is needed, a mailto: or https: URL", ErrPushNotConfigured)
	}

	return &Pusher{
		subscriptions: subscriptions,
		publicKey:     cfg.PublicKey,
		privateKey:    cfg.PrivateKey,
		subject:       cfg.Subject,
	}, nil
}

// PublicKey is what a browser needs before it can subscribe.
func (p *Pusher) PublicKey() string {
	if p == nil {
		return ""
	}

	return p.publicKey
}

// payload is what arrives in the browser's service worker.
type payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Link  string `json:"link"`
}

// SendPush pushes a notice to everything the person has subscribed and reports
// how many it reached.
//
// One browser refusing does not stop the others: somebody with a dead phone
// subscription should still hear about it on their laptop.
func (p *Pusher) SendPush(ctx context.Context, userID string, notice Notice) (int, error) {
	if p == nil {
		return 0, ErrPushNotConfigured
	}

	subscriptions, err := p.subscriptions.ListForUser(ctx, userID)
	if err != nil {
		return 0, err
	}

	if len(subscriptions) == 0 {
		return 0, nil
	}

	body, err := json.Marshal(payload{Title: notice.Subject, Body: notice.Body, Link: notice.Link})
	if err != nil {
		return 0, fmt.Errorf("encoding a push payload: %w", err)
	}

	reached := 0

	var failures error

	for _, subscription := range subscriptions {
		if err := p.push(ctx, subscription, body); err != nil {
			failures = errors.Join(failures, err)

			continue
		}

		reached++
	}

	// Only a failure if nothing got through. One dead subscription out of two is
	// a delivery that worked.
	if reached == 0 && failures != nil {
		return 0, failures
	}

	return reached, nil
}

func (p *Pusher) push(ctx context.Context, subscription *domain.PushSubscription, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()

	response, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: subscription.P256dh,
			Auth:   subscription.Auth,
		},
	}, &webpush.Options{
		Subscriber:      p.subject,
		VAPIDPublicKey:  p.publicKey,
		VAPIDPrivateKey: p.privateKey,
		TTL:             int(pushTTL.Seconds()),
	})
	if err != nil {
		return fmt.Errorf("pushing to %s: %w", subscription.Endpoint, err)
	}

	defer func() { _ = response.Body.Close() }()

	// The push service telling us an endpoint is gone is the truth. Keeping it
	// would mean failing against it on every notice from now on.
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone {
		if err := p.subscriptions.Unsubscribe(ctx, subscription.Endpoint); err != nil {
			return fmt.Errorf("forgetting a dead subscription: %w", err)
		}

		return fmt.Errorf("the subscription at %s is gone", subscription.Endpoint)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("pushing to %s: the push service answered %d",
			subscription.Endpoint, response.StatusCode)
	}

	return nil
}

// pushTTL is how long a push service holds a notice for a browser that is
// offline. A day: longer than a phone spends in a pocket, shorter than the news
// stays worth hearing.
const pushTTL = 24 * time.Hour
