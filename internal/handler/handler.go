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
	"ticket-reservation/internal/service"
)

// ReservationService is the slice of the service this package needs. It is
// declared here, on the consuming side, so that tests can supply a hand written
// fake and the service package stays unaware of HTTP.
type ReservationService interface {
	Events(ctx context.Context) ([]*domain.Event, error)
	Event(ctx context.Context, eventID string) (*domain.Event, error)
	SeatMap(ctx context.Context, eventID string) ([]*domain.Seat, error)
	MyTickets(ctx context.Context, userID string) ([]service.Ticket, error)
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
	service      ReservationService
	accounts     AccountService
	waiting      WaitingListService
	inventory    InventoryService
	notifier     NotificationService
	push         PushService
	verification VerificationService
	roles        RoleReader
	clock        Clock
	logger       *slog.Logger
	web          http.Handler
	metrics      http.Handler
	broker       Broker
	prober       Prober
	bodyLimit    int64
}

// New returns a handler over the given service.
func New(service ReservationService, accounts AccountService, clock Clock, logger *slog.Logger) *Handler {
	return &Handler{service: service, accounts: accounts, clock: clock, logger: logger}
}

// WithBodyLimit caps what a request may send. Zero leaves the built in limit.
func (h *Handler) WithBodyLimit(limit int64) *Handler {
	h.bodyLimit = limit

	return h
}

// WithWaitingList turns on the queue endpoints. Optional, so a handler can be
// built without one.
func (h *Handler) WithWaitingList(waiting WaitingListService) *Handler {
	h.waiting = waiting

	return h
}

// WithNotifications turns on the notification endpoints. Optional, so a handler
// built without a store does not offer a list it cannot fill.
func (h *Handler) WithNotifications(notifier NotificationService) *Handler {
	h.notifier = notifier

	return h
}

// WithVerification turns on the address verification endpoints.
func (h *Handler) WithVerification(verification VerificationService) *Handler {
	h.verification = verification

	return h
}

// WithPush turns on the push endpoints. Optional, so a deployment with no VAPID
// keys does not offer a subscription it could never use.
func (h *Handler) WithPush(push PushService) *Handler {
	h.push = push

	return h
}

// WithAdmin turns on the inventory endpoints. Both parts are needed: without a
// way to read roles there is nothing to authorize against, so the routes are
// better absent than open.
func (h *Handler) WithAdmin(inventory InventoryService, roles RoleReader) *Handler {
	h.inventory = inventory
	h.roles = roles

	return h
}

// WithProber wires in the dependency check behind /ready. Without one the
// service has nothing to reach, which is the in-memory case.
func (h *Handler) WithProber(prober Prober) *Handler {
	h.prober = prober

	return h
}

