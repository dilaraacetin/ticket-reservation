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

	"ticket-reservation/internal/auth"
	"ticket-reservation/internal/config"
	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/event"
	"ticket-reservation/internal/handler"
	"ticket-reservation/internal/metrics"
	"ticket-reservation/internal/notify"
	"ticket-reservation/internal/repository"
	"ticket-reservation/internal/service"
	"ticket-reservation/internal/web"
)

// version is set at build time with -ldflags. A deployment that cannot say which
// build it is running is one nobody can reason about.
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("configuration failed", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg)

	logger.Info("configuration loaded", "version", version, "config", cfg)

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

	defer startPprof(ctx, cfg, logger)()

	tokens, err := auth.NewTokens(cfg.AuthSecret)
	if err != nil {
		return err
	}

	db, err := openStores(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer db.close()

	broker := event.NewBroker(logger)
	defer broker.Close()

	measurements := metrics.New()
	if err := measurements.ObserveStreams(broker); err != nil {
		return fmt.Errorf("registering the stream metrics: %w", err)
	}

	// Taking a seat needs a confirmed address. The same rule guards queueing for
	// one, because a waiter who cannot be given a seat would otherwise sit at the
	// front of the queue being refused.
	seats := service.NewVerifiedOnly(db.users)

	reservations := service.NewReservationService(service.Config{
		Seats:     db.seats,
		Events:    db.events,
		Clock:     clock,
		NewID:     service.NewRandomID,
		HoldTTL:   cfg.HoldTTL,
		Publisher: broker,
		Gate:      seats,
	})

	verification := service.NewVerificationService(service.VerificationConfig{
		Verifications: db.verifications,
		Users:         db.users,
		Clock:         clock,
		TTL:           cfg.VerificationTTL,
		PublicURL:     cfg.PublicURL,
		Logger:        logger,
	})

	accounts, err := service.NewAccountService(service.AccountConfig{
		Users:       db.users,
		Verifier:    verification,
		Revocations: db.revocations,
		Hasher:      auth.NewPasswordHasher(argon2Params(cfg)),
		Tokens:      tokens,
		Clock:       clock,
		NewID:       service.NewRandomID,
		TokenTTL:    cfg.TokenTTL,
		Logger:      logger,
	})
	if err != nil {
		return err
	}

	waitingList := service.NewWaitingListService(service.WaitingListConfig{
		Waiting: db.waiting,
		Events:  db.events,
		Clock:   clock,
		NewID:   service.NewRandomID,
		Gate:    seats,
	})

	handoff := service.NewHandoff(service.HandoffConfig{
		Waiting:   db.waiting,
		Holder:    reservations,
		Publisher: broker,
		Clock:     clock,
		Logger:    logger,
		Workers:   cfg.HandoffWorkers,
		Buffer:    cfg.HandoffBuffer,
	})

	// The two places a seat comes free: somebody gives it up, or their hold runs
	// out. Both feed the same queue.
	reservations.WithOfferer(handoff)

	mailer, err := notify.NewMailer(notify.MailerConfig{
		Addr:     cfg.SMTPAddr,
		From:     cfg.SMTPFrom,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
	})
	if err != nil {
		return fmt.Errorf("configuring mail: %w", err)
	}

	// Which channels exist is this deployment's business. The service is told
	// rather than working it out, so turning email off is a configuration
	// change and not a code path.
	pusher, err := notify.NewPusher(db.push, notify.PusherConfig{
		PublicKey:  cfg.VAPIDPublicKey,
		PrivateKey: cfg.VAPIDPrivateKey,
		Subject:    cfg.VAPIDSubject,
	})
	if err != nil {
		return fmt.Errorf("configuring push: %w", err)
	}

	var channels []domain.DeliveryChannel
	if mailer != nil {
		channels = append(channels, domain.ChannelEmail)
	}

	if pusher != nil {
		channels = append(channels, domain.ChannelPush)
	}

	inventory := service.NewInventoryService(service.InventoryConfig{
		Events:        db.events,
		Seats:         db.seats,
		Waiting:       db.waiting,
		Notifications: db.notifications,
		Deliveries:    db.deliveries,
		Channels:      channels,
		Publisher:     broker,
		Clock:         clock,
		NewID:         service.NewRandomID,
		Logger:        logger,
	})

	notifier := service.NewNotificationService(db.notifications, clock)

	sweeper := service.NewHoldSweeper(db.seats, clock, cfg.SweepInterval, logger).
		WithOfferer(handoff)

	if err := measurements.ObserveHandoff(handoff); err != nil {
		return fmt.Errorf("registering the handoff metrics: %w", err)
	}

	handoffDone := make(chan struct{})
	go func() {
		defer close(handoffDone)

		if err := handoff.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "the seat handoff failed", "err", err)
		}
	}()

	// A nil mailer would be a non-nil interface holding a nil pointer, which is
	// not the same as "no mailer" and would make the worker try to send.
	// A typed nil in an interface is not nil, and would make the worker try to
	// send through something that is not there.
	var emailSender service.EmailSender
	if mailer != nil {
		emailSender = mailer
	}

	var pushSender service.PushSender
	if pusher != nil {
		pushSender = pusher
	}

	deliveries := service.NewDeliveryWorker(service.DeliveryConfig{
		Deliveries:    db.deliveries,
		Notifications: db.notifications,
		Users:         db.users,
		Mailer:        emailSender,
		Pusher:        pushSender,
		Clock:         clock,
		Logger:        logger,
	})

	deliveriesDone := make(chan struct{})
	go func() {
		defer close(deliveriesDone)

		if err := deliveries.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "the delivery worker failed", "err", err)
		}
	}()

	verificationSender := service.NewVerificationSender(service.VerificationSenderConfig{
		Verifications: db.verifications,
		Mailer:        emailSender,
		Linker:        verification,
		Clock:         clock,
		Logger:        logger,
	})

	verificationsDone := make(chan struct{})
	go func() {
		defer close(verificationsDone)

		if err := verificationSender.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "the verification sender failed", "err", err)
		}
	}()

	revocationSweeper := service.NewTokenSweeper(accounts, clock, service.DefaultRevocationSweepInterval, logger)

	revocationsDone := make(chan struct{})
	go func() {
		defer close(revocationsDone)

		if err := revocationSweeper.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "the revocation sweeper failed", "err", err)
		}
	}()

	sweeperDone := make(chan struct{})
	go func() {
		defer close(sweeperDone)

		if err := sweeper.Run(ctx); err != nil {
			logger.ErrorContext(ctx, "hold sweeper failed", "err", err)
		}
	}()

	ui, err := web.Handler()
	if err != nil {
		return fmt.Errorf("preparing the web interface: %w", err)
	}

	api := handler.New(reservations, accounts, clock, logger).
		WithWeb(ui).
		WithWaitingList(waitingList).
		WithMetrics(measurements.Handler()).
		WithProber(db.prober).
		WithAdmin(inventory, accounts).
		WithNotifications(notifier).
		WithVerification(verification).
		WithBroker(broker).
		WithBodyLimit(cfg.RequestBodyLimit)

	// Only when there are keys to sign a push with. Offering a subscription that
	// could never be pushed to would be a button that does nothing.
	if pusher != nil {
		api = api.WithPush(service.NewPushService(db.push, pusher.PublicKey(), clock, service.NewRandomID))
	}

	// RequestID first so that everything inside it can log the id, and Recovery
	// inside Logging so that a panicking request still produces a log line.
	// Idempotency sits innermost, so a replayed answer is still logged and a
	// panic inside it is still recovered.
	limiter := handler.NewRateLimiter(clock, cfg.RateLimitTTL)

	limiterDone := make(chan struct{})
	go func() {
		defer close(limiterDone)

		if err := limiter.Run(ctx, cfg.RateLimitTTL, logger); err != nil {
			logger.ErrorContext(ctx, "the rate limiter's cleanup failed", "err", err)
		}
	}()

	// Authenticate first, so a signed in caller is rate limited as itself and an
	// idempotency key is scoped to it. RateLimit before Idempotency, so a refused
	// caller never reaches the store at all.
	routes := api.Routes()

	root := handler.Chain(routes,
		// Measure outermost, so what it times is what the caller waited for
		// rather than what was left after the other middleware had its turn.
		handler.Measure(measurements, routes),
		// Then the headers, set before anything downstream can write a body.
		handler.SecurityHeaders,
		handler.RequestID,
		handler.Logging(logger),
		handler.Recovery(logger),
		handler.Authenticate(tokens, accounts, clock, logger),
		handler.RateLimit(limiter, rateLimitPolicies(cfg), logger),
		handler.Idempotency(db.keys, clock, cfg.IdempotencyTTL, logger),
	)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           root,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
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
	<-limiterDone
	<-handoffDone
	<-revocationsDone
	<-deliveriesDone
	<-verificationsDone
	logger.Info("server stopped")

	return nil
}

