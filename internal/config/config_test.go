package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	for _, name := range []string{"ADDR", "DATABASE_URL", "HOLD_TTL", "SWEEP_INTERVAL", "SHUTDOWN_TIMEOUT", "LOG_LEVEL"} {
		t.Setenv(name, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != DefaultAddr {
		t.Errorf("Addr = %q, want %q", cfg.Addr, DefaultAddr)
	}
	if cfg.HoldTTL != DefaultHoldTTL {
		t.Errorf("HoldTTL = %s, want %s", cfg.HoldTTL, DefaultHoldTTL)
	}
	if cfg.SweepInterval != DefaultSweepInterval {
		t.Errorf("SweepInterval = %s, want %s", cfg.SweepInterval, DefaultSweepInterval)
	}

	// No DATABASE_URL means the in-memory stores, which is what keeps the server
	// runnable without Docker.
	if cfg.UsesDatabase() {
		t.Error("UsesDatabase() = true with DATABASE_URL unset")
	}
}

func TestLoad_ReadsTheEnvironment(t *testing.T) {
	t.Setenv("ADDR", ":9000")
	t.Setenv("DATABASE_URL", "postgres://ticket@localhost:5432/ticket_reservation")
	t.Setenv("HOLD_TTL", "90s")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != ":9000" {
		t.Errorf("Addr = %q, want :9000", cfg.Addr)
	}
	if cfg.HoldTTL != 90*time.Second {
		t.Errorf("HoldTTL = %s, want 1m30s", cfg.HoldTTL)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if !cfg.UsesDatabase() {
		t.Error("UsesDatabase() = false with DATABASE_URL set")
	}
}

func TestLoad_DurationFormats(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"go duration", "2m30s", 150 * time.Second},
		{"bare seconds", "45", 45 * time.Second},
		{"hours", "1h", time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOLD_TTL", tt.value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.HoldTTL != tt.want {
				t.Errorf("HoldTTL = %s, want %s", cfg.HoldTTL, tt.want)
			}
		})
	}
}

func TestLoad_RejectsBadValues(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		wants string
	}{
		{"unparseable duration", map[string]string{"HOLD_TTL": "soon"}, "HOLD_TTL"},
		{"zero hold ttl", map[string]string{"HOLD_TTL": "0"}, "HOLD_TTL"},
		{"negative hold ttl", map[string]string{"HOLD_TTL": "-5s"}, "HOLD_TTL"},
		{"zero sweep interval", map[string]string{"SWEEP_INTERVAL": "0s"}, "SWEEP_INTERVAL"},
		{"zero shutdown timeout", map[string]string{"SHUTDOWN_TIMEOUT": "0"}, "SHUTDOWN_TIMEOUT"},
		{"unknown log level", map[string]string{"LOG_LEVEL": "chatty"}, "LOG_LEVEL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			_, err := Load()
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Load() error = %v, want it to wrap %v", err, ErrInvalidConfig)
			}

			if got := err.Error(); !strings.Contains(got, tt.wants) {
				t.Errorf("error %q does not mention %s", got, tt.wants)
			}
		})
	}
}
