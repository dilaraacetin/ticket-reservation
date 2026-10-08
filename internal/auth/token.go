// Package auth issues and verifies the tokens that say who a caller is.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const MinSecretLength = 32

const separator = "|"

// fields is how many parts a payload has: the user, the expiry, and the token's
// own id.
const fields = 3

// Claims is what a verified token says.
//
// The id is what makes a token revocable: signing out records that one id, so a
// token can be refused before it expires without the signing secret having to
// change for everybody.
type Claims struct {
	UserID    string
	TokenID   string
	ExpiresAt time.Time
}

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
//
// The id is generated here rather than taken from the caller, because a token
// id that repeats is one that revokes somebody else's token as well.
func (t *Tokens) Issue(userID string, expiresAt time.Time) (string, error) {
	if userID == "" || strings.Contains(userID, separator) {
		return "", fmt.Errorf("%w: %q", ErrInvalidUserID, userID)
	}

	payload := strings.Join([]string{
		userID,
		strconv.FormatInt(expiresAt.Unix(), 10),
		rand.Text(),
	}, separator)

	return encode(payload) + "." + encode(string(t.sign(payload))), nil
}

// Verify returns what a token says, or why it cannot be trusted.
//
// The signature is checked before anything in the payload is read, so a payload
// nobody signed never reaches the parsing below.
func (t *Tokens) Verify(token string, now time.Time) (Claims, error) {
	encodedPayload, encodedSignature, found := strings.Cut(token, ".")
	if !found {
		return Claims{}, ErrMalformedToken
	}

	payload, err := decode(encodedPayload)
	if err != nil {
		return Claims{}, ErrMalformedToken
	}

	signature, err := decode(encodedSignature)
	if err != nil {
		return Claims{}, ErrMalformedToken
	}

	if !hmac.Equal([]byte(signature), t.sign(payload)) {
		return Claims{}, ErrInvalidSignature
	}

	parts := strings.Split(payload, separator)
	if len(parts) != fields || parts[0] == "" || parts[2] == "" {
		return Claims{}, ErrMalformedToken
	}

	seconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Claims{}, ErrMalformedToken
	}

	expiresAt := time.Unix(seconds, 0)
	if !now.Before(expiresAt) {
		return Claims{}, ErrTokenExpired
	}

	return Claims{UserID: parts[0], TokenID: parts[2], ExpiresAt: expiresAt}, nil
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
