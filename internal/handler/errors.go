package handler

import (
	"errors"
	"net/http"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// errUserIDRequired is the one failure the handler layer owns: the caller did not
// say who it is. Authentication would normally answer that, and until there is
// any, the X-User-ID header stands in for it.
var errUserIDRequired = errors.New("X-User-ID header is required")

// errorBody is the single shape every failure comes back in, so that clients can
// branch on code instead of matching on prose.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var errorMapping = []struct {
	err    error
	status int
	code   string
}{
	{repository.ErrEventNotFound, http.StatusNotFound, "event_not_found"},
	{repository.ErrSeatNotFound, http.StatusNotFound, "seat_not_found"},
	{repository.ErrHoldNotFound, http.StatusNotFound, "hold_not_found"},
	{domain.ErrSeatNotAvailable, http.StatusConflict, "seat_not_available"},
	{domain.ErrSeatNotHeld, http.StatusConflict, "seat_not_held"},
	{domain.ErrHoldExpired, http.StatusGone, "hold_expired"},
	{domain.ErrNotHoldOwner, http.StatusForbidden, "not_hold_owner"},
	{errUserIDRequired, http.StatusBadRequest, "user_id_required"},
	{domain.ErrEmptyUserID, http.StatusBadRequest, "user_id_required"},
	{domain.ErrEmptyHoldID, http.StatusBadRequest, "hold_id_required"},
	{domain.ErrInvalidHoldDuration, http.StatusBadRequest, "invalid_hold_duration"},
}

func statusForError(err error) (int, string) {
	for _, mapping := range errorMapping {
		if errors.Is(err, mapping.err) {
			return mapping.status, mapping.code
		}
	}

	return http.StatusInternalServerError, "internal_error"
}
