package eqsl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"jstlogger/internal/feed"
)

// DefaultURL is eQSL.cc's list of Authenticity Guaranteed members.
const DefaultURL = "https://www.eqsl.cc/qslcard/DownloadedFiles/AGMemberList.txt"

// MaxAge is how old a synced copy of the list may get before it should be
// downloaded again.
const MaxAge = 7 * 24 * time.Hour

// DefaultMaxBytes caps the size of a downloaded list, about ten times the
// list's size in 2026, so a misbehaving server cannot exhaust memory.
const DefaultMaxBytes = 16 << 20

// ErrListTooLarge reports a list longer than the syncer's limit.
var ErrListTooLarge = feed.ErrTooLarge

// Syncer downloads the eQSL AG member list into a Store.
type Syncer struct {
	// Store receives the list. It is required.
	Store *Store
	// Client makes the request; nil means http.DefaultClient.
	Client *http.Client
	// URL is the list's address; empty means DefaultURL.
	URL string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// MaxBytes is the longest list accepted; zero means DefaultMaxBytes.
	MaxBytes int64
}

// Result describes a completed sync.
type Result struct {
	// NotModified is true when the server reported the list unchanged and
	// the stored members were kept.
	NotModified bool
	// Members is the number of AG members stored.
	Members int
	// Skipped is the number of malformed lines ignored.
	Skipped int
}

// maxBytes returns the longest list to accept.
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

// url returns the list address to use.
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

// Sync downloads the list, sending the stored ETag and Last-Modified so an
// unchanged list costs no download, and replaces the stored members with
// it. A failed download, or a response with no valid callsigns (such as an
// HTML error page), leaves the stored members and sync metadata as they
// were.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	res, err := s.sync(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("eqsl: sync: %w", err)
	}
	return res, nil
}

// sync does the work of Sync, returning unprefixed errors.
func (s *Syncer) sync(ctx context.Context) (Result, error) {
	// LastSync returns the zero Meta when there is no stored list, which
	// makes the request unconditional.
	prev, _, err := s.Store.LastSync(ctx)
	if err != nil {
		return Result{}, err
	}
	// Parse the whole body before writing, so the database write lock is
	// held only for the insert, not for the download.
	var calls []string
	var skipped int
	resp, err := feed.Fetch(ctx, s.client(), s.url(), prev, s.maxBytes(), func(r io.Reader) error {
		var err error
		calls, skipped, err = Parse(r)
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
	if len(calls) == 0 {
		return Result{}, fmt.Errorf("list has no valid callsigns (%d skipped)", skipped)
	}
	meta := Meta{ETag: resp.ETag, LastModified: resp.LastModified, SyncedAt: s.now().UTC()}
	if err := s.Store.replace(ctx, calls, meta); err != nil {
		return Result{}, err
	}
	return Result{Members: len(calls), Skipped: skipped}, nil
}

// SyncIfStale syncs when the list has never been synced or was last synced
// more than maxAge ago. synced reports whether a sync was attempted; when
// it is false, res is zero and err is nil unless reading the sync time
// failed.
func (s *Syncer) SyncIfStale(ctx context.Context, maxAge time.Duration) (res Result, synced bool, err error) {
	meta, _, err := s.Store.LastSync(ctx)
	if err != nil {
		return Result{}, false, fmt.Errorf("eqsl: sync: %w", err)
	}
	if !meta.Stale(s.now(), maxAge) {
		return Result{}, false, nil
	}
	res, err = s.Sync(ctx)
	return res, true, err
}
