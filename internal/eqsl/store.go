package eqsl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"jstlogger/internal/callsign"
	"jstlogger/internal/feed"
)

// Meta describes the last successful download of the list.
type Meta = feed.Meta

// Store keeps the eQSL AG member list in the logbook database. It is safe
// for concurrent use.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store on db, a logbook database migrated to include
// the eqsl_ag and eqsl_sync tables.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// IsAG reports whether call is an eQSL AG member. It looks up the base
// callsign (so VE3/W1AW/P finds W1AW) and then, if that is not listed, the
// whole callsign as given.
func (s *Store) IsAG(ctx context.Context, call string) (bool, error) {
	base := callsign.Base(call)
	if base == "" {
		return false, nil
	}
	ok, err := s.isAG(ctx, base)
	if err != nil || ok {
		return ok, err
	}
	if exact := strings.ToUpper(strings.TrimSpace(call)); exact != base {
		return s.isAG(ctx, exact)
	}
	return false, nil
}

// isAG looks up exactly call.
func (s *Store) isAG(ctx context.Context, call string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM eqsl_ag WHERE call = ?", call).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("eqsl: look up %s: %w", call, err)
	}
	return true, nil
}

// LastSync returns the metadata of the last successful sync, and false if
// the list has never been synced.
func (s *Store) LastSync(ctx context.Context) (Meta, bool, error) {
	var m Meta
	var syncedAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT etag, last_modified, synced_at FROM eqsl_sync WHERE id = 1").
		Scan(&m.ETag, &m.LastModified, &syncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, fmt.Errorf("eqsl: last sync: %w", err)
	}
	if m.SyncedAt, err = feed.ParseTime(syncedAt); err != nil {
		return Meta{}, false, fmt.Errorf("eqsl: last sync: %w", err)
	}
	return m, true, nil
}

// upsertMeta writes the single eqsl_sync row.
const upsertMeta = `INSERT INTO eqsl_sync (id, etag, last_modified, synced_at) VALUES (1, ?, ?, ?)
	ON CONFLICT (id) DO UPDATE SET
		etag = excluded.etag, last_modified = excluded.last_modified, synced_at = excluded.synced_at`

// replace swaps the stored members for calls and records meta, all in one
// transaction, so readers see either the old list or the new one.
func (s *Store) replace(ctx context.Context, calls []string, meta Meta) error {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM eqsl_ag"); err != nil {
			return fmt.Errorf("clear members: %w", err)
		}
		if err := insertCalls(ctx, tx, calls); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, upsertMeta, meta.ETag, meta.LastModified, feed.FormatTime(meta.SyncedAt)); err != nil {
			return fmt.Errorf("write meta: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("eqsl: replace members: %w", err)
	}
	return nil
}

// insertCalls inserts calls with one prepared statement, ignoring a
// callsign listed twice.
func insertCalls(ctx context.Context, tx *sql.Tx, calls []string) (err error) {
	stmt, err := tx.PrepareContext(ctx, "INSERT OR IGNORE INTO eqsl_ag (call) VALUES (?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() {
		if cerr := stmt.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close insert: %w", cerr))
		}
	}()
	for _, call := range calls {
		if _, err := stmt.ExecContext(ctx, call); err != nil {
			return fmt.Errorf("insert %s: %w", call, err)
		}
	}
	return nil
}

// touch records meta without changing the stored members, for a list the
// server reports unchanged.
func (s *Store) touch(ctx context.Context, meta Meta) error {
	if _, err := s.db.ExecContext(ctx, upsertMeta, meta.ETag, meta.LastModified, feed.FormatTime(meta.SyncedAt)); err != nil {
		return fmt.Errorf("eqsl: touch sync time: %w", err)
	}
	return nil
}

// inTx runs fn in a transaction, committing if it succeeds and rolling back
// otherwise.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() {
		if err != nil {
			// A cancelled context has already rolled the transaction back.
			if rerr := tx.Rollback(); rerr != nil && !errors.Is(rerr, sql.ErrTxDone) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rerr))
			}
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
