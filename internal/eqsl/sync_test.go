package eqsl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// listServer is a fake eQSL AG member list. It serves body with etag and
// lastModified, answers 304 to a matching If-None-Match, and records the
// conditional headers of the last request.
type listServer struct {
	mu           sync.Mutex
	status       int // when non-zero, every request gets this status
	body         string
	etag         string
	lastModified string
	ifNoneMatch  string
	ifModSince   string
	requests     int
}

func (f *listServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	f.ifNoneMatch = r.Header.Get("If-None-Match")
	f.ifModSince = r.Header.Get("If-Modified-Since")
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if f.etag != "" {
		w.Header().Set("ETag", f.etag)
	}
	if f.lastModified != "" {
		w.Header().Set("Last-Modified", f.lastModified)
	}
	if f.etag != "" && f.ifNoneMatch == f.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = fmt.Fprint(w, f.body)
}

// set changes the server's behavior under its lock.
func (f *listServer) set(fn func(f *listServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// snapshot returns the recorded request state under the lock.
func (f *listServer) snapshot() (requests int, ifNoneMatch, ifModSince string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.ifNoneMatch, f.ifModSince
}

// newSyncer returns a Syncer against a fake list server whose clock reads
// *now.
func newSyncer(t *testing.T, f *listServer, now *time.Time) *Syncer {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &Syncer{
		Store:  newTestStore(t),
		Client: srv.Client(),
		URL:    srv.URL,
		Now:    func() time.Time { return *now },
	}
}

const testList = "List of AG members as of 31-Oct-2026 08:19 UTC\r\nW1AW\r\nk1abc\r\n2E0CRV-2\r\n"

func TestSyncStoresMembersAndMeta(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &listServer{body: testList, etag: `"v1"`, lastModified: "Sun, 08 Nov 2026 09:00:00 GMT"}
	s := newSyncer(t, f, &now)

	res, err := s.Sync(ctx)

	require.NoError(t, err)
	assert.Equal(t, Result{Members: 2, Skipped: 1}, res)
	_, inm, ims := f.snapshot()
	assert.Empty(t, inm, "first sync is unconditional")
	assert.Empty(t, ims)
	ok, err := s.Store.IsAG(ctx, "K1ABC")
	require.NoError(t, err)
	assert.True(t, ok)
	meta, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Meta{ETag: `"v1"`, LastModified: "Sun, 08 Nov 2026 09:00:00 GMT", SyncedAt: now}, meta)
}

func TestSyncNotModified(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &listServer{body: testList, etag: `"v1"`, lastModified: "Sun, 08 Nov 2026 09:00:00 GMT"}
	s := newSyncer(t, f, &now)
	_, err := s.Sync(ctx)
	require.NoError(t, err)

	now = now.Add(8 * 24 * time.Hour)
	res, err := s.Sync(ctx)

	require.NoError(t, err)
	assert.Equal(t, Result{NotModified: true}, res)
	n, inm, ims := f.snapshot()
	assert.Equal(t, 2, n)
	assert.Equal(t, `"v1"`, inm)
	assert.Equal(t, "Sun, 08 Nov 2026 09:00:00 GMT", ims)
	ok, err := s.Store.IsAG(ctx, "W1AW")
	require.NoError(t, err)
	assert.True(t, ok, "304 keeps the data")
	meta, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Meta{ETag: `"v1"`, LastModified: "Sun, 08 Nov 2026 09:00:00 GMT", SyncedAt: now}, meta)
}

func TestSyncNotModifiedWithoutStoredList(t *testing.T) {
	now := utc(2026, 11, 8, 10, 0, 0)
	s := newSyncer(t, &listServer{status: http.StatusNotModified}, &now)

	_, err := s.Sync(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "eqsl: sync")
	assert.Contains(t, err.Error(), "not modified")
	_, ok, err := s.Store.LastSync(t.Context())
	require.NoError(t, err)
	assert.False(t, ok, "nothing recorded")
}

