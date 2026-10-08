package service

import (
	"context"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// SeatGate decides whether an account may take a seat at all.
//
// One question asked from three places — holding, confirming and queueing — so
// it is one thing rather than the same check written out three times.
type SeatGate interface {
	MayTakeSeat(ctx context.Context, userID string) error
}

// AllowEveryone lets any account take a seat. The default, and what the tests
// that are about seats rather than about accounts use.
type AllowEveryone struct{}

func (AllowEveryone) MayTakeSeat(context.Context, string) error {
	return nil
}

// VerifiedOnly refuses accounts whose address nobody has confirmed.
//
// It costs one lookup by primary key on the paths that take a seat. Worth it
// because the alternative is a ticket sold to an address that may not exist:
// anybody can register with somebody else's, and until a link sent there has
// been followed the address is only a claim.
type VerifiedOnly struct {
	users repository.UserRepository
}

func NewVerifiedOnly(users repository.UserRepository) VerifiedOnly {
	return VerifiedOnly{users: users}
}

func (g VerifiedOnly) MayTakeSeat(ctx context.Context, userID string) error {
	user, err := g.users.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if !user.EmailVerified() {
		return domain.ErrEmailNotVerified
	}

	return nil
}

var (
	_ SeatGate = AllowEveryone{}
	_ SeatGate = VerifiedOnly{}
)
