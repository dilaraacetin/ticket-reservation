package config

import (
	"log/slog"
	"net/url"
	"strings"
)

// redacted is what stands in for anything that must not reach a log.
const redacted = "***"

func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("addr", c.Addr),
		slog.String("store", c.storeDescription()),
		slog.String("logLevel", c.LogLevel),
		slog.String("pprof", c.pprofDescription()),
		slog.Bool("seedDemoData", c.SeedDemoData),

		slog.Group("auth",
			slog.String("secret", c.secretDescription()),
			slog.String("tokenTtl", c.TokenTTL.String()),
			slog.Uint64("argon2MemoryKib", uint64(c.Argon2Memory)),
			slog.Uint64("argon2Iterations", uint64(c.Argon2Iterations)),
			slog.Uint64("argon2Parallelism", uint64(c.Argon2Parallelism)),
		),

		slog.Group("holds",
			slog.String("ttl", c.HoldTTL.String()),
			slog.String("sweepInterval", c.SweepInterval.String()),
			slog.String("idempotencyTtl", c.IdempotencyTTL.String()),
		),

		slog.Group("mail",
			slog.String("server", c.mailDescription()),
			slog.String("from", c.SMTPFrom),
		),

		// The public key is public by definition; the private one is never named,
		// only confirmed.
		slog.Group("push",
			slog.String("state", c.pushDescription()),
			slog.String("subject", c.VAPIDSubject),
		),

		slog.Group("handoff",
			slog.Int("workers", c.HandoffWorkers),
			slog.Int("buffer", c.HandoffBuffer),
		),

		slog.Group("limits",
			slog.Int("authBurst", c.AuthBurst),
			slog.String("authEvery", c.AuthEvery.String()),
			slog.Int("requestBurst", c.RequestBurst),
			slog.String("requestEvery", c.RequestEvery.String()),
			slog.Int64("requestBodyLimit", c.RequestBodyLimit),
		),

		slog.Group("timeouts",
			slog.String("readHeader", c.ReadHeaderTimeout.String()),
			slog.String("shutdown", c.ShutdownTimeout.String()),
		),
	)
}

// storeDescription names the store without giving away how to reach it.
func (c Config) storeDescription() string {
	if !c.UsesDatabase() {
		return "memory"
	}

	parsed, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return "postgres"
	}

	return parsed.Scheme + "://" + parsed.Host + strings.TrimSuffix(parsed.Path, "/")
}

// pprofDescription makes a profiler that is listening impossible to miss in the
// startup line, because one left on is a way to read the process's memory.
func (c Config) pprofDescription() string {
	if c.PprofAddr == "" {
		return "off"
	}

	return "listening on " + c.PprofAddr
}

// mailDescription names the server without the password that reaches it.
func (c Config) mailDescription() string {
	if c.SMTPAddr == "" {
		return "off"
	}

	if c.SMTPUsername != "" {
		return c.SMTPAddr + " (authenticated)"
	}

	return c.SMTPAddr
}

// pushDescription says whether push is on without printing the key that signs it.
func (c Config) pushDescription() string {
	if c.VAPIDPublicKey == "" {
		return "off"
	}

	return "on"
}

// secretDescription confirms a secret is set without being one.
func (c Config) secretDescription() string {
	if c.AuthSecret == "" {
		return "unset"
	}

	return redacted
}

var _ slog.LogValuer = Config{}
