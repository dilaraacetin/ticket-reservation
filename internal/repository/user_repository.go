package repository

import (
	"context"
	"time"

	"ticket-reservation/internal/domain"
)

// UserRepository stores accounts.
type UserRepository interface {
	CreateUser(ctx context.Context, user *domain.User) error

	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)

	// GetUserByID resolves the id carried by a token. The authorization
	// middleware reads the role from here on every request that needs one rather
	// than from the token, so that taking an administrator's role away takes
	// effect at once instead of whenever their token happens to expire.
	GetUserByID(ctx context.Context, userID string) (*domain.User, error)

	// SetRole changes what an account may do.
	SetRole(ctx context.Context, email string, role domain.Role) error

	// MarkEmailVerified records that an address has been shown to belong to the
	// account, and only while the account still has that address: a link sent to
	// an old address must not verify a new one. A mismatch is reported as
	// ErrUserNotFound, because the account and address together are what was
	// being looked for.
	MarkEmailVerified(ctx context.Context, userID, email string, now time.Time) error

	UpdatePasswordHash(ctx context.Context, userID, hash string) error
}
