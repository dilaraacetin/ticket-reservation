package handler

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// movingClock lets a test spend time without waiting for it.
type movingClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMovingClock(now time.Time) *movingClock {
	return &movingClock{now: now}
}

func (c *movingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *movingClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

// limited wraps a handler that counts how often it actually ran.
func limited(clock Clock, next *countingHandler) http.Handler {
	return RateLimit(NewRateLimiter(clock, time.Minute), discardLogger())(next)
}

func callFrom(handler http.Handler, target, remoteAddr, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.RemoteAddr = remoteAddr

	if authorization != "" {
		req.Header.Set(authorizationHeader, authorization)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

func TestRateLimit_AllowsTheBurstThenRefuses(t *testing.T) {
	clock := newMovingClock(testTime())
	next := &countingHandler{status: http.StatusOK, body: `{}`}
	handler := limited(clock, next)

	// The burst is what may arrive at once.
	for i := range AuthPolicy.Burst {
		if rec := callFrom(handler, "/auth/login", "203.0.113.7:1234", ""); rec.Code != http.StatusOK {
			t.Fatalf("request %d was refused with %d, want it inside the burst", i+1, rec.Code)
		}
	}

	rec := callFrom(handler, "/auth/login", "203.0.113.7:1234", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d once the burst is used up", rec.Code, http.StatusTooManyRequests)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "too_many_requests" {
		t.Errorf("code = %q, want too_many_requests", code)
	}

	// The refused request must not have reached the handler, which is the whole
	// point when each one costs an Argon2id hash.
	if got := next.calls.Load(); got != int32(AuthPolicy.Burst) {
		t.Errorf("the handler ran %d times, want %d", got, AuthPolicy.Burst)
	}
}

// A refusal has to say when to come back, or a client can only guess.
func TestRateLimit_SaysWhenToRetry(t *testing.T) {
	clock := newMovingClock(testTime())
	handler := limited(clock, &countingHandler{status: http.StatusOK, body: `{}`})

	for range AuthPolicy.Burst {
		callFrom(handler, "/auth/login", "203.0.113.7:1234", "")
	}

	rec := callFrom(handler, "/auth/login", "203.0.113.7:1234", "")

	header := rec.Header().Get("Retry-After")
	if header == "" {
		t.Fatal("no Retry-After header on a refusal")
	}

	seconds, err := strconv.Atoi(header)
	if err != nil {
		t.Fatalf("Retry-After = %q, want a number of seconds", header)
	}
	if seconds < 1 {
		t.Errorf("Retry-After = %d, want at least 1", seconds)
	}
}

// The allowance refills over time rather than resetting on a fixed boundary.
func TestRateLimit_RefillsOverTime(t *testing.T) {
	clock := newMovingClock(testTime())
	handler := limited(clock, &countingHandler{status: http.StatusOK, body: `{}`})

	for range AuthPolicy.Burst {
		callFrom(handler, "/auth/login", "203.0.113.7:1234", "")
	}

	if rec := callFrom(handler, "/auth/login", "203.0.113.7:1234", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want the burst to be used up", rec.Code)
	}

	clock.Advance(AuthPolicy.Every)

	if rec := callFrom(handler, "/auth/login", "203.0.113.7:1234", ""); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want one token to have refilled", rec.Code)
	}

	// And only one: the bucket refills at the policy's rate, not all at once.
	if rec := callFrom(handler, "/auth/login", "203.0.113.7:1234", ""); rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want the refill to be one token", rec.Code)
	}
}

// One caller using up its allowance must not affect anybody else.
func TestRateLimit_KeepsCallersApart(t *testing.T) {
	clock := newMovingClock(testTime())
	handler := limited(clock, &countingHandler{status: http.StatusOK, body: `{}`})

	for range AuthPolicy.Burst + 1 {
		callFrom(handler, "/auth/login", "203.0.113.7:1234", "")
	}

	if rec := callFrom(handler, "/auth/login", "198.51.100.4:5678", ""); rec.Code != http.StatusOK {
		t.Errorf("a second address was refused with %d", rec.Code)
	}
}

// Signing in is limited far more tightly than reading, so a burst of ordinary
// requests must not be cut off at the sign in allowance.
func TestRateLimit_SeparatePolicies(t *testing.T) {
	clock := newMovingClock(testTime())
	handler := limited(clock, &countingHandler{status: http.StatusOK, body: `{}`})

	for range AuthPolicy.Burst + 1 {
		callFrom(handler, "/auth/login", "203.0.113.7:1234", "")
	}

	// The same address, past its sign in allowance, still has its ordinary one.
	if rec := callFrom(handler, "/events/event-1/seats/A1/hold", "203.0.113.7:1234", ""); rec.Code != http.StatusOK {
		t.Errorf("an ordinary request was refused with %d, want the policies to be separate", rec.Code)
	}
}

// A signed in caller is limited as itself, so changing address does not hand it a
// fresh allowance.
func TestRateLimit_SignedInCallersAreLimitedByAccount(t *testing.T) {
	clock := newMovingClock(testTime())
	next := &countingHandler{status: http.StatusOK, body: `{}`}

	// Authenticate first, so the limiter can see who is calling.
	handler := Chain(next,
		Authenticate(testTokenSigner, fixedClock{now: testTime()}, discardLogger()),
		RateLimit(NewRateLimiter(clock, time.Minute), discardLogger()),
	)

	token := bearerForUser("usr_00000001")

	for range DefaultPolicy.Burst {
		callFrom(handler, "/events/event-1/seats/A1/hold", "203.0.113.7:1234", token)
	}

	// Same account, different address: still refused.
	rec := callFrom(handler, "/events/event-1/seats/A1/hold", "198.51.100.4:5678", token)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want the account's allowance to follow it", rec.Code)
	}

	// A different account from the same address is unaffected.
	other := bearerForUser("usr_00000002")
	if rec := callFrom(handler, "/events/event-1/seats/A1/hold", "203.0.113.7:1234", other); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want another account to have its own allowance", rec.Code)
	}
}

