package callinfo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"jstlogger/internal/callbook"
	"jstlogger/internal/logbook"
)

// SourceLogbook is the Details.Source of details taken from an earlier QSO.
const SourceLogbook = "logbook"

// Default timeouts, used when a Looker's are zero.
const (
	DefaultCallbookTimeout = 15 * time.Second
	DefaultLogbookTimeout  = 5 * time.Second
)

// Looker looks up contacted stations. Each of its sources may be nil, and
// is then skipped. The zero value finds nothing. A Looker is safe for
// concurrent use when its sources are.
type Looker struct {
	// DXCC resolves the call's DXCC entity, continent and zones. Only Fill
	// uses it.
	DXCC DXCCLookup
	// Callbook looks up the operator's details.
	Callbook callbook.Looker
	// Logbook gives the details of an earlier QSO with the call when the
	// callbook is nil or fails other than by not finding the call.
	Logbook logbook.Store
	// CallbookTimeout bounds a callbook lookup; zero means
	// DefaultCallbookTimeout.
	CallbookTimeout time.Duration
	// LogbookTimeout bounds the logbook lookup that may follow it; zero
	// means DefaultLogbookTimeout.
	LogbookTimeout time.Duration
}

// Details are what is known about a call's operator.
type Details struct {
	Name  string
	QTH   string // city
	Grid  string
	State string
	// Cnty is the county as ADIF CNTY has it, with the state, such as
	// "MA,Middlesex".
	Cnty  string
	Email string
	// Source is where the details came from: callbook.ServiceQRZ,
	// callbook.ServiceHamQTH, SourceLogbook, or "" when nothing was found.
	Source string
	// Limited reports that the callbook returned limited data (QRZ.com
	// without an XML subscription).
	Limited bool
}

// Details returns the details of call from the callbook. When the
// callbook fails other than by not finding the call, or is nil, the details
// come from the latest QSO in the logbook with the call's base callsign
// instead, and the error is the callbook's failure. A call the callbook
// does not find has no details. The callbook lookup is bounded by
// CallbookTimeout and the logbook lookup by LogbookTimeout, so a callbook
// that runs out of time still leaves time for the logbook. A canceled
// lookup returns the error canceling it.
func (l Looker) Details(ctx context.Context, call string) (Details, error) {
	var failed error
	if l.Callbook != nil {
		cctx, cancel := context.WithTimeout(ctx, cmp.Or(l.CallbookTimeout, DefaultCallbookTimeout))
		r, err := l.Callbook.Lookup(cctx, call)
		cancel()
		switch {
		case err == nil:
			return resultDetails(r), nil
		case errors.Is(err, context.Canceled):
			return Details{}, err
		case notFoundOnly(err):
			return Details{}, nil
		}
		failed = err
	}
	if l.Logbook == nil {
		return Details{}, failed
	}
	lctx, cancel := context.WithTimeout(ctx, cmp.Or(l.LogbookTimeout, DefaultLogbookTimeout))
	defer cancel()
	d, err := logbookDetails(lctx, l.Logbook, call)
	if err != nil {
		return Details{}, errors.Join(failed, err)
	}
	return d, failed
}

// notFoundOnly reports whether err reports only that the call is not
// found, by every service when it joins the errors of several.
func notFoundOnly(err error) bool {
	if j, ok := errors.AsType[interface {
		error
		Unwrap() []error
	}](err); ok {
		errs := j.Unwrap()
		for _, e := range errs {
			if !notFoundOnly(e) {
				return false
			}
		}
		return len(errs) > 0
	}
	return errors.Is(err, callbook.ErrNotFound)
}

// resultDetails returns the details of r. The county is given as ADIF
// CNTY has it, with the state, so a county without a state is left out.
func resultDetails(r callbook.Result) Details {
	var cnty string
	if r.State != "" && r.County != "" {
		cnty = r.State + "," + r.County
	}
	return Details{
		Name: r.Name, QTH: r.QTH, Grid: r.Grid, State: r.State, Cnty: cnty, Email: r.Email,
		Source: r.Service, Limited: r.Limited,
	}
}

// logbookDetails returns the details of call from the latest QSO in s with
// its base callsign: the name and email, and where the station was, its
// QTH, grid, state and county, only when that QSO's call is call itself.
// It returns no details when there is no such QSO.
func logbookDetails(ctx context.Context, s logbook.Store, call string) (Details, error) {
	rows, err := s.Index(ctx, logbook.CallBaseIs{Call: call}, logbook.DefaultSort, 0, 1)
	if err != nil {
		return Details{}, fmt.Errorf("look up QSOs with %s: %w", call, err)
	}
	if len(rows) == 0 {
		return Details{}, nil
	}
	q, err := s.Get(ctx, rows[0].ID)
	if err != nil {
		return Details{}, fmt.Errorf("load QSO %d with %s: %w", rows[0].ID, call, err)
	}
	d := Details{Name: q.Name, Email: q.Fields["EMAIL"]}
	if strings.EqualFold(q.Call, call) {
		d.QTH, d.Grid, d.State, d.Cnty = q.QTH, q.Gridsquare, q.State, q.Cnty
	}
	if d != (Details{}) {
		d.Source = SourceLogbook
	}
	return d, nil
}
