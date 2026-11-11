package tui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/callsign"
	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
	"jstlogger/internal/lotw"
)

// LoTWLookup reports when callsigns last uploaded to LoTW.
type LoTWLookup interface {
	// LastUpload returns the time call last uploaded, and whether call is
	// a known LoTW user.
	LastUpload(ctx context.Context, call string) (time.Time, bool, error)
	// LastUploads returns, for each of calls that is a known LoTW user,
	// the time it last uploaded, keyed by the call as given.
	LastUploads(ctx context.Context, calls []string) (map[string]time.Time, error)
}

// WithLoTW shows, on the pager's QSO tab and in an index %L column, whether
// the QSO's callsign uses LoTW and how recently it uploaded, as looked up
// in l.
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
// forgets the pager's lookup and the index's, so the QSO shown is looked up
// again, and returns a command looking up the index's cached rows again.
func (m *Model) applyLoTWSynced(msg LoTWSynced) tea.Cmd {
	if msg.Err != nil {
		m.lotwErr = fmt.Errorf("LoTW sync failed: %w", msg.Err)
		return nil
	}
	m.lotwErr = nil
	m.pager.lotw = lotwCache{}
	m.idx.lotw.forget()
	return m.lookupIndexLoTW(slices.Concat(slices.Collect(maps.Values(m.idx.blocks))...))
}

// indexLoTW caches, by base callsign, the LoTW lookups an index %L column
// shows. It holds one entry per distinct base callsign shown, which is
// bounded by the logbook, so entries are not evicted with their blocks.
type indexLoTW struct {
	// last holds the last upload of each base callsign looked up, or the
	// zero time for one that is not a LoTW user.
	last map[string]time.Time
	// prev holds lookups made before the latest sync, shown until a new
	// lookup replaces them so the column doesn't blank while it runs.
	prev map[string]time.Time
	// pending holds the base callsigns being looked up.
	pending map[string]bool
	// gen counts syncs; lookups started before the latest sync are
	// dropped.
	gen int
	// now returns the time recency is measured at.
	now func() time.Time
}

// newIndexLoTW returns an empty indexLoTW on the system clock.
func newIndexLoTW() indexLoTW {
	return indexLoTW{
		last:    map[string]time.Time{},
		prev:    map[string]time.Time{},
		pending: map[string]bool{},
		now:     time.Now,
	}
}

// forget marks every lookup out of date, so it is looked up again, and drops
// the lookups under way. The old lookups show until new ones replace them.
func (c *indexLoTW) forget() {
	c.gen++
	maps.Copy(c.prev, c.last)
	c.last = map[string]time.Time{}
	c.pending = map[string]bool{}
}

// lastUpload returns when the base callsign of call last uploaded to LoTW,
// and whether it is known to be a LoTW user. It does no I/O.
func (c indexLoTW) lastUpload(call string) (time.Time, bool) {
	base := callsign.Base(call)
	last, ok := c.last[base]
	if !ok {
		last = c.prev[base]
	}
	return last, !last.IsZero()
}

// lotwUsersMsg carries the result of a batch LoTW lookup for the index:
// the last upload of each of calls that is a LoTW user.
type lotwUsersMsg struct {
	gen   int
	calls []string
	last  map[string]time.Time
	err   error
}

// lotwUsersCmd returns a command that looks up the base callsigns calls in
// l in one batch.
func lotwUsersCmd(l LoTWLookup, gen int, calls []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		last, err := l.LastUploads(ctx, calls)
		if err != nil {
			err = fmt.Errorf("look up LoTW users: %w", err)
		}
		return lotwUsersMsg{gen: gen, calls: calls, last: last, err: err}
	}
}

// lookupIndexLoTW returns a command looking up, in one batch, the base
// callsigns of rows that are neither looked up nor being looked up. It
// returns nil when the index format has no %L column or no LoTW lookup is
// configured.
func (m *Model) lookupIndexLoTW(rows []logbook.QSO) tea.Cmd {
	if m.lotw == nil || !m.idx.format.Has(indexfmt.FieldLoTW) {
		return nil
	}
	c := &m.idx.lotw
	want := map[string]bool{}
	for _, q := range rows {
		base := callsign.Base(q.Call)
		if _, done := c.last[base]; base != "" && !done && !c.pending[base] {
			want[base] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	calls := slices.Sorted(maps.Keys(want))
	for _, call := range calls {
		c.pending[call] = true
	}
	return lotwUsersCmd(m.lotw, c.gen, calls)
}

// apply caches a batch lookup. A lookup started before the latest sync is
// dropped. A failed lookup is returned to be shown; its callsigns are not
// cached, so they are looked up again when their rows next load.
func (c *indexLoTW) apply(msg lotwUsersMsg) error {
	if msg.gen != c.gen {
		return nil
	}
	for _, call := range msg.calls {
		delete(c.pending, call)
	}
	if msg.err != nil {
		return msg.err
	}
	for _, call := range msg.calls {
		c.last[call] = msg.last[call]
		delete(c.prev, call)
	}
	return nil
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
	if style, ok := lotwStyle(st, lotw.RecencyOf(s.last, s.now)); ok {
		return style.Render("[L]")
	}
	return ""
}

// lotwStyle returns the style the pager's [L] badge and an index %L column
// show a LoTW user in, by how recently it uploaded, and false for Unknown.
func lotwStyle(st styles, r lotw.Recency) (lipgloss.Style, bool) {
	switch r {
	case lotw.Recent:
		return st.lotwRecent, true
	case lotw.Active:
		return st.lotwActive, true
	case lotw.Inactive:
		return st.lotwInactive, true
	default:
		return lipgloss.Style{}, false
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
