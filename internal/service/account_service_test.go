package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"ticket-reservation/internal/auth"
	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

const (
	testEmail    = "dilara.cetin@example.com"
	testPassword = "correct-horse-battery-staple"
)

// sequentialIDs hands out ids shaped like the real ones but in a predictable
// order, so that a failure message points at a particular account.
//
// The obvious version, "user-" + string(rune('0'+n)), silently produces "user-:"
// on the tenth call, which is the sort of thing that turns one failing test into
// an hour of confusion.
func sequentialIDs() func() string {
	n := 0

	return func() string {
		n++

		return fmt.Sprintf("usr_%08d", n)
	}
}

func newTestAccountService(t *testing.T) (*AccountService, *repository.MemoryUserRepository) {
	t.Helper()

	tokens, err := auth.NewTokens("a-test-signing-secret-of-enough-length")
	if err != nil {
		t.Fatalf("building the signer failed: %v", err)
	}

	users := repository.NewMemoryUserRepository()

	// MinCost, because the tests care about the logic and not about spending a
	// tenth of a second per hash.
	svc, err := NewAccountService(AccountConfig{
		Users:    users,
		Hasher:   auth.NewPasswordHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}),
		Tokens:   tokens,
		Clock:    newFakeClock(testTime()),
		NewID:    sequentialIDs(),
		TokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewAccountService() error = %v", err)
	}

	return svc, users
}

func TestAccountService_Register(t *testing.T) {
	ctx := context.Background()

	t.Run("creates an account", func(t *testing.T) {
		svc, users := newTestAccountService(t)

		user, err := svc.Register(ctx, testEmail, testPassword)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		if user.Email != testEmail {
			t.Errorf("email = %q, want %q", user.Email, testEmail)
		}

		// The hash must not travel back out of the service.
		if user.PasswordHash != "" {
			t.Errorf("the returned user carries a hash: %q", user.PasswordHash)
		}

		// But it must be stored, and it must not be the password.
		stored, err := users.GetUserByEmail(ctx, testEmail)
		if err != nil {
			t.Fatalf("the account was not stored: %v", err)
		}
		if stored.PasswordHash == "" {
			t.Error("no hash was stored")
		}
		if stored.PasswordHash == testPassword {
			t.Error("the password was stored as it was typed")
		}
	})

	// Two people with the same password must not end up with the same stored
	// value, or one leaked hash would give away every account sharing it.
	t.Run("the same password produces different hashes", func(t *testing.T) {
		svc, users := newTestAccountService(t)

		if _, err := svc.Register(ctx, "ayse.yilmaz@example.com", testPassword); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		if _, err := svc.Register(ctx, "mehmet.demir@example.com", testPassword); err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		first, _ := users.GetUserByEmail(ctx, "ayse.yilmaz@example.com")
		second, _ := users.GetUserByEmail(ctx, "mehmet.demir@example.com")

		if first.PasswordHash == second.PasswordHash {
			t.Error("both accounts stored the same hash, so the salt is not doing its job")
		}
	})

	t.Run("the address is normalised", func(t *testing.T) {
		svc, _ := newTestAccountService(t)

		user, err := svc.Register(ctx, "  Dilara.Cetin@Example.COM ", testPassword)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		if user.Email != testEmail {
			t.Errorf("email = %q, want %q", user.Email, testEmail)
		}

		// And signing in with any casing reaches the same account.
		if _, err := svc.Login(ctx, "DILARA.CETIN@EXAMPLE.COM", testPassword); err != nil {
			t.Errorf("Login() with different casing error = %v", err)
		}
	})

	t.Run("a taken address is refused", func(t *testing.T) {
		svc, _ := newTestAccountService(t)

		if _, err := svc.Register(ctx, testEmail, testPassword); err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		_, err := svc.Register(ctx, testEmail, "tr0ubador&3-elephant-mule")
		if !errors.Is(err, repository.ErrEmailTaken) {
			t.Errorf("Register() error = %v, want %v", err, repository.ErrEmailTaken)
		}
	})

	t.Run("bad input is refused before anything is stored", func(t *testing.T) {
		tests := []struct {
			name     string
			email    string
			password string
			wantErr  error
		}{
			{"no at sign", "dilara.cetin.example.com", testPassword, domain.ErrInvalidEmail},
			{"empty email", "", testPassword, domain.ErrInvalidEmail},
			{"short password", testEmail, "kisa123", domain.ErrWeakPassword},
			{"empty password", testEmail, "", domain.ErrWeakPassword},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, users := newTestAccountService(t)

				if _, err := svc.Register(ctx, tt.email, tt.password); !errors.Is(err, tt.wantErr) {
					t.Fatalf("Register() error = %v, want %v", err, tt.wantErr)
				}

				if _, err := users.GetUserByEmail(ctx, domain.NormalizeEmail(tt.email)); !errors.Is(err, repository.ErrUserNotFound) {
					t.Error("an account was stored despite the input being refused")
				}
			})
		}
	})
}

