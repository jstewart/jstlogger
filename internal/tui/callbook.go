package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/callbook"
	"jstlogger/internal/callsign"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

const (
	// callbookDelay is how long the call must stay unchanged while it is
	// typed before the New QSO screen looks it up in the callbook, so
	// typing a call does not query the callbook on every key press.
	callbookDelay = 400 * time.Millisecond
	// sourceLogbook is the source shown for details filled in from an
	// earlier QSO.
	sourceLogbook = "logbook"
)

// callbookTimeout bounds a callbook lookup. The logbook lookup that may
// follow it has logbookTimeout of its own. Tests shorten it.
var callbookTimeout = 15 * time.Second

// WithCallbook fills in, on the New QSO screen, the name, QTH, grid, state,
// county and email of the call typed, as looked up in l, and shows where
// they came from next to the call. Without it, they are filled in from the
// latest earlier QSO with the call.
func WithCallbook(l callbook.Looker) Option {
	return func(m *Model) {
		m.callbook = l
	}
}

// callbookFields are the fields filled in from the callbook, in the order
// of callbookDetails.values.
var callbookFields = [6]qsoentry.Field{
	qsoentry.Name, qsoentry.QTH, qsoentry.Gridsquare, qsoentry.State, qsoentry.Cnty, qsoentry.Email,
}

// serviceNames are the names shown for the callbook services.
var serviceNames = map[string]string{
	callbook.ServiceQRZ:    "QRZ.com",
	callbook.ServiceHamQTH: "HamQTH",
}

// callbookLookup is the New QSO screen's lookup of the details of the call
// being entered.
type callbookLookup struct {
	// wanted is the call last asked for, waiting out callbookDelay or under
	// way, and seq numbers the asks, so a tick or reply for an earlier one,
	// even for the same call, is dropped.
	wanted string
	seq    int
	// waiting reports that the lookup of wanted waits out callbookDelay.
	waiting bool
	// cancel cancels the lookup under way, if any.
	cancel context.CancelFunc
	// filled holds the values last filled in, in the order of
	// callbookFields. A value that still equals them has not been typed.
	filled [6]string
	// status is where the details of the wanted call came from, or why the
	// callbook failed to give them.
	status callbookStatus
}

// callbookStatus is the outcome of a lookup, shown next to the call.
type callbookStatus struct {
	// call is the call looked up; "" when no lookup has finished.
	call string
	// source is where the details came from, such as "QRZ.com", or "" when
	// nothing was found.
	source string
	// err is the callbook's failure, when it failed other than by not
	// finding the call.
	err error
}

// callbookDetails are the details found for a call: the values of
// callbookFields and where they came from.
type callbookDetails struct {
	values [6]string
	source string
}

// entryCallbookTickMsg ends the wait before the callbook lookup asked for
// as seq, on the New QSO screen's opening gen.
type entryCallbookTickMsg struct {
	gen, seq int
	call     string
}

// entryCallbookMsg carries the details found for call by the lookup asked
// for as seq, on the New QSO screen's opening gen, and the callbook's
// failure, if any.
type entryCallbookMsg struct {
	gen, seq int
	call     string
	details  callbookDetails
	err      error
}

// stop cancels the lookup under way, if any.
func (c *callbookLookup) stop() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
}

// reset cancels the lookup under way and forgets the call asked for, the
// values filled in and the status, so the form starts clean.
func (c *callbookLookup) reset() {
	c.stop()
	*c = callbookLookup{seq: c.seq + 1}
}

// callbookCall returns the call to look up for the value of the call field:
// upper-cased and trimmed, or "" when it has no base callsign.
func callbookCall(v string) string {
	call := strings.ToUpper(strings.TrimSpace(v))
	if callsign.Base(call) == "" {
		return ""
	}
	return call
}

