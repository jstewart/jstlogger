package dxcc

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"jstlogger/internal/feed"
)

// Meta describes the last successful download of the country file.
type Meta = feed.Meta

// Store keeps the last downloaded country file in the logbook database. It
// is safe for concurrent use.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store on db, a logbook database migrated to include
// the dxcc_cty table.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Table returns the table parsed from the stored country file, or the
// embedded snapshot from Default when none has been downloaded.
func (s *Store) Table(ctx context.Context) (*Table, error) {
	var cty []byte
	err := s.db.QueryRowContext(ctx, "SELECT cty FROM dxcc_cty WHERE id = 1").Scan(&cty)
	if errors.Is(err, sql.ErrNoRows) {
		return Default(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("dxcc: load country file: %w", err)
	}
	t, err := Parse(bytes.NewReader(cty))
	if err != nil {
		return nil, fmt.Errorf("dxcc: load country file: %w", err)
	}
	return t, nil
}

// LastSync returns the metadata of the last successful sync, and false if
// the country file has never been synced.
func (s *Store) LastSync(ctx context.Context) (Meta, bool, error) {
	var m Meta
	var syncedAt string
	err := s.db.QueryRowContext(ctx,
		"SELECT etag, last_modified, synced_at FROM dxcc_cty WHERE id = 1").
		Scan(&m.ETag, &m.LastModified, &syncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, fmt.Errorf("dxcc: last sync: %w", err)
	}
	if m.SyncedAt, err = feed.ParseTime(syncedAt); err != nil {
		return Meta{}, false, fmt.Errorf("dxcc: last sync: %w", err)
	}
	return m, true, nil
}

// replace stores cty, a valid cty.csv, and records meta.
func (s *Store) replace(ctx context.Context, cty []byte, meta Meta) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO dxcc_cty (id, cty, etag, last_modified, synced_at) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			cty = excluded.cty, etag = excluded.etag,
			last_modified = excluded.last_modified, synced_at = excluded.synced_at`,
		cty, meta.ETag, meta.LastModified, feed.FormatTime(meta.SyncedAt))
	if err != nil {
		return fmt.Errorf("dxcc: replace country file: %w", err)
	}
	return nil
}

// touch records meta without changing the stored country file, for a file
// the server reports unchanged. A 304 comes only in answer to a request
// that sent a stored copy's validators, so the row already exists.
func (s *Store) touch(ctx context.Context, meta Meta) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE dxcc_cty SET etag = ?, last_modified = ?, synced_at = ? WHERE id = 1",
		meta.ETag, meta.LastModified, feed.FormatTime(meta.SyncedAt))
	if err != nil {
		return fmt.Errorf("dxcc: touch sync time: %w", err)
	}
	return nil
}
