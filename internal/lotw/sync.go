package lotw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultURL is the ARRL's LoTW user activity feed.
const DefaultURL = "https://lotw.arrl.org/lotw-user-activity.csv"

// MaxAge is how old a synced copy of the feed may get before it should be
// downloaded again.
const MaxAge = 7 * 24 * time.Hour

// DefaultMaxBytes caps the size of a downloaded feed, about ten times the
// feed's size in 2026, so a misbehaving server cannot exhaust memory.
const DefaultMaxBytes = 64 << 20

// ErrFeedTooLarge reports a feed longer than the syncer's limit.
var ErrFeedTooLarge = errors.New("lotw: feed exceeds the size limit")

// Syncer downloads the LoTW activity feed into a Store.
type Syncer struct {
	// Store receives the feed. It is required.
	Store *Store
	// Client makes the request; nil means http.DefaultClient.
	Client *http.Client
	// URL is the feed's address; empty means DefaultURL.
	URL string
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// MaxBytes is the longest feed accepted; zero means DefaultMaxBytes.
	MaxBytes int64
}

// maxBytes returns the longest feed to accept.
func (s *Syncer) maxBytes() int64 {
	if s.MaxBytes == 0 {
		return DefaultMaxBytes
	}
	return s.MaxBytes
}

// cappedReader reads up to n bytes from r and fails with ErrFeedTooLarge if
// r holds more, rather than ending early, so a feed cut off at the limit is
// never mistaken for a whole one.
type cappedReader struct {
	r io.Reader
	n int64
}

// Read reads from the underlying reader within the limit.
func (c *cappedReader) Read(p []byte) (int, error) {
	if c.n <= 0 {
		// At the limit: the feed is whole only if nothing follows.
		var one [1]byte
		if n, err := c.r.Read(one[:]); n > 0 {
			return 0, ErrFeedTooLarge
		} else if err != nil {
			return 0, err
		}
		return 0, nil
	}
	p = p[:min(int64(len(p)), c.n)]
	n, err := c.r.Read(p)
	c.n -= int64(n)
	return n, err
}

// Result describes a completed sync.
type Result struct {
	// NotModified is true when the server reported the feed unchanged and
	// the stored users were kept.
	NotModified bool
	// Users is the number of users stored.
	Users int
	// Skipped is the number of malformed feed rows ignored.
	Skipped int
}

// client returns the HTTP client to use.
func (s *Syncer) client() *http.Client {
	if s.Client == nil {
		return http.DefaultClient
	}
	return s.Client
}

// url returns the feed address to use.
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

// Sync downloads the feed, sending the stored ETag and Last-Modified so an
// unchanged feed costs no download, and replaces the stored users with it.
// A failed download, or a response with no valid rows (such as an HTML
// error page), leaves the stored users and sync metadata as they were.
func (s *Syncer) Sync(ctx context.Context) (Result, error) {
	res, err := s.sync(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("lotw: sync: %w", err)
	}
	return res, nil
}

func (s *Syncer) sync(ctx context.Context) (res Result, err error) {
	meta, synced, err := s.Store.LastSync(ctx)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	if synced {
		if meta.ETag != "" {
			req.Header.Set("If-None-Match", meta.ETag)
		}
		if meta.LastModified != "" {
			req.Header.Set("If-Modified-Since", meta.LastModified)
		}
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("download: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("close body: %w", cerr))
		}
	}()

	switch resp.StatusCode {
	case http.StatusNotModified:
		if !synced {
			return Result{}, errors.New("server reported not modified, but there is no stored feed")
		}
		meta.SyncedAt = s.now().UTC()
		if err := s.Store.touch(ctx, meta); err != nil {
			return Result{}, err
		}
		return Result{NotModified: true}, nil
	case http.StatusOK:
		// Parse the whole body before writing, so the database write lock
		// is held only for the insert, not for the download.
		users, skipped, err := Parse(&cappedReader{r: resp.Body, n: s.maxBytes()})
		if err != nil {
			return Result{}, err
		}
		if len(users) == 0 {
			return Result{}, fmt.Errorf("feed has no valid rows (%d skipped)", skipped)
		}
		meta := Meta{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			SyncedAt:     s.now().UTC(),
		}
		if err := s.Store.replace(ctx, users, meta); err != nil {
			return Result{}, err
		}
		return Result{Users: len(users), Skipped: skipped}, nil
	default:
		return Result{}, fmt.Errorf("unexpected status %s", resp.Status)
	}
}

// SyncIfStale syncs when the feed has never been synced or was last synced
// more than maxAge ago. synced reports whether a sync was attempted; when
// it is false, res is zero and err is nil unless reading the sync time
// failed.
func (s *Syncer) SyncIfStale(ctx context.Context, maxAge time.Duration) (res Result, synced bool, err error) {
	meta, ok, err := s.Store.LastSync(ctx)
	if err != nil {
		return Result{}, false, fmt.Errorf("lotw: sync: %w", err)
	}
	if ok && s.now().Sub(meta.SyncedAt) <= maxAge {
		return Result{}, false, nil
	}
	res, err = s.Sync(ctx)
	return res, true, err
}
