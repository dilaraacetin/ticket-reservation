package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

func newVerificationSetup(t *testing.T) (
	*VerificationService,
	*repository.MemoryUserRepository,
	*repository.MemoryVerificationRepository,
	*fakeClock,
) {
	t.Helper()

	var (
		clock         = newFakeClock(testTime())
		users         = repository.NewMemoryUserRepository()
		verifications = repository.NewMemoryVerificationRepository()
	)

	service := NewVerificationService(VerificationConfig{
		Verifications: verifications,
		Users:         users,
		Clock:         clock,
		TTL:           domain.DefaultVerificationTTL,
		PublicURL:     "https://tickets.example.com",
		Logger:        discardLogger(),
	})

	return service, users, verifications, clock
}

func seedUser(t *testing.T, users *repository.MemoryUserRepository, id, email string) *domain.User {
	t.Helper()

	user := domain.NewUser(id, email, "$argon2id$fake", testTime())
	if err := users.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	return user
}

func TestVerification_AFreshLinkVerifiesTheAddress(t *testing.T) {
	service, users, _, _ := newVerificationSetup(t)
	user := seedUser(t, users, "user-1", "dilara@example.com")

	token, err := service.Issue(t.Context(), user.ID, user.Email)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if token == "" {
		t.Fatal("Issue() returned no token")
	}

	before, _ := users.GetUserByID(t.Context(), user.ID)
	if before.EmailVerified() {
		t.Fatal("a new account is already verified")
	}

	if err := service.Verify(t.Context(), token); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	after, _ := users.GetUserByID(t.Context(), user.ID)
	if !after.EmailVerified() {
		t.Error("the address is still not verified after following the link")
	}
}

// A link that keeps working is one still lying around in a mailbox a year later.
func TestVerification_ALinkWorksOnce(t *testing.T) {
	service, users, _, _ := newVerificationSetup(t)
	user := seedUser(t, users, "user-1", "dilara@example.com")

	token, err := service.Issue(t.Context(), user.ID, user.Email)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if err := service.Verify(t.Context(), token); err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}

	err = service.Verify(t.Context(), token)
	if !errors.Is(err, domain.ErrVerificationNotUsable) {
		t.Errorf("second Verify() = %v, want %v", err, domain.ErrVerificationNotUsable)
	}
}

func TestVerification_ARefusalSaysNothingAboutWhy(t *testing.T) {
	service, users, verifications, clock := newVerificationSetup(t)
	user := seedUser(t, users, "user-1", "dilara@example.com")

	expired, err := service.Issue(t.Context(), user.ID, user.Email)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	clock.Advance(domain.DefaultVerificationTTL + time.Second)

	// Unknown, spent and expired all answer the same way, because telling them
	// apart would say whether a token was ever real.
	for _, token := range []string{"", "a-token-nobody-issued", expired} {
		if err := service.Verify(t.Context(), token); !errors.Is(err, domain.ErrVerificationNotUsable) {
			t.Errorf("Verify(%q) = %v, want %v", token, err, domain.ErrVerificationNotUsable)
		}
	}

	// And an expired link is swept rather than kept for ever.
	cleared, err := service.SweepVerifications(t.Context(), clock.Now())
	if err != nil {
		t.Fatalf("SweepVerifications() error = %v", err)
	}
	if cleared != 1 {
		t.Errorf("cleared %d expired links, want 1", cleared)
	}

	if _, err := verifications.ClaimUnsent(t.Context(), clock.Now(), 10); err != nil {
		t.Fatalf("ClaimUnsent() error = %v", err)
	}
}

func TestVerification_ResendRefusesWhenThereIsNothingToDo(t *testing.T) {
	service, users, _, _ := newVerificationSetup(t)
	user := seedUser(t, users, "user-1", "dilara@example.com")

	token, err := service.Issue(t.Context(), user.ID, user.Email)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := service.Verify(t.Context(), token); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if err := service.Resend(t.Context(), user.ID); !errors.Is(err, ErrAlreadyVerified) {
		t.Errorf("Resend() = %v, want %v", err, ErrAlreadyVerified)
	}
}

// A link in an email has to be absolute, or it goes nowhere from a mail client.
func TestVerification_TheLinkIsAbsolute(t *testing.T) {
	service, _, _, _ := newVerificationSetup(t)

	link := service.Link("a-token/with?characters")

	if !strings.HasPrefix(link, "https://tickets.example.com/verify?token=") {
		t.Errorf("link = %q, want it absolute", link)
	}

	// And the token survives the URL it is put in.
	if strings.Contains(link, "with?characters") {
		t.Errorf("link = %q, want the token escaped", link)
	}
}
