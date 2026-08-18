package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ticket-reservation/internal/repository"
)

const testKeyTTL = 24 * time.Hour

type countingHandler struct {
	status int
	body   string
	calls  atomic.Int32
}

func (h *countingHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.calls.Add(1)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.status)
	_, _ = w.Write([]byte(h.body))
}

func withIdempotency(t *testing.T, next http.Handler, store IdempotencyStore) http.Handler {
	t.Helper()

	return Idempotency(store, fixedClock{now: testTime()}, testKeyTTL, discardLogger())(next)
}

func newRequest(method, target, userID, key string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	if userID != "" {
		r.Header.Set(userIDHeader, userID)
	}
	if key != "" {
		r.Header.Set(idempotencyKeyHeader, key)
	}

	return r
}

func TestIdempotency_RetryReplaysTheFirstAnswer(t *testing.T) {
	next := &countingHandler{status: http.StatusOK, body: `{"seatId":"A1","status":"reserved"}`}
	handler := withIdempotency(t, next, repository.NewMemoryIdempotencyRepository())

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	if got := next.calls.Load(); got != 1 {
		t.Errorf("the handler ran %d times, want 1", got)
	}
	if first.Code != http.StatusOK || second.Code != http.StatusOK {
		t.Errorf("statuses = %d and %d, want both 200", first.Code, second.Code)
	}
	if first.Body.String() != second.Body.String() {
		t.Errorf("bodies differ:\n first  = %q\n second = %q", first.Body, second.Body)
	}

	if got := second.Header().Get(idempotentReplayHeader); got != "true" {
		t.Errorf("%s = %q, want true on the replay", idempotentReplayHeader, got)
	}
	if got := first.Header().Get(idempotentReplayHeader); got != "" {
		t.Errorf("%s = %q, want it absent on the first answer", idempotentReplayHeader, got)
	}
}

func TestIdempotency_RepeatedConfirmNoLongerReports404(t *testing.T) {
	var calls atomic.Int32

	confirm := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"reserved"}`))

			return
		}

		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"hold_not_found"}}`))
	})

	handler := withIdempotency(t, confirm, repository.NewMemoryIdempotencyRepository())

	var last *httptest.ResponseRecorder
	for range 3 {
		last = httptest.NewRecorder()
		handler.ServeHTTP(last, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("the confirm ran %d times, want 1", got)
	}
	if last.Code != http.StatusOK {
		t.Errorf("last status = %d, want 200 rather than the 404 a bare retry would get", last.Code)
	}
	if !strings.Contains(last.Body.String(), "reserved") {
		t.Errorf("body = %q, want the first answer", last.Body)
	}
}

func TestIdempotency_PassesThroughWhenNotAsked(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		userID  string
		key     string
		wantRun int32
	}{
		{"no key at all", http.MethodPost, testUser, "", 2},
		{"no user to scope the key to", http.MethodPost, "", "k1", 2},
		{"a GET is already repeatable", http.MethodGet, testUser, "k1", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &countingHandler{status: http.StatusOK, body: `{}`}
			handler := withIdempotency(t, next, repository.NewMemoryIdempotencyRepository())

			for range 2 {
				handler.ServeHTTP(httptest.NewRecorder(), newRequest(tt.method, "/events", tt.userID, tt.key))
			}

			if got := next.calls.Load(); got != tt.wantRun {
				t.Errorf("the handler ran %d times, want %d", got, tt.wantRun)
			}
		})
	}
}

