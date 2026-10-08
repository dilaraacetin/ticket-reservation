package auth

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testSecret = "k9Xm2pQrS4tU6vW8yZ0aB3dEf1GhIjKl"

func testTime() time.Time {
	return time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
}

func newTestTokens(t *testing.T) *Tokens {
	t.Helper()

	tokens, err := NewTokens(testSecret)
	if err != nil {
		t.Fatalf("NewTokens() error = %v", err)
	}

	return tokens
}

func TestTokens_RoundTrip(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	token, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := tokens.Verify(token, now)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.UserID != "dilara" {
		t.Errorf("user = %q, want dilara", claims.UserID)
	}
	if claims.TokenID == "" {
		t.Error("the token carries no id, so it could never be signed out")
	}
	if !claims.ExpiresAt.Equal(now.Add(time.Hour).Truncate(time.Second)) {
		t.Errorf("ExpiresAt = %s, want %s", claims.ExpiresAt, now.Add(time.Hour))
	}
}

// Two tokens must never share an id. An id that repeats is one that signs out
// somebody else's token along with its own.
func TestTokens_EveryTokenGetsItsOwnID(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	seen := make(map[string]bool, 100)

	for range 100 {
		token, err := tokens.Issue("dilara", now.Add(time.Hour))
		if err != nil {
			t.Fatalf("Issue() error = %v", err)
		}

		claims, err := tokens.Verify(token, now)
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}

		if seen[claims.TokenID] {
			t.Fatalf("token id %q was issued twice", claims.TokenID)
		}

		seen[claims.TokenID] = true
	}
}

// A token in the old two-field format has no id, so it cannot be signed out.
// Accepting one would leave a token nothing can refuse.
func TestTokens_RefusesTheOldFormat(t *testing.T) {
	tokens := newTestTokens(t)

	payload := "dilara" + separator + strconv.FormatInt(testTime().Add(time.Hour).Unix(), 10)
	old := encode(payload) + "." + encode(string(tokens.sign(payload)))

	if _, err := tokens.Verify(old, testTime()); !errors.Is(err, ErrMalformedToken) {
		t.Errorf("Verify() on a token with no id = %v, want %v", err, ErrMalformedToken)
	}
}

func TestNewTokens_RejectsAWeakSecret(t *testing.T) {
	for _, secret := range []string{"", "short", strings.Repeat("x", MinSecretLength-1)} {
		if _, err := NewTokens(secret); !errors.Is(err, ErrWeakSecret) {
			t.Errorf("NewTokens(%d bytes) error = %v, want %v", len(secret), err, ErrWeakSecret)
		}
	}

	if _, err := NewTokens(strings.Repeat("x", MinSecretLength)); err != nil {
		t.Errorf("NewTokens() at the minimum length error = %v", err)
	}
}

// A user id carrying the separator could otherwise be signed into a payload that
// reads as a different user entirely.
func TestTokens_RejectsAUserIDThatWouldForgeAPayload(t *testing.T) {
	tokens := newTestTokens(t)

	for _, userID := range []string{"", "dilara|9999999999"} {
		if _, err := tokens.Issue(userID, testTime().Add(time.Hour)); !errors.Is(err, ErrInvalidUserID) {
			t.Errorf("Issue(%q) error = %v, want %v", userID, err, ErrInvalidUserID)
		}
	}
}

func TestTokens_Expiry(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	token, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	tests := []struct {
		name    string
		at      time.Time
		wantErr error
	}{
		{"well before expiry", now, nil},
		{"one second before expiry", now.Add(time.Hour - time.Second), nil},
		{"at the expiry instant", now.Add(time.Hour), ErrTokenExpired},
		{"after expiry", now.Add(2 * time.Hour), ErrTokenExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tokens.Verify(token, tt.at)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Verify() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// The point of signing: a payload can be read, but it cannot be changed.
func TestTokens_ATamperedPayloadIsRejected(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	token, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	_, signature, _ := strings.Cut(token, ".")

	// Anyone can read the payload and write a new one naming somebody else. What
	// they cannot do is produce the signature that goes with it.
	forged := encode("mehmet"+separator+"99999999999") + "." + signature

	if _, err := tokens.Verify(forged, now); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify() error = %v, want %v", err, ErrInvalidSignature)
	}
}

func TestTokens_ATokenFromAnotherSecretIsRejected(t *testing.T) {
	mine := newTestTokens(t)

	theirs, err := NewTokens(strings.Repeat("z", MinSecretLength))
	if err != nil {
		t.Fatalf("NewTokens() error = %v", err)
	}

	token, err := theirs.Issue("dilara", testTime().Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := mine.Verify(token, testTime()); !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify() error = %v, want %v", err, ErrInvalidSignature)
	}
}

func TestTokens_MalformedTokens(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	valid, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	payload, signature, _ := strings.Cut(valid, ".")

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no separator", "just-one-part"},
		{"payload is not base64", "not!base64." + signature},
		{"signature is not base64", payload + ".not!base64"},
		{"payload has no expiry", encode("dilara") + "." + encode("whatever")},
		{"expiry is not a number", func() string {
			p := "dilara" + separator + "soon"

			return encode(p) + "." + encode(string(tokens.sign(p)))
		}()},
		{"empty user id", func() string {
			p := separator + "9999999999"

			return encode(p) + "." + encode(string(tokens.sign(p)))
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tokens.Verify(tt.token, now); err == nil {
				t.Error("Verify() error = nil, want a refusal")
			}
		})
	}
}

// Tokens used to be deterministic: the same user and expiry produced the same
// string. Carrying an id of their own deliberately ends that, because two sign
// ins have to be revocable one at a time rather than together.
func TestTokens_AreNotDeterministic(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	first, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	second, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if first == second {
		t.Error("two sign ins produced the same token, so signing out of one would sign out of both")
	}

	// Both still name the same person and expire at the same moment.
	for _, token := range []string{first, second} {
		claims, err := tokens.Verify(token, now)
		if err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if claims.UserID != "dilara" {
			t.Errorf("user = %q, want dilara", claims.UserID)
		}
		if !claims.ExpiresAt.Equal(now.Add(time.Hour).Truncate(time.Second)) {
			t.Errorf("ExpiresAt = %s, want %s", claims.ExpiresAt, now.Add(time.Hour))
		}
	}
}
