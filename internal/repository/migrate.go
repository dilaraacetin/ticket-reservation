package repository

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const (
	migrationDialect = "postgres"
	migrationDir     = "migrations"
)

func Migrate(ctx context.Context, databaseURL string) (int64, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return 0, fmt.Errorf("opening the database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("reaching the database: %w", err)
	}

	provider, err := newMigrationProvider(db)
	if err != nil {
		return 0, err
	}

	if _, err := provider.Up(ctx); err != nil {
		return 0, fmt.Errorf("applying migrations: %w", err)
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("reading the schema version: %w", err)
	}

	return version, nil
}

func MigrateDown(ctx context.Context, databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("opening the database: %w", err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db)
	if err != nil {
		return err
	}

	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("rolling back a migration: %w", err)
	}

	return nil
}

func newMigrationProvider(db *sql.DB) (*goose.Provider, error) {
	root, err := migrationRoot()
	if err != nil {
		return nil, err
	}

	provider, err := goose.NewProvider(migrationDialect, db, root)
	if err != nil {
		return nil, fmt.Errorf("preparing migrations: %w", err)
	}

	return provider, nil
}

func migrationRoot() (fs.FS, error) {
	root, err := fs.Sub(migrationFiles, migrationDir)
	if err != nil {
		return nil, fmt.Errorf("reading embedded migrations: %w", err)
	}

	return root, nil
}
