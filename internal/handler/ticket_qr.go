package handler

import (
	"net/http"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	// qrSize is the rendered width in pixels. Big enough that a scanner reads it
	// off a phone held at arm's length, small enough to send on a slow
	// connection at the door.
	qrSize = 512

	// qrCacheSeconds is how long a browser may keep one. A ticket code never
	// changes, so this is only ever re-fetched because somebody cleared it.
	qrCacheSeconds = "86400"
)

// ticketQR renders the caller's ticket code as an image.
//
// Rendered here rather than in the browser, because encoding a QR is error
// correction and bit placement, and a page that gets it subtly wrong produces a
// code that scans everywhere except at the door.
//
// Only the caller's own: the code is looked for among their tickets, so knowing
// somebody else's gets a 404 rather than their ticket.
func (h *Handler) ticketQR(w http.ResponseWriter, r *http.Request) {
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

	wanted := r.PathValue("ticketCode")

	var found bool

	for _, ticket := range tickets {
		if ticket.Seat.TicketCode == wanted && wanted != "" {
			found = true

			break
		}
	}

	if !found {
		h.writeError(w, r, errTicketNotFound)

		return
	}

	// Medium correction: enough to survive a fingerprint on a screen without
	// making the pattern so dense that a cheap scanner struggles.
	png, err := qrcode.Encode(wanted, qrcode.Medium, qrSize)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age="+qrCacheSeconds)

	if _, err := w.Write(png); err != nil {
		h.logger.ErrorContext(r.Context(), "writing a ticket image failed", "err", err)
	}
}
