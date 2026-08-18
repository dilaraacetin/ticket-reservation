package repository

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type idempotencyRepositoryFactory func(t *testing.T) IdempotencyRepository

func newMemoryIdempotency(_ *testing.T) IdempotencyRepository {
	return NewMemoryIdempotencyRepository()
}

func newPostgresIdempotency(t *testing.T) IdempotencyRepository {
	t.Helper()

	pool := newTestPool(t)
	if _, err := pool.Exec(t.Context(), "truncate table idempotency_keys"); err != nil {
		t.Fatalf("resetting idempotency_keys failed: %v", err)
	}

	return NewPostgresIdempotencyRepository(pool)
}

func newRecord(userID, key string, now time.Time) IdempotencyRecord {
	return IdempotencyRecord{
		UserID:      userID,
		Key:         key,
		Fingerprint: "POST /holds/h1/confirm",
		CreatedAt:   now,
		ExpiresAt:   now.Add(24 * time.Hour),
	}
}

func TestIdempotencyRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo idempotencyRepositoryFactory
	}{
		{"memory", newMemoryIdempotency},
		{"postgres", newPostgresIdempotency},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runIdempotencyRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runIdempotencyRepositoryContract(t *testing.T, newRepo idempotencyRepositoryFactory) {
	t.Helper()

	now := testTime()

	t.Run("a free key is claimed", func(t *testing.T) {
		repo := newRepo(t)

		got, claimed, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now))
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if !claimed {
			t.Fatal("Claim() = false, want the key to be granted")
		}
		if got.State != IdempotencyInProgress {
			t.Errorf("state = %q, want %q", got.State, IdempotencyInProgress)
		}
	})

	t.Run("a claimed key is refused and the holder is returned", func(t *testing.T) {
		repo := newRepo(t)

		if _, _, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now)); err != nil {
			t.Fatalf("first Claim() error = %v", err)
		}

		got, claimed, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now))
		if err != nil {
			t.Fatalf("second Claim() error = %v", err)
		}
		if claimed {
			t.Fatal("Claim() = true, want the second claim to be refused")
		}
		if got.State != IdempotencyInProgress || got.IsCompleted() {
			t.Errorf("record = %+v, want an in progress holder", got)
		}
	})

	// The key is scoped by user, so one caller cannot block or read another's.
	t.Run("the same key belongs to each user separately", func(t *testing.T) {
		repo := newRepo(t)

		if _, _, err := repo.Claim(t.Context(), newRecord(testUser, "shared", now)); err != nil {
			t.Fatalf("first Claim() error = %v", err)
		}

		_, claimed, err := repo.Claim(t.Context(), newRecord("someone-else", "shared", now))
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if !claimed {
			t.Error("Claim() = false, want another user's key to be independent")
		}
	})

	t.Run("a completed record carries the answer to replay", func(t *testing.T) {
		repo := newRepo(t)
		body := []byte(`{"seatId":"A1"}`)

		if _, _, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now)); err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if err := repo.Complete(t.Context(), testUser, "k1", 200, body); err != nil {
			t.Fatalf("Complete() error = %v", err)
		}

		got, claimed, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now))
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if claimed {
			t.Fatal("Claim() = true, want a completed key to stay taken")
		}
		if !got.IsCompleted() {
			t.Errorf("state = %q, want %q", got.State, IdempotencyCompleted)
		}
		if got.StatusCode != 200 {
			t.Errorf("status = %d, want 200", got.StatusCode)
		}
		if string(got.Response) != string(body) {
			t.Errorf("response = %q, want %q", got.Response, body)
		}
	})

	t.Run("the fingerprint of the holder is preserved", func(t *testing.T) {
		repo := newRepo(t)

		first := newRecord(testUser, "k1", now)
		first.Fingerprint = "POST /a"
		if _, _, err := repo.Claim(t.Context(), first); err != nil {
			t.Fatalf("Claim() error = %v", err)
		}

		second := newRecord(testUser, "k1", now)
		second.Fingerprint = "POST /b"

		got, claimed, err := repo.Claim(t.Context(), second)
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if claimed {
			t.Fatal("Claim() = true, want the key to stay taken")
		}
		// The caller compares this to decide whether the key was reused for a
		// different request, so it has to be the first request's fingerprint.
		if got.Fingerprint != "POST /a" {
			t.Errorf("fingerprint = %q, want the first one", got.Fingerprint)
		}
	})

	// Without this, a request that died halfway would hold its key until expiry.
	t.Run("a released key can be claimed again", func(t *testing.T) {
		repo := newRepo(t)

		if _, _, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now)); err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if err := repo.Release(t.Context(), testUser, "k1"); err != nil {
			t.Fatalf("Release() error = %v", err)
		}

		_, claimed, err := repo.Claim(t.Context(), newRecord(testUser, "k1", now))
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if !claimed {
			t.Error("Claim() = false, want a released key to be free")
		}
	})

	// Expiry is what stops a forgotten in progress record from blocking a key for
	// good, so a lapsed record has to be taken over rather than honoured.
	t.Run("an expired record is taken over", func(t *testing.T) {
		repo := newRepo(t)

		stale := newRecord(testUser, "k1", now)
		stale.ExpiresAt = now.Add(time.Minute)
		if _, _, err := repo.Claim(t.Context(), stale); err != nil {
			t.Fatalf("Claim() error = %v", err)
		}

		later := newRecord(testUser, "k1", now.Add(2*time.Minute))

		got, claimed, err := repo.Claim(t.Context(), later)
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
		if !claimed {
			t.Fatalf("Claim() = false, want an expired record to be replaced: %+v", got)
		}
	})

	t.Run("completing an unknown key is reported", func(t *testing.T) {
		repo := newRepo(t)

		err := repo.Complete(t.Context(), testUser, "never-claimed", 200, []byte(`{}`))
		if !errors.Is(err, ErrIdempotencyKeyNotFound) {
			t.Errorf("Complete() error = %v, want %v", err, ErrIdempotencyKeyNotFound)
		}
	})

	// The guarantee that matters: out of many simultaneous retries, exactly one
	// gets to do the work.
	t.Run("only one of many simultaneous claims wins", func(t *testing.T) {
		const attempts = 30

		repo := newRepo(t)
		ctx := t.Context()

		var (
			wg      sync.WaitGroup
			granted atomic.Int32
			refused atomic.Int32
		)

		start := make(chan struct{})

		for i := range attempts {
			wg.Add(1)

			go func() {
				defer wg.Done()

				<-start

				_, claimed, err := repo.Claim(ctx, newRecord(testUser, "hot-key", now))
				switch {
				case err != nil:
					t.Errorf("attempt %d: Claim() error = %v", i, err)
				case claimed:
					granted.Add(1)
				default:
					refused.Add(1)
				}
			}()
		}

		close(start)
		wg.Wait()

		if got := granted.Load(); got != 1 {
			t.Errorf("granted claims = %d, want exactly 1", got)
		}
		if got := refused.Load(); got != attempts-1 {
			t.Errorf("refused claims = %d, want %d", got, attempts-1)
		}
	})

	t.Run("keys do not collide across users under load", func(t *testing.T) {
		const users = 20

		repo := newRepo(t)
		ctx := t.Context()

		var (
			wg      sync.WaitGroup
			granted atomic.Int32
		)

		for i := range users {
			wg.Add(1)

			go func() {
				defer wg.Done()

				_, claimed, err := repo.Claim(ctx, newRecord(fmt.Sprintf("user-%d", i), "same-key", now))
				if err != nil {
					t.Errorf("Claim() error = %v", err)

					return
				}
				if claimed {
					granted.Add(1)
				}
			}()
		}

		wg.Wait()

		if got := granted.Load(); got != users {
			t.Errorf("granted claims = %d, want %d", got, users)
		}
	})
}

var (
	_ IdempotencyRepository = (*MemoryIdempotencyRepository)(nil)
	_ IdempotencyRepository = (*PostgresIdempotencyRepository)(nil)
)
