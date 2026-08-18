package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// Verifier turns a token into the user it names.
type Verifier interface {
	Verify(token string, now time.Time) (string, error)
}

// userKey is an unexported type so that no other package can reach or overwrite
// the authenticated user in a context.
type userKey struct{}

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
func Authenticate(verifier Verifier, clock Clock, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, present := bearerToken(r)
			if !present {
				next.ServeHTTP(w, r)

				return
			}

			userID, err := verifier.Verify(token, clock.Now())
			if err != nil {
				logger.WarnContext(r.Context(), "rejected a token",
					"err", err,
					"path", r.URL.Path,
					"requestId", RequestIDFromContext(r.Context()),
				)

				writeAPIError(w, r, logger, errInvalidToken)

				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, userID)))
		})
	}
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