// Without eviction the map grows by one entry per address ever seen.
func TestRateLimiter_EvictsIdleBuckets(t *testing.T) {
	clock := newMovingClock(testTime())
	limiter := NewRateLimiter(clock, time.Minute)

	for i := range 5 {
		limiter.Allow(DefaultPolicy, "ip:203.0.113."+strconv.Itoa(i))
	}

	if got := limiter.Size(); got != 5 {
		t.Fatalf("size = %d, want 5", got)
	}

	// Not yet idle enough.
	if removed := limiter.Cleanup(clock.Now().Add(30 * time.Second)); removed != 0 {
		t.Errorf("evicted %d buckets early", removed)
	}

	if removed := limiter.Cleanup(clock.Now().Add(2 * time.Minute)); removed != 5 {
		t.Errorf("evicted %d buckets, want 5", removed)
	}
	if got := limiter.Size(); got != 0 {
		t.Errorf("size = %d, want 0", got)
	}
}

// A bucket still in use must survive the eviction pass.
func TestRateLimiter_KeepsBucketsInUse(t *testing.T) {
	clock := newMovingClock(testTime())
	limiter := NewRateLimiter(clock, time.Minute)

	limiter.Allow(DefaultPolicy, "ip:203.0.113.1")

	clock.Advance(2 * time.Minute)
	limiter.Allow(DefaultPolicy, "ip:203.0.113.1")

	if removed := limiter.Cleanup(clock.Now()); removed != 0 {
		t.Errorf("evicted %d buckets that were just used", removed)
	}
}

// The limiter is shared by every request, so its bookkeeping has to hold up under
// concurrent use.
func TestRateLimiter_IsSafeUnderConcurrentUse(t *testing.T) {
	const callers = 50

	clock := newMovingClock(testTime())
	limiter := NewRateLimiter(clock, time.Minute)

	var (
		wg      sync.WaitGroup
		allowed atomic.Int32
	)

	start := make(chan struct{})

	for range callers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			// One key, so every goroutine contends for the same bucket.
			if ok, _ := limiter.Allow(AuthPolicy, "ip:203.0.113.7"); ok {
				allowed.Add(1)
			}
		}()
	}

	close(start)
	wg.Wait()

	// Exactly the burst gets through, no more and no fewer.
	if got := allowed.Load(); got != int32(AuthPolicy.Burst) {
		t.Errorf("allowed = %d, want exactly the burst of %d", got, AuthPolicy.Burst)
	}
}
