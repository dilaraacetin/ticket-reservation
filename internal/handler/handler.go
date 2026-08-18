// Package handler exposes the reservation service over HTTP. It owns the JSON
// shapes and the mapping from domain errors to status codes, and holds no rules
// of its own.
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"ticket-reservation/internal/domain"
)

// ReservationService is the slice of the service this package needs. It is
// declared here, on the consuming side, so that tests can supply a hand written
// fake and the service package stays unaware of HTTP.
type ReservationService interface {
	Events(ctx context.Context) ([]*domain.Event, error)
	SeatMap(ctx context.Context, eventID string) ([]*domain.Seat, error)
	HoldSeat(ctx context.Context, eventID, seatID, userID string) (*domain.Hold, error)
	ConfirmReservation(ctx context.Context, holdID, userID string) (*domain.Seat, error)
	ReleaseSeat(ctx context.Context, holdID, userID string) error
}

// Clock is the handler's own view of time, used only to describe state to
// clients: how long a hold has left, whether an event has begun.
type Clock interface {
	Now() time.Time
}

// Handler serves the API.
type Handler struct {
	service  ReservationService
	accounts AccountService
	clock    Clock
	logger   *slog.Logger
	web      http.Handler
	broker   Broker
}

// New returns a handler over the given service.
func New(service ReservationService, accounts AccountService, clock Clock, logger *slog.Logger) *Handler {
	return &Handler{service: service, accounts: accounts, clock: clock, logger: logger}
}

// WithBroker turns on the live update stream. Optional, so a handler can be
// built without one.
func (h *Handler) WithBroker(broker Broker) *Handler {
	h.broker = broker

	return h
}

// WithWeb mounts the browser interface at the root. Optional, so the tests can
// build a handler that is nothing but the API.
func (h *Handler) WithWeb(web http.Handler) *Handler {
	h.web = web

	return h
}

func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	if h.web != nil {
		mux.Handle("GET /", h.web)
	}

	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /docs", h.docs)
	mux.HandleFunc("GET "+specPath, h.openAPI)
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /events", h.listEvents)
	mux.HandleFunc("GET /events/{eventID}/seats", h.seatMap)
	mux.HandleFunc("GET /events/{eventID}/stream", h.stream)
	mux.HandleFunc("POST /events/{eventID}/seats/{seatID}/hold", h.holdSeat)
	mux.HandleFunc("POST /holds/{holdID}/confirm", h.confirmReservation)
	mux.HandleFunc("DELETE /holds/{holdID}", h.releaseSeat)

	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, r, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.Events(r.Context())
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	now := h.clock.Now()
	body := make([]eventResponse, 0, len(events))
	for _, event := range events {
		body = append(body, newEventResponse(event, now))
	}

	h.writeJSON(w, r, http.StatusOK, body)
}

func (h *Handler) seatMap(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")

	seats, err := h.service.SeatMap(r.Context(), eventID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	now := h.clock.Now()

	body := seatMapResponse{EventID: eventID, Seats: make([]seatResponse, 0, len(seats))}
	for _, seat := range seats {
		body.Seats = append(body.Seats, newSeatResponse(seat, now))
	}

	h.writeJSON(w, r, http.StatusOK, body)
}

func (h *Handler) holdSeat(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	hold, err := h.service.HoldSeat(r.Context(), r.PathValue("eventID"), r.PathValue("seatID"), userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	w.Header().Set("Location", "/holds/"+hold.ID)
	h.writeJSON(w, r, http.StatusCreated, newHoldResponse(hold, h.clock.Now()))
}

func (h *Handler) confirmReservation(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	seat, err := h.service.ConfirmReservation(r.Context(), r.PathValue("holdID"), userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, newReservationResponse(seat))
}

func (h *Handler) releaseSeat(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	if err := h.service.ReleaseSeat(r.Context(), r.PathValue("holdID"), userID); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func userIDFrom(r *http.Request) (string, error) {
	userID := UserIDFromContext(r.Context())
	if userID == "" {
		return "", errUnauthenticated
	}

	return userID, nil
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.logger.ErrorContext(r.Context(), "writing response failed",
			"err", err,
			"path", r.URL.Path,
		)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	writeAPIError(w, r, h.logger, err)
}

func writeAPIError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, code := statusForError(err)

	message := err.Error()
	if status == http.StatusInternalServerError {
		logger.ErrorContext(r.Context(), "request failed",
			"err", err,
			"method", r.Method,
			"path", r.URL.Path,
		)

		message = "internal error"
	}

	var body errorBody
	body.Error.Code = code
	body.Error.Message = message

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.ErrorContext(r.Context(), "writing the error response failed", "err", err)
	}
}
