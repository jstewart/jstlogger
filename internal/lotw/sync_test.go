package lotw

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

// feedServer is a fake LoTW activity feed. It serves body with etag and
// lastModified, answers 304 to a matching If-None-Match, and records the
// conditional headers of the last request.
type feedServer struct {
	mu           sync.Mutex
	status       int // when non-zero, every request gets this status
	body         string
	etag         string
	lastModified string
	ifNoneMatch  string
	ifModSince   string
	requests     int
}

func (f *feedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	_, _ = fmt.Fprint(w, f.body)
}

// set changes the server's behavior under its lock.
func (f *feedServer) set(fn func(f *feedServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// snapshot returns the recorded request state under the lock.
func (f *feedServer) snapshot() (requests int, ifNoneMatch, ifModSince string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.ifNoneMatch, f.ifModSince
}

// newSyncer returns a Syncer against a fake feed server whose clock reads
// *now.
func newSyncer(t *testing.T, f *feedServer, now *time.Time) *Syncer {
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

const testFeed = "W1AW,2026-11-01,12:00:00\nK1ABC,2026-10-01,00:00:00\nbad row\n"

func TestSyncStoresUsersAndMeta(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &feedServer{body: testFeed, etag: `"v1"`, lastModified: "Sun, 08 Nov 2026 09:00:00 GMT"}
	s := newSyncer(t, f, &now)

	res, err := s.Sync(ctx)

	require.NoError(t, err)
	assert.Equal(t, Result{Users: 2, Skipped: 1}, res)
	_, inm, ims := f.snapshot()
	assert.Empty(t, inm, "first sync is unconditional")
	assert.Empty(t, ims)
	got, ok, err := s.Store.LastUpload(ctx, "W1AW")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, utc(2026, 11, 1, 12, 0, 0).Equal(got))
	meta, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Meta{ETag: `"v1"`, LastModified: "Sun, 08 Nov 2026 09:00:00 GMT", SyncedAt: now}, meta)
}

func TestSyncNotModified(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &feedServer{body: testFeed, etag: `"v1"`, lastModified: "Sun, 08 Nov 2026 09:00:00 GMT"}
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
	_, ok, err := s.Store.LastUpload(ctx, "K1ABC")
	require.NoError(t, err)
	assert.True(t, ok, "304 keeps the data")
	meta, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Meta{ETag: `"v1"`, LastModified: "Sun, 08 Nov 2026 09:00:00 GMT", SyncedAt: now}, meta)
}

func TestSyncFailuresKeepOldData(t *testing.T) {
	tests := []struct {
		name    string
		change  func(f *feedServer)
		wantErr string
	}{
		{"server error", func(f *feedServer) { f.status = http.StatusInternalServerError }, "500"},
		{"empty body", func(f *feedServer) { f.etag, f.body = `"v2"`, "" }, "no valid"},
		{"html error page", func(f *feedServer) {
			f.etag, f.body = `"v2"`, "<html>\n<body>Service unavailable</body>\n</html>\n"
		}, "no valid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			now := utc(2026, 11, 8, 10, 0, 0)
			f := &feedServer{body: testFeed, etag: `"v1"`}
			s := newSyncer(t, f, &now)
			_, err := s.Sync(ctx)
			require.NoError(t, err)
			f.set(tt.change)

			now = now.Add(time.Hour)
			_, err = s.Sync(ctx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "lotw: sync")
			assert.Contains(t, err.Error(), tt.wantErr)
			_, ok, err := s.Store.LastUpload(ctx, "W1AW")
			require.NoError(t, err)
			assert.True(t, ok, "old data kept")
			meta, _, err := s.Store.LastSync(ctx)
			require.NoError(t, err)
			assert.Equal(t, Meta{ETag: `"v1"`, SyncedAt: utc(2026, 11, 8, 10, 0, 0)}, meta, "meta unchanged")
		})
	}
}

func TestSyncCancelledContext(t *testing.T) {
	now := utc(2026, 11, 8, 10, 0, 0)
	s := newSyncer(t, &feedServer{body: testFeed}, &now)
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
	assert.WithinDuration(t, time.Now(), s.now(), time.Minute)
}

func TestSyncIfStale(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	f := &feedServer{body: testFeed, etag: `"v1"`}
	s := newSyncer(t, f, &now)

	res, synced, err := s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.True(t, synced, "no meta means stale")
	assert.Equal(t, 2, res.Users)

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
	s := newSyncer(t, &feedServer{status: http.StatusBadGateway}, &now)

	_, synced, err := s.SyncIfStale(t.Context(), MaxAge)

	require.Error(t, err)
	assert.True(t, synced, "a sync was attempted")
}

func TestLastUploadDuringSync(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 8, 10, 0, 0)
	var feed strings.Builder
	feed.WriteString("W1AW,2026-11-01,12:00:00\n")
	for i := range 5000 {
		fmt.Fprintf(&feed, "K%dXX,2026-10-01,00:00:00\n", i)
	}
	f := &feedServer{body: feed.String()}
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
				_, ok, err := s.Store.LastUpload(ctx, "w1aw")
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
