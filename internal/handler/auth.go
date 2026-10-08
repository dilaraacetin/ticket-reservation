package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ticket-reservation/internal/auth"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// Verifier turns a token into what it says.
type Verifier interface {
	Verify(token string, now time.Time) (auth.Claims, error)
}

// RevocationCheck reports whether a token has been signed out. Optional: a
// chain built without one trusts every valid signature, which is what the
// service did before signing out existed.
type RevocationCheck interface {
	IsTokenRevoked(ctx context.Context, tokenID string) (bool, error)
}

// userKey is an unexported type so that no other package can reach or overwrite
// the authenticated user in a context.
type userKey struct{}

// claimsKey carries the whole of what the token said, which signing out needs:
// a token can only be refused by the id it carries.
type claimsKey struct{}

// Authenticate attaches the caller's identity to the request.
//
// It is deliberately permissive about a missing token: the seat map and the event
// list are public, and a middleware that refused every anonymous request would
// have to be told which paths those are. A token that is present must be valid,
// though, because a bad token is a caller trying something rather than a caller
// browsing.
//
// Refusing an anonymous request is then the handlers' job, through userIDFrom,
// which is the single place identity enters the system.
func Authenticate(verifier Verifier, revocations RevocationCheck, clock Clock, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, present := bearerToken(r)
			if !present {
				next.ServeHTTP(w, r)

				return
			}

			claims, err := verifier.Verify(token, clock.Now())
			if err != nil {
				logger.WarnContext(r.Context(), "rejected a token",
					"err", err,
					"path", r.URL.Path,
					"requestId", RequestIDFromContext(r.Context()),
				)

				writeAPIError(w, r, logger, errInvalidToken)

				return
			}

			if revocations != nil {
				revoked, err := revocations.IsTokenRevoked(r.Context(), claims.TokenID)
				if err != nil {
					// A check that cannot run refuses rather than waves through.
					// The tokens on that list are the whole reason it exists, and
					// a store that is unreachable fails nearly every other
					// request anyway, so failing closed here costs little and
					// guesses nothing.
					writeAPIError(w, r, logger, err)

					return
				}

				if revoked {
					logger.WarnContext(r.Context(), "rejected a signed out token",
						"userId", claims.UserID,
						"path", r.URL.Path,
						"requestId", RequestIDFromContext(r.Context()),
					)

					writeAPIError(w, r, logger, errInvalidToken)

					return
				}
			}

			ctx := context.WithValue(r.Context(), userKey{}, claims.UserID)
			ctx = context.WithValue(ctx, claimsKey{}, claims)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext returns what the caller's token said, if it had one.
func ClaimsFromContext(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(auth.Claims)

	return claims, ok
}

// UserIDFromContext returns the authenticated user, or an empty string.
func UserIDFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(userKey{}).(string)

	return userID
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get(authorizationHeader)
	if header == "" {
		return "", false
	}

	if len(header) < len(bearerPrefix) || !strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}

	token := strings.TrimSpace(header[len(bearerPrefix):])

	return token, token != ""
}
