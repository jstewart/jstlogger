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

	"jstlogger/internal/eqsl"
	"jstlogger/internal/logbook"
	"jstlogger/internal/tui"
)

// testEQSLList is a small eQSL AG member list with one malformed line.
const testEQSLList = "List of AG members as of 2026-11-01\nW1AW\nK1ABC\nnot a call!\n"

// eqslList is a fake eQSL AG member list. It serves testEQSLList with an
// ETag, answers 304 to a matching If-None-Match, and fails every request
// with status when that is set.
type eqslList struct {
	mu       sync.Mutex
	status   int
	requests int
}

func (f *eqslList) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	_, _ = fmt.Fprint(w, testEQSLList)
}

// requestCount returns the number of requests served.
func (f *eqslList) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

// serveEQSLList starts f on an httptest server and points the eQSL sync at
// it for the duration of the test.
func serveEQSLList(t *testing.T, f *eqslList) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	orig := eqslListURL
	eqslListURL = srv.URL
	t.Cleanup(func() { eqslListURL = orig })
}

func TestRunSyncEQSL(t *testing.T) {
	f := &eqslList{}
	serveEQSLList(t, f)
	p := writeConfig(t, validConfig)
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir()}

	r := invoke(t, vars, nil, "-c", p, "--sync-eqsl")
	require.Equal(t, 0, r.code, r.stderr)
	assert.False(t, r.tuiCalled)
	assert.Equal(t, "eQSL: 2 AG members (1 row skipped)\n", r.stdout)

	r = invoke(t, vars, nil, "-c", p, "--sync-eqsl")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "eQSL: AG list unchanged\n", r.stdout, "a second sync is conditional")
	assert.Equal(t, 2, f.requestCount(), "--sync-eqsl syncs even when the list is fresh")

	// The members are stored in the logbook.
	store, err := logbook.Open(t.Context(), filepath.Join(vars["XDG_DATA_HOME"], "jstlogger", "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	ag, err := eqsl.NewStore(store.DB()).IsAG(t.Context(), "W1AW")
	require.NoError(t, err)
	assert.True(t, ag)
}

func TestRunSyncEQSLError(t *testing.T) {
	serveEQSLList(t, &eqslList{status: http.StatusInternalServerError})
	p := writeConfig(t, validConfig)

	r := invoke(t, nil, nil, "-c", p, "--sync-eqsl")
	assert.Equal(t, 1, r.code)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "syncing eQSL")
	assert.Contains(t, r.stderr, "500")
	assert.False(t, r.tuiCalled)
}

func TestRunHelpDescribesSyncEQSL(t *testing.T) {
	r := invoke(t, nil, nil, "--help")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stderr, "--sync-eqsl")
}

func TestEQSLSummary(t *testing.T) {
	tests := []struct {
		res  eqsl.Result
		want string
	}{
		{eqsl.Result{Members: 199457, Skipped: 850}, "eQSL: 199457 AG members (850 rows skipped)"},
		{eqsl.Result{Members: 1, Skipped: 1}, "eQSL: 1 AG member (1 row skipped)"},
		{eqsl.Result{Members: 10}, "eQSL: 10 AG members"},
		{eqsl.Result{NotModified: true}, "eQSL: AG list unchanged"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, eqslSummary(tt.res))
	}
}

// newTestEQSLSyncer returns a syncer on a fresh logbook, fetching from f.
func newTestEQSLSyncer(t *testing.T, f *eqslList) *eqsl.Syncer {
	t.Helper()
	serveEQSLList(t, f)
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	return newEQSLSyncer(eqsl.NewStore(store.DB()))
}

func TestSyncEQSLInBackground(t *testing.T) {
	f := &eqslList{}
	s := newTestEQSLSyncer(t, f)
	send, sent := recordSends()

	syncEQSLInBackground(t.Context(), s, slog.New(slog.DiscardHandler), send)
	assert.Equal(t, []tea.Msg{tui.EQSLSynced{}}, sent(), "a stale list is synced")

	syncEQSLInBackground(t.Context(), s, slog.New(slog.DiscardHandler), send)
	assert.Len(t, sent(), 1, "a fresh list is not synced again, and nothing is sent")
	assert.Equal(t, 1, f.requestCount())
}

func TestSyncEQSLInBackgroundError(t *testing.T) {
	s := newTestEQSLSyncer(t, &eqslList{status: http.StatusServiceUnavailable})
	send, sent := recordSends()

	syncEQSLInBackground(t.Context(), s, slog.New(slog.DiscardHandler), send)
	msgs := sent()
	require.Len(t, msgs, 1)
	synced, ok := msgs[0].(tui.EQSLSynced)
	require.True(t, ok)
	assert.ErrorContains(t, synced.Err, "503")
}

func TestSyncEQSLInBackgroundCanceled(t *testing.T) {
	s := newTestEQSLSyncer(t, &eqslList{})
	send, sent := recordSends()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	syncEQSLInBackground(ctx, s, slog.New(slog.DiscardHandler), send)
	assert.Empty(t, sent(), "a sync stopped by exit is not reported")
}

func TestSyncEQSLCanceledErrorIsCanceled(t *testing.T) {
	// The canceled check in syncEQSLInBackground relies on the syncer
	// wrapping the context error.
	s := newTestEQSLSyncer(t, &eqslList{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := s.Sync(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
