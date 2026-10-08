package config

import (
	"errors"
	"strings"
	"testing"
	"time"

	"ticket-reservation/internal/auth"
)

const testAuthSecret = "3f9c1d47a8e05b62c4907fd318ae5b7d20c6e394af8152bd7e0c46a91d3fb508"

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("AUTH_SECRET", testAuthSecret)

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
	t.Setenv("AUTH_SECRET", testAuthSecret)
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
			t.Setenv("AUTH_SECRET", testAuthSecret)
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
		{"short auth secret", map[string]string{"AUTH_SECRET": "too-short"}, "AUTH_SECRET"},
		{"missing auth secret", map[string]string{"AUTH_SECRET": ""}, "AUTH_SECRET"},
		{"auth secret of only whitespace", map[string]string{"AUTH_SECRET": "   "}, "AUTH_SECRET"},
		{"burst of zero", map[string]string{"RATE_LIMIT_BURST": "0"}, "burst"},
		{"negative auth burst", map[string]string{"RATE_LIMIT_AUTH_BURST": "-1"}, "burst"},
		{"burst is not a number", map[string]string{"RATE_LIMIT_BURST": "lots"}, "RATE_LIMIT_BURST"},
		{"zero rate interval", map[string]string{"RATE_LIMIT_EVERY": "0s"}, "interval"},
		{"body limit below a kilobyte", map[string]string{"REQUEST_BODY_LIMIT": "10"}, "REQUEST_BODY_LIMIT"},
		{"zero argon2 iterations", map[string]string{"ARGON2_ITERATIONS": "0"}, "ARGON2_ITERATIONS"},
		{"zero argon2 parallelism", map[string]string{"ARGON2_PARALLELISM": "0"}, "ARGON2_PARALLELISM"},
		{"seeding is not a boolean", map[string]string{"SEED_DEMO_DATA": "maybe"}, "SEED_DEMO_DATA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUTH_SECRET", testAuthSecret)

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

// The minimum is stated in two packages so that config imports nothing. This
// keeps the two from drifting apart.
func TestMinAuthSecretLengthMatchesTheAuthPackage(t *testing.T) {
	if MinAuthSecretLength != auth.MinSecretLength {
		t.Errorf("config says %d, auth says %d", MinAuthSecretLength, auth.MinSecretLength)
	}
}

// The binary must carry no secret of its own. A default in the source is a
// default everyone reading the repository knows, and a deployment that starts on
// it is one anyone can forge a token for. This is why the refusal exists, so it
// is worth a test that says so.
func TestLoad_RefusesToStartWithoutASecret(t *testing.T) {
	t.Setenv("AUTH_SECRET", "")

	_, err := Load()
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Load() error = %v, want it to wrap %v", err, ErrInvalidConfig)
	}

	if !strings.Contains(err.Error(), SecretGenerationHint) {
		t.Errorf("the refusal does not say how to generate a secret: %q", err)
	}
}

// Values arrive through shells, .env files, Make and container runtimes, any of
// which can leave a space behind. One invisible character should not be the
// difference between a process that starts and one that does not.
func TestLoad_TrimsSurroundingWhitespace(t *testing.T) {
	t.Setenv("AUTH_SECRET", "  "+testAuthSecret+"\n")
	t.Setenv("HOLD_TTL", "5m   ")
	t.Setenv("ADDR", " :8080 ")
	t.Setenv("SEED_DEMO_DATA", "true ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AuthSecret != testAuthSecret {
		t.Errorf("AuthSecret = %q, want it trimmed", cfg.AuthSecret)
	}
	if cfg.HoldTTL != 5*time.Minute {
		t.Errorf("HoldTTL = %s, want 5m", cfg.HoldTTL)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if !cfg.SeedDemoData {
		t.Error("SeedDemoData = false, want true")
	}
}

// Migrating the schema has nothing to do with signing tokens. Demanding a secret
// for it would be a rule with no reason behind it, and those are the ones people
// work around.
func TestLoadDatabaseURL(t *testing.T) {
	t.Setenv("AUTH_SECRET", "")
	t.Setenv("DATABASE_URL", " postgres://ticket@localhost:5433/ticket_reservation ")

	url, err := LoadDatabaseURL()
	if err != nil {
		t.Fatalf("LoadDatabaseURL() error = %v", err)
	}
	if url != "postgres://ticket@localhost:5433/ticket_reservation" {
		t.Errorf("url = %q, want it trimmed", url)
	}

	t.Setenv("DATABASE_URL", "")

	if _, err := LoadDatabaseURL(); !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("LoadDatabaseURL() error = %v, want it to wrap %v", err, ErrInvalidConfig)
	}
}

// Everything has a default, so a process with nothing set at all still starts.
func TestLoad_NewSettingsHaveDefaults(t *testing.T) {
	for _, name := range []string{
		"RATE_LIMIT_AUTH_BURST", "RATE_LIMIT_AUTH_EVERY", "RATE_LIMIT_BURST", "RATE_LIMIT_EVERY",
		"ARGON2_ITERATIONS", "ARGON2_PARALLELISM", "READ_HEADER_TIMEOUT",
		"REQUEST_BODY_LIMIT", "SEED_DEMO_DATA",
	} {
		t.Setenv(name, "")
	}

	t.Setenv("AUTH_SECRET", testAuthSecret)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AuthBurst != DefaultAuthBurst || cfg.RequestBurst != DefaultRequestBurst {
		t.Errorf("bursts = %d and %d, want %d and %d",
			cfg.AuthBurst, cfg.RequestBurst, DefaultAuthBurst, DefaultRequestBurst)
	}
	if cfg.RequestBodyLimit != DefaultRequestBodyLimit {
		t.Errorf("RequestBodyLimit = %d, want %d", cfg.RequestBodyLimit, DefaultRequestBodyLimit)
	}

	// Off by default: writing demo rows has to be asked for.
	if cfg.SeedDemoData {
		t.Error("SeedDemoData is on by default")
	}
}

// The spellings people actually use, not only Go's.
func TestLoad_BooleanSpellings(t *testing.T) {
	tests := map[string]bool{
		"true": true, "TRUE": true, "1": true, "yes": true, "on": true,
		"false": false, "0": false, "no": false, "off": false,
	}

	for value, want := range tests {
		t.Run(value, func(t *testing.T) {
			t.Setenv("AUTH_SECRET", testAuthSecret)
			t.Setenv("SEED_DEMO_DATA", value)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.SeedDemoData != want {
				t.Errorf("SeedDemoData = %v, want %v", cfg.SeedDemoData, want)
			}
		})
	}
}
