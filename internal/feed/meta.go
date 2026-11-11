package feed

import (
	"fmt"
	"time"
)

// Meta describes the last successful download of a feed.
type Meta struct {
	// ETag is the feed's ETag response header, sent back as If-None-Match.
	ETag string
	// LastModified is the feed's Last-Modified response header, sent back
	// as If-Modified-Since.
	LastModified string
	// SyncedAt is when the feed was last downloaded or confirmed unchanged.
	// It is zero when the feed has never been downloaded.
	SyncedAt time.Time
}

// Stale reports whether a feed described by m should be downloaded again
// at now: it has never been downloaded, or was last synced more than maxAge
// before now.
func (m Meta) Stale(now time.Time, maxAge time.Duration) bool {
	return m.SyncedAt.IsZero() || now.Sub(m.SyncedAt) > maxAge
}

// FormatTime encodes t as RFC 3339 UTC text with second precision, so that
// text order equals time order.
func FormatTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}

// ParseTime decodes a time written by FormatTime, returning it in UTC.
func ParseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time: %w", err)
	}
	return t.UTC(), nil
}
