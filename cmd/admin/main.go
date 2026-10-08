// Command admin changes what an account may do.
//
// A command rather than an HTTP route, because the first administrator has to
// come from somewhere and an endpoint that can create one is an endpoint that
// can be talked into creating one. This needs database access, which is the
// point: whoever already has that is already trusted.
//
//	admin promote someone@example.com
//	admin demote  someone@example.com
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"ticket-reservation/internal/config"
	"ticket-reservation/internal/domain"
	"ticket-reservation/internal/repository"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: admin promote|demote <email>, got %d arguments", len(os.Args)-1)
	}

	var role domain.Role

	switch command := os.Args[1]; command {
	case "promote":
		role = domain.RoleAdmin
	case "demote":
		role = domain.RoleCustomer
	default:
		return fmt.Errorf("unknown command %q, expected promote or demote", command)
	}

	// Only the database address. Signing tokens has nothing to do with changing
	// a role, so demanding a signing secret here would be a rule with no reason.
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("configuring the connection pool: %w", err)
	}
	defer pool.Close()

	users := repository.NewPostgresUserRepository(pool)
	email := domain.NormalizeEmail(os.Args[2])

	// Read first, so that an address nobody has registered is reported as such
	// rather than as an update that changed no rows.
	if _, err := users.GetUserByEmail(ctx, email); err != nil {
		return fmt.Errorf("%s: %w", email, err)
	}

	if err := users.SetRole(ctx, email, role); err != nil {
		return err
	}

	fmt.Printf("%s now has the role %s\n", email, role)

	return nil
}
