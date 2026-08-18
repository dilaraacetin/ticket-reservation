package auth

import "errors"

var (
	ErrMalformedToken   = errors.New("the token is not in the expected form")
	ErrInvalidSignature = errors.New("the token signature does not match")
	ErrTokenExpired     = errors.New("the token has expired")
	ErrWeakSecret       = errors.New("the signing secret is too short")
	ErrInvalidUserID    = errors.New("the user id is empty or contains the separator")
	ErrPasswordMismatch = errors.New("the password does not match")
	ErrMalformedHash    = errors.New("the stored password hash cannot be read")
)
