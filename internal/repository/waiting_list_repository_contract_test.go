package repository

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ticket-reservation/internal/domain"
)

type waitingListFactory func(t *testing.T, eventID string) WaitingListRepository

func newMemoryWaitingList(_ *testing.T, _ string) WaitingListRepository {
	return NewMemoryWaitingListRepository()
}

func newPostgresWaitingList(t *testing.T, eventID string) WaitingListRepository {
	t.Helper()

	return NewPostgresWaitingListRepository(seededPool(t, eventID))
}

func TestWaitingListRepositoryContract(t *testing.T) {
	implementations := []struct {
		name    string
		newRepo waitingListFactory
	}{
		{"memory", newMemoryWaitingList},
		{"postgres", newPostgresWaitingList},
	}

	for _, implementation := range implementations {
		t.Run(implementation.name, func(t *testing.T) {
			runWaitingListRepositoryContract(t, implementation.newRepo)
		})
	}
}

// joinAt puts a user in the queue at a given instant and fails the test if that
// does not work, so the tests below stay about what they are testing.
func joinAt(
	t *testing.T,
	repo WaitingListRepository,
	eventID, userID string,
	at time.Time,
) *domain.WaitingEntry {
	t.Helper()

	entry, err := domain.NewWaitingEntry("wait-"+userID, eventID, userID, at)
	if err != nil {
		t.Fatalf("NewWaitingEntry() error = %v", err)
	}

	joined, position, err := repo.Join(t.Context(), entry)
	if err != nil {
		t.Fatalf("Join() error = %v", err)
	}

	// A join that worked always leaves the caller somewhere in the queue, so a
	// position of zero would mean the two halves disagreed.
	if position < 1 {
		t.Fatalf("Join() reported position %d, want at least 1", position)
	}

	return joined
}

