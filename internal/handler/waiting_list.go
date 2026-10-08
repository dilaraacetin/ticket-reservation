package handler

import (
	"context"
	"net/http"
)

// WaitingListService is the slice of the waiting list this package needs,
// declared here on the consuming side like the rest.
type WaitingListService interface {
	Join(ctx context.Context, eventID, userID string) (int, error)
	Leave(ctx context.Context, eventID, userID string) error
	Position(ctx context.Context, eventID, userID string) (int, error)
}

type waitingListResponse struct {
	EventID  string `json:"eventId"`
	UserID   string `json:"userId"`
	Position int    `json:"position"`
}

// joinWaitingList puts the caller in the queue for an event.
//
// It answers 200 rather than 201 because joining twice is the same request made
// twice: the caller ends up in the queue either way, in the place they already
// had.
func (h *Handler) joinWaitingList(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	eventID := r.PathValue("eventID")

	position, err := h.waiting.Join(r.Context(), eventID, userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, waitingListResponse{
		EventID:  eventID,
		UserID:   userID,
		Position: position,
	})
}

// leaveWaitingList gives up the caller's place. It answers 204 whether or not
// they were in the queue, since either way they are out of it now.
func (h *Handler) leaveWaitingList(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	if err := h.waiting.Leave(r.Context(), r.PathValue("eventID"), userID); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// waitingListPosition reports where the caller stands.
func (h *Handler) waitingListPosition(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	eventID := r.PathValue("eventID")

	position, err := h.waiting.Position(r.Context(), eventID, userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, waitingListResponse{
		EventID:  eventID,
		UserID:   userID,
		Position: position,
	})
}
