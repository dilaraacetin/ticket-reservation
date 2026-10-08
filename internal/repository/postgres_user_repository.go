package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	userColumns = `id, email, password_hash, role, email_verified_at, created_at`

	insertUserSQL = `insert into users (id, email, password_hash, role, created_at)
		 values ($1, $2, $3, $4, $5)
		 on conflict (email) do nothing`

	selectUserByEmailSQL = `select ` + userColumns + ` from users where email = $1`

	selectUserByIDSQL = `select ` + userColumns + ` from users where id = $1`

	updatePasswordHashSQL = `update users set password_hash = $2 where id = $1`

	setRoleSQL = `update users set role = $2 where email = $1`

	markEmailVerifiedSQL = `update users
		   set email_verified_at = $3
		 where id = $1 and email = $2`
)

// CreateUser stores an account unless the address is taken.
func (r *PostgresUserRepository) CreateUser(ctx context.Context, user *domain.User) error {
	tag, err := r.pool.Exec(ctx, insertUserSQL,
		user.ID, user.Email, user.PasswordHash, user.Role.String(), user.CreatedAt)
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
	return scanUser(r.pool.QueryRow(ctx, selectUserByEmailSQL, email), email)
}

// GetUserByID returns the account a token names.
func (r *PostgresUserRepository) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
	return scanUser(r.pool.QueryRow(ctx, selectUserByIDSQL, userID), userID)
}

// SetRole changes what an account may do.
func (r *PostgresUserRepository) SetRole(ctx context.Context, email string, role domain.Role) error {
	tag, err := r.pool.Exec(ctx, setRoleSQL, email, role.String())
	if err != nil {
		return fmt.Errorf("setting the role for %q: %w", email, err)
	}

	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
}

// scanUser reads a row into a user. The role goes through the domain's parser,
// so a column that somehow holds something else fails here rather than being
// carried into an authorization check.
func scanUser(row pgx.Row, looked string) (*domain.User, error) {
	var (
		user       domain.User
		role       string
		verifiedAt *time.Time
	)

	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &role, &verifiedAt, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("reading the user for %q: %w", looked, err)
	}

	if user.Role, err = domain.ParseRole(role); err != nil {
		return nil, fmt.Errorf("reading the user for %q: %w", looked, err)
	}

	user.EmailVerifiedAt = instant(verifiedAt)

	return &user, nil
}

// MarkEmailVerified records that an address belongs to the account.
func (r *PostgresUserRepository) MarkEmailVerified(
	ctx context.Context,
	userID, email string,
	now time.Time,
) error {
	tag, err := r.pool.Exec(ctx, markEmailVerifiedSQL, userID, email, now)
	if err != nil {
		return fmt.Errorf("marking an address verified: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}

	return nil
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
