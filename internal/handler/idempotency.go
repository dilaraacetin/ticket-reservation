package handler

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"time"

	"ticket-reservation/internal/repository"
)

const (
	idempotencyKeyHeader   = "Idempotency-Key"
	idempotentReplayHeader = "Idempotent-Replay"
)

// IdempotencyStore is the slice of the repository this middleware needs.
type IdempotencyStore interface {
	Claim(ctx context.Context, record repository.IdempotencyRecord) (repository.IdempotencyRecord, bool, error)
	Complete(ctx context.Context, userID, key string, statusCode int, response []byte) error
	Release(ctx context.Context, userID, key string) error
}

// Idempotency makes a retried request safe. A caller that repeats a request with
// the same Idempotency-Key gets the first attempt's answer back instead of the
// work being done twice.
//
// This is what fixes the behaviour the service tests already document: a repeated
// confirm currently reports 404, because the first confirm consumed the hold, and
// the caller cannot tell that from a hold that never existed.
//
// Only unsafe methods are covered, and only when the caller asks for it by
// sending a key. A GET is already repeatable.
func Idempotency(store IdempotencyStore, clock Clock, ttl time.Duration, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(idempotencyKeyHeader)
			userID := UserIDFromContext(r.Context())

			if key == "" || userID == "" || !isRepeatable(r.Method) {
				next.ServeHTTP(w, r)

				return
			}

			now := clock.Now()

			fingerprint := r.Method + " " + r.URL.Path

			existing, claimed, err := store.Claim(r.Context(), repository.IdempotencyRecord{
				UserID:      userID,
				Key:         key,
				Fingerprint: fingerprint,
				CreatedAt:   now,
				ExpiresAt:   now.Add(ttl),
			})
			if err != nil {

				writeAPIError(w, r, logger, err)

				return
			}

			if !claimed {
				replay(w, r, logger, existing, fingerprint)

				return
			}

			recorder := &recordingWriter{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			finish(r.Context(), store, logger, userID, key, recorder)
		})
	}
}

// finish remembers the answer, or gives the key up when the answer is not worth
// remembering.
func finish(
	ctx context.Context,
	store IdempotencyStore,
	logger *slog.Logger,
	userID, key string,
	recorder *recordingWriter,
) {
	if recorder.status >= http.StatusInternalServerError {
		if err := store.Release(ctx, userID, key); err != nil {
			logger.ErrorContext(ctx, "releasing an idempotency key failed", "err", err, "key", key)
		}

		return
	}

	if err := store.Complete(ctx, userID, key, recorder.status, recorder.body.Bytes()); err != nil {
		logger.ErrorContext(ctx, "storing an idempotency result failed", "err", err, "key", key)
	}
}

// replay answers from the record that already holds the key.
func replay(
	w http.ResponseWriter,
	r *http.Request,
	logger *slog.Logger,
	record repository.IdempotencyRecord,
	fingerprint string,
) {
	if record.Fingerprint != fingerprint {
		writeAPIError(w, r, logger, errIdempotencyKeyReused)

		return
	}

	if !record.IsCompleted() {
		writeAPIError(w, r, logger, errIdempotencyInProgress)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(idempotentReplayHeader, "true")
	w.WriteHeader(record.StatusCode)

	if _, err := w.Write(record.Response); err != nil {
		logger.ErrorContext(r.Context(), "replaying an idempotent response failed", "err", err)
	}
}

func isRepeatable(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// recordingWriter keeps a copy of what was written. The response still goes out
// immediately; the copy is only what gets remembered for a later replay.
type recordingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *recordingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	w.body.Write(b)

	return w.ResponseWriter.Write(b)
}
