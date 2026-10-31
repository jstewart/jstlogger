package logbook

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
)

// ErrSchemaTooNew is returned (wrapped) when the database was migrated by a
// newer version of the program than this one.
var ErrSchemaTooNew = errors.New("logbook: database schema is newer than this program")

// migrationName matches NNNN_description.sql.
var migrationName = regexp.MustCompile(`^([0-9]+)_[^/]+\.sql$`)

// migration is one schema migration file.
type migration struct {
	version int
	name    string
}

// migrate applies every migration in fsys newer than the database's
// user_version, each in its own transaction, and records its number in
// user_version.
func migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error {
	ms, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("logbook: migrate: %w", err)
	}

	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("logbook: migrate: read user_version: %w", err)
	}
	if current < 0 {
		return fmt.Errorf("logbook: migrate: invalid database version %d", current)
	}
	if current > len(ms) {
		return fmt.Errorf("logbook: migrate: database at version %d, latest known is %d: %w",
			current, len(ms), ErrSchemaTooNew)
	}

	for _, m := range ms[current:] {
		if err := applyMigration(ctx, db, fsys, m); err != nil {
			return fmt.Errorf("logbook: migrate: %s: %w", m.name, err)
		}
	}
	return nil
}

// loadMigrations lists the *.sql files in fsys sorted by number. It rejects
// malformed names, duplicate numbers and gaps, so the result's element i is
// migration i+1.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	names, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}

	ms := make([]migration, 0, len(names))
	for _, name := range names {
		m := migrationName.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("migration %q: name must be NNNN_description.sql", name)
		}
		v, err := strconv.Atoi(m[1])
		if err != nil || v < 1 {
			return nil, fmt.Errorf("migration %q: number must be a positive integer", name)
		}
		ms = append(ms, migration{version: v, name: name})
	}
	slices.SortFunc(ms, func(a, b migration) int { return cmp.Compare(a.version, b.version) })

	for i, m := range ms {
		want := i + 1
		switch {
		case m.version < want:
			return nil, fmt.Errorf("duplicate migration number %d: %q and %q", m.version, ms[i-1].name, m.name)
		case m.version > want:
			return nil, fmt.Errorf("missing migration %d before %q", want, m.name)
		}
	}
	return ms, nil
}

// applyMigration runs m and sets user_version to its number in one
// transaction.
func applyMigration(ctx context.Context, db *sql.DB, fsys fs.FS, m migration) (err error) {
	body, err := fs.ReadFile(fsys, m.name)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, rollback(tx))
		}
	}()

	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	// PRAGMA takes no placeholders; m.version is an integer parsed from the
	// file name, never user input.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// rollback rolls tx back, ignoring the error from a transaction that has
// already been committed or rolled back.
func rollback(tx *sql.Tx) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}
