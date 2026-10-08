package service

import (
	"context"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

// NotificationService hands people what they still have to be told.
type NotificationService struct {
	notifications repository.NotificationRepository
	clock         Clock
}

func NewNotificationService(notifications repository.NotificationRepository, clock Clock) *NotificationService {
	return &NotificationService{notifications: notifications, clock: clock}
}

// Notifications returns a person's notices, newest first.
func (s *NotificationService) Notifications(
	ctx context.Context,
	userID string,
) ([]*domain.Notification, error) {
	if userID == "" {
		return nil, domain.ErrEmptyUserID
	}

	return s.notifications.ListForUser(ctx, userID)
}

// MarkNotificationsRead marks everything the person has not seen and reports how
// many that was. All of them at once, because that is what opening the list
// means.
func (s *NotificationService) MarkNotificationsRead(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, domain.ErrEmptyUserID
	}

	return s.notifications.MarkAllRead(ctx, userID, s.clock.Now())
}
