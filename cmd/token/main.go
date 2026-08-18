package main

import (
	"fmt"
	"os"
	"time"

	"ticket-reservation/internal/auth"
	"ticket-reservation/internal/config"
)

const defaultLifetime = time.Hour

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "token:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		return fmt.Errorf("usage: token <user-id> [lifetime], got %d arguments", len(os.Args)-1)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	tokens, err := auth.NewTokens(cfg.AuthSecret)
	if err != nil {
		return err
	}

	lifetime := defaultLifetime
	if len(os.Args) == 3 {
		if lifetime, err = time.ParseDuration(os.Args[2]); err != nil {
			return fmt.Errorf("lifetime %q is not a duration: %w", os.Args[2], err)
		}
	}

	expiresAt := time.Now().Add(lifetime)

	token, err := tokens.Issue(os.Args[1], expiresAt)
	if err != nil {
		return err
	}

	// The token on its own line, so it can be piped straight into a request.
	fmt.Fprintf(os.Stderr, "user %s, valid until %s\n", os.Args[1], expiresAt.Format(time.RFC3339))
	fmt.Println(token)

	return nil
}
