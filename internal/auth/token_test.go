package auth

import (
	"errors"
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

	got, err := tokens.Verify(token, now)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got != "dilara" {
		t.Errorf("user = %q, want dilara", got)
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

// Two tokens for the same user at the same expiry are identical, which is what
// makes them cacheable and comparable. Different expiries must differ.
func TestTokens_AreDeterministic(t *testing.T) {
	tokens := newTestTokens(t)
	now := testTime()

	first, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	same, err := tokens.Issue("dilara", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	later, err := tokens.Issue("dilara", now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if first != same {
		t.Error("the same user and expiry produced different tokens")
	}
	if first == later {
		t.Error("a different expiry produced the same token")
	}
}
