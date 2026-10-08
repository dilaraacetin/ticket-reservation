package repository

import (
	"fmt"
	"sync/atomic"
	"testing"

	"ticket-reservation/internal/domain"
)

// The question stage 4 left open: does holding the row lock cost more than
// retrying a conflict, and does the answer change with how much contention
// there is?
//
// The mutate does nothing at all, and that is the point. What is being timed is
// the trip through BEGIN, SELECT FOR UPDATE, UPDATE, COMMIT on one side, and
// SELECT, UPDATE ... WHERE version, retry on the other, with no domain work in
// between to blur the difference.
func benchmarkUpdateSeat(b *testing.B, newRepo seatRepositoryFactory, eventID string, seats int) {
	b.Helper()

	all := make([]*domain.Seat, 0, seats)
	for i := range seats {
		all = append(all, domain.NewSeat(eventID, fmt.Sprintf("A%d", i+1), "A", i+1))
	}

	repo := newRepo(b, eventID, all...)
	ctx := b.Context()

	var conflicts atomic.Int64

	b.ResetTimer()

	var next atomic.Int64

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Round robin, so every worker touches every seat rather than each
			// settling on one of its own.
			seat := all[int(next.Add(1))%seats]

			err := repo.UpdateSeat(ctx, eventID, seat.ID, func(*domain.Seat) error { return nil })
			if err != nil {
				conflicts.Add(1)
			}
		}
	})

	b.StopTimer()

	// ns/op on its own would flatter whichever store gives up soonest, because
	// an update that failed took time but did no work. These two are what make
	// the comparison an honest one: how often a caller was turned away, and what
	// an update that actually landed cost.
	gaveUp := int(conflicts.Load())

	b.ReportMetric(float64(gaveUp)/float64(b.N)*100, "%gave-up")

	if landed := b.N - gaveUp; landed > 0 {
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(landed), "ns/landed")
	}
}

// One seat, every worker: the worst case the seat map can produce, which is what
// a popular row on sale looks like.
func BenchmarkUpdateSeat_OneSeat(b *testing.B) {
	for _, implementation := range benchImplementations() {
		b.Run(implementation.name, func(b *testing.B) {
			benchmarkUpdateSeat(b, implementation.newRepo, uniqueEventID(b), 1)
		})
	}
}

// Twenty seats: the ordinary case, where two callers rarely want the same chair.
func BenchmarkUpdateSeat_TwentySeats(b *testing.B) {
	for _, implementation := range benchImplementations() {
		b.Run(implementation.name, func(b *testing.B) {
			benchmarkUpdateSeat(b, implementation.newRepo, uniqueEventID(b), 20)
		})
	}
}

func benchImplementations() []struct {
	name    string
	newRepo seatRepositoryFactory
} {
	return []struct {
		name    string
		newRepo seatRepositoryFactory
	}{
		{"memory", newMemorySeats},
		{"postgres-pessimistic", newPostgresSeats},
		{"postgres-optimistic", newOptimisticSeats},
	}
}
