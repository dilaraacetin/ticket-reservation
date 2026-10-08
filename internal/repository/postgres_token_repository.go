package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresTokenRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresTokenRepository(pool *pgxpool.Pool) *PostgresTokenRepository {
	return &PostgresTokenRepository{pool: pool}
}

const (
	// Revoking the same token twice is the same request made twice, so the
	// conflict is ignored rather than reported.
	revokeTokenSQL = `insert into revoked_tokens (id, expires_at)
		 values ($1, $2)
		 on conflict (id) do nothing`

	isTokenRevokedSQL = `select exists (select 1 from revoked_tokens where id = $1)`

	deleteExpiredTokensSQL = `delete from revoked_tokens where expires_at <= $1`
)

func (r *PostgresTokenRepository) Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error {
	if _, err := r.pool.Exec(ctx, revokeTokenSQL, tokenID, expiresAt); err != nil {
		return fmt.Errorf("revoking a token: %w", err)
	}

	return nil
}

func (r *PostgresTokenRepository) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	var revoked bool
	if err := r.pool.QueryRow(ctx, isTokenRevokedSQL, tokenID).Scan(&revoked); err != nil {
		return false, fmt.Errorf("checking whether a token is revoked: %w", err)
	}

	return revoked, nil
}

func (r *PostgresTokenRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, deleteExpiredTokensSQL, now)
	if err != nil {
		return 0, fmt.Errorf("clearing expired revocations: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

var (
	_ TokenRepository = (*PostgresTokenRepository)(nil)
	_ TokenRepository = (*MemoryTokenRepository)(nil)
)
