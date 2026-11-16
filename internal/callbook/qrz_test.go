package callbook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPassword is a password with characters that must be escaped in a
// query string.
const testPassword = "s3cr&t;p@ss word"

// qrzServer is a fake QRZ.com XML API. Logins get the login fixture;
// searches get the search fixtures in order, the last one repeating. It
// records the logins and the searched calls.
type qrzServer struct {
	mu       sync.Mutex
	status   int    // when non-zero, every request gets this status
	body     string // when non-empty, every request gets this body
	login    string // fixture name for login replies
	searches []string
	logins   int
	username string
	password string
	agent    string
	searched []string
	keys     []string
}

func (f *qrzServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status != 0 {
		w.WriteHeader(f.status)
		return
	}
	if f.body != "" {
		_, _ = w.Write([]byte(f.body))
		return
	}
	q := r.URL.Query()
	var fixture string
	switch {
	case q.Has("username"):
		f.logins++
		f.username, f.password, f.agent = q.Get("username"), q.Get("password"), q.Get("agent")
		fixture = f.login
	case q.Has("s"):
		f.searched = append(f.searched, q.Get("callsign"))
		f.keys = append(f.keys, q.Get("s"))
		if len(f.searches) == 0 {
			http.Error(w, "no search fixture", http.StatusInternalServerError)
			return
		}
		fixture = f.searches[0]
		if len(f.searches) > 1 {
			f.searches = f.searches[1:]
		}
	default:
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write(data)
}

// set changes the server's behavior under its lock.
func (f *qrzServer) set(fn func(f *qrzServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// counts returns the number of logins and the searched calls.
func (f *qrzServer) counts() (logins int, searched []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logins, append([]string(nil), f.searched...)
}

// newQRZ returns a QRZ client against a fake server.
func newQRZ(t *testing.T, f *qrzServer) *QRZ {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &QRZ{Username: "N0CALL", Password: testPassword, Client: srv.Client(), URL: srv.URL}
}

func TestQRZLookup(t *testing.T) {
	f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
	q := newQRZ(t, f)

	r, err := q.Lookup(t.Context(), " w1aw ")
	require.NoError(t, err)
	assert.Equal(t, Result{
		Call:    "W1AW",
		Name:    "Hiram",
		QTH:     "Newington",
		Grid:    "FN31pr",
		State:   "CT",
		County:  "Hartford",
		Email:   "w1aw@arrl.org",
		Service: ServiceQRZ,
	}, r)

	logins, searched := f.counts()
	assert.Equal(t, 1, logins)
	assert.Equal(t, []string{"W1AW"}, searched)
	f.set(func(f *qrzServer) {
		assert.Equal(t, "N0CALL", f.username)
		assert.Equal(t, testPassword, f.password, "password must survive query escaping")
		assert.Equal(t, "jstlogger", f.agent)
		assert.Equal(t, []string{"2331uf894c4bd29f3923f3bacf02c532d7bd9"}, f.keys)
	})
}

func TestQRZLookupFullName(t *testing.T) {
	tests := []struct {
		name     string
		fullName bool
		want     string
	}{
		{"first name only", false, "Hiram"},
		{"first and last name", true, "Hiram Maxim"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
			q := newQRZ(t, f)
			q.FullName = tt.fullName
			r, err := q.Lookup(t.Context(), "W1AW")
			require.NoError(t, err)
			assert.Equal(t, tt.want, r.Name)
		})
	}
}

func TestQRZLookupNonSubscriber(t *testing.T) {
	tests := []struct {
		name   string
		login  string
		search string
	}{
		{"search reply says non-subscriber", "qrz_login_nonsub.xml", "qrz_search_nonsub.xml"},
		{"search reply overrides login", "qrz_login_ok.xml", "qrz_search_nonsub.xml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &qrzServer{login: tt.login, searches: []string{tt.search}}
			q := newQRZ(t, f)
			r, err := q.Lookup(t.Context(), "W1AW")
			require.NoError(t, err)
			assert.Equal(t, Result{
				Call:    "W1AW",
				Name:    "Hiram",
				QTH:     "Newington",
				Service: ServiceQRZ,
				Limited: true,
			}, r)
		})
	}
}

func TestQRZLookupReusesKey(t *testing.T) {
	f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
	q := newQRZ(t, f)
	for range 3 {
		_, err := q.Lookup(t.Context(), "W1AW")
		require.NoError(t, err)
	}
	logins, searched := f.counts()
	assert.Equal(t, 1, logins)
	assert.Len(t, searched, 3)
}

func TestQRZLookupExpiredSession(t *testing.T) {
	for _, fixture := range []string{"qrz_session_timeout.xml", "qrz_invalid_key.xml"} {
		t.Run(fixture, func(t *testing.T) {
			f := &qrzServer{
				login:    "qrz_login_ok.xml",
				searches: []string{"qrz_search_ok.xml", fixture, "qrz_search_ok.xml"},
			}
			q := newQRZ(t, f)
			_, err := q.Lookup(t.Context(), "W1AW")
			require.NoError(t, err)

			r, err := q.Lookup(t.Context(), "W1AW")
			require.NoError(t, err)
			assert.Equal(t, "Hiram", r.Name)

			logins, searched := f.counts()
			assert.Equal(t, 2, logins, "one login, then exactly one more after expiry")
			assert.Len(t, searched, 3, "the expired search is retried once")
		})
	}
}

func TestQRZLookupRepeatedExpiry(t *testing.T) {
	f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_session_timeout.xml"}}
	q := newQRZ(t, f)
	_, err := q.Lookup(t.Context(), "W1AW")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Session Timeout")

	logins, searched := f.counts()
	assert.Equal(t, 2, logins)
	assert.Len(t, searched, 2)
}

func TestQRZLookupErrors(t *testing.T) {
	tests := []struct {
		name   string
		login  string
		search string
		want   error
	}{
		{"bad password", "qrz_login_badpass.xml", "qrz_search_ok.xml", ErrAuth},
		{"session error at login", "qrz_session_timeout.xml", "qrz_search_ok.xml", ErrAuth},
		{"not found", "qrz_login_ok.xml", "qrz_not_found.xml", ErrNotFound},
		{"count limit", "qrz_login_ok.xml", "qrz_count_limit.xml", ErrRateLimit},
		{"count limit at login", "qrz_count_limit.xml", "qrz_search_ok.xml", ErrRateLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &qrzServer{login: tt.login, searches: []string{tt.search}}
			q := newQRZ(t, f)
			_, err := q.Lookup(t.Context(), "W1XYZ")
			require.ErrorIs(t, err, tt.want)
			assert.NotContains(t, err.Error(), testPassword)
		})
	}
}

func TestQRZLookupNetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		set  func(f *qrzServer)
	}{
		{"http 500", func(f *qrzServer) { f.status = http.StatusInternalServerError }},
		{"bad body", func(f *qrzServer) { f.body = "this is not XML" }},
		{"wrong root element", func(f *qrzServer) { f.body = "<html><body>oops</body></html>" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
			f.set(tt.set)
			q := newQRZ(t, f)
			_, err := q.Lookup(t.Context(), "W1AW")
			var ne *NetworkError
			require.ErrorAs(t, err, &ne)
			assert.Equal(t, ServiceQRZ, ne.Service)
		})
	}
}

func TestQRZLookupNetworkErrorOnSearch(t *testing.T) {
	f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
	q := newQRZ(t, f)
	_, err := q.Lookup(t.Context(), "W1AW")
	require.NoError(t, err)

	f.set(func(f *qrzServer) { f.status = http.StatusInternalServerError })
	_, err = q.Lookup(t.Context(), "W1AW")
	var ne *NetworkError
	require.ErrorAs(t, err, &ne)
}

func TestQRZLookupPasswordNotInTransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens now, so the request fails in transport

	q := &QRZ{Username: "N0CALL", Password: testPassword, URL: url}
	_, err := q.Lookup(t.Context(), "W1AW")
	var ne *NetworkError
	require.ErrorAs(t, err, &ne)
	for _, s := range []string{testPassword, "s3cr", "password="} {
		assert.NotContains(t, err.Error(), s)
	}
}

func TestQRZLookupCanceledContext(t *testing.T) {
	f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
	q := newQRZ(t, f)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := q.Lookup(ctx, "W1AW")
	require.ErrorIs(t, err, context.Canceled)
}

func TestQRZLookupPortable(t *testing.T) {
	for _, call := range []string{"VE3/W1AW", "w1aw/p"} {
		t.Run(call, func(t *testing.T) {
			f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
			q := newQRZ(t, f)
			r, err := q.Lookup(t.Context(), call)
			require.NoError(t, err)
			assert.Equal(t, Result{
				Call:    normalize(call),
				Name:    "Hiram",
				Email:   "w1aw@arrl.org",
				Service: ServiceQRZ,
			}, r)
			_, searched := f.counts()
			assert.Equal(t, []string{"W1AW"}, searched)
		})
	}
}

func TestQRZLookupConcurrent(t *testing.T) {
	f := &qrzServer{login: "qrz_login_ok.xml", searches: []string{"qrz_search_ok.xml"}}
	q := newQRZ(t, f)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := q.Lookup(t.Context(), "W1AW")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}
	logins, searched := f.counts()
	assert.Equal(t, 1, logins, "concurrent lookups share one login")
	assert.Len(t, searched, 20)
}
