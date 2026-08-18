package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"already normal", "dilara@example.com", "dilara@example.com"},
		{"upper case", "Dilara@Example.COM", "dilara@example.com"},
		{"surrounding space", "  dilara@example.com \t", "dilara@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeEmail(tt.email); got != tt.want {
				t.Errorf("NormalizeEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"ordinary", "dilara@example.com", false},
		{"subdomain", "dilara@mail.example.co.uk", false},
		{"plus addressing", "dilara+tickets@example.com", false},
		{"empty", "", true},
		{"no at sign", "dilara.example.com", true},
		{"no local part", "@example.com", true},
		{"no domain", "dilara@", true},
		{"domain without a dot", "dilara@localhost", true},
		{"contains a space", "dilara @example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.wantErr && !errors.Is(err, ErrInvalidEmail) {
				t.Errorf("ValidateEmail(%q) error = %v, want %v", tt.email, err, ErrInvalidEmail)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateEmail(%q) error = %v, want nil", tt.email, err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"at the minimum", strings.Repeat("a", MinPasswordLength), false},
		{"comfortably long", "a-perfectly-ordinary-password", false},
		{"one short", strings.Repeat("a", MinPasswordLength-1), true},
		{"empty", "", true},
		{"one byte over the maximum", strings.Repeat("a", MaxPasswordLength+1), true},
		{"at the maximum", strings.Repeat("a", MaxPasswordLength), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.wantErr && !errors.Is(err, ErrWeakPassword) {
				t.Errorf("ValidatePassword(%d chars) error = %v, want %v", len(tt.password), err, ErrWeakPassword)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidatePassword(%d chars) error = %v, want nil", len(tt.password), err)
			}
		})
	}
}

// The minimum counts characters and the maximum counts bytes, because a person
// types characters and bcrypt reads bytes. A short password of wide runes must
// still pass, and a long one must still be refused.
func TestValidatePassword_CountsCharactersAndBytesSeparately(t *testing.T) {
	// Eight characters, well over eight bytes.
	if err := ValidatePassword("şğüöçİĞÜ"); err != nil {
		t.Errorf("an eight character password was refused: %v", err)
	}

	// Under 72 characters, over 72 bytes: bcrypt would silently ignore the tail.
	long := strings.Repeat("ş", 40)
	if len(long) <= MaxPasswordLength {
		t.Fatalf("the test string is only %d bytes, adjust it", len(long))
	}
	if err := ValidatePassword(long); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("a %d byte password was accepted, which bcrypt would truncate", len(long))
	}
}
