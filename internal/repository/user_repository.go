package repository

import (
	"context"

	"ticket-reservation/internal/domain"
)

// UserRepository stores accounts.
type UserRepository interface {
	CreateUser(ctx context.Context, user *domain.User) error

	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)

	UpdatePasswordHash(ctx context.Context, userID, hash string) error
}
