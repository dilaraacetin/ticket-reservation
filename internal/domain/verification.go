package domain

import "time"

// DefaultVerificationTTL is how long a verification link works for. Long enough
// to survive a night in a spam folder, short enough that an old mailbox is not
// a way in.
const DefaultVerificationTTL = 24 * time.Hour

// EmailVerification is one outstanding "is this really your address" link.
//
// The token is stored, and that is a trade worth naming. Hashing it would stop a
// read-only leak — a backup, a log dump — from yielding a working link, but it
// would also make the token unrecoverable at send time, which means sending
// inside the request that registered the account and so making registration
// depend on a mail server being up.
//
// What keeps the exposure small instead is that the row is deleted the moment
// the link is followed, and swept once it expires. The table therefore holds
// only live, unused tokens, each for at most a day. And the threat is narrow to
// begin with: anybody who can read this table can set email_verified_at
// directly.
type EmailVerification struct {
	// Token is what was emailed. Deleting the row is what spends it, so a row
	// that exists is a link that still works.
	Token string

	UserID string

	// Email is the address the link was sent to, kept so that verifying an
	// address the account has since changed away from does not verify the new
	// one.
	Email string

	CreatedAt time.Time
	ExpiresAt time.Time

	// SentAt is zero until the message has gone, which is what the sender reads.
	SentAt time.Time
}

// NewEmailVerification returns a verification valid from now.
func NewEmailVerification(token, userID, email string, now time.Time, ttl time.Duration) (*EmailVerification, error) {
	if token == "" {
		return nil, ErrEmptyVerificationToken
	}
	if userID == "" {
		return nil, ErrEmptyUserID
	}
	if email == "" {
		return nil, ErrInvalidEmail
	}
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}

	return &EmailVerification{
		Token:     token,
		UserID:    userID,
		Email:     email,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}, nil
}

// IsUsable reports whether following this link should still do anything. A row
// that has been used does not exist, so only expiry is left to check.
func (v *EmailVerification) IsUsable(now time.Time) bool {
	return now.Before(v.ExpiresAt)
}
