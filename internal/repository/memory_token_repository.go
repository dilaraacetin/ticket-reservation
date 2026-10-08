package repository

import (
	"context"
	"sync"
	"time"
)

// MemoryTokenRepository keeps revoked token ids in a map.
//
// Worth saying out loud: this is per process. Two servers sharing a database
// share their revocations; two servers with in-memory stores do not, so signing
// out of one would leave the other still accepting the token.
type MemoryTokenRepository struct {
	mu      sync.RWMutex
	revoked map[string]time.Time
}

func NewMemoryTokenRepository() *MemoryTokenRepository {
	return &MemoryTokenRepository{revoked: make(map[string]time.Time)}
}

func (r *MemoryTokenRepository) Revoke(_ context.Context, tokenID string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.revoked[tokenID] = expiresAt

	return nil
}

func (r *MemoryTokenRepository) IsRevoked(_ context.Context, tokenID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, found := r.revoked[tokenID]

	return found, nil
}

func (r *MemoryTokenRepository) DeleteExpired(_ context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	removed := 0

	for id, expiresAt := range r.revoked {
		// Once a token would have expired on its own, the record refuses nothing
		// that the expiry check does not already refuse.
		if !now.Before(expiresAt) {
			delete(r.revoked, id)
			removed++
		}
	}

	return removed, nil
}
