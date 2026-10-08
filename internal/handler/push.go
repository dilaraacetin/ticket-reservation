package handler

import (
	"context"
	"net/http"
)

// PushService is the slice of the push machinery this package needs.
type PushService interface {
	// PublicKey is what a browser needs before it can subscribe.
	PublicKey() string

	Subscribe(ctx context.Context, userID, endpoint, p256dh, auth string) error
	Unsubscribe(ctx context.Context, userID, endpoint string) error
}

type pushKeyResponse struct {
	PublicKey string `json:"publicKey"`
}

// subscribeRequest matches the shape a browser's PushSubscription serialises to,
// so the client can send what it was given rather than taking it apart first.
type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// pushKey hands out the public VAPID key.
//
// Open, because it is public by definition and a browser needs it before it can
// ask anybody for permission.
func (h *Handler) pushKey(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, r, http.StatusOK, pushKeyResponse{PublicKey: h.push.PublicKey()})
}

// subscribeToPush records a browser's permission to be pushed to.
func (h *Handler) subscribeToPush(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	body, err := decodeJSON[subscribeRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	err = h.push.Subscribe(r.Context(), userID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// unsubscribeFromPush forgets one browser.
//
// The endpoint is named in the body because a person has one per browser and
// this is the one saying stop; the caller may only forget their own.
func (h *Handler) unsubscribeFromPush(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	body, err := decodeJSON[unsubscribeRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	if err := h.push.Unsubscribe(r.Context(), userID, body.Endpoint); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}
