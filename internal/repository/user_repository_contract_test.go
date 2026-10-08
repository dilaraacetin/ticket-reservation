package repository

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"ticket-reservation/internal/auth"
	"ticket-reservation/internal/domain"
)

type userRepositoryFactory func(t *testing.T) UserRepository

func newMemoryUsers(_ *testing.T) UserRepository {
	return NewMemoryUserRepository()
}

func newPostgresUsers(t *testing.T) UserRepository {
	t.Helper()

	pool := newTestPool(t)
	if _, err := pool.Exec(t.Context(), "truncate table users"); err != nil {
		t.Fatalf("resetting users failed: %v", err)
	}

	return NewPostgresUserRepository(pool)
}

// testPasswordHash is a real Argon2id hash of testPassword, produced once with
// cheap settings. A hand written string that merely looks like a hash would be
// refused the moment anything tried to verify it, and the store would then be
// tested with data the rest of the system could never produce.
var (
	testPassword     = "a-perfectly-fine-password"
	testPasswordHash = func() string {
		hasher := auth.NewPasswordHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1})

		hash, err := hasher.Hash(testPassword)
		if err != nil {
			panic(err)
		}

		return hash
	}()
)

func testUserRecord(id, email string) *domain.User {
	return domain.NewUser(id, email, testPasswordHash, testTime())
}

func TestUserRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo userRepositoryFactory
	}{
		{"memory", newMemoryUsers},
		{"postgres", newPostgresUsers},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runUserRepositoryContract(t, implementation.newRepo)
		})
	}
}