func runWaitingListRepositoryContract(t *testing.T, newRepo waitingListFactory) {
	t.Helper()

	now := testTime()

	t.Run("the queue hands people back in the order they joined", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		joinAt(t, repo, eventID, "ayse", now)
		joinAt(t, repo, eventID, "mehmet", now.Add(time.Second))
		joinAt(t, repo, eventID, "zeynep", now.Add(2*time.Second))

		for _, want := range []string{"ayse", "mehmet", "zeynep"} {
			taken, err := repo.TakeNext(t.Context(), eventID)
			if err != nil {
				t.Fatalf("TakeNext() error = %v", err)
			}
			if taken.UserID != want {
				t.Errorf("TakeNext() = %q, want %q", taken.UserID, want)
			}
		}

		if _, err := repo.TakeNext(t.Context(), eventID); !errors.Is(err, ErrWaitingListEmpty) {
			t.Errorf("TakeNext() on a drained queue = %v, want %v", err, ErrWaitingListEmpty)
		}
	})

	// A second tap on the button must not send someone to the back of the queue
	// they have already been standing in.
	// The position comes from the same operation as the join, so a handoff
	// running in between cannot turn a join that worked into "you are not
	// waiting".
	t.Run("joining reports where the caller now stands", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		for i, user := range []string{"ayse", "mehmet", "zeynep"} {
			entry, err := domain.NewWaitingEntry("wait-"+user, eventID, user, now.Add(time.Duration(i)*time.Second))
			if err != nil {
				t.Fatalf("NewWaitingEntry() error = %v", err)
			}

			_, position, err := repo.Join(t.Context(), entry)
			if err != nil {
				t.Fatalf("Join() error = %v", err)
			}
			if position != i+1 {
				t.Errorf("Join(%s) reported position %d, want %d", user, position, i+1)
			}
		}
	})

	t.Run("joining twice keeps the original place", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		first := joinAt(t, repo, eventID, "ayse", now)
		joinAt(t, repo, eventID, "mehmet", now.Add(time.Second))

		again := joinAt(t, repo, eventID, "ayse", now.Add(time.Hour))
		if !again.JoinedAt.Equal(first.JoinedAt) {
			t.Errorf("JoinedAt = %s, want the original %s", again.JoinedAt, first.JoinedAt)
		}

		position, err := repo.Position(t.Context(), eventID, "ayse")
		if err != nil {
			t.Fatalf("Position() error = %v", err)
		}
		if position != 1 {
			t.Errorf("Position() = %d, want 1", position)
		}
	})

	t.Run("position counts from one and is reported per user", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		joinAt(t, repo, eventID, "ayse", now)
		joinAt(t, repo, eventID, "mehmet", now.Add(time.Second))
		joinAt(t, repo, eventID, "zeynep", now.Add(2*time.Second))

		for user, want := range map[string]int{"ayse": 1, "mehmet": 2, "zeynep": 3} {
			got, err := repo.Position(t.Context(), eventID, user)
			if err != nil {
				t.Fatalf("Position(%s) error = %v", user, err)
			}
			if got != want {
				t.Errorf("Position(%s) = %d, want %d", user, got, want)
			}
		}

		if _, err := repo.Position(t.Context(), eventID, "kerem"); !errors.Is(err, ErrNotWaiting) {
			t.Errorf("Position() for a stranger = %v, want %v", err, ErrNotWaiting)
		}
	})

	t.Run("leaving frees the place and is harmless when not waiting", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		joinAt(t, repo, eventID, "ayse", now)
		joinAt(t, repo, eventID, "mehmet", now.Add(time.Second))

		if err := repo.Leave(t.Context(), eventID, "ayse"); err != nil {
			t.Fatalf("Leave() error = %v", err)
		}

		position, err := repo.Position(t.Context(), eventID, "mehmet")
		if err != nil {
			t.Fatalf("Position() error = %v", err)
		}
		if position != 1 {
			t.Errorf("Position() = %d after the person ahead left, want 1", position)
		}

		if err := repo.Leave(t.Context(), eventID, "kerem"); err != nil {
			t.Errorf("Leave() for a stranger = %v, want no error", err)
		}
	})

	// When the seat someone was taken for is gone before a hold can be made,
	// they go back where they stood rather than to the back of the queue.
	t.Run("rejoining restores the original place", func(t *testing.T) {
		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		joinAt(t, repo, eventID, "ayse", now)
		joinAt(t, repo, eventID, "mehmet", now.Add(time.Second))

		taken, err := repo.TakeNext(t.Context(), eventID)
		if err != nil {
			t.Fatalf("TakeNext() error = %v", err)
		}

		if err := repo.Rejoin(t.Context(), taken); err != nil {
			t.Fatalf("Rejoin() error = %v", err)
		}

		position, err := repo.Position(t.Context(), eventID, taken.UserID)
		if err != nil {
			t.Fatalf("Position() error = %v", err)
		}
		if position != 1 {
			t.Errorf("Position() = %d after rejoining, want 1", position)
		}
	})

	// The reason this interface has TakeNext rather than a read and a delete.
	// Several seats can come free at the same instant, and each freed seat is
	// offered by its own goroutine.
	t.Run("concurrent takers never get the same person twice", func(t *testing.T) {
		const waiters = 20

		eventID := uniqueEventID(t)
		repo := newRepo(t, eventID)

		for i := range waiters {
			joinAt(t, repo, eventID, fmt.Sprintf("user-%02d", i), now.Add(time.Duration(i)*time.Second))
		}

		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			taken []string
		)

		for range waiters {
			wg.Add(1)

			go func() {
				defer wg.Done()

				entry, err := repo.TakeNext(t.Context(), eventID)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()

				taken = append(taken, entry.UserID)
			}()
		}

		wg.Wait()

		seen := make(map[string]bool, len(taken))
		for _, user := range taken {
			if seen[user] {
				t.Errorf("%s was handed to two takers at once", user)
			}

			seen[user] = true
		}

		if len(taken) != waiters {
			t.Errorf("took %d of %d waiters; the queue should have emptied", len(taken), waiters)
		}
	})
}
