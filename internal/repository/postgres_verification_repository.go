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

type PostgresVerificationRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresVerificationRepository(pool *pgxpool.Pool) *PostgresVerificationRepository {
	return &PostgresVerificationRepository{pool: pool}
}

const verificationColumns = `token, user_id, email, created_at, expires_at, sent_at`

const (
	createVerificationSQL = `insert into email_verifications
		 (token, user_id, email, created_at, expires_at)
		 values ($1, $2, $3, $4, $5)`

	// Claimed and marked sent in one statement, so two senders cannot both email
	// the same link. skip locked lets the second get on with the next one.
	claimUnsentVerificationsSQL = `with unsent as (
		     select token
		       from email_verifications
		      where sent_at is null
		        and expires_at > $1
		      order by created_at
		      limit $2
		        for update skip locked
		 )
		 update email_verifications v
		    set sent_at = $1
		   from unsent
		  where v.token = unsent.token
	   returning v.token, v.user_id, v.email, v.created_at, v.expires_at, v.sent_at`

	releaseVerificationSQL = `update email_verifications set sent_at = null where token = $1`

	// Deleting is what spends it. One statement, because single use means two
	// people following the same link must not both succeed, and a row that is
	// gone cannot be followed again.
	consumeVerificationSQL = `delete from email_verifications
		  where token = $1
		    and expires_at > $2
	   returning ` + verificationColumns

	deleteExpiredVerificationsSQL = `delete from email_verifications where expires_at <= $1`
)

func (r *PostgresVerificationRepository) Create(
	ctx context.Context,
	verification *domain.EmailVerification,
) error {
	_, err := r.pool.Exec(ctx, createVerificationSQL,
		verification.Token, verification.UserID, verification.Email,
		verification.CreatedAt, verification.ExpiresAt)
	if err != nil {
		return fmt.Errorf("storing a verification: %w", err)
	}

	return nil
}

func (r *PostgresVerificationRepository) ClaimUnsent(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]*domain.EmailVerification, error) {
	rows, err := r.pool.Query(ctx, claimUnsentVerificationsSQL, now, limit)
	if err != nil {
		return nil, fmt.Errorf("claiming verifications: %w", err)
	}
	defer rows.Close()

	claimed := make([]*domain.EmailVerification, 0)

	for rows.Next() {
		verification, err := scanVerification(rows)
		if err != nil {
			return nil, fmt.Errorf("claiming verifications: %w", err)
		}

		claimed = append(claimed, verification)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claiming verifications: %w", err)
	}

	return claimed, nil
}

func (r *PostgresVerificationRepository) Release(ctx context.Context, token string) error {
	if _, err := r.pool.Exec(ctx, releaseVerificationSQL, token); err != nil {
		return fmt.Errorf("releasing a verification: %w", err)
	}

	return nil
}

func (r *PostgresVerificationRepository) Consume(
	ctx context.Context,
	token string,
	now time.Time,
) (*domain.EmailVerification, error) {
	verification, err := scanVerification(r.pool.QueryRow(ctx, consumeVerificationSQL, token, now))

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Unknown, already spent, or expired. One answer for all three, because
		// telling them apart would say whether a token ever existed.
		return nil, domain.ErrVerificationNotUsable
	case err != nil:
		return nil, fmt.Errorf("consuming a verification: %w", err)
	}

	return verification, nil
}

func (r *PostgresVerificationRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, deleteExpiredVerificationsSQL, now)
	if err != nil {
		return 0, fmt.Errorf("clearing expired verifications: %w", err)
	}

	return int(tag.RowsAffected()), nil
}

func scanVerification(row pgx.Row) (*domain.EmailVerification, error) {
	var (
		verification domain.EmailVerification
		sentAt       *time.Time
	)

	err := row.Scan(&verification.Token, &verification.UserID, &verification.Email,
		&verification.CreatedAt, &verification.ExpiresAt, &sentAt)
	if err != nil {
		return nil, err
	}

	verification.SentAt = instant(sentAt)

	return &verification, nil
}

var _ VerificationRepository = (*PostgresVerificationRepository)(nil)
