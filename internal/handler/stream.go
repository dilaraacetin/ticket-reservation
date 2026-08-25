package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ticket-reservation/internal/event"
)

// heartbeatInterval is how often a comment is sent down an idle stream.
const heartbeatInterval = 25 * time.Second

// Broker is the slice of the broker this package needs.
type Broker interface {
	Subscribe(eventID, userID string) *event.Subscriber
	Unsubscribe(subscriber *event.Subscriber)
}

// stream holds a connection open and pushes notices down it.
func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		h.writeError(w, r, errStreamUnavailable)

		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, r, errStreamUnavailable)

		return
	}

	eventID := r.PathValue("eventID")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	subscriber := h.broker.Subscribe(eventID, UserIDFromContext(r.Context()))
	defer h.broker.Unsubscribe(subscriber)

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}

			flusher.Flush()

		case notice, open := <-subscriber.Events:
			if !open {
				return
			}

			payload, err := json.Marshal(notice)
			if err != nil {
				h.logger.ErrorContext(r.Context(), "encoding a notice failed", "err", err)

				continue
			}

			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", notice.Kind, payload); err != nil {
				return
			}

			flusher.Flush()
		}
	}
}
