package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

type fakeInventory struct {
	event *domain.Event
	err   error

	gotName    string
	gotVenue   string
	gotDetails domain.EventDetails

	// storedDetails is what an edit is applied to.
	storedDetails domain.EventDetails

	gotEventID string
	gotRows    []string
	gotPerRow  int
}

func (f *fakeInventory) CreateEvent(
	_ context.Context,
	name, venue string,
	startsAt time.Time,
	details domain.EventDetails,
) (*domain.Event, error) {
	f.gotName, f.gotVenue, f.gotDetails = name, venue, details

	if f.err != nil {
		return nil, f.err
	}

	if f.event != nil {
		return f.event, nil
	}

	return &domain.Event{
		ID: "event-new", Name: name, Venue: venue, StartsAt: startsAt, Details: details,
	}, nil
}

func (f *fakeInventory) UpdateEvent(
	_ context.Context,
	eventID, name, venue string,
	startsAt time.Time,
	details func(domain.EventDetails) domain.EventDetails,
) (*domain.Event, error) {
	f.gotEventID, f.gotName, f.gotVenue = eventID, name, venue

	// Run against what the stored event is standing in for, so a test can see
	// which fields the request actually asked to change.
	f.gotDetails = details(f.storedDetails)

	if f.err != nil {
		return nil, f.err
	}

	return &domain.Event{
		ID: eventID, Name: name, Venue: venue, StartsAt: startsAt, Details: f.gotDetails,
	}, nil
}

func (f *fakeInventory) CancelEvent(_ context.Context, eventID string) (*domain.Event, error) {
	f.gotEventID = eventID

	if f.err != nil {
		return nil, f.err
	}

	return &domain.Event{ID: eventID, Name: "Radiohead", CancelledAt: testTime()}, nil
}

func (f *fakeInventory) DeleteEvent(_ context.Context, eventID string) error {
	f.gotEventID = eventID

	return f.err
}

func (f *fakeInventory) AddSeats(_ context.Context, eventID string, rows []string, perRow int) (int, error) {
	f.gotEventID, f.gotRows, f.gotPerRow = eventID, rows, perRow

	if f.err != nil {
		return 0, f.err
	}

	return len(rows) * perRow, nil
}

// fakeRoles can be changed between requests, which is how the test below checks
// that the role is read each time rather than trusted from the token.
type fakeRoles struct {
	mu   sync.Mutex
	role domain.Role
	err  error
}

func (f *fakeRoles) Role(context.Context, string) (domain.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.role, f.err
}

func (f *fakeRoles) set(role domain.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.role = role
}

func admin(
	t *testing.T,
	inventory InventoryService,
	roles RoleReader,
	method, target, body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	h := New(&fakeService{}, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger())
	if inventory != nil && roles != nil {
		h = h.WithAdmin(inventory, roles)
	}

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	Chain(h.Routes(), Authenticate(testTokenSigner, nil, fixedClock{now: testTime()}, discardLogger())).
		ServeHTTP(rec, req)

	return rec
}

const newEventBody = `{"name":"Radiohead","venue":"Volkswagen Arena","startsAt":"2026-11-14T20:30:00Z","city":"Istanbul"}`

func TestAdmin_CreateEvent(t *testing.T) {
	inventory := &fakeInventory{}

	rec := admin(t, inventory, &fakeRoles{role: domain.RoleAdmin},
		http.MethodPost, "/admin/events", newEventBody, withUser(testUser))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if inventory.gotName != "Radiohead" || inventory.gotVenue != "Volkswagen Arena" {
		t.Errorf("service got %q at %q", inventory.gotName, inventory.gotVenue)
	}

	// The id is the service's to assign, so the answer has to carry it back.
	if got := decode[eventResponse](t, rec).ID; got == "" {
		t.Error("the answer carries no event id")
	}

	if got := rec.Header().Get("Location"); got != "/events/event-new" {
		t.Errorf("Location = %q, want /events/event-new", got)
	}
}

