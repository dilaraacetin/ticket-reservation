package repository

import (
	"io/fs"
	"strings"
	"testing"
)

// TestMigrationsAreEmbedded catches the mistake that cost us a debugging round:
// The embed directive keeps the directory prefix, so a provider reading from the
// file system finds nothing and reports "no migrations found" at runtime. This
// fails at test time instead, with no database involved.
func TestMigrationsAreEmbedded(t *testing.T) {
	root, err := migrationRoot()
	if err != nil {
		t.Fatalf("migrationRoot() error = %v", err)
	}

	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		t.Fatalf("reading the migration root failed: %v", err)
	}

	var found []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			found = append(found, entry.Name())
		}
	}

	if len(found) == 0 {
		t.Fatal("no .sql migration is embedded at the root goose reads from")
	}

	for _, name := range found {
		content, err := fs.ReadFile(root, name)
		if err != nil {
			t.Fatalf("reading %s failed: %v", name, err)
		}

		for _, marker := range []string{"-- +goose Up", "-- +goose Down"} {
			if !strings.Contains(string(content), marker) {
				t.Errorf("%s has no %q section", name, marker)
			}
		}
	}
}
