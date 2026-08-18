package handler

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Policy is how often a caller may do something.
type Policy struct {
	Name  string
	Every time.Duration
	Burst int
}

// Default policies. Signing in is far stricter than reading a seat map, because
// each attempt costs an Argon2id hash, and because unlimited guesses at a
// password are how passwords get guessed.
var (
	AuthPolicy = Policy{Name: "auth", Every: 6 * time.Second, Burst: 5}

	DefaultPolicy = Policy{Name: "default", Every: time.Second, Burst: 30}
)

// visitorTTL is how long an idle bucket is kept. Without eviction the map grows
// by one entry per address seen, for as long as the process runs.
const visitorTTL = 10 * time.Minute

// RateLimiter holds one token bucket per caller per policy.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	clock    Clock
	ttl      time.Duration
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter returns an empty limiter.
func NewRateLimiter(clock Clock, ttl time.Duration) *RateLimiter {
	if ttl <= 0 {
		ttl = visitorTTL
	}

	return &RateLimiter{
		visitors: make(map[string]*visitor),
		clock:    clock,
		ttl:      ttl,
	}
}

// Allow reports whether the caller may proceed, and how long to wait if not.
func (l *RateLimiter) Allow(policy Policy, key string) (bool, time.Duration) {
	now := l.clock.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bucketKey := policy.Name + "\x00" + key

	bucket, ok := l.visitors[bucketKey]
	if !ok {
		bucket = &visitor{limiter: rate.NewLimiter(rate.Every(policy.Every), policy.Burst)}
		l.visitors[bucketKey] = bucket
	}

	bucket.lastSeen = now

	reservation := bucket.limiter.ReserveN(now, 1)
	if !reservation.OK() {
		return false, policy.Every
	}

	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)

		return false, delay
	}

	return true, 0
}

// Cleanup drops buckets nobody has used lately, and reports how many went.
func (l *RateLimiter) Cleanup(now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()

	removed := 0

	for key, bucket := range l.visitors {
		if now.Sub(bucket.lastSeen) > l.ttl {
			delete(l.visitors, key)
			removed++
		}
	}

	return removed
}

// Size reports how many buckets are held, for tests and for a metric later.
func (l *RateLimiter) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	return len(l.visitors)
}

// Run evicts idle buckets until ctx is cancelled, in the same shape as the hold
// sweeper.
func (l *RateLimiter) Run(ctx context.Context, interval time.Duration, logger *slog.Logger) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if removed := l.Cleanup(l.clock.Now()); removed > 0 {
				logger.DebugContext(ctx, "evicted idle rate limit buckets", "count", removed)
			}
		}
	}
}

// RateLimit refuses callers that ask too often.
//
// It runs inside Authenticate so that a signed in caller is limited as itself
// rather than as whatever address it happens to be behind, which matters when
// several people share one office connection.
func RateLimit(limiter *RateLimiter, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			policy := policyFor(r)
			key := limitKey(r)

			allowed, retryAfter := limiter.Allow(policy, key)
			if allowed {
				next.ServeHTTP(w, r)

				return
			}

			logger.WarnContext(r.Context(), "rate limited a caller",
				"policy", policy.Name,
				"path", r.URL.Path,
				"retryAfter", retryAfter,
				"requestId", RequestIDFromContext(r.Context()),
			)

			seconds := int(retryAfter.Seconds())
			if retryAfter > time.Duration(seconds)*time.Second {
				seconds++
			}

			w.Header().Set("Retry-After", strconv.Itoa(max(seconds, 1)))
			writeAPIError(w, r, logger, errTooManyRequests)
		})
	}
}

// policyFor picks the allowance a request falls under.
func policyFor(r *http.Request) Policy {
	if strings.HasPrefix(r.URL.Path, "/auth/") {
		return AuthPolicy
	}

	return DefaultPolicy
}

// limitKey identifies the caller to limit.
func limitKey(r *http.Request) string {
	if userID := UserIDFromContext(r.Context()); userID != "" {
		return "user:" + userID
	}

	return "ip:" + clientIP(r)
}

// clientIP returns the address the connection came from.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