// What a PATCH leaves out and what it sends empty are different requests. A
// plain string field cannot tell them apart, which would make an edit of the
// start time either erase the description or make one impossible to clear.
func TestAdmin_EditKeepsTheDetailsItWasNotSent(t *testing.T) {
	stored := domain.EventDetails{
		City:        "Istanbul",
		ImageURL:    "https://images.seathold.test/radiohead.jpg",
		Description: "Touring In Rainbows, with a string section for the second half.",
		Rules:       "Doors at 19:00. Under 16s must come with an adult.",
	}

	tests := []struct {
		name string
		body string
		want domain.EventDetails
	}{
		{
			"a field that was not sent is left alone",
			`{"venue":"Cemal Resit Rey"}`,
			stored,
		},
		{
			"a field sent empty is cleared",
			`{"rules":""}`,
			domain.EventDetails{
				City:        stored.City,
				ImageURL:    stored.ImageURL,
				Description: stored.Description,
			},
		},
		{
			"a field sent is replaced",
			`{"city":"Ankara"}`,
			domain.EventDetails{
				City:        "Ankara",
				ImageURL:    stored.ImageURL,
				Description: stored.Description,
				Rules:       stored.Rules,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inventory := &fakeInventory{storedDetails: stored}

			rec := admin(t, inventory, &fakeRoles{role: domain.RoleAdmin},
				http.MethodPatch, "/admin/events/event-1", tt.body, withUser(testUser))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
			}

			if inventory.gotDetails != tt.want {
				t.Errorf("details = %+v, want %+v", inventory.gotDetails, tt.want)
			}
		})
	}
}

func TestAdmin_AddSeats(t *testing.T) {
	inventory := &fakeInventory{}

	rec := admin(t, inventory, &fakeRoles{role: domain.RoleAdmin},
		http.MethodPost, "/admin/events/event-1/seats", `{"rows":["A","B"],"seatsPerRow":10}`, withUser(testUser))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}

	if inventory.gotEventID != "event-1" || inventory.gotPerRow != 10 {
		t.Errorf("service got event %q, %d per row", inventory.gotEventID, inventory.gotPerRow)
	}

	if got := decode[addSeatsResponse](t, rec).Created; got != 20 {
		t.Errorf("created = %d, want 20", got)
	}
}

// Who is refused, and with which code. 403 rather than 404: the caller is known
// and turned away, which is a different thing from the route not existing.
func TestAdmin_RefusesEveryoneElse(t *testing.T) {
	tests := []struct {
		name    string
		roles   RoleReader
		headers map[string]string
		want    int
		code    string
	}{
		{"no token", &fakeRoles{role: domain.RoleAdmin}, nil, http.StatusUnauthorized, "unauthenticated"},
		{"an ordinary customer", &fakeRoles{role: domain.RoleCustomer}, withUser(testUser), http.StatusForbidden, "not_permitted"},
		{"an account that is gone", &fakeRoles{err: repository.ErrUserNotFound}, withUser(testUser), http.StatusForbidden, "not_permitted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := admin(t, &fakeInventory{}, tt.roles,
				http.MethodPost, "/admin/events", newEventBody, tt.headers)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}

			if got := decode[errorBody](t, rec).Error.Code; got != tt.code {
				t.Errorf("code = %q, want %q", got, tt.code)
			}
		})
	}
}

// The reason the role is read from storage on every request instead of being
// carried in the token: a token is signed once and believed until it expires, so
// a role put inside one outlives being taken away.
func TestAdmin_RoleIsReadOnEveryRequest(t *testing.T) {
	roles := &fakeRoles{role: domain.RoleAdmin}
	inventory := &fakeInventory{}

	h := New(&fakeService{}, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger()).
		WithAdmin(inventory, roles)
	root := Chain(h.Routes(), Authenticate(testTokenSigner, nil, fixedClock{now: testTime()}, discardLogger()))

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/admin/events", strings.NewReader(newEventBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(authorizationHeader, bearerForUser(testUser))

		rec := httptest.NewRecorder()
		root.ServeHTTP(rec, req)

		return rec.Code
	}

	if got := post(); got != http.StatusCreated {
		t.Fatalf("as an administrator = %d, want %d", got, http.StatusCreated)
	}

	// The same token, after the role is taken away.
	roles.set(domain.RoleCustomer)

	if got := post(); got != http.StatusForbidden {
		t.Errorf("after being demoted = %d, want %d; the token still carries the old role",
			got, http.StatusForbidden)
	}
}