// openStores picks the stores from the configuration. This is the only place in
// the process that knows which one is in use; nothing above it can tell, which is
// the payoff of the repository interfaces.
// stores is everything the process reads and writes, and how to let go of it. A
// struct rather than a row of return values, which had already grown to six.
type stores struct {
	events  repository.EventRepository
	seats   repository.SeatRepository
	keys    repository.IdempotencyRepository
	users   repository.UserRepository
	waiting repository.WaitingListRepository

	// revocations is what signing out is recorded in.
	revocations repository.TokenRepository

	// notifications is what people still have to be told.
	notifications repository.NotificationRepository

	// deliveries is the outbox: what is still owed outside the application.
	deliveries repository.DeliveryRepository

	// push is which browsers have agreed to be pushed to.
	push repository.PushRepository

	// verifications is the outstanding "is this really your address" links.
	verifications repository.VerificationRepository

	// prober is nil for the in-memory stores, which have nothing to reach.
	prober handler.Prober

	close func()
}

func openStores(ctx context.Context, cfg config.Config, logger *slog.Logger) (stores, error) {
	if !cfg.UsesDatabase() {
		logger.InfoContext(ctx, "using in-memory stores", "reason", "DATABASE_URL is not set")

		events, seats := seedMemoryStores()

		return stores{
			events:        events,
			seats:         seats,
			keys:          repository.NewMemoryIdempotencyRepository(),
			users:         repository.NewMemoryUserRepository(),
			waiting:       repository.NewMemoryWaitingListRepository(),
			revocations:   repository.NewMemoryTokenRepository(),
			notifications: repository.NewMemoryNotificationRepository(),
			deliveries:    repository.NewMemoryDeliveryRepository(),
			push:          repository.NewMemoryPushRepository(),
			verifications: repository.NewMemoryVerificationRepository(),
			close:         func() {},
		}, nil
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return stores{}, fmt.Errorf("configuring the connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return stores{}, fmt.Errorf("reaching the database: %w", err)
	}

	logger.InfoContext(ctx, "using postgres stores")

	events := repository.NewPostgresEventRepository(pool)
	seats := repository.NewPostgresSeatRepository(pool)

	if cfg.SeedDemoData {
		logger.WarnContext(ctx, "writing demo data into the database", "reason", "SEED_DEMO_DATA is on")

		if err := seedPostgresStores(ctx, events, seats); err != nil {
			pool.Close()

			return stores{}, err
		}
	}

	return stores{
		events:        events,
		seats:         seats,
		keys:          repository.NewPostgresIdempotencyRepository(pool),
		users:         repository.NewPostgresUserRepository(pool),
		waiting:       repository.NewPostgresWaitingListRepository(pool),
		revocations:   repository.NewPostgresTokenRepository(pool),
		notifications: repository.NewPostgresNotificationRepository(pool),
		deliveries:    repository.NewPostgresDeliveryRepository(pool),
		push:          repository.NewPostgresPushRepository(pool),
		verifications: repository.NewPostgresVerificationRepository(pool),
		prober:        pool,
		close:         pool.Close,
	}, nil
}

func demoEvent() *domain.Event {
	return &domain.Event{
		ID:       "event-1",
		Name:     "Radiohead",
		Venue:    "Volkswagen Arena",
		StartsAt: time.Now().Add(72 * time.Hour).Truncate(time.Second),
		Details: domain.EventDetails{
			City:     "Istanbul",
			Category: domain.CategoryConcert,
			// No poster on purpose. A URL here would make the demo depend on
			// somebody else's server, and leaving it empty is what exercises the
			// fallback the catalogue draws when an event has none.
			Description: "Touring In Rainbows, with a string section for the second half.",
			Rules:       "Doors at 19:00. Under 16s must come with an adult. No professional cameras.",
		},
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
	seats := repository.NewMemorySeatRepository(demoSeats(event.ID)...)

	// The event store is shown the seats so that deleting an event can refuse
	// while one of them is held or sold, the way the foreign key check does in
	// Postgres.
	return repository.NewMemoryEventRepository(event).WithSeats(seats), seats
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

// argon2Params takes the configured memory cost, leaving everything else at the
// hasher's defaults.
func argon2Params(cfg config.Config) auth.Argon2Params {
	params := auth.DefaultArgon2Params()

	params.Memory = cfg.Argon2Memory
	params.Iterations = cfg.Argon2Iterations
	params.Parallelism = cfg.Argon2Parallelism

	return params
}

// rateLimitPolicies turns the configured numbers into the allowances the
// middleware enforces.
func rateLimitPolicies(cfg config.Config) handler.Policies {
	return handler.Policies{
		Auth:    handler.Policy{Name: "auth", Every: cfg.AuthEvery, Burst: cfg.AuthBurst},
		Default: handler.Policy{Name: "default", Every: cfg.RequestEvery, Burst: cfg.RequestBurst},
	}
}
