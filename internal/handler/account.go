package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/service"
)

// maxBodyBytes caps what a request may send. Without it a caller could stream
// gigabytes into a JSON decoder and take the process down with it.
// defaultBodyLimit caps what a request may send when nothing else says so.
const defaultBodyLimit = 8 << 10

// AccountService is the slice of the account service this package needs.
type AccountService interface {
	Account(ctx context.Context, userID string) (*domain.User, error)
	Register(ctx context.Context, email, password string) (*domain.User, error)
	Login(ctx context.Context, email, password string) (service.Session, error)
	Logout(ctx context.Context, tokenID string, expiresAt time.Time) error
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type sessionResponse struct {
	Token            string    `json:"token"`
	UserID           string    `json:"userId"`
	ExpiresAt        time.Time `json:"expiresAt"`
	ExpiresInSeconds int       `json:"expiresInSeconds"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	credentials, err := decodeJSON[credentialsRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	user, err := h.accounts.Register(r.Context(), credentials.Email, credentials.Password)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusCreated, userResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	credentials, err := decodeJSON[credentialsRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	session, err := h.accounts.Login(r.Context(), credentials.Email, credentials.Password)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, sessionResponse{
		Token:            session.Token,
		UserID:           session.UserID,
		ExpiresAt:        session.ExpiresAt,
		ExpiresInSeconds: int(time.Until(session.ExpiresAt).Seconds()),
	})
}

// decodeJSON reads a request body into T, refusing anything that is not exactly
// the expected shape.
//
// DisallowUnknownFields turns a typo into a 400 rather than a silently ignored
// field: a caller sending "passwrod" should be told, not left wondering why its
// password never arrived.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request, limit int64) (T, error) {
	var body T

	if limit <= 0 {
		limit = defaultBodyLimit
	}

	r.Body = http.MaxBytesReader(w, r.Body, limit)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&body); err != nil {
		return body, invalidBody(err)
	}

	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return body, errInvalidRequestBody
	}

	return body, nil
}

func invalidBody(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return errRequestBodyTooLarge
	}

	return errInvalidRequestBody
}

// logout refuses the caller's own token for whatever is left of its life.
//
// It takes no body and names no token: the one being signed out is the one the
// request arrived with. A caller that could name a token could sign out
// somebody else's.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		h.writeError(w, r, errUnauthenticated)

		return
	}

	if err := h.accounts.Logout(r.Context(), claims.TokenID, claims.ExpiresAt); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type accountResponse struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role"`

	// EmailVerified decides whether the interface offers seats or asks for the
	// address to be confirmed first, so it is part of knowing who you are rather
	// than something to be discovered by being refused.
	EmailVerified bool `json:"emailVerified"`
}

// account answers with who the caller is.
func (h *Handler) account(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	user, err := h.accounts.Account(r.Context(), userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, accountResponse{
		UserID:        user.ID,
		Email:         user.Email,
		Role:          user.Role.String(),
		EmailVerified: user.EmailVerified(),
	})
}
