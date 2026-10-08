package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/service"
)

// stubRevocations answers whatever the test puts in it.
type stubRevocations struct {
	revoked bool
	err     error

	gotTokenID string
}

func (s *stubRevocations) IsTokenRevoked(_ context.Context, tokenID string) (bool, error) {
	s.gotTokenID = tokenID

	return s.revoked, s.err
}

// authenticated drives a request through the real Authenticate middleware, which
// is where a signed out token has to be caught.
func authenticated(
	t *testing.T,
	accounts AccountService,
	revocations RevocationCheck,
	method, target string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	// A hold to hand back, so that a request which gets past the middleware has
	// something to succeed with rather than reaching a nil.
	service := &fakeService{hold: &domain.Hold{
		ID:        "hold-1",
		EventID:   "event-1",
		SeatID:    "A1",
		UserID:    testUser,
		CreatedAt: testTime(),
		ExpiresAt: testTime().Add(5 * time.Minute),
	}}

	h := New(service, accounts, fixedClock{now: testTime()}, discardLogger())

	req := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	Chain(h.Routes(), Authenticate(testTokenSigner, revocations, fixedClock{now: testTime()}, discardLogger())).
		ServeHTTP(rec, req)

	return rec
}

func TestLogout_RevokesTheTokenTheRequestArrivedWith(t *testing.T) {
	accounts := &fakeAccounts{}

	rec := authenticated(t, accounts, nil, http.MethodPost, "/auth/logout", withUser(testUser))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNoContent, rec.Body)
	}

	// The id has to come from the token, not from a body. A caller that could
	// name a token could sign out somebody else's.
	if accounts.gotTokenID == "" {
		t.Error("nothing was revoked")
	}
	if accounts.gotExpiresAt.IsZero() {
		t.Error("the revocation has no expiry, so the record would be kept forever")
	}
}

func TestLogout_NeedsAToken(t *testing.T) {
	rec := authenticated(t, &fakeAccounts{}, nil, http.MethodPost, "/auth/logout", nil)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// 501 rather than 500: the deployment has not wired this up, which is a
// different thing from something having gone wrong.
func TestLogout_SaysWhenItIsNotAvailable(t *testing.T) {
	rec := authenticated(t,
		&fakeAccounts{logoutErr: service.ErrLogoutUnavailable},
		nil, http.MethodPost, "/auth/logout", withUser(testUser),
	)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}

	if got := decode[errorBody](t, rec).Error.Code; got != "logout_unavailable" {
		t.Errorf("code = %q, want logout_unavailable", got)
	}
}

// The whole point of the denylist: a token whose signature is perfectly good is
// still refused once it has been signed out.
func TestAuthenticate_RefusesASignedOutToken(t *testing.T) {
	revocations := &stubRevocations{revoked: true}

	rec := authenticated(t, &fakeAccounts{}, revocations,
		http.MethodPost, "/events/event-1/seats/A1/hold", withUser(testUser))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body)
	}

	if got := decode[errorBody](t, rec).Error.Code; got != "invalid_token" {
		t.Errorf("code = %q, want invalid_token", got)
	}

	if revocations.gotTokenID == "" {
		t.Error("the middleware did not look the token id up")
	}
}

func TestAuthenticate_AcceptsATokenThatWasNotSignedOut(t *testing.T) {
	rec := authenticated(t, &fakeAccounts{}, &stubRevocations{},
		http.MethodPost, "/events/event-1/seats/A1/hold", withUser(testUser))

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}
}

// A check that cannot run refuses rather than waves through. The tokens on that
// list are the entire reason it exists, so guessing in the caller's favour is
// the one answer that cannot be defended.
func TestAuthenticate_FailsClosedWhenTheCheckCannotRun(t *testing.T) {
	rec := authenticated(t, &fakeAccounts{},
		&stubRevocations{err: errors.New("connection refused")},
		http.MethodPost, "/events/event-1/seats/A1/hold", withUser(testUser))

	if rec.Code == http.StatusCreated {
		t.Fatal("the request went through although the revocation check failed")
	}

	if rec.Code < http.StatusInternalServerError {
		t.Errorf("status = %d, want a server error; a store that cannot answer is not the caller's fault", rec.Code)
	}
}
