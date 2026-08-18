package handler

import (
	"time"

	"ticket-reservation/internal/auth"
	"ticket-reservation/internal/domain"
)

const testAuthSecret = "a-test-signing-secret-of-enough-length"

// testTokenSigner is the one signer every test uses, so a token minted by one
// helper verifies in another. Built once at package level because a fixed secret
// of a known length cannot fail to load.
var testTokenSigner = func() *auth.Tokens {
	tokens, err := auth.NewTokens(testAuthSecret)
	if err != nil {
		panic(err)
	}

	return tokens
}()

// bearerForUser returns the Authorization header value naming userID, or an empty
// string for a caller who is not signed in.
func bearerForUser(userID string) string {
	if userID == "" {
		return ""
	}

	token, err := testTokenSigner.Issue(userID, testTime().Add(time.Hour))
	if err != nil {
		panic(err)
	}

	return bearerPrefix + token
}

// reservedSeatFor is a seat a confirm would have produced, for the cases that
// only care that the handler ran at all.
func reservedSeatFor(userID string) *domain.Seat {
	seat := domain.NewSeat(testEventID, testSeatID, "A", 1)
	if err := seat.Hold(testHoldID, userID, time.Minute, testTime()); err != nil {
		panic(err)
	}
	if err := seat.Confirm(userID, testTime()); err != nil {
		panic(err)
	}

	return seat
}
