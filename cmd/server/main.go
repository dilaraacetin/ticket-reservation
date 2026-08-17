// Command server is the entry point of the ticket reservation service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/handler"
	"ticket-reservation/internal/repository"
	"ticket-reservation/internal/service"
)

const (
	defaultAddr     = ":8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("server failed", "err", err)
		os.Exit(1)
	}
}

// run holds the whole lifecycle so that every deferred cleanup gets to happen.
// os.Exit in main skips defers, which is why the real work lives here and main
// only decides the exit code.
func run(logger *slog.Logger) error {
	// One context for the whole process: Ctrl+C or SIGTERM cancels it, and both
	// the sweeper and the HTTP server take their cue from it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clock := service.SystemClock{}
	events, seats := seedStore()

	reservations := service.NewReservationService(service.Config{
		Seats:   seats,
		Events:  events,
		Clock:   clock,
		NewID:   service.NewRandomID,
		HoldTTL: service.DefaultHoldTTL,
	})

	sweeper := service.NewHoldSweeper(seats, clock, service.DefaultSweepInterval, logger)

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
	root := handler.Chain(api.Routes(),
		handler.RequestID,
		handler.Logging(logger),
		handler.Recovery(logger),
	)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.InfoContext(ctx, "starting server", "addr", addr)

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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	<-sweeperDone
	logger.Info("server stopped")

	return nil
}

// seedStore fills the in-memory stores with one event and a small seat map, so
// that the API has something to serve until stage 4 brings a database.
func seedStore() (*repository.MemoryEventRepository, *repository.MemorySeatRepository) {
	event := &domain.Event{
		ID:       "event-1",
		Name:     "Radiohead",
		Venue:    "Volkswagen Arena",
		StartsAt: time.Now().Add(72 * time.Hour),
	}

	var seats []*domain.Seat
	for _, row := range []string{"A", "B"} {
		for number := 1; number <= 5; number++ {
			seatID := row + strconv.Itoa(number)
			seats = append(seats, domain.NewSeat(event.ID, seatID, row, number))
		}
	}

	return repository.NewMemoryEventRepository(event), repository.NewMemorySeatRepository(seats...)
}
