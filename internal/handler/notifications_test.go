package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

type fakeNotifier struct {
	notices []*domain.Notification
	marked  int
	err     error

	gotUserID string
}

func (f *fakeNotifier) Notifications(_ context.Context, userID string) ([]*domain.Notification, error) {
	f.gotUserID = userID

	return f.notices, f.err
}

func (f *fakeNotifier) MarkNotificationsRead(_ context.Context, userID string) (int, error) {
	f.gotUserID = userID

	return f.marked, f.err
}

func notified(
	t *testing.T,
	notifier NotificationService,
	method, target string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	h := New(&fakeService{}, &fakeAccounts{}, fixedClock{now: testTime()}, discardLogger())
	if notifier != nil {
		h = h.WithNotifications(notifier)
	}

	req := httptest.NewRequest(method, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	Chain(h.Routes(), Authenticate(testTokenSigner, nil, fixedClock{now: testTime()}, discardLogger())).
		ServeHTTP(rec, req)

	return rec
}

func TestNotifications_ListsTheCallersOwnWithACount(t *testing.T) {
	notifier := &fakeNotifier{notices: []*domain.Notification{
		{
			ID: "n-1", UserID: testUser, Kind: domain.NotifyEventCancelled,
			EventID: "event-1", EventName: "Radiohead", SeatID: "A1",
			CreatedAt: testTime(),
		},
		{
			ID: "n-2", UserID: testUser, Kind: domain.NotifyEventCancelled,
			EventID: "event-2", EventName: "Fazil Say",
			CreatedAt: testTime().Add(-time.Hour), ReadAt: testTime(),
		},
	}}

	rec := notified(t, notifier, http.MethodGet, "/notifications", withUser(testUser))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	body := decode[notificationsResponse](t, rec)

	// The count is given separately so a page can show a badge without walking
	// the list first.
	if body.Unread != 1 {
		t.Errorf("unread = %d, want 1", body.Unread)
	}
	if len(body.Notifications) != 2 {
		t.Fatalf("got %d notices, want 2", len(body.Notifications))
	}

	first := body.Notifications[0]
	if first.EventName != "Radiohead" || first.SeatID != "A1" || first.Read {
		t.Errorf("first notice is wrong: %+v", first)
	}

	// A notice that is not about one seat says nothing about seats.
	if strings.Contains(rec.Body.String(), `"seatId":""`) {
		t.Error("an empty seat id was sent rather than left out")
	}

	if notifier.gotUserID != testUser {
		t.Errorf("the service was asked about %q", notifier.gotUserID)
	}
}

func TestNotifications_MarkingReadReportsHowMany(t *testing.T) {
	notifier := &fakeNotifier{marked: 3}

	rec := notified(t, notifier, http.MethodPost, "/notifications/read", withUser(testUser))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if got := decode[markReadResponse](t, rec).MarkedRead; got != 3 {
		t.Errorf("markedRead = %d, want 3", got)
	}
}

func TestNotifications_NeedAToken(t *testing.T) {
	for _, route := range []struct{ method, target string }{
		{http.MethodGet, "/notifications"},
		{http.MethodPost, "/notifications/read"},
	} {
		t.Run(route.method+" "+route.target, func(t *testing.T) {
			rec := notified(t, &fakeNotifier{}, route.method, route.target, nil)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

// Without a store there is no list to fill, so the routes are better absent.
func TestNotifications_RoutesAreAbsentWhenNotWiredIn(t *testing.T) {
	rec := notified(t, nil, http.MethodGet, "/notifications", withUser(testUser))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNotifications_EmptyIsNotNull(t *testing.T) {
	rec := notified(t, &fakeNotifier{}, http.MethodGet, "/notifications", withUser(testUser))

	if !strings.Contains(rec.Body.String(), `"notifications":[]`) {
		t.Errorf("body = %s, want an empty array", rec.Body.String())
	}
}
