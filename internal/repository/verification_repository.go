package repository

import (
	"context"
	"time"

	"ticket-reservation/internal/domain"
)

// VerificationRepository stores outstanding "is this really your address" links,
// and doubles as their sending queue.
type VerificationRepository interface {
	// Create stores a verification waiting to be sent.
	Create(ctx context.Context, verification *domain.EmailVerification) error

	// ClaimUnsent takes up to limit verifications that have not gone out, and
	// marks them sent in the same step so two senders cannot both email the same
	// link. A send that then fails is given back with Release.
	ClaimUnsent(ctx context.Context, now time.Time, limit int) ([]*domain.EmailVerification, error)

	// Release puts a claimed verification back because sending it did not work.
	Release(ctx context.Context, token string) error

	// Consume spends a link by deleting it, and returns what it was, or reports
	// ErrVerificationNotUsable. Deleting is what makes it single use, and has to
	// be one step: two people following the same link must not both succeed.
	Consume(ctx context.Context, token string, now time.Time) (*domain.EmailVerification, error)

	// DeleteExpired drops links that can no longer be followed.
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}