func runUserRepositoryContract(t *testing.T, newRepo userRepositoryFactory) {
	t.Helper()

	now := testTime()

	t.Run("a new account is stored and read back", func(t *testing.T) {
		repo := newRepo(t)
		user := testUserRecord("u1", "dilara@example.com")

		if err := repo.CreateUser(t.Context(), user); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		got, err := repo.GetUserByEmail(t.Context(), "dilara@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if got.ID != user.ID || got.Email != user.Email {
			t.Errorf("user = %+v, want %+v", got, user)
		}
		if got.PasswordHash != user.PasswordHash {
			t.Errorf("hash = %q, want %q", got.PasswordHash, user.PasswordHash)
		}
		if !got.CreatedAt.Equal(user.CreatedAt) {
			t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, user.CreatedAt)
		}
	})

	// The authorization middleware resolves the id a token carries, so this is
	// the lookup every administrative request goes through.
	t.Run("an account can be read back by id", func(t *testing.T) {
		repo := newRepo(t)

		stored := testUserRecord("user-by-id", "dilara@example.com")
		if err := repo.CreateUser(t.Context(), stored); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		found, err := repo.GetUserByID(t.Context(), stored.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if found.Email != stored.Email {
			t.Errorf("email = %q, want %q", found.Email, stored.Email)
		}

		// Nobody becomes an administrator by signing up.
		if found.Role != domain.RoleCustomer {
			t.Errorf("role = %q, want %q", found.Role, domain.RoleCustomer)
		}

		if _, err := repo.GetUserByID(t.Context(), "nobody"); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GetUserByID() for a stranger = %v, want %v", err, ErrUserNotFound)
		}
	})

	t.Run("a role can be changed and read back", func(t *testing.T) {
		repo := newRepo(t)

		stored := testUserRecord("user-promoted", "admin@example.com")
		if err := repo.CreateUser(t.Context(), stored); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		if err := repo.SetRole(t.Context(), stored.Email, domain.RoleAdmin); err != nil {
			t.Fatalf("SetRole() error = %v", err)
		}

		// Read by both routes, because the middleware uses one and the command
		// line uses the other.
		byID, err := repo.GetUserByID(t.Context(), stored.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if !byID.Role.IsAdmin() {
			t.Errorf("role by id = %q, want %q", byID.Role, domain.RoleAdmin)
		}

		byEmail, err := repo.GetUserByEmail(t.Context(), stored.Email)
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if !byEmail.Role.IsAdmin() {
			t.Errorf("role by email = %q, want %q", byEmail.Role, domain.RoleAdmin)
		}

		// And back again, so the command is not one way.
		if err := repo.SetRole(t.Context(), stored.Email, domain.RoleCustomer); err != nil {
			t.Fatalf("SetRole() error = %v", err)
		}

		demoted, err := repo.GetUserByID(t.Context(), stored.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if demoted.Role.IsAdmin() {
			t.Error("the account is still an administrator after being demoted")
		}
	})

	t.Run("an address can be marked verified", func(t *testing.T) {
		repo := newRepo(t)

		stored := testUserRecord("user-verified", "dilara@example.com")
		if err := repo.CreateUser(t.Context(), stored); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		before, err := repo.GetUserByID(t.Context(), stored.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if before.EmailVerified() {
			t.Fatal("a new account is already verified")
		}

		if err := repo.MarkEmailVerified(t.Context(), stored.ID, stored.Email, now); err != nil {
			t.Fatalf("MarkEmailVerified() error = %v", err)
		}

		after, err := repo.GetUserByID(t.Context(), stored.ID)
		if err != nil {
			t.Fatalf("GetUserByID() error = %v", err)
		}
		if !after.EmailVerified() {
			t.Error("the address is still not verified")
		}
	})

	// A link proves the address it was sent to. If the account is at a different
	// one now, the link is about a question nobody is asking any more — so the
	// account and the address have to match together, not one or the other.
	t.Run("marking verified needs the account and the address to match", func(t *testing.T) {
		repo := newRepo(t)

		stored := testUserRecord("user-moved", "current@example.com")
		if err := repo.CreateUser(t.Context(), stored); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		err := repo.MarkEmailVerified(t.Context(), stored.ID, "old@example.com", now)
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("MarkEmailVerified() with the wrong address = %v, want %v", err, ErrUserNotFound)
		}

		// And nothing was verified.
		after, _ := repo.GetUserByID(t.Context(), stored.ID)
		if after.EmailVerified() {
			t.Error("a link for a different address verified this one")
		}

		// The same goes the other way: the right address but the wrong account.
		err = repo.MarkEmailVerified(t.Context(), "somebody-else", stored.Email, now)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("MarkEmailVerified() for the wrong account = %v, want %v", err, ErrUserNotFound)
		}
	})

	t.Run("setting the role of an unknown account is reported", func(t *testing.T) {
		repo := newRepo(t)

		err := repo.SetRole(t.Context(), "nobody@example.com", domain.RoleAdmin)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("SetRole() error = %v, want %v", err, ErrUserNotFound)
		}
	})

	t.Run("an unknown address returns ErrUserNotFound", func(t *testing.T) {
		repo := newRepo(t)

		if _, err := repo.GetUserByEmail(t.Context(), "nobody@example.com"); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("GetUserByEmail() error = %v, want %v", err, ErrUserNotFound)
		}
	})

	t.Run("a taken address is refused", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.CreateUser(t.Context(), testUserRecord("u1", "dilara@example.com")); err != nil {
			t.Fatalf("first CreateUser() error = %v", err)
		}

		err := repo.CreateUser(t.Context(), testUserRecord("u2", "dilara@example.com"))
		if !errors.Is(err, ErrEmailTaken) {
			t.Errorf("second CreateUser() error = %v, want %v", err, ErrEmailTaken)
		}

		// And the first account is the one that survives.
		got, err := repo.GetUserByEmail(t.Context(), "dilara@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if got.ID != "u1" {
			t.Errorf("stored id = %q, want u1", got.ID)
		}
	})

	t.Run("mutating a returned user does not touch the store", func(t *testing.T) {
		repo := newRepo(t)

		if err := repo.CreateUser(t.Context(), testUserRecord("u1", "dilara@example.com")); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		got, err := repo.GetUserByEmail(t.Context(), "dilara@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}

		got.PasswordHash = "tampered"

		again, err := repo.GetUserByEmail(t.Context(), "dilara@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if again.PasswordHash == "tampered" {
			t.Error("the stored hash was changed through the returned copy")
		}
	})

	t.Run("a stored hash can be replaced", func(t *testing.T) {
		repo := newRepo(t)
		user := testUserRecord("u1", "dilara@example.com")

		if err := repo.CreateUser(t.Context(), user); err != nil {
			t.Fatalf("CreateUser() error = %v", err)
		}

		// A second real hash of the same password: different salt, so it differs
		// from the first while still being something the system could produce.
		hasher := auth.NewPasswordHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1})

		replacement, err := hasher.Hash(testPassword)
		if err != nil {
			t.Fatalf("hashing the replacement failed: %v", err)
		}
		if replacement == testPasswordHash {
			t.Fatal("the replacement is identical to the original, so this proves nothing")
		}

		if err := repo.UpdatePasswordHash(t.Context(), "u1", replacement); err != nil {
			t.Fatalf("UpdatePasswordHash() error = %v", err)
		}

		got, err := repo.GetUserByEmail(t.Context(), "dilara@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail() error = %v", err)
		}
		if got.PasswordHash != replacement {
			t.Errorf("hash = %q, want the replacement", got.PasswordHash)
		}

		// And the stored replacement still verifies, which a made up string would
		// not.
		if err := hasher.Compare(got.PasswordHash, testPassword); err != nil {
			t.Errorf("the stored replacement does not verify: %v", err)
		}
	})

	t.Run("replacing the hash of an unknown account is reported", func(t *testing.T) {
		repo := newRepo(t)

		err := repo.UpdatePasswordHash(t.Context(), "nobody", testPasswordHash)
		if !errors.Is(err, ErrUserNotFound) {
			t.Errorf("UpdatePasswordHash() error = %v, want %v", err, ErrUserNotFound)
		}
	})

	// The same race the seats and the idempotency keys have: two registrations of
	// one address arriving together must not both succeed.
	t.Run("only one of many simultaneous registrations wins", func(t *testing.T) {
		const (
			rounds   = 5
			attempts = 30
		)

		repo := newRepo(t)
		ctx := t.Context()

		for round := range rounds {
			email := fmt.Sprintf("contested-%d@example.com", round)

			var (
				wg      sync.WaitGroup
				created atomic.Int32
				refused atomic.Int32
			)

			start := make(chan struct{})

			for i := range attempts {
				wg.Add(1)

				go func() {
					defer wg.Done()

					<-start

					err := repo.CreateUser(ctx, testUserRecord(fmt.Sprintf("u-%d-%d", round, i), email))
					switch {
					case err == nil:
						created.Add(1)
					case errors.Is(err, ErrEmailTaken):
						refused.Add(1)
					default:
						t.Errorf("round %d: unexpected error: %v", round, err)
					}
				}()
			}

			close(start)
			wg.Wait()

			if got := created.Load(); got != 1 {
				t.Fatalf("round %d: created accounts = %d, want exactly 1", round, got)
			}
			if got := refused.Load(); got != attempts-1 {
				t.Fatalf("round %d: refused = %d, want %d", round, got, attempts-1)
			}
		}
	})
}

var (
	_ UserRepository = (*MemoryUserRepository)(nil)
	_ UserRepository = (*PostgresUserRepository)(nil)
)
