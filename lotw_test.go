package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
	"jstlogger/internal/lotw"
	"jstlogger/internal/tui"
)

// testLoTWFeed is a small LoTW activity feed with one malformed row.
const testLoTWFeed = "W1AW,2026-11-01,12:00:00\nK1ABC,2026-10-01,00:00:00\nbad row\n"

// lotwFeed is a fake LoTW activity feed. It serves testLoTWFeed with an
// ETag, answers 304 to a matching If-None-Match, and fails every request
// with status when that is set.
type lotwFeed struct {
	mu       sync.Mutex
	status   int
	requests int
}

func (f *lotwFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	_, _ = fmt.Fprint(w, testLoTWFeed)
}

// requestCount returns the number of requests served.
func (f *lotwFeed) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// serveLoTWFeed starts f on an httptest server and points the LoTW sync at
// it for the duration of the test.
func serveLoTWFeed(t *testing.T, f *lotwFeed) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	orig := lotwFeedURL
	lotwFeedURL = srv.URL
	t.Cleanup(func() { lotwFeedURL = orig })
}

func TestRunSyncLoTW(t *testing.T) {
	f := &lotwFeed{}
	serveLoTWFeed(t, f)
	p := writeConfig(t, validConfig)
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir()}

	r := invoke(t, vars, nil, "-c", p, "--sync-lotw")
	require.Equal(t, 0, r.code, r.stderr)
	assert.False(t, r.tuiCalled)
	assert.Equal(t, "LoTW: 2 users (1 row skipped)\n", r.stdout)

	r = invoke(t, vars, nil, "-c", p, "--sync-lotw")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "LoTW: feed unchanged\n", r.stdout, "a second sync is conditional")
	assert.Equal(t, 2, f.requestCount(), "--sync-lotw syncs even when the feed is fresh")

	// The users are stored in the logbook.
	store, err := logbook.Open(t.Context(), filepath.Join(vars["XDG_DATA_HOME"], "jstlogger", "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	_, ok, err := lotw.NewStore(store.DB()).LastUpload(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRunSyncLoTWError(t *testing.T) {
	serveLoTWFeed(t, &lotwFeed{status: http.StatusInternalServerError})
	p := writeConfig(t, validConfig)

	r := invoke(t, nil, nil, "-c", p, "--sync-lotw")
	assert.Equal(t, 1, r.code)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "syncing LoTW")
	assert.Contains(t, r.stderr, "500")
	assert.False(t, r.tuiCalled)
}

func TestRunHelpDescribesSyncLoTW(t *testing.T) {
	r := invoke(t, nil, nil, "--help")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stderr, "--sync-lotw")
}

func TestLoTWSummary(t *testing.T) {
	tests := []struct {
		res  lotw.Result
		want string
	}{
		{lotw.Result{Users: 237123, Skipped: 2}, "LoTW: 237123 users (2 rows skipped)"},
		{lotw.Result{Users: 1, Skipped: 1}, "LoTW: 1 user (1 row skipped)"},
		{lotw.Result{Users: 10}, "LoTW: 10 users"},
		{lotw.Result{NotModified: true}, "LoTW: feed unchanged"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, lotwSummary(tt.res))
	}
}

// newTestLoTWSyncer returns a syncer on a fresh logbook, fetching from f.
func newTestLoTWSyncer(t *testing.T, f *lotwFeed) *lotw.Syncer {
	t.Helper()
	serveLoTWFeed(t, f)
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	return newLoTWSyncer(lotw.NewStore(store.DB()))
}

// recordSends returns a send func that records the messages sent.
func recordSends() (func(tea.Msg), func() []tea.Msg) {
	var mu sync.Mutex
	var msgs []tea.Msg
	return func(msg tea.Msg) {
			mu.Lock()
			defer mu.Unlock()
			msgs = append(msgs, msg)
		}, func() []tea.Msg {
			mu.Lock()
			defer mu.Unlock()
			return append([]tea.Msg(nil), msgs...)
		}
}

func TestSyncLoTWInBackground(t *testing.T) {
	f := &lotwFeed{}
	s := newTestLoTWSyncer(t, f)
	send, sent := recordSends()

	syncLoTWInBackground(t.Context(), s, slog.New(slog.DiscardHandler), send)
	assert.Equal(t, []tea.Msg{tui.LoTWSynced{}}, sent(), "a stale feed is synced")

	syncLoTWInBackground(t.Context(), s, slog.New(slog.DiscardHandler), send)
	assert.Len(t, sent(), 1, "a fresh feed is not synced again, and nothing is sent")
	assert.Equal(t, 1, f.requestCount())
}

func TestSyncLoTWInBackgroundError(t *testing.T) {
	s := newTestLoTWSyncer(t, &lotwFeed{status: http.StatusServiceUnavailable})
	send, sent := recordSends()

	syncLoTWInBackground(t.Context(), s, slog.New(slog.DiscardHandler), send)
	msgs := sent()
	require.Len(t, msgs, 1)
	synced, ok := msgs[0].(tui.LoTWSynced)
	require.True(t, ok)
	assert.ErrorContains(t, synced.Err, "503")
}

func TestSyncLoTWInBackgroundCanceled(t *testing.T) {
	s := newTestLoTWSyncer(t, &lotwFeed{})
	send, sent := recordSends()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	syncLoTWInBackground(ctx, s, slog.New(slog.DiscardHandler), send)
	assert.Empty(t, sent(), "a sync stopped by exit is not reported")
}

func TestSyncLoTWCanceledErrorIsCanceled(t *testing.T) {
	// The canceled check in syncLoTWInBackground relies on the syncer
	// wrapping the context error.
	s := newTestLoTWSyncer(t, &lotwFeed{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.Sync(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
