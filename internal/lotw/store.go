package lotw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"jstlogger/internal/callsign"
)

// Meta describes the last successful download of the feed.
type Meta struct {
	// ETag is the feed's ETag response header, sent back as If-None-Match.
	ETag string
	// LastModified is the feed's Last-Modified response header, sent back
	// as If-Modified-Since.
	LastModified string
	// SyncedAt is when the feed was last downloaded or confirmed unchanged.
	SyncedAt time.Time
}

// Store keeps the LoTW activity feed in the logbook database. It is safe
// for concurrent use.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store on db, a logbook database migrated to include
// the lotw_user and lotw_sync tables.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// LastUpload returns the time call last uploaded to LoTW, and whether call
// is a known LoTW user. It looks up the base callsign (so VE3/W1AW/P finds
// W1AW) and then, if that is not listed, the whole callsign as given.
func (s *Store) LastUpload(ctx context.Context, call string) (time.Time, bool, error) {
	base := callsign.Base(call)
	if base == "" {
		return time.Time{}, false, nil
	}
	at, ok, err := s.lastUpload(ctx, base)
	if err != nil || ok {
		return at, ok, err
	}
	if exact := strings.ToUpper(strings.TrimSpace(call)); exact != base {
		return s.lastUpload(ctx, exact)
	}
	return time.Time{}, false, nil
}

// lastUpload looks up exactly call.
func (s *Store) lastUpload(ctx context.Context, call string) (time.Time, bool, error) {
	var text string
	err := s.db.QueryRowContext(ctx, "SELECT last_upload FROM lotw_user WHERE call = ?", call).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("lotw: look up %s: %w", call, err)
	}
	at, err := parseTime(text)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("lotw: look up %s: %w", call, err)
	}
	return at, true, nil
}

// LastSync returns the metadata of the last successful sync, and false if
// the feed has never been synced.
func (s *Store) LastSync(ctx context.Context) (Meta, bool, error) {
	var m Meta
	var syncedAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT etag, last_modified, synced_at FROM lotw_sync WHERE id = 1").
		Scan(&m.ETag, &m.LastModified, &syncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, fmt.Errorf("lotw: last sync: %w", err)
	}
	if m.SyncedAt, err = parseTime(syncedAt); err != nil {
		return Meta{}, false, fmt.Errorf("lotw: last sync: %w", err)
	}
	return m, true, nil
}

// upsertMeta writes the single lotw_sync row.
const upsertMeta = `INSERT INTO lotw_sync (id, etag, last_modified, synced_at) VALUES (1, ?, ?, ?)
	ON CONFLICT (id) DO UPDATE SET
		etag = excluded.etag, last_modified = excluded.last_modified, synced_at = excluded.synced_at`

// replace swaps the stored users for users and records meta, all in one
// transaction, so readers see either the old feed or the new one.
func (s *Store) replace(ctx context.Context, users []User, meta Meta) error {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM lotw_user"); err != nil {
			return fmt.Errorf("clear users: %w", err)
		}
		if err := insertUsers(ctx, tx, users); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, upsertMeta, meta.ETag, meta.LastModified, formatTime(meta.SyncedAt)); err != nil {
			return fmt.Errorf("write meta: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("lotw: replace users: %w", err)
	}
	return nil
}

// insertUsers inserts users with one prepared statement. A callsign listed
// twice keeps its latest upload.
func insertUsers(ctx context.Context, tx *sql.Tx, users []User) (err error) {
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO lotw_user (call, last_upload) VALUES (?, ?)
		ON CONFLICT (call) DO UPDATE SET last_upload = max(last_upload, excluded.last_upload)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() {
		if cerr := stmt.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close insert: %w", cerr))
		}
	}()
	for _, u := range users {
		if _, err := stmt.ExecContext(ctx, u.Call, formatTime(u.LastUpload)); err != nil {
			return fmt.Errorf("insert %s: %w", u.Call, err)
		}
	}
	return nil
}

// touch records meta without changing the stored users, for a feed the
// server reports unchanged.
func (s *Store) touch(ctx context.Context, meta Meta) error {
	if _, err := s.db.ExecContext(ctx, upsertMeta, meta.ETag, meta.LastModified, formatTime(meta.SyncedAt)); err != nil {
		return fmt.Errorf("lotw: touch sync time: %w", err)
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

// formatTime encodes t as RFC 3339 UTC text with second precision, so that
// text order equals time order.
func formatTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// parseTime decodes a time written by formatTime.
func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time: %w", err)
	}
	return t.UTC(), nil
}
