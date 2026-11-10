package tui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/callsign"
	"jstlogger/internal/lotw"
)

// LoTWLookup reports when a callsign last uploaded to LoTW.
type LoTWLookup interface {
	// LastUpload returns the time call last uploaded, and whether call is
	// a known LoTW user.
	LastUpload(ctx context.Context, call string) (time.Time, bool, error)
}

// WithLoTW shows, on the pager's QSO tab, whether the QSO's callsign uses
// LoTW and how recently it uploaded, as looked up in l.
func WithLoTW(l LoTWLookup) Option {
	return func(m *Model) {
		m.lotw = l
	}
}

// WithClock sets the clock the UI reads the current time from; the default
// is time.Now.
func WithClock(now func() time.Time) Option {
	return func(m *Model) {
		m.now = now
	}
}

// LoTWSynced reports that a background sync of the LoTW activity feed
// finished. Err is nil when it succeeded.
type LoTWSynced struct {
	Err error
}

// lotwCache is the pager's LoTW lookup for one base callsign. call is the
// base callsign that last, found and err are for; pending is the base
// callsign being looked up.
type lotwCache struct {
	call    string
	pending string
	last    time.Time
	found   bool
	err     error
}

// lotwLookupMsg carries the result of a LoTW lookup for the pager.
type lotwLookupMsg struct {
	call  string
	last  time.Time
	found bool
	err   error
}

// lotwLookupCmd returns a command that looks up the base callsign call in
// l.
func lotwLookupCmd(l LoTWLookup, call string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		last, found, err := l.LastUpload(ctx, call)
		if err != nil {
			err = fmt.Errorf("look up %s on LoTW: %w", call, err)
		}
		return lotwLookupMsg{call: call, last: last, found: found, err: err}
	}
}

// fetchLoTW returns a command looking up the base callsign of the QSO under
// the index cursor, unless it has been looked up or is being looked up.
func (m *Model) fetchLoTW() tea.Cmd {
	q, ok := m.idx.current()
	if !m.pager.open || m.lotw == nil || !ok {
		return nil
	}
	base, c := callsign.Base(q.Call), &m.pager.lotw
	if base == "" || base == c.call || base == c.pending {
		return nil
	}
	c.pending = base
	return lotwLookupCmd(m.lotw, base)
}

// applyLoTW keeps a LoTW lookup for the pager. A lookup for a callsign the
// cursor has moved off, or one landing after the pager closed, is dropped.
// It reports whether the lookup was kept.
func (p *pager) applyLoTW(msg lotwLookupMsg, current string) bool {
	if msg.call == p.lotw.pending {
		p.lotw.pending = ""
	}
	if !p.open || msg.call != current {
		return false
	}
	p.lotw.call, p.lotw.last, p.lotw.found, p.lotw.err = msg.call, msg.last, msg.found, msg.err
	return true
}

// applyLoTWSynced records the outcome of a LoTW sync. A successful sync
// forgets the pager's lookup, so the QSO shown is looked up again.
func (m *Model) applyLoTWSynced(msg LoTWSynced) {
	if msg.Err != nil {
		m.lotwErr = fmt.Errorf("LoTW sync failed: %w", msg.Err)
		return
	}
	m.lotwErr = nil
	m.pager.lotw = lotwCache{}
}

// lotwStatus is what the QSO tab shows of a callsign's LoTW use.
type lotwStatus struct {
	// enabled reports whether a LoTW lookup is configured; without one the
	// QSO tab has no LoTW row.
	enabled bool
	// loaded reports whether the lookup succeeded; found and last are its
	// result.
	loaded bool
	found  bool
	last   time.Time
	// failed reports whether the lookup failed.
	failed bool
	// now is the time recency is measured at.
	now time.Time
}

// lotwStatus returns the LoTW status of the QSO with call, from the pager's
// lookup. It does no I/O.
func (m Model) lotwStatus(call string) lotwStatus {
	if m.lotw == nil {
		return lotwStatus{}
	}
	c := m.pager.lotw
	s := lotwStatus{enabled: true, now: m.now()}
	if base := callsign.Base(call); base == "" || c.call != base {
		return s
	}
	if c.err != nil {
		s.failed = true
		return s
	}
	s.loaded, s.found, s.last = true, c.found, c.last
	return s
}

// badge renders the [L] badge, colored by how recently the callsign
// uploaded, or "" when it is not known to use LoTW.
func (s lotwStatus) badge(st styles) string {
	if !s.loaded || !s.found {
		return ""
	}
	switch lotw.RecencyOf(s.last, s.now) {
	case lotw.Recent:
		return st.lotwRecent.Render("[L]")
	case lotw.Active:
		return st.lotwActive.Render("[L]")
	case lotw.Inactive:
		return st.lotwInactive.Render("[L]")
	default:
		return ""
	}
}

// text describes the callsign's last upload, or "" while it is not known
// or the callsign is not a LoTW user.
func (s lotwStatus) text() string {
	switch {
	case s.failed:
		return "lookup failed"
	case !s.loaded || !s.found:
		return ""
	}
	switch days := lotw.DaysAgo(s.last, s.now); days {
	case 0:
		return "last upload today"
	case 1:
		return "last upload 1 day ago"
	default:
		return "last upload " + strconv.Itoa(days) + " days ago"
	}
}
