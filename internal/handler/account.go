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
const maxBodyBytes = 8 << 10

// AccountService is the slice of the account service this package needs.
type AccountService interface {
	Register(ctx context.Context, email, password string) (*domain.User, error)
	Login(ctx context.Context, email, password string) (service.Session, error)
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
	credentials, err := decodeJSON[credentialsRequest](w, r)
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
	credentials, err := decodeJSON[credentialsRequest](w, r)
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
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var body T

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

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
