// Command server is the entry point of the ticket reservation service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/config"
	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/handler"
	"ticket-reservation/internal/repository"
	"ticket-reservation/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("configuration failed", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg)

	if err := run(cfg, logger); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	var level slog.Level
	// The config has already checked the value, so a parse failure here is not
	// possible and the default would be harmless anyway.
	_ = level.UnmarshalText([]byte(cfg.LogLevel))

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// run holds the whole lifecycle so that every deferred cleanup gets to happen.
// os.Exit in main skips defers, which is why the real work lives here and main
// only decides the exit code.
func run(cfg config.Config, logger *slog.Logger) error {
	// One context for the whole process: Ctrl+C or SIGTERM cancels it, and both
	// the sweeper and the HTTP server take their cue from it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clock := service.SystemClock{}

	events, seats, keys, closeStores, err := openStores(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeStores()

	reservations := service.NewReservationService(service.Config{
		Seats:   seats,
		Events:  events,
		Clock:   clock,
		NewID:   service.NewRandomID,
		HoldTTL: cfg.HoldTTL,
	})

	sweeper := service.NewHoldSweeper(seats, clock, cfg.SweepInterval, logger)

	sweeperDone := make(chan struct{})
	go func() {
		defer close(sweeperDone)

		if err := sweeper.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "hold sweeper failed", "err", err)
		}
	}()

	api := handler.New(reservations, clock, logger)

	// RequestID first so that everything inside it can log the id, and Recovery
	// inside Logging so that a panicking request still produces a log line.
	// Idempotency sits innermost, so a replayed answer is still logged and a
	// panic inside it is still recovered.
	root := handler.Chain(api.Routes(),
		handler.RequestID,
		handler.Logging(logger),
		handler.Recovery(logger),
		handler.Idempotency(keys, clock, cfg.IdempotencyTTL, logger),
	)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "starting server", "addr", cfg.Addr)

		// ErrServerClosed is what Shutdown causes, so it is the expected ending
		// rather than a failure.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}

		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	// A fresh context: the process context is already cancelled, and Shutdown
	// needs time to let in flight requests finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	<-sweeperDone
	logger.Info("server stopped")

	return nil
}

// openStores picks the stores from the configuration. This is the only place in
// the process that knows which one is in use; nothing above it can tell, which is
// the payoff of the repository interfaces.
func openStores(
	ctx context.Context,
	cfg config.Config,
	logger *slog.Logger,
) (repository.EventRepository, repository.SeatRepository, repository.IdempotencyRepository, func(), error) {
	if !cfg.UsesDatabase() {
		logger.InfoContext(ctx, "using in-memory stores", "reason", "DATABASE_URL is not set")

		events, seats := seedMemoryStores()

		return events, seats, repository.NewMemoryIdempotencyRepository(), func() {}, nil
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("configuring the connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, nil, nil, nil, fmt.Errorf("reaching the database: %w", err)
	}

	logger.InfoContext(ctx, "using postgres stores")

	events := repository.NewPostgresEventRepository(pool)
	seats := repository.NewPostgresSeatRepository(pool)
	keys := repository.NewPostgresIdempotencyRepository(pool)

	if err := seedPostgresStores(ctx, events, seats); err != nil {
		pool.Close()

		return nil, nil, nil, nil, err
	}

	return events, seats, keys, pool.Close, nil
}

func demoEvent() *domain.Event {
	return &domain.Event{
		ID:       "event-1",
		Name:     "Radiohead",
		Venue:    "Volkswagen Arena",
		StartsAt: time.Now().Add(72 * time.Hour).Truncate(time.Second),
	}
}

func demoSeats(eventID string) []*domain.Seat {
	var seats []*domain.Seat

	for _, row := range []string{"A", "B"} {
		for number := 1; number <= 5; number++ {
			seats = append(seats, domain.NewSeat(eventID, row+strconv.Itoa(number), row, number))
		}
	}

	return seats
}

func seedMemoryStores() (*repository.MemoryEventRepository, *repository.MemorySeatRepository) {
	event := demoEvent()

	return repository.NewMemoryEventRepository(event), repository.NewMemorySeatRepository(demoSeats(event.ID)...)
}

// seedPostgresStores inserts the demo data if it is not already there. The
// statements ignore conflicts, so restarting the server does not disturb seats
// that are already held or sold.
func seedPostgresStores(
	ctx context.Context,
	events *repository.PostgresEventRepository,
	seats *repository.PostgresSeatRepository,
) error {
	event := demoEvent()

	if err := events.InsertEvents(ctx, event); err != nil {
		return fmt.Errorf("seeding events: %w", err)
	}

	if err := seats.InsertSeats(ctx, demoSeats(event.ID)...); err != nil {
		return fmt.Errorf("seeding seats: %w", err)
	}

	return nil
}