// fetchCallbook asks for the lookup of the call in the New QSO screen, when
// it has changed since it last asked: it cancels the lookup under way and
// clears the values filled in, then starts the lookup, after callbookDelay
// while the call has focus. Focus leaving the call ends the wait. A form
// without a call asks for nothing. An edited QSO is never looked up.
func (m *Model) fetchCallbook() tea.Cmd {
	e := &m.entry
	c := &e.callbook
	if !e.open || e.editing {
		return nil
	}
	call := callbookCall(e.value(qsoentry.Call))
	if call == c.wanted {
		if c.waiting && e.focus != qsoentry.Call {
			// A new ask, so the wait's tick is dropped.
			c.seq++
			return m.startCallbook()
		}
		return nil
	}
	c.stop()
	c.wanted, c.status, c.waiting = call, callbookStatus{}, false
	c.seq++
	e.fillCallbook(callbookDetails{}.values)
	m.revalidateEntry()
	if call == "" {
		return nil
	}
	if e.focus != qsoentry.Call {
		return m.startCallbook()
	}
	gen, seq := e.gen, c.seq
	c.waiting = true
	return m.tick(callbookDelay, func(time.Time) tea.Msg {
		return entryCallbookTickMsg{gen: gen, seq: seq, call: call}
	})
}

// tickCallbook starts the lookup the tick is for, unless the screen has
// closed or the call has changed since.
func (m Model) tickCallbook(msg entryCallbookTickMsg) (Model, tea.Cmd) {
	e := &m.entry
	if !e.open || e.editing || msg.gen != e.gen || msg.seq != e.callbook.seq || !e.callbook.waiting {
		return m, nil
	}
	return m, m.startCallbook()
}

// startCallbook starts the lookup of the wanted call, keeping the means to
// cancel it.
func (m *Model) startCallbook() tea.Cmd {
	c := &m.entry.callbook
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel, c.waiting = cancel, false
	return callbookCmd(ctx, cancel, m.callbook, m.logbook, m.entry.gen, c.seq, c.wanted)
}

// callbookCmd returns a command that looks up call in l and, failing that,
// in s, for the lookup asked for as seq on the New QSO screen's opening
// gen. It calls cancel once done.
func callbookCmd(ctx context.Context, cancel context.CancelFunc, l callbook.Looker, s logbook.Store, gen, seq int, call string) tea.Cmd {
	return func() tea.Msg {
		defer cancel()
		d, err := lookupDetails(ctx, l, s, call)
		return entryCallbookMsg{gen: gen, seq: seq, call: call, details: d, err: err}
	}
}

// lookupDetails returns the details of call from l, if not nil. When l
// fails other than by not finding the call, or is nil, the details come
// from the latest QSO in s with the call's base callsign instead, and the
// error is l's failure. A call l does not find has no details. The lookup
// in l is bounded by callbookTimeout and the one in s by logbookTimeout, so
// a callbook that runs out of time still leaves time for the logbook. A
// canceled lookup returns the error canceling it.
func lookupDetails(ctx context.Context, l callbook.Looker, s logbook.Store, call string) (callbookDetails, error) {
	var failed error
	if l != nil {
		lctx, cancel := context.WithTimeout(ctx, callbookTimeout)
		r, err := l.Lookup(lctx, call)
		cancel()
		switch {
		case err == nil:
			return resultDetails(r), nil
		case errors.Is(err, context.Canceled):
			return callbookDetails{}, err
		case notFoundOnly(err):
			return callbookDetails{}, nil
		}
		failed = err
	}
	if s == nil {
		return callbookDetails{}, failed
	}
	sctx, cancel := context.WithTimeout(ctx, logbookTimeout)
	defer cancel()
	d, err := logbookDetails(sctx, s, call)
	if err != nil {
		return callbookDetails{}, errors.Join(failed, err)
	}
	return d, failed
}

// notFoundOnly reports whether err reports only that the call is not found,
// by every service when it joins the errors of several.
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

