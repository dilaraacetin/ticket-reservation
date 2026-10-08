package repository

import (
	"context"
	"slices"
	"sync"
	"time"

	"ticket-reservation/internal/domain"
)

// MemoryVerificationRepository keeps verifications keyed by token hash.
type MemoryVerificationRepository struct {
	mu            sync.Mutex
	verifications map[string]*domain.EmailVerification
}

func NewMemoryVerificationRepository() *MemoryVerificationRepository {
	return &MemoryVerificationRepository{verifications: make(map[string]*domain.EmailVerification)}
}

func (r *MemoryVerificationRepository) Create(
	_ context.Context,
	verification *domain.EmailVerification,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	stored := *verification
	r.verifications[stored.Token] = &stored

	return nil
}

// ClaimUnsent marks them sent under the same lock it picks them under, which is
// what the single statement does in Postgres.
func (r *MemoryVerificationRepository) ClaimUnsent(
	_ context.Context,
	now time.Time,
	limit int,
) ([]*domain.EmailVerification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tokens := make([]string, 0, len(r.verifications))

	for token, verification := range r.verifications {
		if !verification.SentAt.IsZero() || !now.Before(verification.ExpiresAt) {
			continue
		}

		tokens = append(tokens, token)
	}

	slices.SortFunc(tokens, func(a, b string) int {
		if created := r.verifications[a].CreatedAt.Compare(r.verifications[b].CreatedAt); created != 0 {
			return created
		}

		return cmpString(a, b)
	})

	if limit > 0 && len(tokens) > limit {
		tokens = tokens[:limit]
	}

	claimed := make([]*domain.EmailVerification, 0, len(tokens))

	for _, token := range tokens {
		r.verifications[token].SentAt = now

		taken := *r.verifications[token]
		claimed = append(claimed, &taken)
	}

	return claimed, nil
}

func (r *MemoryVerificationRepository) Release(_ context.Context, token string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if verification, found := r.verifications[token]; found {
		verification.SentAt = time.Time{}
	}

	return nil
}

func (r *MemoryVerificationRepository) Consume(
	_ context.Context,
	token string,
	now time.Time,
) (*domain.EmailVerification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	verification, found := r.verifications[token]
	if !found || !verification.IsUsable(now) {
		// One answer for unknown, spent and expired, because telling them apart
		// would say whether a token ever existed.
		return nil, domain.ErrVerificationNotUsable
	}

	// Deleting is what spends it, which is what the single statement does in
	// Postgres.
	delete(r.verifications, token)

	consumed := *verification

	return &consumed, nil
}

func (r *MemoryVerificationRepository) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0

	for token, verification := range r.verifications {
		if !now.Before(verification.ExpiresAt) {
			delete(r.verifications, token)
			removed++
		}
	}

	return removed, nil
}

var _ VerificationRepository = (*MemoryVerificationRepository)(nil)
