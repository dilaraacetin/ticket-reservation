// Package auth issues and verifies the tokens that say who a caller is.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const MinSecretLength = 32

const separator = "|"

// Tokens signs and verifies bearer tokens.
type Tokens struct {
	secret []byte
}

// NewTokens returns a signer over the given secret.
func NewTokens(secret string) (*Tokens, error) {
	if len(secret) < MinSecretLength {
		return nil, fmt.Errorf("%w: got %d bytes, want at least %d", ErrWeakSecret, len(secret), MinSecretLength)
	}

	return &Tokens{secret: []byte(secret)}, nil
}

// Issue returns a token that names userID until expiresAt.
func (t *Tokens) Issue(userID string, expiresAt time.Time) (string, error) {
	if userID == "" || strings.Contains(userID, separator) {
		return "", fmt.Errorf("%w: %q", ErrInvalidUserID, userID)
	}

	payload := userID + separator + strconv.FormatInt(expiresAt.Unix(), 10)

	return encode(payload) + "." + encode(string(t.sign(payload))), nil
}

// Verify returns the user id a token names, or why it cannot be trusted.
func (t *Tokens) Verify(token string, now time.Time) (string, error) {
	encodedPayload, encodedSignature, found := strings.Cut(token, ".")
	if !found {
		return "", ErrMalformedToken
	}

	payload, err := decode(encodedPayload)
	if err != nil {
		return "", ErrMalformedToken
	}

	signature, err := decode(encodedSignature)
	if err != nil {
		return "", ErrMalformedToken
	}

	if !hmac.Equal([]byte(signature), t.sign(payload)) {
		return "", ErrInvalidSignature
	}

	userID, expiry, found := strings.Cut(payload, separator)
	if !found || userID == "" {
		return "", ErrMalformedToken
	}

	seconds, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		return "", ErrMalformedToken
	}

	if !now.Before(time.Unix(seconds, 0)) {
		return "", ErrTokenExpired
	}

	return userID, nil
}

func (t *Tokens) sign(payload string) []byte {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write([]byte(payload))

	return mac.Sum(nil)
}

func encode(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decode(value string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}
