package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"log/slog"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

const DefaultTokenTTL = time.Hour

// PasswordHasher stores and checks passwords. Declared here, on the consuming
// side, so the service never learns which algorithm is behind it.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
	NeedsRehash(hash string) bool
}

// TokenIssuer mints the token a signed in caller carries.
type TokenIssuer interface {
	Issue(userID string, expiresAt time.Time) (string, error)
}

// Session is what signing in produces.
type Session struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
}

// Verifier issues the link that proves an address belongs to whoever gave it.
type Verifier interface {
	Issue(ctx context.Context, userID, email string) (string, error)
}

// AccountConfig carries the account service's dependencies.
type AccountConfig struct {
	Users repository.UserRepository

	// Verifier is optional. Without it nothing is ever verified, which means
	// nothing is ever emailed — the safe direction for a deployment with no mail
	// server to prove an address with.
	Verifier Verifier

	// Revocations is where signing out is recorded. Optional: without it the
	// service still signs people in, it just cannot sign them out, and the
	// endpoint is left off rather than answering as if it had worked.
	Revocations repository.TokenRepository

	Hasher   PasswordHasher
	Tokens   TokenIssuer
	Clock    Clock
	NewID    func() string
	TokenTTL time.Duration
	Logger   *slog.Logger
}

// AccountService registers accounts and signs them in.
type AccountService struct {
	users       repository.UserRepository
	verifier    Verifier
	revocations repository.TokenRepository
	hasher      PasswordHasher
	tokens      TokenIssuer
	clock       Clock
	newID       func() string
	tokenTTL    time.Duration
	logger      *slog.Logger
	decoyHash   string
}

// NewAccountService wires an account service and prepares its decoy hash.
func NewAccountService(cfg AccountConfig) (*AccountService, error) {
	if cfg.TokenTTL <= 0 {
		cfg.TokenTTL = DefaultTokenTTL
	}

	decoy, err := cfg.Hasher.Hash(cfg.NewID())
	if err != nil {
		return nil, fmt.Errorf("preparing the account service: %w", err)
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}

	return &AccountService{
		users:       cfg.Users,
		verifier:    cfg.Verifier,
		revocations: cfg.Revocations,
		hasher:      cfg.Hasher,
		tokens:      cfg.Tokens,
		clock:       cfg.Clock,
		newID:       cfg.NewID,
		tokenTTL:    cfg.TokenTTL,
		logger:      cfg.Logger,
		decoyHash:   decoy,
	}, nil
}

// Register creates an account. The returned user never carries the hash.
func (s *AccountService) Register(ctx context.Context, email, password string) (*domain.User, error) {
	normalized := domain.NormalizeEmail(email)

	if err := domain.ValidateEmail(normalized); err != nil {
		return nil, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return nil, err
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, err
	}

	user := domain.NewUser(s.newID(), normalized, hash, s.clock.Now())

	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	// Issued after the account exists, and a failure here is logged rather than
	// returned: the account is real by now, and refusing the registration would
	// leave somebody unable to register at all because a queue was busy. Asking
	// for the link again is a button.
	if s.verifier != nil {
		if _, err := s.verifier.Issue(ctx, user.ID, user.Email); err != nil {
			s.logger.ErrorContext(ctx, "an account was created without a verification link",
				"err", err,
				"userId", user.ID,
			)
		}
	}

	user.PasswordHash = ""

	return user, nil
}

// Login checks a password and returns a session.
// Logout refuses a token for whatever is left of its life.
//
// The record is kept only until the token would have expired anyway, because
// after that the expiry check already refuses it and the row protects nothing.
func (s *AccountService) Logout(ctx context.Context, tokenID string, expiresAt time.Time) error {
	if s.revocations == nil {
		return ErrLogoutUnavailable
	}

	return s.revocations.Revoke(ctx, tokenID, expiresAt)
}

// IsTokenRevoked reports whether a token has been signed out. On the path of
// every authenticated request, so it is one lookup by key.
func (s *AccountService) IsTokenRevoked(ctx context.Context, tokenID string) (bool, error) {
	if s.revocations == nil {
		return false, nil
	}

	return s.revocations.IsRevoked(ctx, tokenID)
}

// SweepRevocations drops revocations that no longer refuse anything.
func (s *AccountService) SweepRevocations(ctx context.Context, now time.Time) (int, error) {
	if s.revocations == nil {
		return 0, nil
	}

	return s.revocations.DeleteExpired(ctx, now)
}

// Account returns who the caller is, without the password hash. What the
// interface needs to decide which parts of itself to show: whether to offer the
// administrative section, and whether to ask for the address to be confirmed.
func (s *AccountService) Account(ctx context.Context, userID string) (*domain.User, error) {
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Never leaves this package. A hash is not secret enough to hand out and not
	// useful enough to be worth the risk.
	user.PasswordHash = ""

	return user, nil
}

// Role reports what an account may do.
//
// Read from storage on every request that needs it rather than carried in the
// token: a token is signed once and believed until it expires, so a role put
// inside one outlives being taken away.
func (s *AccountService) Role(ctx context.Context, userID string) (domain.Role, error) {
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}

	return user.Role, nil
}

// SetRole changes what an account may do. There is no HTTP route for this on
// purpose: the first administrator has to come from somewhere, and an endpoint
// that can create one is an endpoint that can be abused into creating one.
func (s *AccountService) SetRole(ctx context.Context, email string, role domain.Role) error {
	return s.users.SetRole(ctx, domain.NormalizeEmail(email), role)
}

func (s *AccountService) Login(ctx context.Context, email, password string) (Session, error) {
	normalized := domain.NormalizeEmail(email)

	user, err := s.users.GetUserByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			_ = s.hasher.Compare(s.decoyHash, password)

			return Session{}, ErrInvalidCredentials
		}

		return Session{}, err
	}

	if err := s.hasher.Compare(user.PasswordHash, password); err != nil {
		return Session{}, ErrInvalidCredentials
	}

	s.upgradeHash(ctx, user, password)

	expiresAt := s.clock.Now().Add(s.tokenTTL)

	token, err := s.tokens.Issue(user.ID, expiresAt)
	if err != nil {
		return Session{}, err
	}

	return Session{Token: token, UserID: user.ID, ExpiresAt: expiresAt}, nil
}

// upgradeHash quietly replaces a hash made with superseded settings.
func (s *AccountService) upgradeHash(ctx context.Context, user *domain.User, password string) {
	if !s.hasher.NeedsRehash(user.PasswordHash) {
		return
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		s.logger.WarnContext(ctx, "rehashing a password failed", "err", err, "userId", user.ID)

		return
	}

	if err := s.users.UpdatePasswordHash(ctx, user.ID, hash); err != nil {
		s.logger.WarnContext(ctx, "storing a rehashed password failed", "err", err, "userId", user.ID)
	}
}
