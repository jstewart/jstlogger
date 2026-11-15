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

// hamqthServer is a fake HamQTH XML API. Logins get the login fixture;
// searches get the search fixtures in order, the last one repeating. It
// records the logins and the searched calls.
type hamqthServer struct {
	mu       sync.Mutex
	status   int    // when non-zero, every request gets this status
	body     string // when non-empty, every request gets this body
	login    string // fixture name for login replies
	searches []string
	logins   int
	username string
	password string
	prg      []string
	searched []string
	ids      []string
}

func (f *hamqthServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	case q.Has("u"):
		f.logins++
		f.username, f.password = q.Get("u"), q.Get("p")
		fixture = f.login
	case q.Has("id"):
		f.searched = append(f.searched, q.Get("callsign"))
		f.ids = append(f.ids, q.Get("id"))
		f.prg = append(f.prg, q.Get("prg"))
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
func (f *hamqthServer) set(fn func(f *hamqthServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// counts returns the number of logins and the searched calls.
func (f *hamqthServer) counts() (logins int, searched []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logins, append([]string(nil), f.searched...)
}

// newHamQTH returns a HamQTH client against a fake server.
func newHamQTH(t *testing.T, f *hamqthServer) *HamQTH {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &HamQTH{Username: "n0call", Password: testPassword, Client: srv.Client(), URL: srv.URL}
}

func TestHamQTHLookup(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_ok.xml"}}
	h := newHamQTH(t, f)

	r, err := h.Lookup(t.Context(), " ok1rr ")
	require.NoError(t, err)
	assert.Equal(t, Result{
		Call:    "OK1RR",
		Name:    "Petr",
		QTH:     "Neratovice",
		Grid:    "jo70gg",
		Email:   "ok1rr@rajce.net",
		Service: ServiceHamQTH,
	}, r)

	logins, searched := f.counts()
	assert.Equal(t, 1, logins)
	assert.Equal(t, []string{"OK1RR"}, searched)
	f.set(func(f *hamqthServer) {
		assert.Equal(t, "n0call", f.username)
		assert.Equal(t, testPassword, f.password, "password must survive query escaping")
		assert.Equal(t, []string{"jstlogger"}, f.prg)
		assert.Equal(t, []string{"09b0ae90050be03c452ad235a1f2915ad684393c"}, f.ids)
	})
}

func TestHamQTHLookupStateCounty(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_us.xml"}}
	h := newHamQTH(t, f)
	r, err := h.Lookup(t.Context(), "K1ABC")
	require.NoError(t, err)
	assert.Equal(t, Result{
		Call:    "K1ABC",
		Name:    "Bob",
		QTH:     "Newington",
		Grid:    "FN31pr",
		State:   "CT",
		County:  "Hartford",
		Email:   "k1abc@example.com",
		Service: ServiceHamQTH,
	}, r)
}

func TestHamQTHLookupFallbacks(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_no_nick.xml"}}
	h := newHamQTH(t, f)
	r, err := h.Lookup(t.Context(), "OK1RR")
	require.NoError(t, err)
	assert.Equal(t, "Petr Hlozek", r.Name, "empty nick falls back to adr_name")
	assert.Equal(t, "Neratovice", r.QTH, "empty adr_city falls back to qth")
}

func TestHamQTHLookupReusesSession(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_ok.xml"}}
	h := newHamQTH(t, f)
	for range 3 {
		_, err := h.Lookup(t.Context(), "OK1RR")
		require.NoError(t, err)
	}
	logins, searched := f.counts()
	assert.Equal(t, 1, logins)
	assert.Len(t, searched, 3)
}

func TestHamQTHLookupExpiredSession(t *testing.T) {
	f := &hamqthServer{
		login:    "hamqth_login_ok.xml",
		searches: []string{"hamqth_search_ok.xml", "hamqth_session_expired.xml", "hamqth_search_ok.xml"},
	}
	h := newHamQTH(t, f)
	_, err := h.Lookup(t.Context(), "OK1RR")
	require.NoError(t, err)

	r, err := h.Lookup(t.Context(), "OK1RR")
	require.NoError(t, err)
	assert.Equal(t, "Petr", r.Name)

	logins, searched := f.counts()
	assert.Equal(t, 2, logins, "one login, then exactly one more after expiry")
	assert.Len(t, searched, 3, "the expired search is retried once")
}

func TestHamQTHLookupRepeatedExpiry(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_session_expired.xml"}}
	h := newHamQTH(t, f)
	_, err := h.Lookup(t.Context(), "OK1RR")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Session does not exist or expired")

	logins, searched := f.counts()
	assert.Equal(t, 2, logins)
	assert.Len(t, searched, 2)
}

func TestHamQTHLookupErrors(t *testing.T) {
	tests := []struct {
		name   string
		login  string
		search string
		want   error
	}{
		{"bad password", "hamqth_login_badpass.xml", "hamqth_search_ok.xml", ErrAuth},
		{"session error at login", "hamqth_session_expired.xml", "hamqth_search_ok.xml", ErrAuth},
		{"not found", "hamqth_login_ok.xml", "hamqth_not_found.xml", ErrNotFound},
		{"limit", "hamqth_login_ok.xml", "hamqth_limit.xml", ErrRateLimit},
		{"limit at login", "hamqth_limit.xml", "hamqth_search_ok.xml", ErrRateLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &hamqthServer{login: tt.login, searches: []string{tt.search}}
			h := newHamQTH(t, f)
			_, err := h.Lookup(t.Context(), "W1XYZ")
			require.ErrorIs(t, err, tt.want)
			assert.NotContains(t, err.Error(), testPassword)
		})
	}
}

func TestHamQTHLookupNetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		set  func(f *hamqthServer)
	}{
		{"http 500", func(f *hamqthServer) { f.status = http.StatusInternalServerError }},
		{"bad body", func(f *hamqthServer) { f.body = "this is not XML" }},
		{"wrong root element", func(f *hamqthServer) { f.body = "<html><body>oops</body></html>" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_ok.xml"}}
			f.set(tt.set)
			h := newHamQTH(t, f)
			_, err := h.Lookup(t.Context(), "OK1RR")
			var ne *NetworkError
			require.ErrorAs(t, err, &ne)
			assert.Equal(t, ServiceHamQTH, ne.Service)
		})
	}
}

