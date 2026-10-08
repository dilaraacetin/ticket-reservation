package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

// Postgres only, because skipping a locked row is not a thing the memory store
// can have an opinion about.
//
// One worker holds the front of the queue; another must serve the next person
// rather than wait behind it. Drop skip locked from the statement and this test
// stops passing, because the second take blocks until its deadline.
func TestPostgresWaitingList_SkipsLockedEntries(t *testing.T) {
	const eventID = "skip-locked-event"

	pool := seededPool(t, eventID)
	repo := NewPostgresWaitingListRepository(pool)

	for i, user := range []string{"ayse", "mehmet"} {
		entry, err := domain.NewWaitingEntry(
			"wait-"+user, eventID, user,
			testTime().Add(time.Duration(i)*time.Second),
		)
		if err != nil {
			t.Fatalf("NewWaitingEntry() error = %v", err)
		}

		if _, _, err := repo.Join(t.Context(), entry); err != nil {
			t.Fatalf("Join() error = %v", err)
		}
	}

	// Stands in for a worker that has taken the front entry and has not
	// committed yet.
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	defer func() {
		// The test's context is already done by now, so the rollback needs one of
		// its own.
		_ = tx.Rollback(context.Background())
	}()

	var locked string
	if err := tx.QueryRow(t.Context(),
		`select id from waiting_list where event_id = $1 order by joined_at, id limit 1 for update`,
		eventID,
	).Scan(&locked); err != nil {
		t.Fatalf("locking the front of the queue failed: %v", err)
	}

	// Short on purpose: a blocked take has to fail the test rather than hold it
	// up until the package times out.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	taken, err := repo.TakeNext(ctx, eventID)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("TakeNext blocked on the locked entry instead of skipping it")
		}

		t.Fatalf("TakeNext() error = %v", err)
	}

	if taken.UserID != "mehmet" {
		t.Errorf("TakeNext() = %q, want mehmet, the first entry that is not locked", taken.UserID)
	}
}
