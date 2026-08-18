package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
	"ticket-reservation/internal/service"
)

// postJSON sends a body to an endpoint through the real router.
func postJSON(accounts AccountService, target, body string) *httptest.ResponseRecorder {
	h := New(&fakeService{}, accounts, fixedClock{now: testTime()}, discardLogger())

	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	return rec
}

func TestHandler_Register(t *testing.T) {
	const body = `{"email":"dilara.cetin@example.com","password":"correct-horse-battery-staple"}`

	t.Run("returns 201 and the account", func(t *testing.T) {
		accounts := &fakeAccounts{}

		rec := postJSON(accounts, "/auth/register", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
		}

		got := decode[userResponse](t, rec)
		if got.Email != "dilara.cetin@example.com" || got.ID == "" {
			t.Errorf("body = %+v, want an account for dilara@example.com", got)
		}

		if accounts.gotEmail != "dilara.cetin@example.com" {
			t.Errorf("the service got %q", accounts.gotEmail)
		}
	})

	// The password must not come back, in any form.
	t.Run("the response says nothing about the password", func(t *testing.T) {
		rec := postJSON(&fakeAccounts{}, "/auth/register", body)

		for _, leak := range []string{"password", "correct-horse-battery-staple", "hash", "$2a$"} {
			if strings.Contains(strings.ToLower(rec.Body.String()), strings.ToLower(leak)) {
				t.Errorf("the response contains %q: %s", leak, rec.Body)
			}
		}
	})

	t.Run("a taken address is 409", func(t *testing.T) {
		accounts := &fakeAccounts{err: repository.ErrEmailTaken}

		rec := postJSON(accounts, "/auth/register", body)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
		}
		if code := decode[errorBody](t, rec).Error.Code; code != "email_taken" {
			t.Errorf("code = %q, want email_taken", code)
		}
	})

	t.Run("invalid input is 400", func(t *testing.T) {
		tests := []struct {
			name string
			err  error
			code string
		}{
			{"bad address", domain.ErrInvalidEmail, "invalid_email"},
			{"weak password", domain.ErrWeakPassword, "weak_password"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := postJSON(&fakeAccounts{err: tt.err}, "/auth/register", body)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
				}
				if code := decode[errorBody](t, rec).Error.Code; code != tt.code {
					t.Errorf("code = %q, want %q", code, tt.code)
				}
			})
		}
	})
}

// The body tests the roadmap asked for in stage 3, which could not be written
// then because no endpoint accepted one.
func TestHandler_RejectsMalformedBodies(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty", "", http.StatusBadRequest},
		{"not JSON at all", "this is not json", http.StatusBadRequest},
		{"truncated", `{"email":"dilara.cetin@example.com"`, http.StatusBadRequest},
		{"an array rather than an object", `[{"email":"dilara.cetin@example.com"}]`, http.StatusBadRequest},
		{"wrong type for a field", `{"email":42,"password":"correct-horse-battery-staple"}`, http.StatusBadRequest},
		// DisallowUnknownFields: a typo is reported rather than ignored.
		{"a misspelled field", `{"email":"dilara.cetin@example.com","passwrod":"correct-horse-battery-staple"}`, http.StatusBadRequest},
		{"two objects", `{"email":"dilara.cetin@example.com","password":"correct-horse-battery-staple"}{"email":"mehmet.demir@example.com"}`, http.StatusBadRequest},
		{"too large", `{"email":"dilara.cetin@example.com","password":"` + strings.Repeat("x", 9000) + `"}`, http.StatusRequestEntityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := &fakeAccounts{}

			rec := postJSON(accounts, "/auth/register", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}

			// The service must never be reached with a body that did not parse.
			if accounts.gotEmail != "" {
				t.Errorf("the service was called with %q", accounts.gotEmail)
			}
		})
	}
}

func TestHandler_Login(t *testing.T) {
	const body = `{"email":"dilara.cetin@example.com","password":"correct-horse-battery-staple"}`

	t.Run("returns a token", func(t *testing.T) {
		accounts := &fakeAccounts{session: service.Session{
			Token:     "a-token",
			UserID:    "user-1",
			ExpiresAt: testTime().Add(time.Hour),
		}}

		rec := postJSON(accounts, "/auth/login", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
		}

		got := decode[sessionResponse](t, rec)
		if got.Token != "a-token" || got.UserID != "user-1" {
			t.Errorf("body = %+v, want the session", got)
		}
	})

	// One code for a wrong password and for an unknown address alike.
	t.Run("a failed sign in is 401 with one code", func(t *testing.T) {
		accounts := &fakeAccounts{err: service.ErrInvalidCredentials}

		rec := postJSON(accounts, "/auth/login", body)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		got := decode[errorBody](t, rec)
		if got.Error.Code != "invalid_credentials" {
			t.Errorf("code = %q, want invalid_credentials", got.Error.Code)
		}

		// The message must not say which half was wrong.
		for _, leak := range []string{"not found", "unknown", "no such"} {
			if strings.Contains(strings.ToLower(got.Error.Message), leak) {
				t.Errorf("the message says why: %q", got.Error.Message)
			}
		}
	})
}
