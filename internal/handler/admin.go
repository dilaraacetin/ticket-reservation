package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"ticket-reservation/internal/domain"
)

// InventoryService is the slice of the inventory this package needs.
type InventoryService interface {
	CreateEvent(
		ctx context.Context, name, venue string, startsAt time.Time, details domain.EventDetails,
	) (*domain.Event, error)
	// details is given the event's current details and returns what they should
	// become, so that reading them and writing them stay one step.
	UpdateEvent(
		ctx context.Context, eventID, name, venue string, startsAt time.Time,
		details func(domain.EventDetails) domain.EventDetails,
	) (*domain.Event, error)
	CancelEvent(ctx context.Context, eventID string) (*domain.Event, error)
	DeleteEvent(ctx context.Context, eventID string) error
	AddSeats(ctx context.Context, eventID string, rows []string, perRow int) (int, error)
}

// RoleReader reports what an account may do.
type RoleReader interface {
	Role(ctx context.Context, userID string) (domain.Role, error)
}

// requireAdmin refuses anyone who is not an administrator.
//
// Wrapped around the individual routes rather than added to the middleware
// chain, because a chain entry protects everything after it and the order it
// sits in becomes a thing nobody can safely change. Here the protection is
// written next to the route it protects.
func (h *Handler) requireAdmin(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := userIDFrom(r)
		if err != nil {
			h.writeError(w, r, err)

			return
		}

		role, err := h.roles.Role(r.Context(), userID)
		if err != nil {
			// An unknown id in a validly signed token means the account is gone,
			// which is a refusal rather than a server fault. The cause is logged
			// and not joined onto the answer: only 500s have their message
			// replaced, so a joined error would send whatever the store said —
			// connection string included — straight to the caller.
			h.logger.WarnContext(r.Context(), "could not read the caller's role",
				"err", err,
				"userId", userID,
				"path", r.URL.Path,
			)

			h.writeError(w, r, domain.ErrNotPermitted)

			return
		}

		if !role.IsAdmin() {
			h.logger.WarnContext(r.Context(), "refused an administrative request",
				"userId", userID,
				"role", role.String(),
				"path", r.URL.Path,
			)

			h.writeError(w, r, domain.ErrNotPermitted)

			return
		}

		next(w, r)
	})
}

// eventDetailsRequest is the descriptive half of an event, shared by create and
// edit so the two cannot drift apart.
//
// Pointers, so that a field left out of the body and a field sent empty are
// different requests. Without that an edit of the start time would have to
// repeat the description or lose it, and a description could never be cleared.
type eventDetailsRequest struct {
	City        *string `json:"city"`
	Category    *string `json:"category"`
	ImageURL    *string `json:"imageUrl"`
	Description *string `json:"description"`
	Rules       *string `json:"rules"`
}

// applyTo returns current with the fields this request carried replaced.
func (b eventDetailsRequest) applyTo(current domain.EventDetails) domain.EventDetails {
	for _, field := range []struct {
		sent *string
		into *string
	}{
		{b.City, &current.City},
		{b.ImageURL, &current.ImageURL},
		{b.Description, &current.Description},
		{b.Rules, &current.Rules},
	} {
		if field.sent != nil {
			*field.into = strings.TrimSpace(*field.sent)
		}
	}

	// Its own type, so it cannot join the loop above. Whether the value names a
	// real category is the domain's question, not this one's.
	if b.Category != nil {
		current.Category = domain.Category(strings.TrimSpace(*b.Category))
	}

	return current
}

type createEventRequest struct {
	Name     string    `json:"name"`
	Venue    string    `json:"venue"`
	StartsAt time.Time `json:"startsAt"`

	eventDetailsRequest
}

type addSeatsRequest struct {
	Rows        []string `json:"rows"`
	SeatsPerRow int      `json:"seatsPerRow"`
}

type addSeatsResponse struct {
	EventID string `json:"eventId"`
	Created int    `json:"created"`
}

// createEvent stores a new event and answers with the id it was given.
func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[createEventRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	event, err := h.inventory.CreateEvent(
		r.Context(), body.Name, body.Venue, body.StartsAt, body.applyTo(domain.EventDetails{}))
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	w.Header().Set("Location", "/events/"+event.ID)
	h.writeJSON(w, r, http.StatusCreated, newEventResponse(event, h.clock.Now()))
}

// addSeats adds a block of rows to an event.
func (h *Handler) addSeats(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[addSeatsRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	eventID := r.PathValue("eventID")

	created, err := h.inventory.AddSeats(r.Context(), eventID, body.Rows, body.SeatsPerRow)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusCreated, addSeatsResponse{EventID: eventID, Created: created})
}

// updateEventRequest leaves the name, venue and start time optional: a caller
// fixing a typo in the venue should not have to repeat them, and repeating them
// is how one of them gets sent back stale.
// The descriptive fields are optional in their own way: see
// eventDetailsRequest for why they are pointers.
type updateEventRequest struct {
	Name     string    `json:"name"`
	Venue    string    `json:"venue"`
	StartsAt time.Time `json:"startsAt"`

	eventDetailsRequest
}

// updateEvent changes an event in place.
func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[updateEventRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	event, err := h.inventory.UpdateEvent(
		r.Context(), r.PathValue("eventID"), body.Name, body.Venue, body.StartsAt, body.applyTo)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, newEventResponse(event, h.clock.Now()))
}

// cancelEvent withdraws an event from sale. It takes no body: there is one thing
// to say and the path already says which event.
func (h *Handler) cancelEvent(w http.ResponseWriter, r *http.Request) {
	event, err := h.inventory.CancelEvent(r.Context(), r.PathValue("eventID"))
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, newEventResponse(event, h.clock.Now()))
}

// deleteEvent removes an event outright, and only while nothing is held or sold.
func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	if err := h.inventory.DeleteEvent(r.Context(), r.PathValue("eventID")); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
