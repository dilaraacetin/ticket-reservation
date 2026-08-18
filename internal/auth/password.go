package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Argon2Params are the cost settings a hash was produced with. They are stored
// alongside every hash, which is what lets the cost be raised later without
// invalidating the hashes already written.
type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params follows OWASP's current guidance: 19 MiB, two passes, one
// lane. Enough to be expensive for an attacker, small enough that a server can
// serve several sign ins at once without running out of memory.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:      19456,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// PasswordHasher hashes with Argon2id and verifies both Argon2id and bcrypt.
type PasswordHasher struct {
	params Argon2Params
}

// NewPasswordHasher returns a hasher, falling back to the defaults for any
// setting that would produce a hash not worth having.
func NewPasswordHasher(params Argon2Params) *PasswordHasher {
	defaults := DefaultArgon2Params()

	if params.Memory < 8 {
		params.Memory = defaults.Memory
	}
	if params.Iterations < 1 {
		params.Iterations = defaults.Iterations
	}
	if params.Parallelism < 1 {
		params.Parallelism = defaults.Parallelism
	}
	if params.SaltLength < 8 {
		params.SaltLength = defaults.SaltLength
	}
	if params.KeyLength < 16 {
		params.KeyLength = defaults.KeyLength
	}

	return &PasswordHasher{params: params}
}

const argon2idPrefix = "$argon2id$"

// Hash returns the stored form of a password, in the PHC string format that
// carries the algorithm, its settings and the salt alongside the digest.
func (h *PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating a salt: %w", err)
	}

	key := argon2.IDKey(
		[]byte(password), salt,
		h.params.Iterations, h.params.Memory, h.params.Parallelism, h.params.KeyLength,
	)

	return encodeArgon2id(h.params, salt, key), nil
}

// Compare reports whether a password matches a stored hash, whichever algorithm
// produced it.
func (h *PasswordHasher) Compare(hash, password string) error {
	if !strings.HasPrefix(hash, argon2idPrefix) {
		// Anything else is assumed to be one of the bcrypt variants, and bcrypt
		// itself refuses what it does not recognise.
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
			return fmt.Errorf("%w: %w", ErrPasswordMismatch, err)
		}

		return nil
	}

	params, salt, want, err := decodeArgon2id(hash)
	if err != nil {
		return err
	}

	got := argon2.IDKey(
		[]byte(password), salt,
		params.Iterations, params.Memory, params.Parallelism, uint32(len(want)),
	)

	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrPasswordMismatch
	}

	return nil
}

// NeedsRehash reports whether a stored hash should be replaced the next time the
// password is known: either it predates Argon2id, or it was made with settings
// weaker than the ones in force now.
func (h *PasswordHasher) NeedsRehash(hash string) bool {
	if !strings.HasPrefix(hash, argon2idPrefix) {
		return true
	}

	params, _, _, err := decodeArgon2id(hash)
	if err != nil {
		// Unreadable is worse than outdated, and a rehash replaces it either way.
		return true
	}

	return params.Memory < h.params.Memory ||
		params.Iterations < h.params.Iterations ||
		params.Parallelism < h.params.Parallelism
}

// encodeArgon2id writes the PHC string format, which is what other tools and
// languages expect an Argon2 hash to look like.
func encodeArgon2id(params Argon2Params, salt, key []byte) string {
	return fmt.Sprintf("%sv=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2idPrefix,
		argon2.Version,
		params.Memory, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

func decodeArgon2id(hash string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: not an argon2id hash", ErrMalformedHash)
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: unreadable version", ErrMalformedHash)
	}
	if version != argon2.Version {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: version %d is not supported", ErrMalformedHash, version)
	}

	var params Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d",
		&params.Memory, &params.Iterations, &params.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: unreadable settings", ErrMalformedHash)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: unreadable salt", ErrMalformedHash)
	}

	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("%w: unreadable digest", ErrMalformedHash)
	}

	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(key))

	return params, salt, key, nil
}
