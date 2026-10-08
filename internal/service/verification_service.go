package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// ErrAlreadyVerified means there is nothing to do.
var ErrAlreadyVerified = errors.New("the address is already verified")

// VerificationService issues and consumes "is this really your address" links.
type VerificationService struct {
	verifications repository.VerificationRepository
	users         repository.UserRepository
	clock         Clock
	ttl           time.Duration
	publicURL     string
	logger        *slog.Logger
}

// VerificationConfig carries the service's dependencies.
type VerificationConfig struct {
	Verifications repository.VerificationRepository
	Users         repository.UserRepository
	Clock         Clock
	TTL           time.Duration

	// PublicURL is where this service is reached from outside. A link in an
	// email has to be absolute, and the service cannot work that out from a
	// request it is not handling.
	PublicURL string

	Logger *slog.Logger
}

func NewVerificationService(cfg VerificationConfig) *VerificationService {
	if cfg.TTL <= 0 {
		cfg.TTL = domain.DefaultVerificationTTL
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}

	return &VerificationService{
		verifications: cfg.Verifications,
		users:         cfg.Users,
		clock:         cfg.Clock,
		ttl:           cfg.TTL,
		publicURL:     cfg.PublicURL,
		logger:        cfg.Logger,
	}
}

// Issue creates a verification for an address and returns the token, which the
// caller emails and then forgets. Nothing stores it.
func (s *VerificationService) Issue(ctx context.Context, userID, email string) (string, error) {
	token := rand.Text()

	verification, err := domain.NewEmailVerification(
		token, userID, email, s.clock.Now(), s.ttl)
	if err != nil {
		return "", err
	}

	if err := s.verifications.Create(ctx, verification); err != nil {
		return "", err
	}

	return token, nil
}

// Resend issues a fresh link for an account that has not verified yet.
//
// The old links are left alone rather than cancelled: somebody who clicks the
// first message after asking for a second should not be told their link is
// broken.
func (s *VerificationService) Resend(ctx context.Context, userID string) error {
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.EmailVerified() {
		return ErrAlreadyVerified
	}

	_, err = s.Issue(ctx, user.ID, user.Email)

	return err
}

// Verify follows a link: it spends the token and marks the address verified.
func (s *VerificationService) Verify(ctx context.Context, token string) error {
	if token == "" {
		return domain.ErrVerificationNotUsable
	}

	now := s.clock.Now()

	// Spent first. If marking the user fails the token is gone, which is the
	// safe direction: a link that half worked should not be reusable.
	verification, err := s.verifications.Consume(ctx, token, now)
	if err != nil {
		return err
	}

	err = s.users.MarkEmailVerified(ctx, verification.UserID, verification.Email, now)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			// The account has changed address since the link was sent. The link
			// proved the old one, which is no longer the question being asked.
			return domain.ErrVerificationNotUsable
		}

		return err
	}

	s.logger.InfoContext(ctx, "an address was verified", "userId", verification.UserID)

	return nil
}

// Link is the absolute URL that goes in the message.
func (s *VerificationService) Link(token string) string {
	base := s.publicURL
	if base == "" {
		// Better a relative link than a wrong absolute one: a reader on the
		// right host can still follow it.
		return "/verify?token=" + url.QueryEscape(token)
	}

	return fmt.Sprintf("%s/verify?token=%s", base, url.QueryEscape(token))
}

// SweepVerifications drops links that can no longer be followed.
func (s *VerificationService) SweepVerifications(ctx context.Context, now time.Time) (int, error) {
	return s.verifications.DeleteExpired(ctx, now)
}
