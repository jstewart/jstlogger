package main

import (
	"archive/zip"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/dxcc"
	"jstlogger/internal/logbook"
)

// testCTY is a small cty.csv, unlike the embedded snapshot in leaving out
// Japan.
const testCTY = "K,United States,291,NA,5,8,37.60,91.87,5.0,AA K N W =4U1WB(5)[8];\n" +
	"KH6,Hawaii,110,OC,31,61,21.12,157.48,10.0,AH6 KH6;\n"

// dxccArchive is a fake Big CTY download. It serves body, a zip of testCTY
// set by serveDXCCArchive, with an ETag, answers 304 to a matching
// If-None-Match, and fails every request with status when that is set.
type dxccArchive struct {
	mu       sync.Mutex
	status   int
	body     []byte
	requests int
}

func (f *dxccArchive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	w.Header().Set("ETag", `"v1"`)
	if r.Header.Get("If-None-Match") == `"v1"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(f.body)
}

// requestCount returns the number of requests served.
func (f *dxccArchive) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// serveDXCCArchive starts f on an httptest server and points the DXCC sync
// at it for the duration of the test.
func serveDXCCArchive(t *testing.T, f *dxccArchive) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("cty.csv")
	require.NoError(t, err)
	_, err = w.Write([]byte(testCTY))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	f.body = buf.Bytes()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	orig := dxccArchiveURL
	dxccArchiveURL = srv.URL
	t.Cleanup(func() { dxccArchiveURL = orig })
}

func TestRunSyncDXCC(t *testing.T) {
	f := &dxccArchive{}
	serveDXCCArchive(t, f)
	p := writeConfig(t, validConfig)
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir()}

	r := invoke(t, vars, nil, "-c", p, "--sync-dxcc")
	require.Equal(t, 0, r.code, r.stderr)
	assert.False(t, r.tuiCalled)
	assert.Equal(t, "DXCC: 6 prefixes, 1 exception call\n", r.stdout)

	r = invoke(t, vars, nil, "-c", p, "--sync-dxcc")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "DXCC: country file unchanged\n", r.stdout, "a second sync is conditional")
	assert.Equal(t, 2, f.requestCount(), "--sync-dxcc syncs even when the file is fresh")

	// The country file is stored in the logbook.
	store, err := logbook.Open(t.Context(), filepath.Join(vars["XDG_DATA_HOME"], "jstlogger", "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	table, err := dxcc.NewStore(store.DB()).Table(t.Context())
	require.NoError(t, err)
	e, ok := table.Lookup("KH6ABC")
	require.True(t, ok)
	assert.Equal(t, "Hawaii", e.Name)
	_, ok = table.Lookup("JA1ABC")
	assert.False(t, ok, "the downloaded file replaces the snapshot")
}

func TestRunSyncDXCCError(t *testing.T) {
	serveDXCCArchive(t, &dxccArchive{status: http.StatusInternalServerError})
	p := writeConfig(t, validConfig)

	r := invoke(t, nil, nil, "-c", p, "--sync-dxcc")
	assert.Equal(t, 1, r.code)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "syncing DXCC")
	assert.Contains(t, r.stderr, "500")
	assert.False(t, r.tuiCalled)
}

func TestRunHelpDescribesSyncDXCC(t *testing.T) {
	r := invoke(t, nil, nil, "--help")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stderr, "--sync-dxcc")
}

func TestDXCCSummary(t *testing.T) {
	tests := []struct {
		res  dxcc.Result
		want string
	}{
		{dxcc.Result{Prefixes: 6331, Calls: 21430}, "DXCC: 6331 prefixes, 21430 exception calls"},
		{dxcc.Result{Prefixes: 1, Calls: 1}, "DXCC: 1 prefix, 1 exception call"},
		{dxcc.Result{NotModified: true}, "DXCC: country file unchanged"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, dxccSummary(tt.res))
	}
}

// newTestDXCCSyncer returns a syncer on a fresh logbook, fetching from f.
func newTestDXCCSyncer(t *testing.T, f *dxccArchive) *dxcc.Syncer {
	t.Helper()
	serveDXCCArchive(t, f)
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	return newDXCCSyncer(dxcc.NewStore(store.DB()))
}

// logTo returns a logger writing text records to buf.
func logTo(buf *strings.Builder) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestSyncDXCCInBackground(t *testing.T) {
	f := &dxccArchive{}
	s := newTestDXCCSyncer(t, f)
	var log strings.Builder

	syncDXCCInBackground(t.Context(), s, logTo(&log))
	assert.Contains(t, log.String(), `msg="dxcc sync" prefixes=6 calls=1`, "a stale file is synced")

	syncDXCCInBackground(t.Context(), s, logTo(&log))
	assert.Contains(t, log.String(), "dxcc sync not needed")
	assert.Equal(t, 1, f.requestCount(), "a fresh file is not synced again")
}

func TestSyncDXCCInBackgroundError(t *testing.T) {
	s := newTestDXCCSyncer(t, &dxccArchive{status: http.StatusServiceUnavailable})
	var log strings.Builder

	syncDXCCInBackground(t.Context(), s, logTo(&log))
	assert.Contains(t, log.String(), "level=ERROR")
	assert.Contains(t, log.String(), "503")
}

func TestSyncDXCCInBackgroundCanceled(t *testing.T) {
	s := newTestDXCCSyncer(t, &dxccArchive{})
	var log strings.Builder
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	syncDXCCInBackground(ctx, s, logTo(&log))
	assert.Contains(t, log.String(), "dxcc sync canceled")
	assert.NotContains(t, log.String(), "level=ERROR", "a sync stopped by exit is not an error")
}
