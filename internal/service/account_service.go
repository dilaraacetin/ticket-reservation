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

// AccountConfig carries the account service's dependencies.
type AccountConfig struct {
	Users    repository.UserRepository
	Hasher   PasswordHasher
	Tokens   TokenIssuer
	Clock    Clock
	NewID    func() string
	TokenTTL time.Duration
	Logger   *slog.Logger
}

// AccountService registers accounts and signs them in.
type AccountService struct {
	users     repository.UserRepository
	hasher    PasswordHasher
	tokens    TokenIssuer
	clock     Clock
	newID     func() string
	tokenTTL  time.Duration
	logger    *slog.Logger
	decoyHash string
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
		users:     cfg.Users,
		hasher:    cfg.Hasher,
		tokens:    cfg.Tokens,
		clock:     cfg.Clock,
		newID:     cfg.NewID,
		tokenTTL:  cfg.TokenTTL,
		logger:    cfg.Logger,
		decoyHash: decoy,
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

	user := &domain.User{
		ID:           s.newID(),
		Email:        normalized,
		PasswordHash: hash,
		CreatedAt:    s.clock.Now(),
	}

	if err := s.users.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	user.PasswordHash = ""

	return user, nil
}

// Login checks a password and returns a session.
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
