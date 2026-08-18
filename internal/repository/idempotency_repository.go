package repository

import (
	"context"
	"time"
)

// Idempotency states. A record is claimed before the request runs and completed
// once there is an answer worth replaying.
const (
	IdempotencyInProgress = "in_progress"
	IdempotencyCompleted  = "completed"
)

type IdempotencyRecord struct {
	UserID      string
	Key         string
	Fingerprint string
	State       string
	StatusCode  int
	Response    []byte
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

func (r IdempotencyRecord) IsCompleted() bool {
	return r.State == IdempotencyCompleted
}

// IdempotencyRepository remembers what a request answered, so that a retry can be
// told the same thing rather than doing the work again.
type IdempotencyRepository interface {
	Claim(ctx context.Context, record IdempotencyRecord) (IdempotencyRecord, bool, error)

	Complete(ctx context.Context, userID, key string, statusCode int, response []byte) error

	Release(ctx context.Context, userID, key string) error
}
