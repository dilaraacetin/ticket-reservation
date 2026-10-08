package repository

import (
	"testing"
	"time"
)

type tokenRepositoryFactory func(t *testing.T) TokenRepository

func newMemoryTokens(_ *testing.T) TokenRepository {
	return NewMemoryTokenRepository()
}

func newPostgresTokens(t *testing.T) TokenRepository {
	t.Helper()

	pool := newTestPool(t)
	if _, err := pool.Exec(t.Context(), "truncate table revoked_tokens"); err != nil {
		t.Fatalf("resetting revoked_tokens failed: %v", err)
	}

	return NewPostgresTokenRepository(pool)
}

func TestTokenRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo tokenRepositoryFactory
	}{
		{"memory", newMemoryTokens},
		{"postgres", newPostgresTokens},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runTokenRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runTokenRepositoryContract(t *testing.T, newRepo tokenRepositoryFactory) {
	t.Helper()

	now := testTime()

	t.Run("a token nobody revoked is accepted", func(t *testing.T) {
		repo := newRepo(t)

		revoked, err := repo.IsRevoked(t.Context(), "2XK7QW3PZ4MVBH9TJ6RNDS8CFA")
		if err != nil {
			t.Fatalf("IsRevoked() error = %v", err)
		}
		if revoked {
			t.Error("a token that was never revoked is reported as revoked")
		}
	})

	t.Run("a revoked token is refused", func(t *testing.T) {
		repo := newRepo(t)

		const tokenID = "7HP2VKX9ZQ4TMWB6RJ3NDF8CSA"

		if err := repo.Revoke(t.Context(), tokenID, now.Add(time.Hour)); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}

		revoked, err := repo.IsRevoked(t.Context(), tokenID)
		if err != nil {
			t.Fatalf("IsRevoked() error = %v", err)
		}
		if !revoked {
			t.Error("a revoked token is still accepted")
		}
	})

	// Signing out twice is the same request made twice. The caller wanted the
	// token gone either way.
	t.Run("revoking the same token twice is harmless", func(t *testing.T) {
		repo := newRepo(t)

		const tokenID = "QW8ZXK2P7V4NMTB9RJ6HDS3CFA"

		for range 2 {
			if err := repo.Revoke(t.Context(), tokenID, now.Add(time.Hour)); err != nil {
				t.Fatalf("Revoke() error = %v", err)
			}
		}

		revoked, _ := repo.IsRevoked(t.Context(), tokenID)
		if !revoked {
			t.Error("the token is no longer revoked after being revoked twice")
		}
	})

	// Only one token ends. Signing in twice gives two, and ending one must not
	// end the other, which is what makes signing out of one device possible.
	t.Run("revoking one token leaves the others alone", func(t *testing.T) {
		repo := newRepo(t)

		const (
			phone  = "PH8ZXK2P7V4NMTB9RJ6HDS3CFA"
			laptop = "LT2VKX9ZQ4TMWB6RJ3NDF8CSA7"
		)

		if err := repo.Revoke(t.Context(), phone, now.Add(time.Hour)); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}

		if revoked, _ := repo.IsRevoked(t.Context(), laptop); revoked {
			t.Error("signing out of one device signed out of the other")
		}
	})

	// The record only has to outlive nothing. Once the token would have expired,
	// the expiry check already refuses it.
	t.Run("spent records are cleared and live ones are kept", func(t *testing.T) {
		repo := newRepo(t)

		const (
			spent = "SP8ZXK2P7V4NMTB9RJ6HDS3CFA"
			live  = "LV2VKX9ZQ4TMWB6RJ3NDF8CSA7"
		)

		if err := repo.Revoke(t.Context(), spent, now.Add(time.Minute)); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}
		if err := repo.Revoke(t.Context(), live, now.Add(time.Hour)); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}

		cleared, err := repo.DeleteExpired(t.Context(), now.Add(30*time.Minute))
		if err != nil {
			t.Fatalf("DeleteExpired() error = %v", err)
		}
		if cleared != 1 {
			t.Errorf("cleared %d records, want 1", cleared)
		}

		if revoked, _ := repo.IsRevoked(t.Context(), spent); revoked {
			t.Error("a record whose token has expired was kept")
		}
		if revoked, _ := repo.IsRevoked(t.Context(), live); !revoked {
			t.Error("a record whose token is still live was cleared")
		}
	})

	// The boundary rule the rest of the service uses: the instant of expiry
	// counts as expired.
	t.Run("the expiry instant counts as spent", func(t *testing.T) {
		repo := newRepo(t)

		const tokenID = "EX8ZXK2P7V4NMTB9RJ6HDS3CFA"

		expiresAt := now.Add(time.Hour)
		if err := repo.Revoke(t.Context(), tokenID, expiresAt); err != nil {
			t.Fatalf("Revoke() error = %v", err)
		}

		cleared, err := repo.DeleteExpired(t.Context(), expiresAt)
		if err != nil {
			t.Fatalf("DeleteExpired() error = %v", err)
		}
		if cleared != 1 {
			t.Errorf("cleared %d records at the expiry instant, want 1", cleared)
		}
	})
}
