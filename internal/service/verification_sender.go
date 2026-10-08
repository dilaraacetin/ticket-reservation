package service

import (
	"context"
	"log/slog"
	"time"

	"ticket-reservation/internal/notify"
	"ticket-reservation/internal/repository"
)

const (
	// DefaultVerificationInterval is how often unsent links are looked for.
	// Shorter than the delivery worker's, because somebody who has just
	// registered is watching their inbox.
	DefaultVerificationInterval = 2 * time.Second

	// DefaultVerificationBatch bounds one pass.
	DefaultVerificationBatch = 20
)

// Linker turns a token into the absolute URL that goes in the message.
type Linker interface {
	Link(token string) string
}

// VerificationSender emails the links nobody has sent yet.
//
// Its own queue rather than the notification outbox, for two reasons. The outbox
// refuses to send to addresses nobody has verified, which is every address this
// sends to; and a verification token kept in the notifications table would be
// readable through the notifications endpoint.
type VerificationSender struct {
	verifications repository.VerificationRepository
	mailer        EmailSender
	linker        Linker
	clock         Clock
	logger        *slog.Logger
	interval      time.Duration
	batch         int
}

// VerificationSenderConfig carries the sender's dependencies.
type VerificationSenderConfig struct {
	Verifications repository.VerificationRepository
	Mailer        EmailSender
	Linker        Linker
	Clock         Clock
	Logger        *slog.Logger
	Interval      time.Duration
	Batch         int
}

func NewVerificationSender(cfg VerificationSenderConfig) *VerificationSender {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultVerificationInterval
	}

	if cfg.Batch <= 0 {
		cfg.Batch = DefaultVerificationBatch
	}

	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}

	return &VerificationSender{
		verifications: cfg.Verifications,
		mailer:        cfg.Mailer,
		linker:        cfg.Linker,
		clock:         cfg.Clock,
		logger:        cfg.Logger,
		interval:      cfg.Interval,
		batch:         cfg.Batch,
	}
}

// Run sends on every tick until ctx is cancelled. It blocks, so callers start it
// in its own goroutine.
func (s *VerificationSender) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.logger.InfoContext(ctx, "verification sender started", "interval", s.interval)

	for {
		select {
		case <-ctx.Done():
			s.logger.InfoContext(ctx, "verification sender stopped")

			return nil
		case <-ticker.C:
			s.Send(ctx)
		}
	}
}

// Send works through one batch and reports how many went.
func (s *VerificationSender) Send(ctx context.Context) int {
	if s.mailer == nil {
		// Nothing to send with. The links sit unsent rather than being marked
		// sent and lost, so turning email on later still gets them out.
		return 0
	}

	// Claiming marks them sent, so two senders cannot both email the same link.
	// A failure below gives it back.
	claimed, err := s.verifications.ClaimUnsent(ctx, s.clock.Now(), s.batch)
	if err != nil {
		s.logger.ErrorContext(ctx, "claiming verifications failed", "err", err)

		return 0
	}

	sent := 0

	for _, verification := range claimed {
		notice := notify.VerificationNotice(s.linker.Link(verification.Token))

		if err := s.mailer.SendEmail(ctx, verification.Email, notice); err != nil {
			s.logger.WarnContext(ctx, "a verification link could not be sent",
				"err", err,
				"userId", verification.UserID,
			)

			if err := s.verifications.Release(ctx, verification.Token); err != nil {
				s.logger.ErrorContext(ctx, "a verification could not be given back", "err", err)
			}

			continue
		}

		sent++
	}

	if sent > 0 {
		s.logger.InfoContext(ctx, "sent verification links", "count", sent)
	}

	return sent
}
