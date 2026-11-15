package dxcc

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"jstlogger/internal/feed"
)

// DefaultURL is AD1C's Big CTY release, a zip archive holding cty.csv.
const DefaultURL = "https://www.country-files.com/bigcty/download/bigcty.zip"

// MaxAge is how old a synced copy of the country file may get before it
// should be downloaded again. AD1C releases one every few weeks, and an
// unchanged file costs only a 304.
const MaxAge = 7 * 24 * time.Hour

// DefaultMaxBytes caps the size of a downloaded archive, and of the cty.csv
// inside it, at about twenty times their sizes in 2026, so a misbehaving
// server or a zip bomb cannot exhaust memory.
const DefaultMaxBytes = 8 << 20

// ErrTooLarge reports an archive, or a cty.csv inside one, longer than the
// syncer's limit.
var ErrTooLarge = feed.ErrTooLarge

// ctyName is the country file's name inside the Big CTY archive.
const ctyName = "cty.csv"

// Syncer downloads AD1C's Big CTY country file into a Store.
type Syncer struct {
	// Store receives the country file. It is required.
	Store *Store
	// Client makes the request; nil means http.DefaultClient.
	Client *http.Client
	// URL is the archive's address; empty means DefaultURL.
	URL string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// MaxBytes is the longest archive, and the longest cty.csv inside it,
	// accepted; zero means DefaultMaxBytes.
	MaxBytes int64
}

// Result describes a completed sync.
type Result struct {
	// NotModified is true when the server reported the country file
	// unchanged and the stored copy was kept.
	NotModified bool
	// Prefixes and Calls are the numbers of prefixes and exception calls
	// in the stored file.
	Prefixes, Calls int
}

// maxBytes returns the longest archive to accept.
func (s *Syncer) maxBytes() int64 {
	if s.MaxBytes == 0 {
		return DefaultMaxBytes
	}
	return s.MaxBytes
}

// client returns the HTTP client to use.
func (s *Syncer) client() *http.Client {
	if s.Client == nil {
		return http.DefaultClient
	}
	return s.Client
}

// url returns the archive address to use.
func (s *Syncer) url() string {
	if s.URL == "" {
		return DefaultURL
	}
	return s.URL
}

// now returns the current time.
func (s *Syncer) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// Sync downloads the Big CTY archive, sending the stored ETag and
// Last-Modified so an unchanged file costs no download, and stores the
// cty.csv inside it. A failed download, or an archive whose cty.csv is
// missing or does not parse, leaves the stored file and sync metadata as
// they were.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	res, err := s.sync(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("dxcc: sync: %w", err)
	}
	return res, nil
}

// sync does the work of Sync, returning unprefixed errors.
func (s *Syncer) sync(ctx context.Context) (Result, error) {
	// LastSync returns the zero Meta when there is no stored file, which
	// makes the request unconditional.
	prev, _, err := s.Store.LastSync(ctx)
	if err != nil {
		return Result{}, err
	}
	var cty []byte
	var table *Table
	resp, err := feed.Fetch(ctx, s.client(), s.url(), prev, s.maxBytes(), func(r io.Reader) error {
		var err error
		if cty, err = extractCTY(r, s.maxBytes()); err != nil {
			return err
		}
		table, err = Parse(bytes.NewReader(cty))
		return err
	})
	if err != nil {
		return Result{}, err
	}
	if resp.NotModified {
		prev.SyncedAt = s.now().UTC()
		if err := s.Store.touch(ctx, prev); err != nil {
			return Result{}, err
		}
		return Result{NotModified: true}, nil
	}
	meta := Meta{ETag: resp.ETag, LastModified: resp.LastModified, SyncedAt: s.now().UTC()}
	if err := s.Store.replace(ctx, cty, meta); err != nil {
		return Result{}, err
	}
	return Result{Prefixes: len(table.prefixes), Calls: len(table.exact)}, nil
}

// SyncIfStale syncs when the country file has never been synced or was
// last synced more than maxAge ago. synced reports whether a sync was
// attempted; when it is false, res is zero and err is nil unless reading
// the sync time failed.
func (s *Syncer) SyncIfStale(ctx context.Context, maxAge time.Duration) (res Result, synced bool, err error) {
	meta, _, err := s.Store.LastSync(ctx)
	if err != nil {
		return Result{}, false, fmt.Errorf("dxcc: sync: %w", err)
	}
	if !meta.Stale(s.now(), maxAge) {
		return Result{}, false, nil
	}
	res, err = s.Sync(ctx)
	return res, true, err
}

// extractCTY reads a zip archive from r and returns the cty.csv inside it,
// failing with ErrTooLarge if that is longer than maxBytes.
func extractCTY(r io.Reader, maxBytes int64) (_ []byte, err error) {
	// zip needs random access, so hold the whole archive; r is already
	// capped at maxBytes.
	archive, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	f, err := zr.Open(ctyName)
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("read archive: close %s: %w", ctyName, cerr))
		}
	}()
	// Read one byte past the limit to tell a file at the limit from a
	// longer one, without trusting the size the archive declares.
	cty, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read archive: %s: %w", ctyName, err)
	}
	if int64(len(cty)) > maxBytes {
		return nil, fmt.Errorf("read archive: %s: %w", ctyName, ErrTooLarge)
	}
	return cty, nil
}
