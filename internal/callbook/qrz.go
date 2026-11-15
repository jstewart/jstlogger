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

// DefaultQRZURL is QRZ.com's XML API endpoint.
const DefaultQRZURL = "https://xmldata.qrz.com/xml/current/"

// qrzAgent identifies jstlogger to QRZ.com at login.
const qrzAgent = "jstlogger"

// qrzNonSubscriber is the SubExp QRZ.com reports for an account without
// an XML subscription.
const qrzNonSubscriber = "non-subscriber"

// errQRZSession reports a session key QRZ.com no longer accepts.
var errQRZSession = errors.New("session expired")

// QRZ is a Looker for QRZ.com's XML callbook.
//
// It logs in on the first lookup and reuses the session key until QRZ.com
// reports it invalid or expired, then logs in once more and retries. An
// account without an XML subscription gets limited data, and its Results
// have Limited set. A QRZ is safe for concurrent use and must not be
// copied after first use.
type QRZ struct {
	// Username and Password are the QRZ.com account's credentials.
	Username, Password string
	// FullName adds the last name to the first name in Result.Name.
	FullName bool
	// Client makes the requests; nil means http.DefaultClient. Give it a
	// Timeout: a login holds a lock that other lookups wait on.
	Client *http.Client
	// URL is the API endpoint; empty means DefaultQRZURL.
	URL string

	mu      sync.Mutex
	key     string // cached session key
	limited bool   // the session's account has no XML subscription
}

// qrzReply is a QRZ.com XML reply.
type qrzReply struct {
	XMLName  xml.Name    `xml:"QRZDatabase"`
	Callsign qrzCallsign `xml:"Callsign"`
	Session  qrzSession  `xml:"Session"`
}

// qrzCallsign is the callsign record of a search reply.
type qrzCallsign struct {
	Call   string `xml:"call"`
	FName  string `xml:"fname"`
	Name   string `xml:"name"`
	Addr2  string `xml:"addr2"`
	State  string `xml:"state"`
	County string `xml:"county"`
	Grid   string `xml:"grid"`
	Email  string `xml:"email"`
}

// qrzSession is the session block every reply carries.
type qrzSession struct {
	Key    string `xml:"Key"`
	SubExp string `xml:"SubExp"`
	Error  string `xml:"Error"`
}

// limited reports whether the session belongs to a non-subscriber.
func (s qrzSession) limited() bool {
	return strings.EqualFold(strings.TrimSpace(s.SubExp), qrzNonSubscriber)
}

// client returns the HTTP client to use.
func (q *QRZ) client() *http.Client {
	if q.Client == nil {
		return http.DefaultClient
	}
	return q.Client
}

// url returns the API endpoint to use.
func (q *QRZ) url() string {
	if q.URL == "" {
		return DefaultQRZURL
	}
	return q.URL
}

// Lookup implements Looker.
func (q *QRZ) Lookup(ctx context.Context, call string) (Result, error) {
	base, portable := baseCall(call)
	key, limited, err := q.session(ctx, "")
	if err != nil {
		return Result{}, err
	}
	reply, err := q.search(ctx, key, base)
	if errors.Is(err, errQRZSession) {
		if key, limited, err = q.session(ctx, key); err != nil {
			return Result{}, err
		}
		reply, err = q.search(ctx, key, base)
	}
	if err != nil {
		return Result{}, fmt.Errorf("callbook: qrz: lookup %s: %w", call, err)
	}
	if reply.Session.SubExp != "" {
		limited = reply.Session.limited()
	}
	r := q.result(normalize(call), reply.Callsign, limited)
	if portable {
		r = r.portable()
	}
	return r, nil
}

// result maps a callsign record to a Result.
func (q *QRZ) result(call string, c qrzCallsign, limited bool) Result {
	name := c.FName
	if q.FullName {
		name += " " + c.Name
	}
	return Result{
		Call:    call,
		Name:    strings.TrimSpace(name),
		QTH:     strings.TrimSpace(c.Addr2),
		Grid:    strings.TrimSpace(c.Grid),
		State:   strings.TrimSpace(c.State),
		County:  strings.TrimSpace(c.County),
		Email:   strings.TrimSpace(c.Email),
		Service: ServiceQRZ,
		Limited: limited,
	}
}

// session returns the cached session key, logging in when there is none
// or when the cached key is still stale, the one a search just rejected.
// A login started by another goroutine for the same stale key is reused.
func (q *QRZ) session(ctx context.Context, stale string) (key string, limited bool, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.key != "" && q.key != stale {
		return q.key, q.limited, nil
	}
	q.key = ""
	reply, err := q.get(ctx, url.Values{
		"username": {q.Username},
		"password": {q.Password},
		"agent":    {qrzAgent},
	})
	if err != nil {
		return "", false, fmt.Errorf("callbook: qrz: login as %s: %w", q.Username, err)
	}
	s := reply.Session
	if s.Key == "" {
		msg := strings.TrimSpace(s.Error)
		if msg == "" {
			msg = "no session key in reply"
		}
		cause := ErrAuth
		if strings.Contains(strings.ToLower(msg), "limit") {
			cause = ErrRateLimit
		}
		return "", false, fmt.Errorf("callbook: qrz: login as %s: %s: %w", q.Username, msg, cause)
	}
	q.key, q.limited = s.Key, s.limited()
	return q.key, q.limited, nil
}

// search looks up call with session key, mapping an error reported in the
// reply to ErrNotFound, ErrRateLimit or errQRZSession where it matches.
func (q *QRZ) search(ctx context.Context, key, call string) (qrzReply, error) {
	reply, err := q.get(ctx, url.Values{"s": {key}, "callsign": {call}})
	if err != nil {
		return qrzReply{}, err
	}
	if msg := strings.TrimSpace(reply.Session.Error); msg != "" {
		return qrzReply{}, qrzError(msg)
	}
	if reply.Callsign.Call == "" {
		return qrzReply{}, &NetworkError{Service: ServiceQRZ, Err: errors.New("reply has no callsign record")}
	}
	return reply, nil
}

// qrzError maps the error text of a search reply to an error.
func qrzError(msg string) error {
	lower := strings.ToLower(msg)
	switch {
	case strings.HasPrefix(lower, "not found"):
		return fmt.Errorf("%s: %w", msg, ErrNotFound)
	case strings.Contains(lower, "invalid session key"), strings.Contains(lower, "session timeout"):
		return fmt.Errorf("%s: %w", msg, errQRZSession)
	case strings.Contains(lower, "limit"):
		return fmt.Errorf("%s: %w", msg, ErrRateLimit)
	default:
		return errors.New(msg)
	}
}

// get sends a request with the query params and decodes the reply; see
// getXML.
func (q *QRZ) get(ctx context.Context, params url.Values) (qrzReply, error) {
	var reply qrzReply
	err := getXML(ctx, q.client(), q.url(), ServiceQRZ, params, &reply)
	return reply, err
}