func TestIdempotency_KeyReusedForAnotherRequestIs422(t *testing.T) {
	next := &countingHandler{status: http.StatusOK, body: `{}`}
	handler := withIdempotency(t, next, repository.NewMemoryIdempotencyRepository())

	handler.ServeHTTP(httptest.NewRecorder(), newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest(http.MethodPost, "/holds/h2/confirm", testUser, "k1"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "idempotency_key_reused" {
		t.Errorf("code = %q, want idempotency_key_reused", code)
	}
	if got := next.calls.Load(); got != 1 {
		t.Errorf("the handler ran %d times, want 1", got)
	}
}

func TestIdempotency_KeysAreScopedToTheUser(t *testing.T) {
	next := &countingHandler{status: http.StatusOK, body: `{}`}
	handler := withIdempotency(t, next, repository.NewMemoryIdempotencyRepository())

	handler.ServeHTTP(httptest.NewRecorder(), newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "shared"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest(http.MethodPost, "/holds/h1/confirm", "someone-else", "shared"))

	if got := next.calls.Load(); got != 2 {
		t.Errorf("the handler ran %d times, want 2", got)
	}
	if rec.Header().Get(idempotentReplayHeader) != "" {
		t.Error("another user's request was answered from the replay")
	}
}

func TestIdempotency_ServerErrorsAreNotRemembered(t *testing.T) {
	var calls atomic.Int32

	flaky := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"internal_error"}}`))

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"reserved"}`))
	})

	handler := withIdempotency(t, flaky, repository.NewMemoryIdempotencyRepository())

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	if first.Code != http.StatusInternalServerError {
		t.Fatalf("first status = %d, want 500", first.Code)
	}
	if second.Code != http.StatusOK {
		t.Errorf("second status = %d, want the retry to be allowed through", second.Code)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("the handler ran %d times, want 2", got)
	}
}

type blockingStore struct {
	fingerprint string
}

func (s blockingStore) Claim(
	_ context.Context,
	record repository.IdempotencyRecord,
) (repository.IdempotencyRecord, bool, error) {
	return repository.IdempotencyRecord{
		UserID:      record.UserID,
		Key:         record.Key,
		Fingerprint: s.fingerprint,
		State:       repository.IdempotencyInProgress,
	}, false, nil
}

func (s blockingStore) Complete(context.Context, string, string, int, []byte) error { return nil }
func (s blockingStore) Release(context.Context, string, string) error               { return nil }

func TestIdempotency_RequestStillInProgressIs409(t *testing.T) {
	next := &countingHandler{status: http.StatusOK, body: `{}`}
	handler := withIdempotency(t, next, blockingStore{fingerprint: "POST /holds/h1/confirm"})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "idempotency_in_progress" {
		t.Errorf("code = %q, want idempotency_in_progress", code)
	}
	if got := next.calls.Load(); got != 0 {
		t.Errorf("the handler ran %d times, want 0", got)
	}
}

type failingStore struct{ err error }

func (s failingStore) Claim(
	context.Context,
	repository.IdempotencyRecord,
) (repository.IdempotencyRecord, bool, error) {
	return repository.IdempotencyRecord{}, false, s.err
}

func (s failingStore) Complete(context.Context, string, string, int, []byte) error { return nil }
func (s failingStore) Release(context.Context, string, string) error               { return nil }

func TestIdempotency_AnUnreachableStoreRefusesTheRequest(t *testing.T) {
	next := &countingHandler{status: http.StatusOK, body: `{}`}
	handler := withIdempotency(t, next, failingStore{err: errors.New("the store is down")})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := next.calls.Load(); got != 0 {
		t.Errorf("the handler ran %d times, want 0", got)
	}
	if strings.Contains(rec.Body.String(), "the store is down") {
		t.Errorf("body leaks the internal error: %s", rec.Body)
	}
}

func TestIdempotency_ConcurrentRetriesRunTheHandlerOnce(t *testing.T) {
	const attempts = 40

	next := &countingHandler{status: http.StatusOK, body: `{"status":"reserved"}`}
	handler := withIdempotency(t, next, repository.NewMemoryIdempotencyRepository())

	var (
		wg       sync.WaitGroup
		statuses sync.Map
	)

	start := make(chan struct{})

	for i := range attempts {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newRequest(http.MethodPost, "/holds/h1/confirm", testUser, "k1"))
			statuses.Store(i, rec.Code)
		}()
	}

	close(start)
	wg.Wait()

	if got := next.calls.Load(); got != 1 {
		t.Errorf("the handler ran %d times, want exactly 1", got)
	}

	statuses.Range(func(_, value any) bool {
		if status, _ := value.(int); status != http.StatusOK && status != http.StatusConflict {
			t.Errorf("unexpected status %d", status)
		}

		return true
	})
}
