package repository

import (
	"context"
	"time"
)

// TokenRepository remembers which tokens have been signed out.
//
// A denylist rather than a list of valid sessions: tokens are self-contained and
// verified by their signature, so the only thing storage has to know is the
// exception. It stays small because a record is only useful until the token it
// names would have expired anyway.
type TokenRepository interface {
	// Revoke records a token as refused from now until expiresAt. Revoking the
	// same token twice is not an error, because the caller wanted it gone either
	// way.
	Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error

	// IsRevoked reports whether a token has been signed out. This is on the path
	// of every authenticated request, so it is a single lookup by key.
	IsRevoked(ctx context.Context, tokenID string) (bool, error)

	// DeleteExpired drops records that no longer refuse anything, and reports how
	// many went.
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}
