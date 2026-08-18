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
	DefaultLogLevel        = "info"
)

// Config is the whole of the process's configuration.
type Config struct {
	Addr            string
	DatabaseURL     string
	HoldTTL         time.Duration
	SweepInterval   time.Duration
	ShutdownTimeout time.Duration
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
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("%w: LOG_LEVEL %q is not one of debug, info, warn, error", ErrInvalidConfig, c.LogLevel)
	}

	return nil
}

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
