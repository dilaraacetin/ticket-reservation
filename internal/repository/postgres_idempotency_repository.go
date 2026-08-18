package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresIdempotencyRepository remembers requests in PostgreSQL, so that a retry
// reaching a different instance of the service still gets the first answer.
type PostgresIdempotencyRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresIdempotencyRepository returns a store over the given pool.
func NewPostgresIdempotencyRepository(pool *pgxpool.Pool) *PostgresIdempotencyRepository {
	return &PostgresIdempotencyRepository{pool: pool}
}

const (
	idempotencyColumns = `user_id, key, fingerprint, state, status_code, response, created_at, expires_at`

	// FOR UPDATE for the same reason the seat store uses it: two retries arriving
	// together must not both decide the key is free.
	selectIdempotencyForUpdateSQL = `select ` + idempotencyColumns + `
		  from idempotency_keys
		 where user_id = $1 and key = $2
		   for update`

	// One statement covers both a first claim and taking over an expired record,
	// so no branch in Go has to decide which case it is looking at.
	claimIdempotencySQL = `insert into idempotency_keys
		     (user_id, key, fingerprint, state, created_at, expires_at)
		 values ($1, $2, $3, 'in_progress', $4, $5)
		 on conflict (user_id, key) do update
		    set fingerprint = excluded.fingerprint,
		        state       = 'in_progress',
		        status_code = null,
		        response    = null,
		        created_at  = excluded.created_at,
		        expires_at  = excluded.expires_at`

	completeIdempotencySQL = `update idempotency_keys
		   set state = 'completed', status_code = $3, response = $4
		 where user_id = $1 and key = $2`

	releaseIdempotencySQL = `delete from idempotency_keys where user_id = $1 and key = $2`
)

// Claim reserves the key unless a live record already holds it.
func (r *PostgresIdempotencyRepository) Claim(
	ctx context.Context,
	record IdempotencyRecord,
) (IdempotencyRecord, bool, error) {
	var (
		existing IdempotencyRecord
		claimed  bool
	)

	err := r.inTransaction(ctx, func(tx pgx.Tx) error {
		found, err := scanIdempotency(tx.QueryRow(ctx, selectIdempotencyForUpdateSQL, record.UserID, record.Key))

		switch {
		case err == nil && found.ExpiresAt.After(record.CreatedAt):
			existing, claimed = found, false

			return nil
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("locking idempotency key: %w", err)
		}

		if _, err := tx.Exec(ctx, claimIdempotencySQL,
			record.UserID, record.Key, record.Fingerprint, record.CreatedAt, record.ExpiresAt,
		); err != nil {
			return fmt.Errorf("claiming idempotency key: %w", err)
		}

		record.State = IdempotencyInProgress
		record.StatusCode = 0
		record.Response = nil
		existing, claimed = record, true

		return nil
	})
	if err != nil {
		return IdempotencyRecord{}, false, err
	}

	return existing, claimed, nil
}

// Complete stores the answer to replay.
func (r *PostgresIdempotencyRepository) Complete(
	ctx context.Context,
	userID, key string,
	statusCode int,
	response []byte,
) error {
	tag, err := r.pool.Exec(ctx, completeIdempotencySQL, userID, key, statusCode, response)
	if err != nil {
		return fmt.Errorf("completing idempotency key: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrIdempotencyKeyNotFound
	}

	return nil
}

// Release forgets the claim.
func (r *PostgresIdempotencyRepository) Release(ctx context.Context, userID, key string) error {
	if _, err := r.pool.Exec(ctx, releaseIdempotencySQL, userID, key); err != nil {
		return fmt.Errorf("releasing idempotency key: %w", err)
	}

	return nil
}

func scanIdempotency(row pgx.Row) (IdempotencyRecord, error) {
	var (
		record     IdempotencyRecord
		statusCode *int
	)

	err := row.Scan(
		&record.UserID, &record.Key, &record.Fingerprint, &record.State,
		&statusCode, &record.Response, &record.CreatedAt, &record.ExpiresAt,
	)
	if err != nil {
		return IdempotencyRecord{}, err
	}

	if statusCode != nil {
		record.StatusCode = *statusCode
	}

	return record, nil
}

func (r *PostgresIdempotencyRepository) inTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning a transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}
