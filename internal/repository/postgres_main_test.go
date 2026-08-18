package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDatabaseURL string

func TestMain(m *testing.M) {
	code, err := runTests(m)
	if err != nil {

		fmt.Fprintf(os.Stderr, "postgres tests will be skipped: %v\n", err)
	}

	os.Exit(code)
}

func runTests(m *testing.M) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("ticket_reservation_test"),
		tcpostgres.WithUsername("ticket"),
		tcpostgres.WithPassword("ticket"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return m.Run(), err
	}

	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "terminating the test container: %v\n", err)
		}
	}()

	url, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return m.Run(), err
	}

	if _, err := Migrate(ctx, url); err != nil {
		return m.Run(), fmt.Errorf("migrating the test database: %w", err)
	}

	testDatabaseURL = url

	return m.Run(), nil
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testDatabaseURL == "" {
		t.Skip("no test database: is Docker running?")
	}

	pool, err := pgxpool.New(t.Context(), testDatabaseURL)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}

	t.Cleanup(pool.Close)

	return pool
}

func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(t.Context(), "truncate table seats, events"); err != nil {
		t.Fatalf("resetting the schema failed: %v", err)
	}
}

func uniqueEventID(t *testing.T) string {
	t.Helper()

	return "event-" + t.Name()
}