func TestAccountService_Login(t *testing.T) {
	ctx := context.Background()

	t.Run("returns a session for the right password", func(t *testing.T) {
		svc, _ := newTestAccountService(t)

		user, err := svc.Register(ctx, testEmail, testPassword)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		session, err := svc.Login(ctx, testEmail, testPassword)
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}
		if session.UserID != user.ID {
			t.Errorf("user = %q, want %q", session.UserID, user.ID)
		}
		if session.Token == "" {
			t.Error("no token was issued")
		}
		if want := testTime().Add(time.Hour); !session.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %v, want %v", session.ExpiresAt, want)
		}
	})

	// The point: a wrong password and an unknown address are the same answer, so
	// nobody can discover which addresses are registered by asking.
	t.Run("every failure looks the same", func(t *testing.T) {
		svc, _ := newTestAccountService(t)

		if _, err := svc.Register(ctx, testEmail, testPassword); err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		tests := []struct {
			name     string
			email    string
			password string
		}{
			{"wrong password", testEmail, "correct-horse-battery-stapler"},
			{"unknown address", "kimse.yok@example.com", testPassword},
			{"both wrong", "kimse.yok@example.com", "correct-horse-battery-stapler"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := svc.Login(ctx, tt.email, tt.password)
				if !errors.Is(err, ErrInvalidCredentials) {
					t.Errorf("Login() error = %v, want %v", err, ErrInvalidCredentials)
				}
			})
		}
	})

	// The token has to name the account, not the address, so that changing an
	// address later does not invalidate every session.
	t.Run("the token names the account", func(t *testing.T) {
		svc, _ := newTestAccountService(t)

		user, err := svc.Register(ctx, testEmail, testPassword)
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}

		session, err := svc.Login(ctx, testEmail, testPassword)
		if err != nil {
			t.Fatalf("Login() error = %v", err)
		}

		tokens, err := auth.NewTokens("a-test-signing-secret-of-enough-length")
		if err != nil {
			t.Fatalf("building the signer failed: %v", err)
		}

		named, err := tokens.Verify(session.Token, testTime())
		if err != nil {
			t.Fatalf("the issued token does not verify: %v", err)
		}
		if named != user.ID {
			t.Errorf("the token names %q, want %q", named, user.ID)
		}
	})
}

// A decoy hash is compared against when the address is unknown, so that the two
// paths take a comparable amount of work. This checks the comparison happens at
// all; the timing itself is not something a unit test can assert reliably.
func TestAccountService_LoginComparesAgainstADecoyForUnknownAddresses(t *testing.T) {
	tokens, err := auth.NewTokens("a-test-signing-secret-of-enough-length")
	if err != nil {
		t.Fatalf("building the signer failed: %v", err)
	}

	counter := &countingHasher{inner: auth.NewPasswordHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1})}

	svc, err := NewAccountService(AccountConfig{
		Users:    repository.NewMemoryUserRepository(),
		Hasher:   counter,
		Tokens:   tokens,
		Clock:    newFakeClock(testTime()),
		NewID:    sequentialIDs(),
		TokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewAccountService() error = %v", err)
	}

	before := counter.compares

	if _, err := svc.Login(context.Background(), "kimse.yok@example.com", testPassword); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}

	if counter.compares == before {
		t.Error("no comparison was made for an unknown address, so the answer comes back faster than a real one")
	}
}

type countingHasher struct {
	inner    PasswordHasher
	compares int
}

func (h *countingHasher) Hash(password string) (string, error) {
	return h.inner.Hash(password)
}

func (h *countingHasher) Compare(hash, password string) error {
	h.compares++

	return h.inner.Compare(hash, password)
}

func (h *countingHasher) NeedsRehash(hash string) bool {
	return h.inner.NeedsRehash(hash)
}

// TestAccountService_LoginUpgradesABcryptHash is the point of NeedsRehash: sign
// in is the only moment the plain password is known, so it is the only chance to
// replace a hash made with a superseded algorithm. Nobody is asked to reset
// anything, and the account is on Argon2id from the next sign in onwards.
func TestAccountService_LoginUpgradesABcryptHash(t *testing.T) {
	ctx := context.Background()

	tokens, err := auth.NewTokens("a-test-signing-secret-of-enough-length")
	if err != nil {
		t.Fatalf("building the signer failed: %v", err)
	}

	users := repository.NewMemoryUserRepository()

	// An account as it would have been stored before the move to Argon2id.
	legacy, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("producing a bcrypt hash failed: %v", err)
	}

	existing := &domain.User{
		ID:           "user-1",
		Email:        testEmail,
		PasswordHash: string(legacy),
		CreatedAt:    testTime(),
	}
	if err := users.CreateUser(ctx, existing); err != nil {
		t.Fatalf("seeding the account failed: %v", err)
	}

	svc, err := NewAccountService(AccountConfig{
		Users:    users,
		Hasher:   auth.NewPasswordHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}),
		Tokens:   tokens,
		Clock:    newFakeClock(testTime()),
		NewID:    sequentialIDs(),
		TokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewAccountService() error = %v", err)
	}

	// The old hash still verifies: nobody is locked out by the change.
	session, err := svc.Login(ctx, testEmail, testPassword)
	if err != nil {
		t.Fatalf("Login() with a bcrypt hash error = %v", err)
	}
	if session.UserID != "user-1" {
		t.Errorf("signed in as %q, want user-1", session.UserID)
	}

	upgraded, err := users.GetUserByEmail(ctx, testEmail)
	if err != nil {
		t.Fatalf("GetUserByEmail() error = %v", err)
	}
	if !strings.HasPrefix(upgraded.PasswordHash, "$argon2id$") {
		t.Errorf("the hash was not upgraded: %q", upgraded.PasswordHash)
	}

	// And the account still signs in against the replacement.
	if _, err := svc.Login(ctx, testEmail, testPassword); err != nil {
		t.Errorf("Login() after the upgrade error = %v", err)
	}
}

// A hash already at the current settings must not be rewritten on every sign in.
func TestAccountService_LoginLeavesACurrentHashAlone(t *testing.T) {
	ctx := context.Background()

	svc, users := newTestAccountService(t)

	if _, err := svc.Register(ctx, testEmail, testPassword); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	before, _ := users.GetUserByEmail(ctx, testEmail)

	if _, err := svc.Login(ctx, testEmail, testPassword); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	after, _ := users.GetUserByEmail(ctx, testEmail)
	if before.PasswordHash != after.PasswordHash {
		t.Error("a hash at the current settings was rewritten anyway")
	}
}
