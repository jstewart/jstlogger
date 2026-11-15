package dxcc

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archiveServer is a fake Big CTY download. It serves body with etag and
// lastModified, answers 304 to a matching If-None-Match, and records the
// conditional headers of the last request.
type archiveServer struct {
	mu           sync.Mutex
	status       int // when non-zero, every request gets this status
	body         []byte
	etag         string
	lastModified string
	ifNoneMatch  string
	ifModSince   string
	requests     int
}

func (f *archiveServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	_, _ = w.Write(f.body)
}

// set changes the server's behavior under its lock.
func (f *archiveServer) set(fn func(f *archiveServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// snapshot returns the recorded request state under the lock.
func (f *archiveServer) snapshot() (requests int, ifNoneMatch, ifModSince string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, f.ifNoneMatch, f.ifModSince
}

// newSyncer returns a Syncer against a fake archive server whose clock
// reads *now.
func newSyncer(t *testing.T, f *archiveServer, now *time.Time) *Syncer {
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

// testArchive returns a Big CTY-like archive holding the fixture cty.csv.
func testArchive(t *testing.T) []byte {
	t.Helper()
	return zipOf(t, map[string][]byte{"cty.csv": fixtureCSV(t), "README.TXT": []byte("Big CTY\n")})
}

// lookup resolves call in the stored table.
func lookup(t *testing.T, s *Store, call string) (Entity, bool) {
	t.Helper()
	table, err := s.Table(t.Context())
	require.NoError(t, err)
	return table.Lookup(call)
}

func TestSyncStoresFileAndMeta(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 15, 10, 0, 0)
	f := &archiveServer{body: testArchive(t), etag: `"v1"`, lastModified: "Sat, 14 Nov 2026 14:38:28 GMT"}
	s := newSyncer(t, f, &now)

	res, err := s.Sync(ctx)

	require.NoError(t, err)
	assert.Equal(t, Result{Prefixes: 26, Calls: 6}, res)
	_, inm, ims := f.snapshot()
	assert.Empty(t, inm, "first sync is unconditional")
	assert.Empty(t, ims)
	got, ok := lookup(t, s.Store, "KH6ABC")
	require.True(t, ok)
	assert.Equal(t, hawaii, got)
	meta, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Meta{ETag: `"v1"`, LastModified: "Sat, 14 Nov 2026 14:38:28 GMT", SyncedAt: now}, meta)
}

func TestSyncNotModified(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 15, 10, 0, 0)
	f := &archiveServer{body: testArchive(t), etag: `"v1"`, lastModified: "Sat, 14 Nov 2026 14:38:28 GMT"}
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
	assert.Equal(t, "Sat, 14 Nov 2026 14:38:28 GMT", ims)
	_, ok := lookup(t, s.Store, "KH6ABC")
	assert.True(t, ok, "304 keeps the file")
	meta, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, Meta{ETag: `"v1"`, LastModified: "Sat, 14 Nov 2026 14:38:28 GMT", SyncedAt: now}, meta)
}

func TestSyncFailuresKeepOldFile(t *testing.T) {
	tests := []struct {
		name    string
		change  func(t *testing.T, f *archiveServer)
		wantErr string
	}{
		{"server error", func(_ *testing.T, f *archiveServer) { f.status = http.StatusInternalServerError }, "500"},
		{"html error page", func(_ *testing.T, f *archiveServer) {
			f.etag, f.body = `"v2"`, []byte("<html>\n<body>Service unavailable</body>\n</html>\n")
		}, "read archive"},
		{"no cty.csv in the archive", func(t *testing.T, f *archiveServer) {
			f.etag, f.body = `"v2"`, zipOf(t, map[string][]byte{"cty.dat": []byte("K: 5: 8:")})
		}, "cty.csv"},
		{"cty.csv does not parse", func(t *testing.T, f *archiveServer) {
			f.etag, f.body = `"v2"`, zipOf(t, map[string][]byte{"cty.csv": []byte("not,a,country,file\n")})
		}, "want 10 fields"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			now := utc(2026, 11, 15, 10, 0, 0)
			f := &archiveServer{body: testArchive(t), etag: `"v1"`}
			s := newSyncer(t, f, &now)
			_, err := s.Sync(ctx)
			require.NoError(t, err)
			f.set(func(f *archiveServer) { tt.change(t, f) })

			now = now.Add(time.Hour)
			_, err = s.Sync(ctx)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "dxcc: sync")
			assert.Contains(t, err.Error(), tt.wantErr)
			_, ok := lookup(t, s.Store, "KH6ABC")
			assert.True(t, ok, "old file kept")
			meta, _, err := s.Store.LastSync(ctx)
			require.NoError(t, err)
			assert.Equal(t, Meta{ETag: `"v1"`, SyncedAt: utc(2026, 11, 15, 10, 0, 0)}, meta, "meta unchanged")
		})
	}
}

func TestSyncRejectsOversizedArchive(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 15, 10, 0, 0)
	archive := testArchive(t)
	s := newSyncer(t, &archiveServer{body: archive}, &now)

	s.MaxBytes = int64(len(archive)) - 1
	_, err := s.Sync(ctx)

	require.ErrorIs(t, err, ErrTooLarge)
	_, ok, err := s.Store.LastSync(ctx)
	require.NoError(t, err)
	assert.False(t, ok, "nothing stored")
}

func TestSyncRejectsOversizedCTY(t *testing.T) {
	ctx := t.Context()
	now := utc(2026, 11, 15, 10, 0, 0)
	// A long cty.csv compresses to a small archive, as a zip bomb would.
	cty := append(fixtureCSV(t), bytes.Repeat([]byte("\n"), 64<<10)...)
	archive := zipOf(t, map[string][]byte{"cty.csv": cty})
	require.Less(t, len(archive), 8<<10)
	s := newSyncer(t, &archiveServer{body: archive}, &now)

	s.MaxBytes = int64(len(cty)) - 1
	_, err := s.Sync(ctx)
	require.ErrorIs(t, err, ErrTooLarge)

	s.MaxBytes = int64(len(cty))
	_, err = s.Sync(ctx)
	require.NoError(t, err, "a cty.csv exactly at the limit is accepted")
}

func TestSyncCancelledContext(t *testing.T) {
	now := utc(2026, 11, 15, 10, 0, 0)
	s := newSyncer(t, &archiveServer{body: testArchive(t)}, &now)
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
	now := utc(2026, 11, 15, 10, 0, 0)
	f := &archiveServer{body: testArchive(t), etag: `"v1"`}
	s := newSyncer(t, f, &now)

	res, synced, err := s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.True(t, synced, "no meta means stale")
	assert.Equal(t, 6, res.Calls)

	now = now.Add(MaxAge)
	res, synced, err = s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.False(t, synced, "exactly maxAge old is still fresh")
	assert.Equal(t, Result{}, res)
	n, _, _ := f.snapshot()
	assert.Equal(t, 1, n, "a fresh file makes no request")

	now = now.Add(time.Second)
	res, synced, err = s.SyncIfStale(ctx, MaxAge)
	require.NoError(t, err)
	assert.True(t, synced, "older than maxAge is stale")
	assert.True(t, res.NotModified)
	n, _, _ = f.snapshot()
	assert.Equal(t, 2, n)
}

func TestSyncIfStaleReportsError(t *testing.T) {
	now := utc(2026, 11, 15, 10, 0, 0)
	s := newSyncer(t, &archiveServer{status: http.StatusBadGateway}, &now)

	_, synced, err := s.SyncIfStale(t.Context(), MaxAge)

	require.Error(t, err)
	assert.True(t, synced, "a sync was attempted")
}
