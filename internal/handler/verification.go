package handler

import (
	"context"
	"net/http"
)

// VerificationService is the slice of the verification machinery this package
// needs.
type VerificationService interface {
	Verify(ctx context.Context, token string) error
	Resend(ctx context.Context, userID string) error
}

type verifyRequest struct {
	Token string `json:"token"`
}

// verifyEmail follows a link.
//
// A POST rather than a GET on the link itself, because a mailbox scanner or a
// link preview fetching a GET would spend a single-use token before the person
// ever clicked it. The page behind the link reads the token out of the query
// string and sends it here.
func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	body, err := decodeJSON[verifyRequest](w, r, h.bodyLimit)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	if err := h.verification.Verify(r.Context(), body.Token); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// resendVerification sends the caller a fresh link.
//
// It needs a token, so it can only ask for the caller's own address. An endpoint
// that took an address would send mail to anybody who was named.
func (h *Handler) resendVerification(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFrom(r)
	if err != nil {
		h.writeError(w, r, err)

		return
	}

	if err := h.verification.Resend(r.Context(), userID); err != nil {
		h.writeError(w, r, err)

		return
	}

	w.WriteHeader(http.StatusAccepted)
}
