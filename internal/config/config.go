package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	DefaultAuthBurst    = 5
	DefaultAuthEvery    = 6 * time.Second
	DefaultRequestBurst = 30
	DefaultRequestEvery = time.Second

	DefaultArgon2MemoryKiB   = 19456
	DefaultArgon2Iterations  = 2
	DefaultArgon2Parallelism = 1

	DefaultHandoffWorkers = 16
	DefaultHandoffBuffer  = 256

	DefaultReadHeaderTimeout = 10 * time.Second
	DefaultRequestBodyLimit  = 8 << 10
	DefaultLogLevel          = "info"

	DefaultSMTPFrom = "tickets@localhost"

	DefaultVerificationTTL = 24 * time.Hour
)

// Config is the whole of the process's configuration.
type Config struct {
	Addr              string
	DatabaseURL       string
	HoldTTL           time.Duration
	SweepInterval     time.Duration
	ShutdownTimeout   time.Duration
	IdempotencyTTL    time.Duration
	AuthSecret        string
	TokenTTL          time.Duration
	Argon2Memory      uint32
	Argon2Iterations  uint32
	Argon2Parallelism uint8

	HandoffWorkers int
	HandoffBuffer  int

	RateLimitTTL time.Duration
	AuthBurst    int
	AuthEvery    time.Duration
	RequestBurst int
	RequestEvery time.Duration

	ReadHeaderTimeout time.Duration
	RequestBodyLimit  int64

	// SMTPAddr turns email on when it is set, as host:port. Empty means notices
	// are kept in the application and not sent anywhere.
	SMTPAddr     string
	SMTPFrom     string
	SMTPUsername string
	SMTPPassword string

	// PublicURL is where this service is reached from outside, with no trailing
	// slash. A link in an email has to be absolute, and nothing can work that
	// out from a request it is not handling.
	PublicURL string

	// VerificationTTL is how long an address verification link works for.
	VerificationTTL time.Duration

	// VAPID keys turn push on when both are set. Generate a pair with:
	// go run ./cmd/vapid
	VAPIDPublicKey  string
	VAPIDPrivateKey string

	// VAPIDSubject says who is sending, as a mailto: or https: URL. The push
	// services use it to reach somebody when something is wrong.
	VAPIDSubject string

	// PprofAddr turns on the profiler when it is set, on the address it names.
	//
	// Empty by default, and an address rather than a flag on purpose: pprof will
	// hand anyone who can reach it a heap dump, which is every secret the process
	// is holding, and a CPU profile they can ask for over and over. It belongs on
	// 127.0.0.1, never on the port the internet talks to.
	PprofAddr string

	SeedDemoData bool

	LogLevel string
}

func (c Config) UsesDatabase() bool {
	return c.DatabaseURL != ""
}

