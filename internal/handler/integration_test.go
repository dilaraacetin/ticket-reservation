package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ticket-reservation/internal/auth"
	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
	"ticket-reservation/internal/service"
)

type movableClock struct {
	now time.Time
}

func (c *movableClock) Now() time.Time {
	return c.now
}

func (c *movableClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newTestServer(t *testing.T) (*httptest.Server, *movableClock) {
	t.Helper()

	clock := &movableClock{now: testTime()}

	events := repository.NewMemoryEventRepository(&domain.Event{
		ID:       testEventID,
		Name:     "Radiohead",
		Venue:    "Volkswagen Arena",
		StartsAt: testTime().Add(72 * time.Hour),
	})
	seats := repository.NewMemorySeatRepository(
		domain.NewSeat(testEventID, "A1", "A", 1),
		domain.NewSeat(testEventID, "A2", "A", 2),
	)

	reservations := service.NewReservationService(service.Config{
		Seats:   seats,
		Events:  events,
		Clock:   clock,
		NewID:   service.NewRandomID,
		HoldTTL: service.DefaultHoldTTL,
	})

	accounts, err := service.NewAccountService(service.AccountConfig{
		Users:    repository.NewMemoryUserRepository(),
		Hasher:   auth.NewPasswordHasher(auth.Argon2Params{Memory: 64, Iterations: 1, Parallelism: 1}),
		Tokens:   testTokenSigner,
		Clock:    clock,
		NewID:    service.NewRandomID,
		TokenTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("building the account service failed: %v", err)
	}

	api := New(reservations, accounts, clock, discardLogger())
	srv := httptest.NewServer(Chain(api.Routes(),
		RequestID,
		Logging(discardLogger()),
		Recovery(discardLogger()),
		Authenticate(testTokenSigner, clock, discardLogger()),
		Idempotency(repository.NewMemoryIdempotencyRepository(), clock, 24*time.Hour, discardLogger()),
	))

	t.Cleanup(srv.Close)

	return srv, clock
}

type apiResponse struct {
	status int
	header http.Header
	body   []byte
}

func request(t *testing.T, srv *httptest.Server, method, path, userID string) apiResponse {
	t.Helper()

	return requestWithKey(t, srv, method, path, userID, "")
}

func requestWithKey(t *testing.T, srv *httptest.Server, method, path, userID, key string) apiResponse {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("building request failed: %v", err)
	}
	if header := bearerForUser(userID); header != "" {
		req.Header.Set(authorizationHeader, header)
	}
	if key != "" {
		req.Header.Set(idempotencyKeyHeader, key)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body failed: %v", err)
	}

	return apiResponse{status: resp.StatusCode, header: resp.Header, body: body}
}

func decodeBody[T any](t *testing.T, raw []byte) T {
	t.Helper()

	var body T
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decoding %q failed: %v", raw, err)
	}

	return body
}

func TestIntegration_HoldConfirmFlow(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := request(t, srv, http.MethodGet, "/events", "")
	if resp.status != http.StatusOK {
		t.Fatalf("GET /events status = %d, want 200", resp.status)
	}
	if events := decodeBody[[]eventResponse](t, resp.body); len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}

	if resp.header.Get(requestIDHeader) == "" {
		t.Error("response has no request id header")
	}

	resp = request(t, srv, http.MethodGet, "/events/"+testEventID+"/seats", "")
	if resp.status != http.StatusOK {
		t.Fatalf("GET seats status = %d, want 200", resp.status)
	}

	seatMap := decodeBody[seatMapResponse](t, resp.body)
	if len(seatMap.Seats) != 2 || seatMap.Seats[0].Status != "available" {
		t.Fatalf("seat map = %+v, want two available seats", seatMap.Seats)
	}

	resp = request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", testUser)
	if resp.status != http.StatusCreated {
		t.Fatalf("hold status = %d, want 201: %s", resp.status, resp.body)
	}

	hold := decodeBody[holdResponse](t, resp.body)
	if hold.HoldID == "" {
		t.Fatal("hold id is empty")
	}

	resp = request(t, srv, http.MethodGet, "/events/"+testEventID+"/seats", "")
	if got := decodeBody[seatMapResponse](t, resp.body).Seats[0].Status; got != "held" {
		t.Errorf("A1 status = %q, want held", got)
	}

	resp = request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", "user-2")
	if resp.status != http.StatusConflict {
		t.Errorf("second hold status = %d, want 409", resp.status)
	}

	resp = request(t, srv, http.MethodPost, "/holds/"+hold.HoldID+"/confirm", testUser)
	if resp.status != http.StatusOK {
		t.Fatalf("confirm status = %d, want 200: %s", resp.status, resp.body)
	}
	if got := decodeBody[reservationResponse](t, resp.body).Status; got != "reserved" {
		t.Errorf("status = %q, want reserved", got)
	}

	resp = request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", "user-2")
	if resp.status != http.StatusConflict {
		t.Errorf("hold after confirm status = %d, want 409", resp.status)
	}
}

