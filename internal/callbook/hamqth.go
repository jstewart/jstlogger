package callbook

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// DefaultHamQTHURL is HamQTH.com's XML API endpoint.
const DefaultHamQTHURL = "https://www.hamqth.com/xml.php"

// hamqthProgram identifies jstlogger to HamQTH.com on each search.
const hamqthProgram = "jstlogger"

// errHamQTHSession reports a session ID HamQTH.com no longer accepts.
var errHamQTHSession = errors.New("session expired")

// HamQTH is a Looker for HamQTH.com's XML callbook.
//
// It logs in on the first lookup and reuses the session ID, which HamQTH
// keeps for an hour, until HamQTH reports it expired, then logs in once
// more and retries. A HamQTH is safe for concurrent use and must not be
// copied after first use.
type HamQTH struct {
	// Username and Password are the HamQTH.com account's credentials.
	Username, Password string
	// Client makes the requests; nil means http.DefaultClient. Give it a
	// Timeout: a login holds a lock that other lookups wait on.
	Client *http.Client
	// URL is the API endpoint; empty means DefaultHamQTHURL.
	URL string

	mu sync.Mutex
	id string // cached session ID
}

// hamqthReply is a HamQTH.com XML reply. The element names carry the
// https://www.hamqth.com namespace; tags without a namespace match them
// by local name.
type hamqthReply struct {
	XMLName xml.Name      `xml:"HamQTH"`
	Search  hamqthSearch  `xml:"search"`
	Session hamqthSession `xml:"session"`
}

// hamqthSearch is the callsign record of a search reply.
type hamqthSearch struct {
	Callsign string `xml:"callsign"`
	Nick     string `xml:"nick"`
	QTH      string `xml:"qth"`
	Grid     string `xml:"grid"`
	AdrName  string `xml:"adr_name"`
	AdrCity  string `xml:"adr_city"`
	USState  string `xml:"us_state"`
	USCounty string `xml:"us_county"`
	Email    string `xml:"email"`
}

// hamqthSession is the session block of a login or error reply.
type hamqthSession struct {
	ID    string `xml:"session_id"`
	Error string `xml:"error"`
}

// client returns the HTTP client to use.
func (h *HamQTH) client() *http.Client {
	if h.Client == nil {
		return http.DefaultClient
	}
	return h.Client
}

// url returns the API endpoint to use.
func (h *HamQTH) url() string {
	if h.URL == "" {
		return DefaultHamQTHURL
	}
	return h.URL
}

// Lookup implements Looker.
func (h *HamQTH) Lookup(ctx context.Context, call string) (Result, error) {
	base, portable := baseCall(call)
	id, err := h.session(ctx, "")
	if err != nil {
		return Result{}, err
	}
	reply, err := h.search(ctx, id, base)
	if errors.Is(err, errHamQTHSession) {
		if id, err = h.session(ctx, id); err != nil {
			return Result{}, err
		}
		reply, err = h.search(ctx, id, base)
	}
	if err != nil {
		return Result{}, fmt.Errorf("callbook: hamqth: lookup %s: %w", call, err)
	}
	r := hamqthResult(normalize(call), reply.Search)
	if portable {
		r = r.portable()
	}
	return r, nil
}

// hamqthResult maps a callsign record to a Result.
func hamqthResult(call string, s hamqthSearch) Result {
	return Result{
		Call:    call,
		Name:    firstNonEmpty(s.Nick, s.AdrName),
		QTH:     firstNonEmpty(s.AdrCity, s.QTH),
		Grid:    strings.TrimSpace(s.Grid),
		State:   strings.TrimSpace(s.USState),
		County:  strings.TrimSpace(s.USCounty),
		Email:   strings.TrimSpace(s.Email),
		Service: ServiceHamQTH,
	}
}

// firstNonEmpty trims each of values and returns the first that is not
// empty, or "" if none is.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// session returns the cached session ID, logging in when there is none
// or when the cached ID is still stale, the one a search just rejected.
// A login started by another goroutine for the same stale ID is reused.
func (h *HamQTH) session(ctx context.Context, stale string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.id != "" && h.id != stale {
		return h.id, nil
	}
	h.id = ""
	reply, err := h.get(ctx, url.Values{"u": {h.Username}, "p": {h.Password}})
	if err != nil {
		return "", fmt.Errorf("callbook: hamqth: login as %s: %w", h.Username, err)
	}
	s := reply.Session
	id := strings.TrimSpace(s.ID)
	if id == "" {
		msg := strings.TrimSpace(s.Error)
		if msg == "" {
			msg = "no session ID in reply"
		}
		cause := ErrAuth
		if strings.Contains(strings.ToLower(msg), "limit") {
			cause = ErrRateLimit
		}
		return "", fmt.Errorf("callbook: hamqth: login as %s: %s: %w", h.Username, msg, cause)
	}
	h.id = id
	return h.id, nil
}

// search looks up call with session id, mapping an error reported in the
// reply to ErrNotFound, ErrRateLimit or errHamQTHSession where it matches.
func (h *HamQTH) search(ctx context.Context, id, call string) (hamqthReply, error) {
	reply, err := h.get(ctx, url.Values{"id": {id}, "callsign": {call}, "prg": {hamqthProgram}})
	if err != nil {
		return hamqthReply{}, err
	}
	if msg := strings.TrimSpace(reply.Session.Error); msg != "" {
		return hamqthReply{}, hamqthError(msg)
	}
	if strings.TrimSpace(reply.Search.Callsign) == "" {
		return hamqthReply{}, &NetworkError{Service: ServiceHamQTH, Err: errors.New("reply has no search record")}
	}
	return reply, nil
}

// hamqthError maps the error text of a search reply to an error.
func hamqthError(msg string) error {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "callsign not found"):
		return fmt.Errorf("%s: %w", msg, ErrNotFound)
	case strings.Contains(lower, "session does not exist"), strings.Contains(lower, "expired"):
		return fmt.Errorf("%s: %w", msg, errHamQTHSession)
	case strings.Contains(lower, "limit"):
		return fmt.Errorf("%s: %w", msg, ErrRateLimit)
	default:
		return errors.New(msg)
	}
}

// get sends a request with the query params and decodes the reply; see
// getXML.
func (h *HamQTH) get(ctx context.Context, params url.Values) (hamqthReply, error) {
	var reply hamqthReply
	err := getXML(ctx, h.client(), h.url(), ServiceHamQTH, params, &reply)
	return reply, err
}
