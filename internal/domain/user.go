package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Password length limits.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 72
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         Role

	// EmailVerifiedAt is zero until somebody has followed a link sent to the
	// address. Until then the address is only a claim: anybody can type
	// somebody else's, and sending to it would make this service a way to post
	// mail to strangers.
	EmailVerifiedAt time.Time

	CreatedAt time.Time
}

// EmailVerified reports whether the address has been shown to belong to whoever
// registered it.
func (u *User) EmailVerified() bool {
	return !u.EmailVerifiedAt.IsZero()
}

// NewUser returns an account ready to be stored.
//
// It exists so that the role cannot be left out. Building a User literally and
// forgetting it produced an empty role, which the database's check constraint
// refused — correctly, but by then the caller is reading a SQL error instead of
// having been unable to make the mistake.
func NewUser(id, email, passwordHash string, now time.Time) *User {
	return &User{
		ID:           id,
		Email:        email,
		PasswordHash: passwordHash,
		Role:         DefaultRole,
		CreatedAt:    now,
	}
}

// NormalizeEmail returns the form an address is stored and compared in.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidateEmail checks the address is one worth storing.
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: it is empty", ErrInvalidEmail)
	}

	local, domain, found := strings.Cut(email, "@")
	if !found || local == "" || domain == "" {
		return fmt.Errorf("%w: %q is not local@domain", ErrInvalidEmail, email)
	}

	if !strings.Contains(domain, ".") || strings.ContainsAny(email, " \t") {
		return fmt.Errorf("%w: %q", ErrInvalidEmail, email)
	}

	return nil
}

func ValidatePassword(password string) error {

	if utf8.RuneCountInString(password) < MinPasswordLength {
		return fmt.Errorf("%w: it must be at least %d characters", ErrWeakPassword, MinPasswordLength)
	}

	if len(password) > MaxPasswordLength {
		return fmt.Errorf("%w: it must be at most %d bytes", ErrWeakPassword, MaxPasswordLength)
	}

	return nil
}
