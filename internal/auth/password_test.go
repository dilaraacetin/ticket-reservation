package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// fastParams keep the tests quick. Real settings cost tens of milliseconds and
// nineteen megabytes each, which is the point of them and not something a test
// suite should pay hundreds of times over.
func fastParams() Argon2Params {
	return Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}

const password = "a-perfectly-fine-password"

func TestPasswordHasher_RoundTrip(t *testing.T) {
	hasher := NewPasswordHasher(fastParams())

	hash, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if err := hasher.Compare(hash, password); err != nil {
		t.Errorf("Compare() with the right password error = %v", err)
	}
	if err := hasher.Compare(hash, "not-the-password"); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("Compare() with a wrong password error = %v, want %v", err, ErrPasswordMismatch)
	}
}

// The stored form has to be the standard PHC string, or no other tool can read it.
func TestPasswordHasher_StoresThePHCFormat(t *testing.T) {
	hash, err := NewPasswordHasher(fastParams()).Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Errorf("hash = %q, want the PHC form with the settings in it", hash)
	}

	// Six fields, the first empty: $argon2id$v=..$m=..$salt$key
	if got := len(strings.Split(hash, "$")); got != 6 {
		t.Errorf("the hash splits into %d parts, want 6: %q", got, hash)
	}

	if strings.Contains(hash, password) {
		t.Error("the password appears in its own hash")
	}
}

// The salt is what stops one leaked hash from giving away everyone who shares
// that password.
func TestPasswordHasher_SaltsEveryHash(t *testing.T) {
	hasher := NewPasswordHasher(fastParams())

	first, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	second, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	if first == second {
		t.Error("the same password twice produced the same hash")
	}

	// And both still verify, which is only possible because the salt travels
	// inside the hash.
	for _, hash := range []string{first, second} {
		if err := hasher.Compare(hash, password); err != nil {
			t.Errorf("Compare() error = %v", err)
		}
	}
}

// Existing bcrypt hashes must keep working, or every account would need a reset.
func TestPasswordHasher_StillVerifiesBcrypt(t *testing.T) {
	legacy, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("producing a bcrypt hash failed: %v", err)
	}

	hasher := NewPasswordHasher(fastParams())

	if err := hasher.Compare(string(legacy), password); err != nil {
		t.Errorf("Compare() with a bcrypt hash error = %v", err)
	}
	if err := hasher.Compare(string(legacy), "not-the-password"); !errors.Is(err, ErrPasswordMismatch) {
		t.Errorf("Compare() with a wrong password error = %v, want %v", err, ErrPasswordMismatch)
	}
}

func TestPasswordHasher_NeedsRehash(t *testing.T) {
	current := NewPasswordHasher(Argon2Params{Memory: 256, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32})

	weaker, err := NewPasswordHasher(Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}).Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	atCurrent, err := current.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	legacy, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("producing a bcrypt hash failed: %v", err)
	}

	tests := []struct {
		name string
		hash string
		want bool
	}{
		{"made with the current settings", atCurrent, false},
		{"made with weaker settings", weaker, true},
		{"bcrypt", string(legacy), true},
		{"unreadable", "$argon2id$nonsense", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := current.NeedsRehash(tt.hash); got != tt.want {
				t.Errorf("NeedsRehash() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A hash written with old settings still verifies, because the settings travel
// with it. This is what makes raising the cost safe.
func TestPasswordHasher_VerifiesHashesMadeWithOtherSettings(t *testing.T) {
	old, err := NewPasswordHasher(Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}).Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	raised := NewPasswordHasher(Argon2Params{Memory: 512, Iterations: 3, Parallelism: 1, SaltLength: 16, KeyLength: 32})

	if err := raised.Compare(old, password); err != nil {
		t.Errorf("Compare() error = %v, want the old hash to still verify", err)
	}
	if !raised.NeedsRehash(old) {
		t.Error("NeedsRehash() = false, want the old hash to be marked for replacement")
	}
}

func TestPasswordHasher_RefusesUnreadableHashes(t *testing.T) {
	hasher := NewPasswordHasher(fastParams())

	valid, err := hasher.Hash(password)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	parts := strings.Split(valid, "$")

	tests := []struct {
		name string
		hash string
	}{
		{"too few fields", "$argon2id$v=19$m=64,t=1,p=1"},
		{"unknown version", "$argon2id$v=99$m=64,t=1,p=1$" + parts[4] + "$" + parts[5]},
		{"unreadable settings", "$argon2id$v=19$memory=64$" + parts[4] + "$" + parts[5]},
		{"salt is not base64", "$argon2id$v=19$m=64,t=1,p=1$not!base64$" + parts[5]},
		{"digest is not base64", "$argon2id$v=19$m=64,t=1,p=1$" + parts[4] + "$not!base64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := hasher.Compare(tt.hash, password); !errors.Is(err, ErrMalformedHash) {
				t.Errorf("Compare() error = %v, want %v", err, ErrMalformedHash)
			}
		})
	}
}

// A hasher built with nonsense settings must not produce a weak hash.
func TestNewPasswordHasher_FallsBackToTheDefaults(t *testing.T) {
	hasher := NewPasswordHasher(Argon2Params{})
	defaults := DefaultArgon2Params()

	if hasher.params != defaults {
		t.Errorf("params = %+v, want %+v", hasher.params, defaults)
	}
}
