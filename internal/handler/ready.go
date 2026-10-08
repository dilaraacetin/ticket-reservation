package handler

import (
	"context"
	"net/http"
	"time"
)

// readinessTimeout bounds the dependency check. A probe that hangs is worse than
// one that fails: whatever is asking has its own deadline, and a reply of "not
// yet" is useful where no reply at all is not.
const readinessTimeout = 2 * time.Second

// Prober reports whether a dependency can be reached. Declared here for the
// consumer, so nothing below the handler learns that health checks exist.
type Prober interface {
	Ping(ctx context.Context) error
}

type readyResponse struct {
	Status string `json:"status"`
	Store  string `json:"store"`
	Err    string `json:"err,omitempty"`
}

// ready says whether this instance can serve, which is a different question from
// whether it is alive.
//
// A load balancer takes an instance out of rotation when this fails; an
// orchestrator restarts one when /health fails. Checking the database here and
// deliberately not there is the whole point: a database outage should stop
// traffic reaching this instance, not start a restart loop that cannot fix
// anything and loses every in flight request on the way.
func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if h.prober == nil {
		h.writeJSON(w, r, http.StatusOK, readyResponse{Status: "ready", Store: "memory"})

		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := h.prober.Ping(ctx); err != nil {
		h.logger.WarnContext(r.Context(), "not ready to serve", "err", err)

		// The reason is named because this endpoint is read by whoever is already
		// inside the deployment, and "unavailable" on its own sends them to the
		// logs for something the answer could have told them.
		h.writeJSON(w, r, http.StatusServiceUnavailable, readyResponse{
			Status: "unavailable",
			Store:  "postgres",
			Err:    "the database could not be reached",
		})

		return
	}

	h.writeJSON(w, r, http.StatusOK, readyResponse{Status: "ready", Store: "postgres"})
}
