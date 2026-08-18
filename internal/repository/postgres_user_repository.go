package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/domain"
)

// PostgresUserRepository stores accounts in PostgreSQL.
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserRepository returns a store over the given pool.
func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

const (
	userColumns = `id, email, password_hash, created_at`

	insertUserSQL = `insert into users (id, email, password_hash, created_at)
		 values ($1, $2, $3, $4)
		 on conflict (email) do nothing`

	selectUserByEmailSQL = `select ` + userColumns + ` from users where email = $1`

	updatePasswordHashSQL = `update users set password_hash = $2 where id = $1`
)

// CreateUser stores an account unless the address is taken.
func (r *PostgresUserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	tag, err := r.pool.Exec(ctx, insertUserSQL, user.ID, user.Email, user.PasswordHash, user.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating a user: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrEmailTaken
	}

	return nil
}

// GetUserByEmail returns the account for an address.
func (r *PostgresUserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User

	err := r.pool.QueryRow(ctx, selectUserByEmailSQL, email).
		Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("reading the user for %q: %w", email, err)
	}

	return &user, nil
}

// UpdatePasswordHash replaces the stored hash for an account.
func (r *PostgresUserRepository) UpdatePasswordHash(ctx context.Context, userID, hash string) error {
	tag, err := r.pool.Exec(ctx, updatePasswordHashSQL, userID, hash)
	if err != nil {
		return fmt.Errorf("updating a password hash: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}
