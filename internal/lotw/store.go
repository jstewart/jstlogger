package lotw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"jstlogger/internal/callsign"
	"jstlogger/internal/feed"
)

// Meta describes the last successful download of the feed.
type Meta = feed.Meta

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
	at, err := feed.ParseTime(text)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("lotw: look up %s: %w", call, err)
	}
	return at, true, nil
}

// lookupChunk is the most callsigns one LastUploads query looks up, well
// under SQLite's limit on query parameters.
const lookupChunk = 500

// LastUploads returns, for each of calls that is a known LoTW user, the time
// it last uploaded, keyed by the call as given. Calls that are not users,
// and calls with no base callsign, are left out. Each call is looked up as
// LastUpload does: by its base callsign and then, if that is not listed, as
// given.
func (s *Store) LastUploads(ctx context.Context, calls []string) (map[string]time.Time, error) {
	type forms struct{ base, exact string }
	keys := make([]forms, len(calls))
	want := map[string]bool{}
	for i, call := range calls {
		base := callsign.Base(call)
		if base == "" {
			continue
		}
		keys[i] = forms{base: base, exact: strings.ToUpper(strings.TrimSpace(call))}
		want[keys[i].base], want[keys[i].exact] = true, true
	}
	found := map[string]time.Time{}
	for chunk := range slices.Chunk(slices.Sorted(maps.Keys(want)), lookupChunk) {
		if err := s.lastUploads(ctx, chunk, found); err != nil {
			return nil, err
		}
	}
	out := map[string]time.Time{}
	for i, call := range calls {
		if keys[i].base == "" {
			continue
		}
		if at, ok := found[keys[i].base]; ok {
			out[call] = at
		} else if at, ok := found[keys[i].exact]; ok {
			out[call] = at
		}
	}
	return out, nil
}

// lastUploads looks up exactly calls, adding the users among them to found.
func (s *Store) lastUploads(ctx context.Context, calls []string, found map[string]time.Time) (err error) {
	query := "SELECT call, last_upload FROM lotw_user WHERE call IN (?" + strings.Repeat(", ?", len(calls)-1) + ")"
	args := make([]any, len(calls))
	for i, c := range calls {
		args[i] = c
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("lotw: look up users: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("lotw: look up users: %w", cerr))
		}
	}()
	for rows.Next() {
		var call, text string
		if err := rows.Scan(&call, &text); err != nil {
			return fmt.Errorf("lotw: look up users: %w", err)
		}
		at, err := feed.ParseTime(text)
		if err != nil {
			return fmt.Errorf("lotw: look up %s: %w", call, err)
		}
		found[call] = at
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("lotw: look up users: %w", err)
	}
	return nil
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
	if m.SyncedAt, err = feed.ParseTime(syncedAt); err != nil {
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
		if _, err := tx.ExecContext(ctx, upsertMeta, meta.ETag, meta.LastModified, feed.FormatTime(meta.SyncedAt)); err != nil {
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
		if _, err := stmt.ExecContext(ctx, u.Call, feed.FormatTime(u.LastUpload)); err != nil {
			return fmt.Errorf("insert %s: %w", u.Call, err)
		}
	}
	return nil
}

// touch records meta without changing the stored users, for a feed the
// server reports unchanged.
func (s *Store) touch(ctx context.Context, meta Meta) error {
	if _, err := s.db.ExecContext(ctx, upsertMeta, meta.ETag, meta.LastModified, feed.FormatTime(meta.SyncedAt)); err != nil {
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