// resultDetails returns the details of r. The county is filled in as ADIF
// CNTY has it, with the state, such as "MA,Middlesex", so a county without
// a state is left out.
func resultDetails(r callbook.Result) callbookDetails {
	var cnty string
	if r.State != "" && r.County != "" {
		cnty = r.State + "," + r.County
	}
	source, ok := serviceNames[r.Service]
	if !ok {
		source = r.Service
	}
	if r.Limited {
		source += " (limited)"
	}
	return callbookDetails{
		values: [6]string{r.Name, r.QTH, r.Grid, r.State, cnty, r.Email},
		source: source,
	}
}

// logbookDetails returns the details of call from the latest QSO in s with
// its base callsign: the name and email, and where the station was, its
// QTH, grid, state and county, only when that QSO's call is call itself.
// It returns no details when there is no such QSO.
func logbookDetails(ctx context.Context, s logbook.Store, call string) (callbookDetails, error) {
	rows, err := s.Index(ctx, logbook.CallBaseIs{Call: call}, logbook.DefaultSort, 0, 1)
	if err != nil {
		return callbookDetails{}, fmt.Errorf("look up QSOs with %s: %w", call, err)
	}
	if len(rows) == 0 {
		return callbookDetails{}, nil
	}
	q, err := s.Get(ctx, rows[0].ID)
	if err != nil {
		return callbookDetails{}, fmt.Errorf("load QSO %d with %s: %w", rows[0].ID, call, err)
	}
	f := qsoentry.Load(q)
	var d callbookDetails
	for i, k := range callbookFields {
		if k == qsoentry.Name || k == qsoentry.Email || strings.EqualFold(q.Call, call) {
			d.values[i] = f[k]
		}
	}
	if d.values != [6]string{} {
		d.source = sourceLogbook
	}
	return d, nil
}

// applyCallbook fills in the details of the lookup the New QSO screen last
// asked for. A reply for an earlier ask or another call, one landing after
// the screen closed, or one canceled, is dropped.
func (m *Model) applyCallbook(msg entryCallbookMsg) {
	e := &m.entry
	c := &e.callbook
	if !e.open || e.editing || msg.gen != e.gen || msg.seq != c.seq ||
		msg.call != c.wanted || msg.call != callbookCall(e.value(qsoentry.Call)) {
		return
	}
	c.cancel = nil
	if errors.Is(msg.err, context.Canceled) {
		return
	}
	e.fillCallbook(msg.details.values)
	c.status = callbookStatus{call: msg.call, source: msg.details.source, err: msg.err}
	m.revalidateEntry()
}

// fillCallbook fills in values, each in its field of callbookFields if it
// is empty or holds the value last filled in.
func (e *entry) fillCallbook(values [6]string) {
	for i, f := range callbookFields {
		if v := e.value(f); v == "" || v == e.callbook.filled[i] {
			e.setValue(f, values[i])
		}
	}
	e.callbook.filled = values
}

// callbookFailure returns a short reason the callbook failed with err, or
// "" for no error. A failed login comes before a used-up limit, and that
// before an unreachable service, when several services fail.
func callbookFailure(err error) string {
	_, network := errors.AsType[*callbook.NetworkError](err)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, callbook.ErrAuth):
		return "callbook login failed"
	case errors.Is(err, callbook.ErrRateLimit):
		return "lookup limit reached"
	case network, errors.Is(err, context.DeadlineExceeded):
		return "callbook unreachable"
	}
	return "lookup failed"
}

// entryCallbook returns the status of the lookup of the call typed into the
// New QSO screen, styled: where its details came from and why the callbook
// failed, if it did. It is "" while the call is being looked up, when
// nothing was found, and while editing.
func (m Model) entryCallbook() string {
	st, e := m.styles, &m.entry
	s := e.callbook.status
	if e.editing || s.call == "" || s.call != callbookCall(e.value(qsoentry.Call)) {
		return ""
	}
	var parts []string
	if s.source != "" {
		parts = append(parts, st.muted.Render(s.source))
	}
	if why := callbookFailure(s.err); why != "" {
		parts = append(parts, st.err.Render(why))
	}
	return strings.Join(parts, st.muted.Render(" · "))
}