func TestSyncFailuresKeepOldData(t *testing.T) {
	tests := []struct {
		name    string
		change  func(f *listServer)
		wantErr string
	}{
		{"server error", func(f *listServer) { f.status = http.StatusInternalServerError }, "500"},
		{"empty body", func(f *listServer) { f.etag, f.body = `"v2"`, "" }, "no valid"},
		{"header only", func(f *listServer) {
			f.etag, f.body = `"v2"`, "List of AG members as of 01-Nov-2026 08:19 UTC\r\n"
		}, "no valid"},
		{"html error page", func(f *listServer) {
			f.etag, f.body = `"v2"`, "<html>\n<body>Service unavailable</body>\n</html>\n"
		}, "no valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			now := utc(2026, 11, 8, 10, 0, 0)
			f := &listServer{body: testList, etag: `"v1"`}
			s := newSyncer(t, f, &now)
			_, err := s.Sync(ctx)
			require.NoError(t, err)
			f.set(tt.change)

			now = now.Add(time.Hour)
			_, err = s.Sync(ctx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "eqsl: sync")
			assert.Contains(t, err.Error(), tt.wantErr)
			ok, err := s.Store.IsAG(ctx, "W1AW")
			require.NoError(t, err)
			assert.True(t, ok, "old data kept")
			meta, _, err := s.Store.LastSync(ctx)
			require.NoError(t, err)
			assert.Equal(t, Meta{ETag: `"v1"`, SyncedAt: utc(2026, 11, 8, 10, 0, 0)}, meta, "meta unchanged")
		})
	}
}

func TestSyncRejectsOversizedList(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &listServer{body: testList, etag: `"v1"`}
	s := newSyncer(t, f, &now)
	_, err := s.Sync(ctx)
	require.NoError(t, err)

	// A list cut off at the limit would still parse as valid lines, so
	// reaching the limit must fail rather than store a truncated list.
	f.set(func(f *listServer) { f.etag, f.body = `"v2"`, testList+"N0NEW\r\n" })
	s.MaxBytes = int64(len(testList))
	_, err = s.Sync(ctx)

	require.ErrorIs(t, err, ErrListTooLarge)
	ok, err := s.Store.IsAG(ctx, "N0NEW")
	require.NoError(t, err)
	assert.False(t, ok, "truncated list not stored")

	s.MaxBytes = int64(len(f.body))
	_, err = s.Sync(ctx)
	require.NoError(t, err, "a list exactly at the limit is accepted")
}

func TestSyncCancelledContext(t *testing.T) {
	now := utc(2026, 11, 8, 10, 0, 0)
	s := newSyncer(t, &listServer{body: testList}, &now)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := s.Sync(ctx)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSyncDefaults(t *testing.T) {
	s := &Syncer{}
	assert.Equal(t, http.DefaultClient, s.client())
	assert.Equal(t, DefaultURL, s.url())
	assert.Equal(t, int64(DefaultMaxBytes), s.maxBytes())
	assert.WithinDuration(t, time.Now(), s.now(), time.Minute)
}

func TestSyncIfStale(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &listServer{body: testList, etag: `"v1"`}
	s := newSyncer(t, f, &now)

	res, synced, err := s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.True(t, synced, "no meta means stale")
	assert.Equal(t, 2, res.Members)

	now = now.Add(MaxAge)
	res, synced, err = s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.False(t, synced, "exactly maxAge old is still fresh")
	assert.Equal(t, Result{}, res)
	n, _, _ := f.snapshot()
	assert.Equal(t, 1, n, "fresh data makes no request")

	now = now.Add(time.Second)
	res, synced, err = s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.True(t, synced, "older than maxAge is stale")
	assert.True(t, res.NotModified)
	n, _, _ = f.snapshot()
	assert.Equal(t, 2, n)
}

func TestSyncIfStaleReportsError(t *testing.T) {
	now := utc(2026, 11, 8, 10, 0, 0)
	s := newSyncer(t, &listServer{status: http.StatusBadGateway}, &now)

	_, synced, err := s.SyncIfStale(t.Context(), MaxAge)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "eqsl: sync")
	assert.True(t, synced, "a sync was attempted")
}

func TestIsAGDuringSync(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	var list strings.Builder
	list.WriteString("List of AG members as of 31-Oct-2026 08:19 UTC\r\nW1AW\r\n")
	for i := range 5000 {
		fmt.Fprintf(&list, "K%dXX\r\n", i)
	}
	f := &listServer{body: list.String()}
	s := newSyncer(t, f, &now)
	_, err := s.Sync(ctx)
	require.NoError(t, err)

	var wg sync.WaitGroup
	var lookups, misses atomic.Int64
	done := make(chan struct{})
	wg.Go(func() {
		defer close(done)
		for range 3 {
			_, err := s.Sync(ctx)
			assert.NoError(t, err)
		}
	})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				ok, err := s.Store.IsAG(ctx, "w1aw")
				if !assert.NoError(t, err) {
					return
				}
				lookups.Add(1)
				if !ok {
					misses.Add(1)
				}
			}
		})
	}
	wg.Wait()

	assert.Positive(t, lookups.Load())
	assert.Zero(t, misses.Load(), "readers never see the table mid-replace")
}
