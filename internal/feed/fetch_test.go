package feed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeServer serves a fixed response and records the conditional headers
// of the last request. It is used by one goroutine at a time.
type fakeServer struct {
	status       int
	body         string
	etag         string
	lastModified string
	ifNoneMatch  string
	ifModSince   string
	requests     int
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests++
	f.ifNoneMatch = r.Header.Get("If-None-Match")
	f.ifModSince = r.Header.Get("If-Modified-Since")
	if f.etag != "" {
		w.Header().Set("ETag", f.etag)
	}
	if f.lastModified != "" {
		w.Header().Set("Last-Modified", f.lastModified)
	}
	if f.status != 0 {
		w.WriteHeader(f.status)
	}
	_, _ = fmt.Fprint(w, f.body)
}

// serve starts f and returns its client and URL.
func serve(t *testing.T, f *fakeServer) (*http.Client, string) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return srv.Client(), srv.URL
}

// readAll returns a parse func that stores the body in *got.
func readAll(got *string) func(io.Reader) error {
	return func(r io.Reader) error {
		b, err := io.ReadAll(r)
		*got = string(b)
		return err
	}
}

// synced is stored metadata from an earlier download.
var synced = Meta{
	ETag:         `"v1"`,
	LastModified: "Sun, 08 Nov 2026 09:00:00 GMT",
	SyncedAt:     time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC),
}

func TestFetchOK(t *testing.T) {
	f := &fakeServer{body: "hello\n", etag: `"v2"`, lastModified: "Mon, 09 Nov 2026 09:00:00 GMT"}
	client, url := serve(t, f)
	var got string

	res, err := Fetch(t.Context(), client, url, Meta{}, 1<<20, readAll(&got))

	require.NoError(t, err)
	assert.Equal(t, Response{ETag: `"v2"`, LastModified: "Mon, 09 Nov 2026 09:00:00 GMT"}, res)
	assert.Equal(t, "hello\n", got)
	assert.Empty(t, f.ifNoneMatch, "no stored copy means an unconditional request")
	assert.Empty(t, f.ifModSince)
}

func TestFetchSendsConditionalHeaders(t *testing.T) {
	f := &fakeServer{body: "x"}
	client, url := serve(t, f)
	var got string

	_, err := Fetch(t.Context(), client, url, synced, 1<<20, readAll(&got))

	require.NoError(t, err)
	assert.Equal(t, `"v1"`, f.ifNoneMatch)
	assert.Equal(t, "Sun, 08 Nov 2026 09:00:00 GMT", f.ifModSince)
}

func TestFetchOmitsEmptyConditionalHeaders(t *testing.T) {
	f := &fakeServer{body: "x"}
	client, url := serve(t, f)
	var got string

	_, err := Fetch(t.Context(), client, url, Meta{SyncedAt: synced.SyncedAt}, 1<<20, readAll(&got))

	require.NoError(t, err)
	assert.Empty(t, f.ifNoneMatch)
	assert.Empty(t, f.ifModSince)
}

func TestFetchNotModified(t *testing.T) {
	f := &fakeServer{status: http.StatusNotModified}
	client, url := serve(t, f)
	called := false

	res, err := Fetch(t.Context(), client, url, synced, 1<<20, func(io.Reader) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, Response{NotModified: true}, res)
	assert.False(t, called, "a 304 has no body to parse")
}

func TestFetchNotModifiedWithoutStoredCopy(t *testing.T) {
	f := &fakeServer{status: http.StatusNotModified}
	client, url := serve(t, f)

	_, err := Fetch(t.Context(), client, url, Meta{}, 1<<20, func(io.Reader) error { return nil })

	require.ErrorIs(t, err, ErrUnexpectedNotModified)
	assert.Contains(t, err.Error(), "feed:")
}

func TestFetchUnexpectedStatus(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusNotFound, http.StatusMovedPermanently} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := &fakeServer{status: status, body: "<html>oops</html>"}
			client, url := serve(t, f)
			// Do not follow redirects, so a 3xx reaches Fetch.
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			called := false

			_, err := Fetch(t.Context(), client, url, synced, 1<<20, func(io.Reader) error {
				called = true
				return nil
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "feed:")
			assert.Contains(t, err.Error(), fmt.Sprint(status))
			assert.False(t, called)
		})
	}
}

func TestFetchOversizedBody(t *testing.T) {
	body := strings.Repeat("a", 100)
	f := &fakeServer{body: body}
	client, url := serve(t, f)
	var got string

	_, err := Fetch(t.Context(), client, url, Meta{}, int64(len(body)-1), readAll(&got))
	require.ErrorIs(t, err, ErrTooLarge)

	_, err = Fetch(t.Context(), client, url, Meta{}, int64(len(body)), readAll(&got))
	require.NoError(t, err, "a body exactly at the limit is accepted")
	assert.Equal(t, body, got)
}

func TestFetchRejectsNonPositiveLimit(t *testing.T) {
	f := &fakeServer{body: "x"}
	client, url := serve(t, f)

	_, err := Fetch(t.Context(), client, url, Meta{}, 0, func(io.Reader) error { return nil })

	require.Error(t, err)
	assert.Zero(t, f.requests, "no request is made")
}

func TestFetchParseError(t *testing.T) {
	f := &fakeServer{body: "x"}
	client, url := serve(t, f)
	errBad := errors.New("bad feed")

	_, err := Fetch(t.Context(), client, url, Meta{}, 1<<20, func(io.Reader) error { return errBad })

	require.ErrorIs(t, err, errBad)
}

func TestFetchCancelledContext(t *testing.T) {
	f := &fakeServer{body: "x"}
	client, url := serve(t, f)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := Fetch(ctx, client, url, Meta{}, 1<<20, func(io.Reader) error { return nil })

	require.ErrorIs(t, err, context.Canceled)
	assert.Contains(t, err.Error(), "feed:")
}

func TestFetchNilClientUsesDefault(t *testing.T) {
	f := &fakeServer{body: "x"}
	_, url := serve(t, f)
	var got string

	_, err := Fetch(t.Context(), nil, url, Meta{}, 1<<20, readAll(&got))

	require.NoError(t, err)
	assert.Equal(t, "x", got)
}

func TestCappedReader(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		limit   int64
		wantErr error
	}{
		{name: "under the limit", body: "abc", limit: 10},
		{name: "exactly at the limit", body: "abcd", limit: 4},
		{name: "empty", body: "", limit: 1},
		{name: "one byte over", body: "abcde", limit: 4, wantErr: ErrTooLarge},
		{name: "far over", body: strings.Repeat("z", 5000), limit: 10, wantErr: ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := io.ReadAll(&cappedReader{r: strings.NewReader(tt.body), n: tt.limit})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.body, string(got))
		})
	}
}
