package callbook

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"jstlogger/internal/callsign"
)

// Looker looks up a callsign.
//
//go:generate go tool mockgen -source=callbook.go -destination=mock/callbook.go -package=mock
type Looker interface {
	// Lookup returns what the callbook knows about call, or an error that
	// wraps ErrNotFound, ErrAuth, ErrRateLimit or a *NetworkError. An error
	// the service reports in a form it does not recognize wraps none of
	// them.
	Lookup(ctx context.Context, call string) (Result, error)
}

// Service names, as stored in Result.Service.
const (
	// ServiceQRZ is QRZ.com's XML callbook.
	ServiceQRZ = "qrz"
	// ServiceHamQTH is HamQTH.com's XML callbook.
	ServiceHamQTH = "hamqth"
)

// Result is what a callbook returned for a callsign.
type Result struct {
	Call    string // the call looked up, upper-cased, including any prefix or suffix
	Name    string
	QTH     string // city
	Grid    string
	State   string
	County  string
	Email   string
	Service string // ServiceQRZ or ServiceHamQTH
	Limited bool   // the service returned limited data (QRZ without an XML subscription)
}

// portable returns r without the fields that describe where the operator
// lives (QTH, Grid, State and County), which may not apply where a portable
// station is operating.
func (r Result) portable() Result {
	r.QTH, r.Grid, r.State, r.County = "", "", "", ""
	return r
}

// Errors a Looker returns, possibly wrapped.
var (
	// ErrNotFound means the callbook does not list the call.
	ErrNotFound = errors.New("callbook: call not found")
	// ErrAuth means the callbook rejected the configured credentials.
	ErrAuth = errors.New("callbook: login failed")
	// ErrRateLimit means the account has used up its lookups for now.
	ErrRateLimit = errors.New("callbook: lookup limit reached")
)

// NetworkError reports a failure to reach a service, or an unexpected HTTP
// status or unparseable reply from it.
type NetworkError struct {
	Service string // ServiceQRZ or ServiceHamQTH
	Err     error
}

// Error implements error.
func (e *NetworkError) Error() string {
	return fmt.Sprintf("callbook: %s: %v", e.Service, e.Err)
}

// Unwrap returns the underlying error.
func (e *NetworkError) Unwrap() error { return e.Err }

// normalize trims and upper-cases call.
func normalize(call string) string {
	return strings.ToUpper(strings.TrimSpace(call))
}

// baseCall returns the base callsign the callbooks list call under, and
// whether call has a prefix or suffix (VE3/W1AW, W1AW/P, W1AW/7) that the
// base does not.
func baseCall(call string) (base string, portable bool) {
	n := normalize(call)
	base = callsign.Base(n)
	return base, n != base
}
