package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// served drives a request through the security headers and the real mux.
func served(t *testing.T, h *Handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	Chain(h.Routes(), SecurityHeaders).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

	return rec
}

func newSecurityHandler(t *testing.T, prober Prober) *Handler {
	t.Helper()

	h := New(&fakeService{}, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger())
	if prober != nil {
		h = h.WithProber(prober)
	}

	return h
}

func TestSecurityHeaders_AreSetOnEveryAnswer(t *testing.T) {
	rec := served(t, newSecurityHandler(t, nil), "/health")

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}

	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}

	policy := rec.Header().Get("Content-Security-Policy")

	// 'unsafe-inline' is the whole reason the bundled page keeps its script and
	// its stylesheet in files of their own, so finding it here would mean that
	// was for nothing.
	if strings.Contains(policy, "unsafe-inline") {
		t.Errorf("the default policy allows inline code: %q", policy)
	}

	for _, directive := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'"} {
		if !strings.Contains(policy, directive) {
			t.Errorf("the policy is missing %q: %q", directive, policy)
		}
	}
}

// HSTS would be a promise this deployment cannot keep, and a browser told to use
// HTTPS for a year does not fall back when there is none.
func TestSecurityHeaders_DoesNotPromiseHTTPS(t *testing.T) {
	rec := served(t, newSecurityHandler(t, nil), "/health")

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q, want it unset until something terminates TLS", got)
	}
}

// The documentation page needs a CDN and one inline script, so it replaces the
// policy. The replacement still has to carry a nonce rather than open the page
// up to anything inline.
func TestDocs_CarriesItsOwnPolicyWithANonce(t *testing.T) {
	rec := served(t, newSecurityHandler(t, nil), "/docs")

	policy := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(policy, "nonce-") {
		t.Fatalf("the docs policy has no nonce: %q", policy)
	}

	if strings.Contains(policy, "script-src 'unsafe-inline'") {
		t.Errorf("the docs policy allows any inline script: %q", policy)
	}

	// The nonce in the header has to be the one in the markup, or the browser
	// refuses the script and the page renders blank.
	nonce := strings.TrimSuffix(strings.SplitN(policy, "nonce-", 2)[1], "' "+swaggerCDN)
	nonce = strings.SplitN(nonce, "'", 2)[0]

	if nonce == "" {
		t.Fatal("could not read the nonce back out of the policy")
	}

	if !strings.Contains(rec.Body.String(), `nonce="`+nonce+`"`) {
		t.Error("the nonce in the header does not match the one in the page")
	}

	if strings.Contains(rec.Body.String(), "{{nonce}}") {
		t.Error("the nonce placeholder was left in the page")
	}
}

// A nonce that repeats is one an attacker can reuse, which is the same as not
// having one.
func TestDocs_NonceChangesEveryTime(t *testing.T) {
	h := newSecurityHandler(t, nil)

	first := served(t, h, "/docs").Header().Get("Content-Security-Policy")
	second := served(t, h, "/docs").Header().Get("Content-Security-Policy")

	if first == second {
		t.Error("two responses carried the same nonce")
	}
}

type stubProber struct{ err error }

func (p stubProber) Ping(context.Context) error { return p.err }

// Readiness and liveness answer different questions, and the difference is the
// point: a database outage should stop traffic, not restart the process.
func TestReady_ReflectsTheDependency(t *testing.T) {
	tests := []struct {
		name   string
		prober Prober
		want   int
	}{
		{"no dependency to reach", nil, http.StatusOK},
		{"the database answers", stubProber{}, http.StatusOK},
		{"the database does not", stubProber{err: errors.New("connection refused")}, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSecurityHandler(t, tt.prober)

			if got := served(t, h, "/ready").Code; got != tt.want {
				t.Errorf("/ready = %d, want %d", got, tt.want)
			}

			// Liveness must not move with the dependency.
			if got := served(t, h, "/health").Code; got != http.StatusOK {
				t.Errorf("/health = %d, want %d regardless of the database", got, http.StatusOK)
			}
		})
	}
}

// A failing probe must not leak what the connection string says.
func TestReady_DoesNotLeakTheFailure(t *testing.T) {
	broken := errors.New("failed to connect to user=ticket password=hunter2 host=db.internal")

	body := served(t, newSecurityHandler(t, stubProber{err: broken}), "/ready").Body.String()

	for _, secret := range []string{"hunter2", "db.internal"} {
		if strings.Contains(body, secret) {
			t.Errorf("the answer leaks %q: %s", secret, body)
		}
	}
}
