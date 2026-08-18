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
	CreatedAt    time.Time
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
