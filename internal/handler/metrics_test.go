package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

// recordingObserver keeps what it was told, so a test can assert on it.
type recordingObserver struct {
	mu        sync.Mutex
	routes    []string
	statuses  []int
	failures  []string
	inFlight  int
	highWater int
}

func (o *recordingObserver) RecordRequest(_, route string, status int, _ time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.routes = append(o.routes, route)
	o.statuses = append(o.statuses, status)
}

func (o *recordingObserver) RecordFailure(code string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.failures = append(o.failures, code)
}

func (o *recordingObserver) RequestStarted() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.inFlight++
	o.highWater = max(o.highWater, o.inFlight)
}

func (o *recordingObserver) RequestFinished() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.inFlight--
}

// measured drives a request through the middleware and the real mux, because
// the route label only exists once the mux has matched something.
func measured(t *testing.T, svc ReservationService, method, target string, headers map[string]string) *recordingObserver {
	t.Helper()

	observer := &recordingObserver{}

	h := New(svc, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger())
	routes := h.Routes()

	root := Chain(routes,
		Measure(observer, routes),
		Authenticate(testTokenSigner, nil, fixedClock{now: testTime()}, discardLogger()),
	)

	req := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	root.ServeHTTP(httptest.NewRecorder(), req)

	return observer
}

// The reason this middleware reads r.Pattern instead of r.URL.Path. Labelling by
// path would mint a new time series for every hold id that has ever existed,
// which is how a metrics store is brought down by its own data.
func TestMeasure_LabelsByRouteNotByPath(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		want   string
	}{
		{
			"a hold id in the path",
			http.MethodDelete,
			"/holds/01JQZX9ABCDEF/",
			"DELETE /holds/{holdID}",
		},
		{
			"an event and a seat in the path",
			http.MethodGet,
			"/events/event-1/seats",
			"GET /events/{eventID}/seats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := strings.TrimSuffix(tt.target, "/")

			observer := measured(t, &fakeService{}, tt.method, target, withUser(testUser))

			if len(observer.routes) != 1 {
				t.Fatalf("recorded %d requests, want 1", len(observer.routes))
			}

			if got := observer.routes[0]; got != tt.want {
				t.Errorf("route label = %q, want %q", got, tt.want)
			}

			// The identifiers must not reach the label at all.
			for _, part := range []string{"01JQZX9ABCDEF", "event-1"} {
				if strings.Contains(observer.routes[0], part) {
					t.Errorf("route label %q carries the identifier %q", observer.routes[0], part)
				}
			}
		})
	}
}

// A request nothing matches still has to be counted, and still must not carry
// the path it asked for.
func TestMeasure_UnmatchedRequestsShareOneLabel(t *testing.T) {
	observer := measured(t, &fakeService{}, http.MethodGet, "/nothing/here/at/all", nil)

	if len(observer.routes) != 1 || observer.routes[0] != "unmatched" {
		t.Errorf("routes = %v, want one \"unmatched\"", observer.routes)
	}
}

// Status alone puts losing a race for a seat in with every other conflict. The
// code is what tells them apart, and it is what a load test is read by.
func TestMeasure_CountsFailuresByCode(t *testing.T) {
	observer := measured(t,
		&fakeService{err: domain.ErrSeatNotAvailable},
		http.MethodPost, "/events/event-1/seats/A1/hold", withUser(testUser),
	)

	if len(observer.failures) != 1 || observer.failures[0] != "seat_not_available" {
		t.Errorf("failures = %v, want one \"seat_not_available\"", observer.failures)
	}

	if len(observer.statuses) != 1 || observer.statuses[0] != http.StatusConflict {
		t.Errorf("statuses = %v, want one %d", observer.statuses, http.StatusConflict)
	}
}

// A refusal written by a middleware rather than a handler still has to be
// counted, which is why the code is noted in the one place that writes an error.
func TestMeasure_CountsFailuresFromMiddleware(t *testing.T) {
	observer := measured(t, &fakeService{}, http.MethodPost, "/events/event-1/seats/A1/hold", nil)

	if len(observer.failures) != 1 || observer.failures[0] != "unauthenticated" {
		t.Errorf("failures = %v, want one \"unauthenticated\"", observer.failures)
	}
}

// A request that succeeds is not a failure, however tempting it is to count
// everything.
func TestMeasure_SuccessIsNotCountedAsAFailure(t *testing.T) {
	observer := measured(t, &fakeService{}, http.MethodGet, "/health", nil)

	if len(observer.failures) != 0 {
		t.Errorf("failures = %v, want none", observer.failures)
	}
	if observer.inFlight != 0 {
		t.Errorf("in flight = %d after the request finished, want 0", observer.inFlight)
	}
	if observer.highWater != 1 {
		t.Errorf("in flight peaked at %d, want 1", observer.highWater)
	}
}

// A caller that hangs up mid request is not a fault of ours. Counting it as one
// is how a load test, or a flaky mobile network, fills the error budget with
// nothing.
func TestMeasure_ClientDisconnectIsNotAnInternalError(t *testing.T) {
	observer := measured(t,
		&fakeService{err: fmt.Errorf("beginning a transaction: %w", context.Canceled)},
		http.MethodPost, "/events/event-1/seats/A1/hold", withUser(testUser),
	)

	if len(observer.failures) != 1 || observer.failures[0] != "client_closed" {
		t.Errorf("failures = %v, want one \"client_closed\"", observer.failures)
	}

	if len(observer.statuses) != 1 || observer.statuses[0] != StatusClientClosedRequest {
		t.Errorf("statuses = %v, want one %d", observer.statuses, StatusClientClosedRequest)
	}
}

// A request the rate limiter turns away never reaches the mux. Labelling every
// refusal "unmatched" would hide exactly the route being hammered, which is the
// one a 429 is asked about.
func TestMeasure_LabelsRequestsThatNeverReachTheMux(t *testing.T) {
	observer := &recordingObserver{}

	h := New(&fakeService{}, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger())
	routes := h.Routes()

	// A middleware that refuses everything, standing where the rate limiter does.
	refuse := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeAPIError(w, r, discardLogger(), errTooManyRequests)
		})
	}

	root := Chain(routes, Measure(observer, routes), refuse)
	root.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodPost, "/holds/01JQZX9ABCDEF/confirm", nil))

	if len(observer.routes) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(observer.routes))
	}

	if got := observer.routes[0]; got != "POST /holds/{holdID}/confirm" {
		t.Errorf("route label = %q, want the pattern it would have matched", got)
	}

	if len(observer.failures) != 1 || observer.failures[0] != "too_many_requests" {
		t.Errorf("failures = %v, want one too_many_requests", observer.failures)
	}
}
