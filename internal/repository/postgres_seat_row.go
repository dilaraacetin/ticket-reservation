package repository

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ticket-reservation/internal/domain"
)

const seatColumns = `event_id, id, row_label, number, status,
	hold_id, held_by, hold_created_at, hold_expires_at, reserved_by, version`

type seatRow struct {
	eventID       string
	id            string
	rowLabel      string
	number        int
	status        string
	holdID        *string
	heldBy        *string
	holdCreatedAt *time.Time
	holdExpiresAt *time.Time
	reservedBy    *string
	version       int64
}

func (r *seatRow) scan(row pgx.Row) error {
	return row.Scan(
		&r.eventID, &r.id, &r.rowLabel, &r.number, &r.status,
		&r.holdID, &r.heldBy, &r.holdCreatedAt, &r.holdExpiresAt, &r.reservedBy, &r.version,
	)
}

func (r seatRow) toDomain() (*domain.Seat, error) {
	status, err := domain.ParseSeatStatus(r.status)
	if err != nil {
		return nil, fmt.Errorf("seat %s/%s: %w", r.eventID, r.id, err)
	}

	return &domain.Seat{
		ID:            r.id,
		EventID:       r.eventID,
		Row:           r.rowLabel,
		Number:        r.number,
		Status:        status,
		HoldID:        text(r.holdID),
		HeldBy:        text(r.heldBy),
		HoldCreatedAt: instant(r.holdCreatedAt),
		HoldExpiresAt: instant(r.holdExpiresAt),
		ReservedBy:    text(r.reservedBy),
	}, nil
}

func nullText(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

func nullInstant(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	return &value
}

func text(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func instant(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}

	return *value
}
