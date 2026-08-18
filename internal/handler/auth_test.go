package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// authTarget is a protected endpoint: it needs a caller, so it shows what the
// middleware did or did not attach.
const authTarget = "/holds/" + testHoldID + "/confirm"

func doWithAuthorization(svc ReservationService, authorization string) *httptest.ResponseRecorder {
	headers := map[string]string{}
	if authorization != "" {
		headers[authorizationHeader] = authorization
	}

	return do(svc, http.MethodPost, authTarget, headers)
}

func TestAuthenticate_AValidTokenIdentifiesTheCaller(t *testing.T) {
	svc := &fakeService{seat: reservedSeatFor(testUser)}

	rec := doWithAuthorization(svc, bearerForUser(testUser))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
	}

	if svc.gotUserID != testUser {
		t.Errorf("the service acted for %q, want %q", svc.gotUserID, testUser)
	}
}

func TestAuthenticate_NoTokenLeavesTheCallerAnonymous(t *testing.T) {
	svc := &fakeService{}

	rec := doWithAuthorization(svc, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "unauthenticated" {
		t.Errorf("code = %q, want unauthenticated", code)
	}
	if svc.gotUserID != "" {
		t.Error("the service was called for an anonymous request")
	}
}

// Every way a token can be wrong answers the same way, and never says which way.
func TestAuthenticate_BadTokensAreAllRefusedAlike(t *testing.T) {
	valid := bearerForUser(testUser)
	payload, signature, _ := strings.Cut(strings.TrimPrefix(valid, bearerPrefix), ".")

	expired, err := testTokenSigner.Issue(testUser, testTime().Add(-time.Minute))
	if err != nil {
		t.Fatalf("issuing an expired token failed: %v", err)
	}

	tests := []struct {
		name          string
		authorization string
	}{
		{"not a token at all", bearerPrefix + "nonsense"},
		{"signature of another payload", bearerPrefix + "bm90LW1l." + signature},
		{"payload with no signature", bearerPrefix + payload},
		{"expired", bearerPrefix + expired},
		{"empty bearer", bearerPrefix + "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeService{}

			rec := doWithAuthorization(svc, tt.authorization)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if svc.gotUserID != "" {
				t.Error("the service was called for a refused token")
			}

			body := rec.Body.String()
			for _, leak := range []string{"signature", "expired", "malformed"} {
				if strings.Contains(strings.ToLower(body), leak) {
					t.Errorf("the answer says why the token failed: %s", body)
				}
			}
		})
	}
}

// A token that is present but broken is refused outright, rather than being
// treated as an anonymous request that then fails somewhere less obvious.
func TestAuthenticate_ABrokenTokenIsRefusedEvenOnAPublicEndpoint(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, "/events", map[string]string{
		authorizationHeader: bearerPrefix + "nonsense",
	})

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "invalid_token" {
		t.Errorf("code = %q, want invalid_token", code)
	}
}

// The seat map and the event list stay readable without signing in.
func TestAuthenticate_PublicEndpointsNeedNoToken(t *testing.T) {
	for _, target := range []string{"/health", "/events", "/events/event-1/seats"} {
		t.Run(target, func(t *testing.T) {
			rec := do(&fakeService{}, http.MethodGet, target, nil)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
		})
	}
}

// The scheme is case insensitive in the specification, and clients do vary.
func TestAuthenticate_TheSchemeIsCaseInsensitive(t *testing.T) {
	token := strings.TrimPrefix(bearerForUser(testUser), bearerPrefix)

	for _, prefix := range []string{"Bearer ", "bearer ", "BEARER "} {
		t.Run(strings.TrimSpace(prefix), func(t *testing.T) {
			svc := &fakeService{seat: reservedSeatFor(testUser)}

			if rec := doWithAuthorization(svc, prefix+token); rec.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
			}
		})
	}
}

// A caller cannot act as somebody else by naming them: the identity comes from
// the signature, and the old X-User-ID header is now just an unread header.
func TestAuthenticate_TheOldTrustedHeaderIsIgnored(t *testing.T) {
	svc := &fakeService{seat: reservedSeatFor(testUser)}

	rec := do(&fakeService{}, http.MethodPost, authTarget, map[string]string{
		"X-User-ID": "mehmet",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = do(svc, http.MethodPost, authTarget, map[string]string{
		authorizationHeader: bearerForUser(testUser),
		"X-User-ID":         "mehmet",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if svc.gotUserID != testUser {
		t.Errorf("the service acted for %q, want %q", svc.gotUserID, testUser)
	}
}

func TestUserIDFromContext_IsEmptyWithoutAuthentication(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if got := UserIDFromContext(r.Context()); got != "" {
		t.Errorf("UserIDFromContext() = %q, want empty", got)
	}
}
