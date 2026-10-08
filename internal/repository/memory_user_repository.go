package repository

import (
	"context"
	"sync"
	"time"

	"ticket-reservation/internal/domain"
)

// MemoryUserRepository keeps accounts in a map, keyed by the normalised address.
type MemoryUserRepository struct {
	mu    sync.RWMutex
	users map[string]domain.User
}

// NewMemoryUserRepository returns an empty store.
func NewMemoryUserRepository() *MemoryUserRepository {
	return &MemoryUserRepository{users: make(map[string]domain.User)}
}

// CreateUser stores an account unless the address is taken.
func (r *MemoryUserRepository) CreateUser(_ context.Context, user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// The check and the write are both inside the lock, which is what the unique
	// index does for the Postgres store.
	if _, taken := r.users[user.Email]; taken {
		return ErrEmailTaken
	}

	r.users[user.Email] = *user

	return nil
}

// GetUserByEmail returns a copy of the stored account.
func (r *MemoryUserRepository) GetUserByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.users[email]
	if !ok {
		return nil, ErrUserNotFound
	}

	return &user, nil
}

// GetUserByID walks the map, because accounts are keyed by address. The lookups
// that matter for latency go through the address; this one serves authorization
// checks, which are rare.
func (r *MemoryUserRepository) GetUserByID(_ context.Context, userID string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, user := range r.users {
		if user.ID == userID {
			stored := user

			return &stored, nil
		}
	}

	return nil, ErrUserNotFound
}

// SetRole changes what an account may do.
func (r *MemoryUserRepository) SetRole(_ context.Context, email string, role domain.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, ok := r.users[email]
	if !ok {
		return ErrUserNotFound
	}

	user.Role = role
	r.users[email] = user

	return nil
}

// MarkEmailVerified records that an address belongs to the account, and only
// while the account still has that address.
func (r *MemoryUserRepository) MarkEmailVerified(
	_ context.Context,
	userID, email string,
	now time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	user, found := r.users[email]
	if !found || user.ID != userID {
		return ErrUserNotFound
	}

	user.EmailVerifiedAt = now
	r.users[email] = user

	return nil
}

// UpdatePasswordHash replaces the stored hash for an account.
func (r *MemoryUserRepository) UpdatePasswordHash(_ context.Context, userID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for email, user := range r.users {
		if user.ID == userID {
			user.PasswordHash = hash
			r.users[email] = user

			return nil
		}
	}

	return ErrUserNotFound
}
