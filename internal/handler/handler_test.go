package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

const (
	testEventID = "event-1"
	testSeatID  = "A1"
	testHoldID  = "hold-1"
	testUser    = "user-1"
)

func testTime() time.Time {
	return time.Date(2026, 8, 17, 20, 0, 0, 0, time.UTC)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// fixedClock is enough for the handler, which only reads the time.
type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

// fakeService returns whatever the test puts in it. A hand written fake beats a
// mock library here: the interface has five methods and the tests only care
// about what comes back.
type fakeService struct {
	events []*domain.Event
	seats  []*domain.Seat
	hold   *domain.Hold
	seat   *domain.Seat
	err    error

	// Recorded arguments, for the tests that care that the handler passed the
	// right values through.
	gotEventID string
	gotSeatID  string
	gotHoldID  string
	gotUserID  string
}

func (f *fakeService) Events(context.Context) ([]*domain.Event, error) {
	return f.events, f.err
}

func (f *fakeService) SeatMap(_ context.Context, eventID string) ([]*domain.Seat, error) {
	f.gotEventID = eventID

	return f.seats, f.err
}

func (f *fakeService) HoldSeat(_ context.Context, eventID, seatID, userID string) (*domain.Hold, error) {
	f.gotEventID, f.gotSeatID, f.gotUserID = eventID, seatID, userID

	return f.hold, f.err
}

func (f *fakeService) ConfirmReservation(_ context.Context, holdID, userID string) (*domain.Seat, error) {
	f.gotHoldID, f.gotUserID = holdID, userID

	return f.seat, f.err
}

func (f *fakeService) ReleaseSeat(_ context.Context, holdID, userID string) error {
	f.gotHoldID, f.gotUserID = holdID, userID

	return f.err
}

// do runs one request against the router and returns the recorded response.
func do(svc ReservationService, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	h := New(svc, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger())

	req := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()

	authenticated := Authenticate(testTokenSigner, fixedClock{now: testTime()}, discardLogger())(h.Routes())
	authenticated.ServeHTTP(rec, req)

	return rec
}

func withUser(userID string) map[string]string {
	return map[string]string{authorizationHeader: bearerForUser(userID)}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var body T
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %q failed: %v", rec.Body.String(), err)
	}

	return body
}