// Without a way to read roles there is nothing to authorize against, so the
// routes are better absent than open.
func TestAdmin_RoutesAreAbsentWhenNotWiredIn(t *testing.T) {
	rec := admin(t, nil, nil, http.MethodPost, "/admin/events", newEventBody, withUser(testUser))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d when no inventory is wired in", rec.Code, http.StatusNotFound)
	}
}

// A body that is not the expected shape never reaches the service. Whether the
// shape itself makes sense is the domain's call, and what this checks is that
// its refusals come back as the right code rather than as a server fault.
func TestAdmin_RejectsBadRequests(t *testing.T) {
	tests := []struct {
		name   string
		target string
		body   string
		err    error
		code   string
	}{
		{"not an object", "/admin/events", `"Radiohead"`, nil, "invalid_request_body"},
		{"a field guessed wrong", "/admin/events", `{"eventName":"Radiohead"}`, nil, "invalid_request_body"},
		{"an event with no name", "/admin/events", `{"venue":"Volkswagen Arena"}`, domain.ErrInvalidEvent, "invalid_event"},
		{"an id that is taken", "/admin/events", newEventBody, repository.ErrEventExists, "event_exists"},
		{
			"a seat map the domain refuses",
			"/admin/events/event-1/seats", `{"rows":["A"],"seatsPerRow":0}`,
			domain.ErrInvalidSeatMap, "invalid_seat_map",
		},
		{
			"seats for an event that is not there",
			"/admin/events/event-9/seats", `{"rows":["A"],"seatsPerRow":10}`,
			repository.ErrEventNotFound, "event_not_found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := admin(t, &fakeInventory{err: tt.err}, &fakeRoles{role: domain.RoleAdmin},
				http.MethodPost, tt.target, tt.body, withUser(testUser))

			if got := decode[errorBody](t, rec).Error.Code; got != tt.code {
				t.Errorf("code = %q, want %q (status %d)", got, tt.code, rec.Code)
			}

			if rec.Code >= http.StatusInternalServerError {
				t.Errorf("status = %d; a caller's mistake is not a server fault", rec.Code)
			}
		})
	}
}

// Every route that changes inventory is behind the same check. A new one added
// without it would pass unnoticed otherwise.
func TestAdmin_EveryInventoryRouteNeedsAnAdministrator(t *testing.T) {
	routes := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodPost, "/admin/events", newEventBody},
		{http.MethodPatch, "/admin/events/event-1", `{"venue":"Cemal Resit Rey"}`},
		{http.MethodPost, "/admin/events/event-1/cancel", ""},
		{http.MethodDelete, "/admin/events/event-1", ""},
		{http.MethodPost, "/admin/events/event-1/seats", `{"rows":["A"],"seatsPerRow":4}`},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.target, func(t *testing.T) {
			// An ordinary customer.
			rec := admin(t, &fakeInventory{}, &fakeRoles{role: domain.RoleCustomer},
				route.method, route.target, route.body, withUser(testUser))

			if rec.Code != http.StatusForbidden {
				t.Errorf("as a customer = %d, want %d", rec.Code, http.StatusForbidden)
			}

			// Nobody at all.
			rec = admin(t, &fakeInventory{}, &fakeRoles{role: domain.RoleAdmin},
				route.method, route.target, route.body, nil)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("with no token = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestAdmin_CancelAndDelete(t *testing.T) {
	inventory := &fakeInventory{}

	cancelled := admin(t, inventory, &fakeRoles{role: domain.RoleAdmin},
		http.MethodPost, "/admin/events/event-1/cancel", "", withUser(testUser))

	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel = %d, want %d: %s", cancelled.Code, http.StatusOK, cancelled.Body)
	}

	// The answer has to say it is withdrawn, or the page cannot show it.
	if body := decode[eventResponse](t, cancelled); !body.Cancelled {
		t.Errorf("the answer does not report the event as cancelled: %+v", body)
	}

	deleted := admin(t, inventory, &fakeRoles{role: domain.RoleAdmin},
		http.MethodDelete, "/admin/events/event-1", "", withUser(testUser))

	if deleted.Code != http.StatusNoContent {
		t.Errorf("delete = %d, want %d", deleted.Code, http.StatusNoContent)
	}
	if inventory.gotEventID != "event-1" {
		t.Errorf("the service was asked about %q", inventory.gotEventID)
	}
}
