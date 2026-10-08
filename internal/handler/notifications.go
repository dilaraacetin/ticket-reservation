package handler

import (
	"context"
	"net/http"
	"time"

	"ticket-reservation/internal/domain"
)

// NotificationService is the slice of the notifications this package needs.
type NotificationService interface {
	Notifications(ctx context.Context, userID string) ([]*domain.Notification, error)
	MarkNotificationsRead(ctx context.Context, userID string) (int, error)
}

type notificationResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	EventID   string    `json:"eventId"`
	EventName string    `json:"eventName"`
	SeatID    string    `json:"seatId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Read      bool      `json:"read"`
}

type notificationsResponse struct {
	// Unread is given separately so a page can show a count without having to
	// walk the list, which is the one thing it needs before anybody opens it.
	Unread        int                    `json:"unread"`
	Notifications []notificationResponse `json:"notifications"`
}

type markReadResponse struct {
	MarkedRead int `json:"markedRead"`
}

// notifications answers with the caller's own notices, newest first.
func (h *Handler) notifications(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	notices, err := h.notifier.Notifications(r.Context(), userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	body := notificationsResponse{Notifications: make([]notificationResponse, 0, len(notices))}

	for _, notice := range notices {
		if !notice.IsRead() {
			body.Unread++
		}

		body.Notifications = append(body.Notifications, notificationResponse{
			ID:        notice.ID,
			Kind:      notice.Kind.String(),
			EventID:   notice.EventID,
			EventName: notice.EventName,
			SeatID:    notice.SeatID,
			CreatedAt: notice.CreatedAt,
			Read:      notice.IsRead(),
		})
	}

	h.writeJSON(w, r, http.StatusOK, body)
}

// markNotificationsRead marks everything the caller has not seen.
//
// Takes no body and names no notice: it marks the caller's own, and a caller
// that could name one could mark somebody else's.
func (h *Handler) markNotificationsRead(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	marked, err := h.notifier.MarkNotificationsRead(r.Context(), userID)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	h.writeJSON(w, r, http.StatusOK, markReadResponse{MarkedRead: marked})
}
