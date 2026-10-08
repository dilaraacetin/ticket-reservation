package handler

import (
	"context"
	"errors"
	"net/http"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
	"ticket-reservation/internal/service"
)

// errUserIDRequired is the one failure the handler layer owns: the caller did not
// say who it is. Authentication would normally answer that, and until there is
// any, the X-User-ID header stands in for it.
// Authentication failures. Both answer 401, and both say as little as possible:
// a caller told exactly what was wrong with its token learns how to improve it.
var (
	errUnauthenticated = errors.New("this endpoint requires a bearer token")

	errInvalidToken = errors.New("the bearer token is not valid")
)

// Request body failures, both of which are the caller's to fix.
var (
	errInvalidRequestBody = errors.New("the request body is not the expected JSON object")

	errRequestBodyTooLarge = errors.New("the request body is too large")
)

// StatusClientClosedRequest is nginx's code for a caller that hung up before
// being answered. Not in the standard, but widely understood, and the
// alternative is calling somebody else's dropped connection a fault of ours.
const StatusClientClosedRequest = 499

// errTooManyRequests means the caller has used up its allowance. The answer
// carries a Retry-After header saying when to come back.
var errTooManyRequests = errors.New("too many requests, slow down")

// errStreamUnavailable means the server cannot hold a connection open, which is
// a deployment problem rather than anything the caller did.
var errStreamUnavailable = errors.New("live updates are not available")

// errTicketNotFound covers both a code that does not exist and one that belongs
// to somebody else. One answer, because telling them apart would turn this into
// a way to check whether a ticket code is real.
var errTicketNotFound = errors.New("no such ticket")

// Idempotency failures. Both are the client's mistake or the client's timing, so
// neither says anything about the seat itself.
var (
	errIdempotencyKeyReused = errors.New("this idempotency key was already used for a different request")

	errIdempotencyInProgress = errors.New("a request with this idempotency key is still in progress")
)

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
	// First, because a cancelled request can surface as any of the failures
	// below and none of them is what actually happened. Counting these as
	// internal errors fills the dashboards with other people's flaky networks,
	// and logging them at error level is how an alert learns to be ignored.
	{context.Canceled, StatusClientClosedRequest, "client_closed"},

	{repository.ErrEventNotFound, http.StatusNotFound, "event_not_found"},
	{repository.ErrSeatNotFound, http.StatusNotFound, "seat_not_found"},
	{repository.ErrHoldNotFound, http.StatusNotFound, "hold_not_found"},
	{repository.ErrNotWaiting, http.StatusNotFound, "not_waiting"},
	{errTicketNotFound, http.StatusNotFound, "ticket_not_found"},
	{domain.ErrSeatNotAvailable, http.StatusConflict, "seat_not_available"},
	{domain.ErrSeatNotHeld, http.StatusConflict, "seat_not_held"},
	{domain.ErrHoldExpired, http.StatusGone, "hold_expired"},
	{domain.ErrNotHoldOwner, http.StatusForbidden, "not_hold_owner"},
	// 403, not 404: the caller is known and refused, which is a different thing
	// from the route not existing.
	{domain.ErrNotPermitted, http.StatusForbidden, "not_permitted"},
	{repository.ErrEventExists, http.StatusConflict, "event_exists"},
	{repository.ErrEventInUse, http.StatusConflict, "event_in_use"},
	{domain.ErrEventCancelled, http.StatusConflict, "event_cancelled"},
	{domain.ErrEventAlreadyCancelled, http.StatusConflict, "event_already_cancelled"},
	{domain.ErrInvalidEvent, http.StatusBadRequest, "invalid_event"},
	{domain.ErrInvalidSeatMap, http.StatusBadRequest, "invalid_seat_map"},
	{domain.ErrInvalidPushSubscription, http.StatusBadRequest, "invalid_push_subscription"},
	{service.ErrNotYourSubscription, http.StatusForbidden, "not_your_subscription"},
	// 410, not 404: the link existed and is spent or expired, and saying which
	// would tell somebody whether a token was ever real.
	{domain.ErrVerificationNotUsable, http.StatusGone, "verification_not_usable"},
	{service.ErrAlreadyVerified, http.StatusConflict, "already_verified"},
	// 403, not 401: we know who the caller is and the account is in good
	// standing — it just has not shown that its address is real yet.
	{domain.ErrEmailNotVerified, http.StatusForbidden, "email_not_verified"},
	{errIdempotencyInProgress, http.StatusConflict, "idempotency_in_progress"},
	{errIdempotencyKeyReused, http.StatusUnprocessableEntity, "idempotency_key_reused"},
	// 401 rather than 403: we do not know who the caller is, as opposed to
	// knowing and refusing them.
	{errUnauthenticated, http.StatusUnauthorized, "unauthenticated"},
	{errInvalidToken, http.StatusUnauthorized, "invalid_token"},

	// The address is taken: understood, well formed, and in conflict with what
	// already exists.
	{repository.ErrEmailTaken, http.StatusConflict, "email_taken"},

	// Deliberately one code for both a wrong password and an unknown address, so
	// that the answer does not reveal which addresses are registered.
	{service.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
	// 501, not 500: the deployment has not wired this up, which is a different
	// thing from something having gone wrong.
	{service.ErrLogoutUnavailable, http.StatusNotImplemented, "logout_unavailable"},

	{domain.ErrInvalidEmail, http.StatusBadRequest, "invalid_email"},
	{domain.ErrWeakPassword, http.StatusBadRequest, "weak_password"},
	{errInvalidRequestBody, http.StatusBadRequest, "invalid_request_body"},
	{errRequestBodyTooLarge, http.StatusRequestEntityTooLarge, "request_body_too_large"},
	{errTooManyRequests, http.StatusTooManyRequests, "too_many_requests"},
	{errStreamUnavailable, http.StatusServiceUnavailable, "stream_unavailable"},

	{domain.ErrEmptyUserID, http.StatusBadRequest, "user_id_required"},
	{domain.ErrEmptyHoldID, http.StatusBadRequest, "hold_id_required"},
	{domain.ErrEmptyEventID, http.StatusBadRequest, "event_id_required"},
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
