// Command migrate applies or rolls back database migrations.
//
// Usage:
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down
//
// It is a separate command rather than something the server does on startup, so
// that changing the schema stays a deliberate step instead of a side effect of a
// deployment.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"ticket-reservation/internal/config"
	"ticket-reservation/internal/repository"
)

const timeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.UsesDatabase() {
		return errors.New("DATABASE_URL is not set")
	}

	if len(os.Args) != 2 {
		return fmt.Errorf("expected one argument, up or down, got %d", len(os.Args)-1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch command := os.Args[1]; command {
	case "up":
		version, err := repository.Migrate(ctx, cfg.DatabaseURL)
		if err != nil {
			return err
		}

		fmt.Printf("schema is at version %d\n", version)
	case "down":
		if err := repository.MigrateDown(ctx, cfg.DatabaseURL); err != nil {
			return err
		}

		fmt.Println("rolled back one migration")
	default:
		return fmt.Errorf("unknown command %q, expected up or down", command)
	}

	return nil
}