func TestHamQTHLookupNetworkErrorOnSearch(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_ok.xml"}}
	h := newHamQTH(t, f)
	_, err := h.Lookup(t.Context(), "OK1RR")
	require.NoError(t, err)

	f.set(func(f *hamqthServer) { f.status = http.StatusInternalServerError })
	_, err = h.Lookup(t.Context(), "OK1RR")
	var ne *NetworkError
	require.ErrorAs(t, err, &ne)
}

func TestHamQTHLookupPasswordNotInTransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens now, so the request fails in transport

	h := &HamQTH{Username: "n0call", Password: testPassword, URL: url}
	_, err := h.Lookup(t.Context(), "OK1RR")
	var ne *NetworkError
	require.ErrorAs(t, err, &ne)
	for _, s := range []string{testPassword, "s3cr", "p="} {
		assert.NotContains(t, err.Error(), s)
	}
}

func TestHamQTHLookupCanceledContext(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_ok.xml"}}
	h := newHamQTH(t, f)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := h.Lookup(ctx, "OK1RR")
	require.ErrorIs(t, err, context.Canceled)
}

func TestHamQTHLookupPortable(t *testing.T) {
	for _, call := range []string{"DL/OK1RR", "ok1rr/p"} {
		t.Run(call, func(t *testing.T) {
			f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_us.xml"}}
			h := newHamQTH(t, f)
			r, err := h.Lookup(t.Context(), call)
			require.NoError(t, err)
			assert.Equal(t, Result{
				Call:    normalize(call),
				Name:    "Bob",
				Email:   "k1abc@example.com",
				Service: ServiceHamQTH,
			}, r)
			_, searched := f.counts()
			assert.Equal(t, []string{"OK1RR"}, searched)
		})
	}
}

func TestHamQTHLookupConcurrent(t *testing.T) {
	f := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_ok.xml"}}
	h := newHamQTH(t, f)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := h.Lookup(t.Context(), "OK1RR")
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