// WithMetrics mounts the scrape endpoint. Optional, so a handler built for a
// test does not have to carry a registry.
func (h *Handler) WithMetrics(metrics http.Handler) *Handler {
	h.metrics = metrics

	return h
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

// Routes returns the API. The mux itself, because the metrics middleware uses
// it to name the route a request would match before anything gets the chance to
// turn that request away.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	if h.web != nil {
		mux.Handle("GET /", h.web)
	}

	// Liveness: is this process working at all. Deliberately answers without
	// touching anything it depends on, because restarting a healthy process over
	// somebody else's outage helps nobody.
	mux.HandleFunc("GET /health", h.health)

	// Readiness: should traffic be sent here right now.
	mux.HandleFunc("GET /ready", h.ready)

	// Open, like /health. In a real deployment this would be on an address the
	// internet cannot reach, because it says a good deal about what the process
	// is doing.
	if h.metrics != nil {
		mux.Handle("GET /metrics", h.metrics)
	}

	mux.HandleFunc("GET /docs", h.docs)
	mux.HandleFunc("GET "+specPath, h.openAPI)
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/logout", h.logout)

	if h.verification != nil {
		mux.HandleFunc("POST /auth/verify", h.verifyEmail)
		mux.HandleFunc("POST /auth/verify/resend", h.resendVerification)
	}
	mux.HandleFunc("GET /events", h.listEvents)
	mux.HandleFunc("GET /events/{eventID}", h.showEvent)
	mux.HandleFunc("GET /events/{eventID}/seats", h.seatMap)
	mux.HandleFunc("GET /events/{eventID}/stream", h.stream)

	if h.waiting != nil {
		mux.HandleFunc("GET /events/{eventID}/waiting-list", h.waitingListPosition)
		mux.HandleFunc("POST /events/{eventID}/waiting-list", h.joinWaitingList)
		mux.HandleFunc("DELETE /events/{eventID}/waiting-list", h.leaveWaitingList)
	}

	mux.HandleFunc("POST /events/{eventID}/seats/{seatID}/hold", h.holdSeat)
	// What the caller has: the seats they are holding right now and the ones
	// they have confirmed. A page that is reloaded mid hold recovers from here.
	mux.HandleFunc("GET /account", h.account)
	mux.HandleFunc("GET /tickets", h.myTickets)
	mux.HandleFunc("GET /tickets/{ticketCode}/qr.png", h.ticketQR)

	if h.notifier != nil {
		mux.HandleFunc("GET /notifications", h.notifications)
		mux.HandleFunc("POST /notifications/read", h.markNotificationsRead)
	}

	if h.push != nil {
		mux.HandleFunc("GET /push/key", h.pushKey)
		mux.HandleFunc("POST /push/subscriptions", h.subscribeToPush)
		mux.HandleFunc("DELETE /push/subscriptions", h.unsubscribeFromPush)
	}

	mux.HandleFunc("POST /holds/{holdID}/confirm", h.confirmReservation)
	mux.HandleFunc("DELETE /holds/{holdID}", h.releaseSeat)

	if h.inventory != nil && h.roles != nil {
		mux.Handle("POST /admin/events", h.requireAdmin(h.createEvent))
		mux.Handle("POST /admin/events/{eventID}/seats", h.requireAdmin(h.addSeats))
		mux.Handle("PATCH /admin/events/{eventID}", h.requireAdmin(h.updateEvent))
		mux.Handle("POST /admin/events/{eventID}/cancel", h.requireAdmin(h.cancelEvent))
		mux.Handle("DELETE /admin/events/{eventID}", h.requireAdmin(h.deleteEvent))
	}

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

// showEvent answers with one event and how much of it is left.
//
// The counts are made here rather than left to the caller, because the only way
// to count from outside is to fetch every seat, and a hall holds thousands of
// them for a number the page shows in two words.
func (h *Handler) showEvent(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")

	event, err := h.service.Event(r.Context(), eventID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	seats, err := h.service.SeatMap(r.Context(), eventID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	now := h.clock.Now()

	body := eventDetailResponse{eventResponse: newEventResponse(event, now), Total: len(seats)}
	for _, seat := range seats {
		if seat.Status == domain.StatusAvailable || seat.IsHoldExpired(now) {
			body.Available++
		}
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

// myTickets answers with the caller's own seats, held and reserved.
func (h *Handler) myTickets(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	tickets, err := h.service.MyTickets(r.Context(), userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	now := h.clock.Now()
	body := ticketsResponse{Holds: []ticketResponse{}, Reservations: []ticketResponse{}}

	for _, ticket := range tickets {
		// An expired hold is nobody's: the sweeper has simply not been round
		// yet, and showing it would promise a seat that is already free.
		if ticket.Seat.Status == domain.StatusHeld && !ticket.Seat.IsHoldExpired(now) {
			body.Holds = append(body.Holds, newTicketResponse(ticket, now))

			continue
		}

		if ticket.Seat.Status == domain.StatusReserved {
			body.Reservations = append(body.Reservations, newTicketResponse(ticket, now))
		}
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

	noteFailure(r.Context(), code)

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