// Load reads the configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		Addr:        stringVar("ADDR", DefaultAddr),
		DatabaseURL: stringVar("DATABASE_URL", ""),
		AuthSecret:  lookup("AUTH_SECRET"),
		LogLevel:    stringVar("LOG_LEVEL", DefaultLogLevel),
		PprofAddr:   stringVar("PPROF_ADDR", ""),

		SMTPAddr:     stringVar("SMTP_ADDR", ""),
		SMTPFrom:     stringVar("SMTP_FROM", DefaultSMTPFrom),
		SMTPUsername: stringVar("SMTP_USERNAME", ""),
		SMTPPassword: stringVar("SMTP_PASSWORD", ""),

		VAPIDPublicKey:  stringVar("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey: stringVar("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:    stringVar("VAPID_SUBJECT", ""),

		PublicURL: strings.TrimSuffix(stringVar("PUBLIC_URL", ""), "/"),
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
	if cfg.VerificationTTL, err = durationVar("VERIFICATION_TTL", DefaultVerificationTTL); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitTTL, err = durationVar("RATE_LIMIT_TTL", DefaultRateLimitTTL); err != nil {
		return Config{}, err
	}
	if cfg.AuthEvery, err = durationVar("RATE_LIMIT_AUTH_EVERY", DefaultAuthEvery); err != nil {
		return Config{}, err
	}
	if cfg.RequestEvery, err = durationVar("RATE_LIMIT_EVERY", DefaultRequestEvery); err != nil {
		return Config{}, err
	}
	if cfg.ReadHeaderTimeout, err = durationVar("READ_HEADER_TIMEOUT", DefaultReadHeaderTimeout); err != nil {
		return Config{}, err
	}

	if cfg.AuthBurst, err = intVar("RATE_LIMIT_AUTH_BURST", DefaultAuthBurst); err != nil {
		return Config{}, err
	}
	if cfg.RequestBurst, err = intVar("RATE_LIMIT_BURST", DefaultRequestBurst); err != nil {
		return Config{}, err
	}
	if cfg.HandoffWorkers, err = intVar("HANDOFF_WORKERS", DefaultHandoffWorkers); err != nil {
		return Config{}, err
	}
	if cfg.HandoffBuffer, err = intVar("HANDOFF_BUFFER", DefaultHandoffBuffer); err != nil {
		return Config{}, err
	}

	iterations, err := intVar("ARGON2_ITERATIONS", DefaultArgon2Iterations)
	if err != nil {
		return Config{}, err
	}

	parallelism, err := intVar("ARGON2_PARALLELISM", DefaultArgon2Parallelism)
	if err != nil {
		return Config{}, err
	}

	bodyLimit, err := intVar("REQUEST_BODY_LIMIT", DefaultRequestBodyLimit)
	if err != nil {
		return Config{}, err
	}

	cfg.Argon2Iterations = uint32(max(0, iterations))
	cfg.Argon2Parallelism = uint8(min(max(0, parallelism), 255))
	cfg.RequestBodyLimit = int64(bodyLimit)

	if cfg.SeedDemoData, err = boolVar("SEED_DEMO_DATA", false); err != nil {
		return Config{}, err
	}

	memory, err := intVar("ARGON2_MEMORY_KIB", DefaultArgon2MemoryKiB)
	if err != nil {
		return Config{}, err
	}

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
	if c.VerificationTTL <= 0 {
		return fmt.Errorf("%w: VERIFICATION_TTL must be positive, got %s", ErrInvalidConfig, c.VerificationTTL)
	}
	// Only when email is on: without it nothing sends a link, so there is
	// nothing for an absolute URL to be in.
	if c.SMTPAddr != "" && c.PublicURL == "" {
		return fmt.Errorf("%w: PUBLIC_URL is required when SMTP_ADDR is set, so links in email are absolute",
			ErrInvalidConfig)
	}
	if c.TokenTTL <= 0 {
		return fmt.Errorf("%w: TOKEN_TTL must be positive, got %s", ErrInvalidConfig, c.TokenTTL)
	}
	if c.AuthSecret == "" {
		return fmt.Errorf("%w: AUTH_SECRET is required. Generate one with: %s",
			ErrInvalidConfig, SecretGenerationHint)
	}
	if len(c.AuthSecret) < MinAuthSecretLength {
		return fmt.Errorf("%w: AUTH_SECRET must be at least %d bytes, got %d. Generate one with: %s",
			ErrInvalidConfig, MinAuthSecretLength, len(c.AuthSecret), SecretGenerationHint)
	}
	if c.HandoffWorkers < 1 {
		return fmt.Errorf("%w: HANDOFF_WORKERS must be at least 1, got %d", ErrInvalidConfig, c.HandoffWorkers)
	}
	if c.HandoffBuffer < 1 {
		return fmt.Errorf("%w: HANDOFF_BUFFER must be at least 1, got %d", ErrInvalidConfig, c.HandoffBuffer)
	}
	if c.AuthBurst < 1 || c.RequestBurst < 1 {
		return fmt.Errorf("%w: rate limit bursts must be at least 1, got %d and %d",
			ErrInvalidConfig, c.AuthBurst, c.RequestBurst)
	}
	if c.AuthEvery <= 0 || c.RequestEvery <= 0 {
		return fmt.Errorf("%w: rate limit intervals must be positive, got %s and %s",
			ErrInvalidConfig, c.AuthEvery, c.RequestEvery)
	}
	if c.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("%w: READ_HEADER_TIMEOUT must be positive, got %s", ErrInvalidConfig, c.ReadHeaderTimeout)
	}
	if c.RequestBodyLimit < 1024 {
		return fmt.Errorf("%w: REQUEST_BODY_LIMIT must be at least 1024, got %d", ErrInvalidConfig, c.RequestBodyLimit)
	}
	if c.Argon2Memory < 8 {
		return fmt.Errorf("%w: ARGON2_MEMORY_KIB must be at least 8, got %d", ErrInvalidConfig, c.Argon2Memory)
	}
	if c.Argon2Iterations < 1 {
		return fmt.Errorf("%w: ARGON2_ITERATIONS must be at least 1, got %d", ErrInvalidConfig, c.Argon2Iterations)
	}
	if c.Argon2Parallelism < 1 {
		return fmt.Errorf("%w: ARGON2_PARALLELISM must be at least 1, got %d", ErrInvalidConfig, c.Argon2Parallelism)
	}
	// Only checked when email is on: an address nobody sends from does not have
	// to be valid.
	if c.SMTPAddr != "" && c.SMTPFrom == "" {
		return fmt.Errorf("%w: SMTP_FROM is required when SMTP_ADDR is set", ErrInvalidConfig)
	}
	// Half a pair is worse than none: it would look configured and refuse every
	// push.
	if (c.VAPIDPublicKey == "") != (c.VAPIDPrivateKey == "") {
		return fmt.Errorf("%w: VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY go together", ErrInvalidConfig)
	}
	if c.VAPIDPublicKey != "" && c.VAPIDSubject == "" {
		return fmt.Errorf("%w: VAPID_SUBJECT is required with the VAPID keys, as a mailto: or https: URL",
			ErrInvalidConfig)
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("%w: LOG_LEVEL %q is not one of debug, info, warn, error", ErrInvalidConfig, c.LogLevel)
	}

	return nil
}

const MinAuthSecretLength = 32
const SecretGenerationHint = "openssl rand -base64 32"

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

func lookup(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func stringVar(name, fallback string) string {
	if value := lookup(name); value != "" {
		return value
	}

	return fallback
}

// intVar reads a whole number, so that a typo is a startup failure rather than a
// silent fallback to the default.
func intVar(name string, fallback int) (int, error) {
	value := lookup(name)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not a whole number", ErrInvalidConfig, name, value)
	}

	return parsed, nil
}

// boolVar accepts the spellings people actually use rather than only Go's.
func boolVar(name string, fallback bool) (bool, error) {
	value := lookup(name)
	if value == "" {
		return fallback, nil
	}

	switch strings.ToLower(value) {
	case "1", "t", "true", "yes", "on":
		return true, nil
	case "0", "f", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %s=%q is not true or false", ErrInvalidConfig, name, value)
	}
}

func durationVar(name string, fallback time.Duration) (time.Duration, error) {
	value := lookup(name)
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

// LoadDatabaseURL reads only the database address.
func LoadDatabaseURL() (string, error) {
	url := lookup("DATABASE_URL")
	if url == "" {
		return "", fmt.Errorf("%w: DATABASE_URL is required", ErrInvalidConfig)
	}

	return url, nil
}
