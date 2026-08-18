package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Defaults used when a variable is unset.
const (
	DefaultAddr            = ":8080"
	DefaultHoldTTL         = 5 * time.Minute
	DefaultSweepInterval   = 30 * time.Second
	DefaultShutdownTimeout = 10 * time.Second
	DefaultIdempotencyTTL  = 24 * time.Hour
	DefaultTokenTTL        = time.Hour

	DefaultRateLimitTTL = 10 * time.Minute
	DefaultLogLevel     = "info"
	DevAuthSecret       = "k9Xm2pQrS4tU6vW8yZ0aB3dEf1GhIjKl"
)

// Config is the whole of the process's configuration.
type Config struct {
	Addr            string
	DatabaseURL     string
	HoldTTL         time.Duration
	SweepInterval   time.Duration
	ShutdownTimeout time.Duration
	IdempotencyTTL  time.Duration
	AuthSecret      string
	TokenTTL        time.Duration
	Argon2Memory    uint32
	RateLimitTTL    time.Duration
	LogLevel        string
}

func (c Config) UsesDatabase() bool {
	return c.DatabaseURL != ""
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		Addr:        stringVar("ADDR", DefaultAddr),
		DatabaseURL: stringVar("DATABASE_URL", ""),
		AuthSecret:  stringVar("AUTH_SECRET", DevAuthSecret),
		LogLevel:    stringVar("LOG_LEVEL", DefaultLogLevel),
	}

	var err error

	if cfg.HoldTTL, err = durationVar("HOLD_TTL", DefaultHoldTTL); err != nil {
		return Config{}, err
	}
	if cfg.SweepInterval, err = durationVar("SWEEP_INTERVAL", DefaultSweepInterval); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationVar("SHUTDOWN_TIMEOUT", DefaultShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.IdempotencyTTL, err = durationVar("IDEMPOTENCY_TTL", DefaultIdempotencyTTL); err != nil {
		return Config{}, err
	}
	if cfg.TokenTTL, err = durationVar("TOKEN_TTL", DefaultTokenTTL); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitTTL, err = durationVar("RATE_LIMIT_TTL", DefaultRateLimitTTL); err != nil {
		return Config{}, err
	}

	memory, _ := strconv.Atoi(os.Getenv("ARGON2_MEMORY_KIB"))
	cfg.Argon2Memory = uint32(max(0, memory))

	return cfg, cfg.validate()
}

// validate rejects values that would produce a broken process. A non-positive
// hold duration, for instance, would make every hold expire the moment it is
// created.
func (c Config) validate() error {
	if c.Addr == "" {
		return fmt.Errorf("%w: ADDR must not be empty", ErrInvalidConfig)
	}
	if c.HoldTTL <= 0 {
		return fmt.Errorf("%w: HOLD_TTL must be positive, got %s", ErrInvalidConfig, c.HoldTTL)
	}
	if c.SweepInterval <= 0 {
		return fmt.Errorf("%w: SWEEP_INTERVAL must be positive, got %s", ErrInvalidConfig, c.SweepInterval)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("%w: SHUTDOWN_TIMEOUT must be positive, got %s", ErrInvalidConfig, c.ShutdownTimeout)
	}
	if c.IdempotencyTTL <= 0 {
		return fmt.Errorf("%w: IDEMPOTENCY_TTL must be positive, got %s", ErrInvalidConfig, c.IdempotencyTTL)
	}
	if c.RateLimitTTL <= 0 {
		return fmt.Errorf("%w: RATE_LIMIT_TTL must be positive, got %s", ErrInvalidConfig, c.RateLimitTTL)
	}
	if c.TokenTTL <= 0 {
		return fmt.Errorf("%w: TOKEN_TTL must be positive, got %s", ErrInvalidConfig, c.TokenTTL)
	}
	if len(c.AuthSecret) < MinAuthSecretLength {
		return fmt.Errorf("%w: AUTH_SECRET must be at least %d bytes, got %d",
			ErrInvalidConfig, MinAuthSecretLength, len(c.AuthSecret))
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("%w: LOG_LEVEL %q is not one of debug, info, warn, error", ErrInvalidConfig, c.LogLevel)
	}

	return nil
}

const MinAuthSecretLength = 32

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

func stringVar(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func durationVar(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not a duration", ErrInvalidConfig, name, value)
	}

	return parsed, nil
}
