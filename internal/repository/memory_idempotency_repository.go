package repository

import (
	"context"
	"sync"
)

// MemoryIdempotencyRepository keeps remembered requests in a map, mirroring the
// other in-memory stores.
type MemoryIdempotencyRepository struct {
	mu      sync.Mutex
	records map[string]IdempotencyRecord
}

func NewMemoryIdempotencyRepository() *MemoryIdempotencyRepository {
	return &MemoryIdempotencyRepository{records: make(map[string]IdempotencyRecord)}
}

func idempotencyKey(userID, key string) string {
	return userID + "\x00" + key
}

// Claim reserves the key unless a live record already holds it.
func (r *MemoryIdempotencyRepository) Claim(
	_ context.Context,
	record IdempotencyRecord,
) (IdempotencyRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	mapKey := idempotencyKey(record.UserID, record.Key)

	if existing, ok := r.records[mapKey]; ok && existing.ExpiresAt.After(record.CreatedAt) {
		return existing, false, nil
	}

	record.State = IdempotencyInProgress
	record.StatusCode = 0
	record.Response = nil
	r.records[mapKey] = record

	return record, true, nil
}

// Complete stores the answer to replay.
func (r *MemoryIdempotencyRepository) Complete(
	_ context.Context,
	userID, key string,
	statusCode int,
	response []byte,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mapKey := idempotencyKey(userID, key)

	record, ok := r.records[mapKey]
	if !ok {
		return ErrIdempotencyKeyNotFound
	}

	record.State = IdempotencyCompleted
	record.StatusCode = statusCode
	record.Response = append([]byte(nil), response...)
	r.records[mapKey] = record

	return nil
}

// Release forgets the claim.
func (r *MemoryIdempotencyRepository) Release(_ context.Context, userID, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.records, idempotencyKey(userID, key))

	return nil
}
