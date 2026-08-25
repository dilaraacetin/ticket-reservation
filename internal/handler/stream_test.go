package handler

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/event"
)

// streamServer starts the API behind the same middleware the server uses, which
// is the point: a wrapper that hides the flusher breaks streaming, and only a
// test that goes through the whole chain notices.
func streamServer(t *testing.T) (*httptest.Server, *event.Broker) {
	t.Helper()

	broker := event.NewBroker(discardLogger())
	t.Cleanup(broker.Close)

	api := New(&fakeService{}, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger()).
		WithBroker(broker)

	srv := httptest.NewServer(Chain(api.Routes(),
		RequestID,
		Logging(discardLogger()),
		Recovery(discardLogger()),
		Authenticate(testTokenSigner, fixedClock{now: testTime()}, discardLogger()),
		RateLimit(NewRateLimiter(fixedClock{now: testTime()}, time.Minute), discardLogger()),
	))
	t.Cleanup(srv.Close)

	return srv, broker
}

// openStream connects and returns a reader over the body, plus the response.
func openStream(t *testing.T, srv *httptest.Server, path, userID string) (*http.Response, *bufio.Reader) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("building the request failed: %v", err)
	}
	if header := bearerForUser(userID); header != "" {
		req.Header.Set(authorizationHeader, header)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening the stream failed: %v", err)
	}

	t.Cleanup(func() { _ = resp.Body.Close() })

	return resp, bufio.NewReader(resp.Body)
}

// readNotice reads until a data line, so a test fails with a message rather than
// hanging when nothing is pushed.
func readNotice(t *testing.T, reader *bufio.Reader) string {
	t.Helper()

	lines := make(chan string, 1)

	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(line, "data: ") {
				lines <- strings.TrimSpace(strings.TrimPrefix(line, "data: "))

				return
			}
		}
	}()

	select {
	case line := <-lines:
		return line
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was pushed down the stream within two seconds")

		return ""
	}
}

// The whole point of the endpoint, and the guard against a wrapper hiding the
// flusher: a notice published while the connection is open arrives immediately.
func TestStream_PushesNoticesThroughTheMiddleware(t *testing.T) {
	srv, broker := streamServer(t)

	//nolint:bodyclose // the body is the stream, and openStream closes it via t.Cleanup
	resp, reader := openStream(t, srv, "/events/event-1/stream", "")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}

	// Published only once the subscription exists, which the first read confirms.
	waitForSubscriber(t, broker, 1)

	broker.Publish(event.Event{
		Kind:    event.SeatChanged,
		EventID: "event-1",
		SeatID:  "A1",
		At:      testTime(),
	})

	if got := readNotice(t, reader); !strings.Contains(got, `"seatId":"A1"`) {
		t.Errorf("notice = %q, want it to name seat A1", got)
	}
}

// A notice for somebody else must not arrive, which is what keeps the waiting
// list's "your turn" private.
func TestStream_OnlyDeliversWhatConcernsTheWatcher(t *testing.T) {
	srv, broker := streamServer(t)

	//nolint:bodyclose // the body is the stream, and openStream closes it via t.Cleanup
	_, reader := openStream(t, srv, "/events/event-1/stream", "usr_00000001")
	waitForSubscriber(t, broker, 1)

	// Addressed at somebody else, then a public one. Only the public one may
	// arrive, and it must be the first thing read.
	broker.Publish(event.Event{
		Kind: event.TurnCame, EventID: "event-1", SeatID: "A9",
		UserID: "usr_00000002", At: testTime(),
	})
	broker.Publish(event.Event{
		Kind: event.SeatChanged, EventID: "event-1", SeatID: "A1", At: testTime(),
	})

	if got := readNotice(t, reader); !strings.Contains(got, `"seatId":"A1"`) {
		t.Errorf("notice = %q, want the public one and not somebody else's", got)
	}
}

// A watcher going away must not leave a subscription behind, or the broker sends
// into a channel nobody reads for the rest of the process's life.
func TestStream_UnsubscribesWhenTheWatcherLeaves(t *testing.T) {
	srv, broker := streamServer(t)

	//nolint:bodyclose // closed below on purpose, to prove the subscription goes with it
	resp, _ := openStream(t, srv, "/events/event-1/stream", "")
	waitForSubscriber(t, broker, 1)

	_ = resp.Body.Close()

	waitForSubscriber(t, broker, 0)
}

// A handler built without a broker answers rather than panicking.
func TestStream_WithoutABrokerIsUnavailable(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, "/events/event-1/stream", nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if code := decode[errorBody](t, rec).Error.Code; code != "stream_unavailable" {
		t.Errorf("code = %q, want stream_unavailable", code)
	}
}

// waitForSubscriber polls the count, because subscribing happens on the server's
// goroutine and the test has no other signal that it has happened.
func waitForSubscriber(t *testing.T, broker *event.Broker, want int) {
	t.Helper()

	deadline := time.After(2 * time.Second)

	for {
		if broker.Subscribers() == want {
			return
		}

		select {
		case <-deadline:
			t.Fatalf("subscribers = %d, want %d", broker.Subscribers(), want)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