func TestIntegration_ReleaseFreesTheSeat(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", testUser)
	hold := decodeBody[holdResponse](t, resp.body)

	resp = request(t, srv, http.MethodDelete, "/holds/"+hold.HoldID, "user-2")
	if resp.status != http.StatusForbidden {
		t.Errorf("release by another user status = %d, want 403", resp.status)
	}

	resp = request(t, srv, http.MethodDelete, "/holds/"+hold.HoldID, testUser)
	if resp.status != http.StatusNoContent {
		t.Fatalf("release status = %d, want 204", resp.status)
	}

	resp = request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", "user-2")
	if resp.status != http.StatusCreated {
		t.Errorf("hold after release status = %d, want 201", resp.status)
	}
}

func TestIntegration_ExpiredHoldIsGone(t *testing.T) {
	srv, clock := newTestServer(t)

	resp := request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", testUser)
	hold := decodeBody[holdResponse](t, resp.body)

	clock.Advance(service.DefaultHoldTTL)

	resp = request(t, srv, http.MethodPost, "/holds/"+hold.HoldID+"/confirm", testUser)
	if resp.status != http.StatusGone {
		t.Fatalf("confirm status = %d, want 410", resp.status)
	}
	if code := decodeBody[errorBody](t, resp.body).Error.Code; code != "hold_expired" {
		t.Errorf("code = %q, want hold_expired", code)
	}

	resp = request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", "user-2")
	if resp.status != http.StatusCreated {
		t.Errorf("hold after expiry status = %d, want 201", resp.status)
	}
}

func TestIntegration_UnknownEventAndSeat(t *testing.T) {
	srv, _ := newTestServer(t)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"unknown event seat map", http.MethodGet, "/events/nope/seats", http.StatusNotFound},
		{"unknown seat hold", http.MethodPost, "/events/" + testEventID + "/seats/Z9/hold", http.StatusNotFound},
		{"unknown hold confirm", http.MethodPost, "/holds/nope/confirm", http.StatusNotFound},
		{"unknown hold release", http.MethodDelete, "/holds/nope", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := request(t, srv, tt.method, tt.path, testUser).status; got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIntegration_RetriedConfirmIsSafe(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", testUser)
	hold := decodeBody[holdResponse](t, resp.body)

	confirm := "/holds/" + hold.HoldID + "/confirm"

	first := requestWithKey(t, srv, http.MethodPost, confirm, testUser, "retry-key")
	if first.status != http.StatusOK {
		t.Fatalf("first confirm status = %d, want 200: %s", first.status, first.body)
	}

	for attempt := range 2 {
		again := requestWithKey(t, srv, http.MethodPost, confirm, testUser, "retry-key")
		if again.status != http.StatusOK {
			t.Errorf("retry %d status = %d, want 200", attempt+1, again.status)
		}
		if string(again.body) != string(first.body) {
			t.Errorf("retry %d body = %q, want %q", attempt+1, again.body, first.body)
		}
		if got := again.header.Get(idempotentReplayHeader); got != "true" {
			t.Errorf("retry %d %s = %q, want true", attempt+1, idempotentReplayHeader, got)
		}
	}

	seats := request(t, srv, http.MethodGet, "/events/"+testEventID+"/seats", "")
	if got := decodeBody[seatMapResponse](t, seats.body).Seats[0].Status; got != "reserved" {
		t.Errorf("A1 status = %q, want reserved", got)
	}
}

func TestIntegration_RetriedConfirmWithoutAKeyStillFails(t *testing.T) {
	srv, _ := newTestServer(t)

	resp := request(t, srv, http.MethodPost, "/events/"+testEventID+"/seats/A1/hold", testUser)
	hold := decodeBody[holdResponse](t, resp.body)
	confirm := "/holds/" + hold.HoldID + "/confirm"

	if got := request(t, srv, http.MethodPost, confirm, testUser).status; got != http.StatusOK {
		t.Fatalf("first confirm status = %d, want 200", got)
	}
	if got := request(t, srv, http.MethodPost, confirm, testUser).status; got != http.StatusNotFound {
		t.Errorf("bare retry status = %d, want 404", got)
	}
}

func TestIntegration_RetriedHoldReturnsTheSameHold(t *testing.T) {
	srv, _ := newTestServer(t)

	path := "/events/" + testEventID + "/seats/A2/hold"

	first := requestWithKey(t, srv, http.MethodPost, path, testUser, "hold-key")
	if first.status != http.StatusCreated {
		t.Fatalf("first hold status = %d, want 201: %s", first.status, first.body)
	}

	second := requestWithKey(t, srv, http.MethodPost, path, testUser, "hold-key")
	if second.status != http.StatusCreated {
		t.Fatalf("retry status = %d, want 201 rather than the 409 a bare retry would get", second.status)
	}

	firstHold := decodeBody[holdResponse](t, first.body)
	secondHold := decodeBody[holdResponse](t, second.body)
	if firstHold.HoldID != secondHold.HoldID {
		t.Errorf("hold ids differ: %q and %q", firstHold.HoldID, secondHold.HoldID)
	}
}
