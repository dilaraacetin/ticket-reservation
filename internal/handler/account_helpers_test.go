package handler

import (
	"context"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/service"
)

// fakeAccounts answers whatever the test puts in it, and records what it was
// asked, in the same style as fakeService.
type fakeAccounts struct {
	user    *domain.User
	session service.Session
	err     error

	gotEmail    string
	gotPassword string
}

func (f *fakeAccounts) Register(_ context.Context, email, password string) (*domain.User, error) {
	f.gotEmail, f.gotPassword = email, password

	if f.err != nil {
		return nil, f.err
	}

	if f.user != nil {
		return f.user, nil
	}

	return &domain.User{ID: "user-1", Email: email, CreatedAt: testTime()}, nil
}

func (f *fakeAccounts) Login(_ context.Context, email, password string) (service.Session, error) {
	f.gotEmail, f.gotPassword = email, password

	if f.err != nil {
		return service.Session{}, f.err
	}

	if f.session.Token != "" {
		return f.session, nil
	}

	return service.Session{
		Token:     bearerForUser("user-1")[len(bearerPrefix):],
		UserID:    "user-1",
		ExpiresAt: testTime().Add(time.Hour),
	}, nil
}