func TestHandler_Health(t *testing.T) {
	rec := do(&fakeService{}, http.MethodGet, "/health", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestHandler_ListEvents(t *testing.T) {
	t.Run("returns the events with a computed hasStarted", func(t *testing.T) {
		svc := &fakeService{events: []*domain.Event{
			{ID: "past", Name: "Yesterday", StartsAt: testTime().Add(-time.Hour)},
			{ID: "future", Name: "Tomorrow", StartsAt: testTime().Add(time.Hour)},
		}}

		rec := do(svc, http.MethodGet, "/events", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		body := decode[[]eventResponse](t, rec)
		if len(body) != 2 {
			t.Fatalf("got %d events, want 2", len(body))
		}
		if !body[0].HasStarted {
			t.Error("past event hasStarted = false, want true")
		}
		if body[1].HasStarted {
			t.Error("future event hasStarted = true, want false")
		}
	})

	// nil marshals to null, which forces clients to handle two shapes of empty.
	t.Run("no events is an empty array, not null", func(t *testing.T) {
		rec := do(&fakeService{}, http.MethodGet, "/events", nil)

		if got := rec.Body.String(); got != "[]\n" {
			t.Errorf("body = %q, want %q", got, "[]\n")
		}
	})
}

func TestHandler_SeatMap(t *testing.T) {
	t.Run("exposes seat status as a word and hides the holder", func(t *testing.T) {
		held := domain.NewSeat(testEventID, "A2", "A", 2)
		if err := held.Hold(testHoldID, testUser, time.Minute, testTime()); err != nil {
			t.Fatalf("seeding hold failed: %v", err)
		}

		svc := &fakeService{seats: []*domain.Seat{
			domain.NewSeat(testEventID, "A1", "A", 1),
			held,
		}}

		rec := do(svc, http.MethodGet, "/events/"+testEventID+"/seats", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if svc.gotEventID != testEventID {
			t.Errorf("service got event %q, want %q", svc.gotEventID, testEventID)
		}

		body := decode[seatMapResponse](t, rec)
		if body.Seats[0].Status != "available" || body.Seats[1].Status != "held" {
			t.Errorf("statuses = %q/%q, want available/held", body.Seats[0].Status, body.Seats[1].Status)
		}
		if raw := rec.Body.String(); strings.Contains(raw, testUser) {
			t.Errorf("body leaks the holder: %s", raw)
		}
	})

	// The seat map is built from stored state, so a hold that has run out must be
	// reported as free even before the sweeper has touched it.
	t.Run("an expired hold is reported as available", func(t *testing.T) {
		expired := domain.NewSeat(testEventID, "A1", "A", 1)
		if err := expired.Hold(testHoldID, testUser, time.Minute, testTime().Add(-time.Hour)); err != nil {
			t.Fatalf("seeding hold failed: %v", err)
		}

		rec := do(&fakeService{seats: []*domain.Seat{expired}}, http.MethodGet, "/events/e/seats", nil)

		body := decode[seatMapResponse](t, rec)
		if body.Seats[0].Status != "available" {
			t.Errorf("status = %q, want available", body.Seats[0].Status)
		}
	})

	t.Run("unknown event is 404", func(t *testing.T) {
		svc := &fakeService{err: repository.ErrEventNotFound}

		rec := do(svc, http.MethodGet, "/events/nope/seats", nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
		if code := decode[errorBody](t, rec).Error.Code; code != "event_not_found" {
			t.Errorf("code = %q, want event_not_found", code)
		}
	})
}

func TestHandler_HoldSeat(t *testing.T) {
	target := "/events/" + testEventID + "/seats/" + testSeatID + "/hold"

	t.Run("returns 201 with the hold and a Location header", func(t *testing.T) {
		svc := &fakeService{hold: &domain.Hold{
			ID:        testHoldID,
			EventID:   testEventID,
			SeatID:    testSeatID,
			UserID:    testUser,
			CreatedAt: testTime(),
			ExpiresAt: testTime().Add(5 * time.Minute),
		}}

		rec := do(svc, http.MethodPost, target, withUser(testUser))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}
		if got := rec.Header().Get("Location"); got != "/holds/"+testHoldID {
			t.Errorf("Location = %q, want /holds/%s", got, testHoldID)
		}

		body := decode[holdResponse](t, rec)
		if body.HoldID != testHoldID || body.SeatID != testSeatID {
			t.Errorf("body = %+v, want hold %q on seat %q", body, testHoldID, testSeatID)
		}
		if body.ExpiresInSeconds != 300 {
			t.Errorf("expiresInSeconds = %d, want 300", body.ExpiresInSeconds)
		}
	})

	t.Run("path values reach the service", func(t *testing.T) {
		svc := &fakeService{hold: &domain.Hold{ID: testHoldID}}

		do(svc, http.MethodPost, target, withUser(testUser))

		if svc.gotEventID != testEventID || svc.gotSeatID != testSeatID || svc.gotUserID != testUser {
			t.Errorf("service got %q/%q/%q, want %q/%q/%q",
				svc.gotEventID, svc.gotSeatID, svc.gotUserID, testEventID, testSeatID, testUser)
		}
	})

	t.Run("an anonymous caller is 401", func(t *testing.T) {
		rec := do(&fakeService{}, http.MethodPost, target, nil)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if code := decode[errorBody](t, rec).Error.Code; code != "unauthenticated" {
			t.Errorf("code = %q, want unauthenticated", code)
		}
	})

	t.Run("a taken seat is 409", func(t *testing.T) {
		svc := &fakeService{err: domain.ErrSeatNotAvailable}

		rec := do(svc, http.MethodPost, target, withUser(testUser))
		if rec.Code != http.StatusConflict {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
		}
	})

	t.Run("an unknown seat is 404", func(t *testing.T) {
		svc := &fakeService{err: repository.ErrSeatNotFound}

		rec := do(svc, http.MethodPost, target, withUser(testUser))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}

func TestHandler_ConfirmReservation(t *testing.T) {
	target := "/holds/" + testHoldID + "/confirm"

	t.Run("returns the reservation", func(t *testing.T) {
		reserved := domain.NewSeat(testEventID, testSeatID, "A", 1)
		if err := reserved.Hold(testHoldID, testUser, time.Minute, testTime()); err != nil {
			t.Fatalf("seeding hold failed: %v", err)
		}
		if err := reserved.Confirm(testUser, testTime()); err != nil {
			t.Fatalf("seeding confirm failed: %v", err)
		}

		rec := do(&fakeService{seat: reserved}, http.MethodPost, target, withUser(testUser))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		body := decode[reservationResponse](t, rec)
		if body.Status != "reserved" || body.UserID != testUser {
			t.Errorf("body = %+v, want reserved by %q", body, testUser)
		}
	})

	// The one status worth insisting on: Gone tells the client the hold existed
	// and it was simply too late, which is different from a conflict.
	t.Run("an expired hold is 410", func(t *testing.T) {
		rec := do(&fakeService{err: domain.ErrHoldExpired}, http.MethodPost, target, withUser(testUser))

		if rec.Code != http.StatusGone {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusGone)
		}
		if code := decode[errorBody](t, rec).Error.Code; code != "hold_expired" {
			t.Errorf("code = %q, want hold_expired", code)
		}
	})

	t.Run("another user's hold is 403", func(t *testing.T) {
		rec := do(&fakeService{err: domain.ErrNotHoldOwner}, http.MethodPost, target, withUser("someone-else"))

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("an unknown hold is 404", func(t *testing.T) {
		rec := do(&fakeService{err: repository.ErrHoldNotFound}, http.MethodPost, target, withUser(testUser))

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("an anonymous caller is 401", func(t *testing.T) {
		svc := &fakeService{}

		rec := do(svc, http.MethodPost, target, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if svc.gotHoldID != "" {
			t.Error("the service was called for an anonymous request")
		}
	})
}

func TestHandler_ReleaseSeat(t *testing.T) {
	target := "/holds/" + testHoldID

	t.Run("returns 204 with no body", func(t *testing.T) {
		svc := &fakeService{}

		rec := do(svc, http.MethodDelete, target, withUser(testUser))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("body = %q, want empty", rec.Body.String())
		}
		if svc.gotHoldID != testHoldID {
			t.Errorf("service got hold %q, want %q", svc.gotHoldID, testHoldID)
		}
	})

	t.Run("another user's hold is 403", func(t *testing.T) {
		rec := do(&fakeService{err: domain.ErrNotHoldOwner}, http.MethodDelete, target, withUser("someone-else"))

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("an anonymous caller is 401", func(t *testing.T) {
		svc := &fakeService{}

		rec := do(svc, http.MethodDelete, target, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if svc.gotHoldID != "" {
			t.Error("the service was called for an anonymous request")
		}
	})
}

// TestHandler_UnexpectedErrorIsHidden checks that an error nobody mapped becomes
// a 500 whose body says nothing about the internals.
func TestHandler_UnexpectedErrorIsHidden(t *testing.T) {
	svc := &fakeService{err: errBoom}

	rec := do(svc, http.MethodGet, "/events", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	body := decode[errorBody](t, rec)
	if body.Error.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", body.Error.Code)
	}
	if body.Error.Message != "internal error" {
		t.Errorf("message = %q, want the generic text", body.Error.Message)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("body leaks the internal error: %s", rec.Body.String())
	}
}

func TestHandler_MethodAndPathMismatches(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		want   int
	}{
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound},
		{"wrong method on health", http.MethodPost, "/health", http.StatusMethodNotAllowed},
		{"wrong method on holds", http.MethodPatch, "/holds/" + testHoldID, http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(&fakeService{}, tt.method, tt.target, nil)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

// errBoom stands for any error the handler was never taught to map.
var errBoom = errors.New("boom: the database caught fire")
