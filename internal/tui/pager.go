package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/callsign"
	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
)

// defaultPagerIndexLines is the number of index rows kept above the pager
// unless WithPagerIndexLines sets another.
const defaultPagerIndexLines = 10

// pagerChrome is the number of body lines the pager uses besides the index
// strip and the tab: the tab bar and the index status line.
const pagerChrome = 2

// pagerEmpty stands in for an empty field, so the field keeps its row.
const pagerEmpty = "—"

// pager shows the details of the QSO under the index cursor, below a strip
// of index rows, in the style of NeoMutt's pager.
type pager struct {
	open bool
	// indexLines is the number of index rows kept above the pager, as
	// NeoMutt's pager_index_lines.
	indexLines int
	// tab is the open tab, kept while moving between QSOs.
	tab pagerTab
	// scroll is the first line of the tab shown. The view clamps it to the
	// tab's length.
	scroll int
	// stationName names our station from its callsign and grid; nil names
	// none.
	stationName func(callsign, grid string) string
	// lookupPrefix reports whether the lookup-prefix is armed: the next
	// key opens a lookup site for the QSO's callsign.
	lookupPrefix bool

	// The index rows lack some fields, so the tabs other than the QSO tab
	// show the whole QSO, fetched from the logbook. fetched is the ID of
	// the QSO that qso, or err, is for; pending is the ID being fetched.
	fetched int64
	pending int64
	qso     logbook.QSO
	err     error
}

// pagerQSOMsg carries the whole QSO, or the error fetching it, for the
// pager.
type pagerQSOMsg struct {
	id  int64
	qso logbook.QSO
	err error
}

// fetchQSOCmd returns a command that fetches the whole QSO with the given
// ID.
func fetchQSOCmd(s logbook.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		q, err := s.Get(ctx, id)
		if err != nil {
			err = fmt.Errorf("load QSO %d: %w", id, err)
		}
		return pagerQSOMsg{id: id, qso: q, err: err}
	}
}

// fetch returns a command fetching the whole QSO under the index cursor,
// unless it has been fetched or is being fetched.
func (m *Model) fetch() tea.Cmd {
	q, ok := m.idx.current()
	if !m.pager.open || m.logbook == nil || !ok || q.ID == m.pager.fetched || q.ID == m.pager.pending {
		return nil
	}
	m.pager.pending = q.ID
	return fetchQSOCmd(m.logbook, q.ID)
}

// applyQSO keeps a fetched QSO for the pager. A QSO the cursor has moved
// off, or one landing after the pager closed, is dropped.
func (p *pager) applyQSO(msg pagerQSOMsg, current int64) {
	if msg.id == p.pending {
		p.pending = 0
	}
	if !p.open || msg.id != current {
		return
	}
	p.fetched, p.qso, p.err = msg.id, msg.qso, msg.err
}

// WithStationName names our station on the pager's Station tab with name,
// called with the QSO's station callsign and grid.
func WithStationName(name func(callsign, grid string) string) Option {
	return func(m *Model) {
		m.pager.stationName = name
	}
}

// WithURLOpener enables the pager's lookup keys, which open the QSO's
// callsign on a lookup site by calling open with the site's URL.
func WithURLOpener(open func(ctx context.Context, url string) error) Option {
	return func(m *Model) {
		m.openURL = open
	}
}

// WithPagerIndexLines keeps n index rows above the pager, or none when n is
// not positive.
func WithPagerIndexLines(n int) Option {
	return func(m *Model) {
		m.pager.indexLines = max(n, 0)
	}
}

// pagerKeyMap holds the keybindings handled while the pager is open.
type pagerKeyMap struct {
	Next     key.Binding
	Prev     key.Binding
	NextTab  key.Binding
	PrevTab  key.Binding
	PageDown key.Binding
	PageUp   key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Close    key.Binding
	// Quit replaces the global quit binding, since q closes the pager.
	Quit key.Binding
	// LookupPrefix arms the lookup-prefix; the lookup key that follows
	// opens the QSO's callsign on its site, and LookupCancel cancels it.
	LookupPrefix key.Binding
	LookupCancel key.Binding
	LookupQRZ    key.Binding
	LookupHamQTH key.Binding
	LookupEQSL   key.Binding
}

// defaultPagerKeys returns the pager's keybindings, with the lookup keys
// enabled only when lookup is true.
func defaultPagerKeys(lookup bool) pagerKeyMap {
	k := pagerKeyMap{
		Next: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next QSO"),
		),
		Prev: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev QSO"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev tab"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("space", "pgdown"),
			key.WithHelp("space/pgdn", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("-", "pgup"),
			key.WithHelp("-/pgup", "page up"),
		),
		Top: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "bottom"),
		),
		Close: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q/esc", "close"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		LookupPrefix: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "lookup-prefix"),
		),
		LookupCancel: key.NewBinding(
			key.WithKeys("esc"),
		),
		LookupQRZ: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("gq", "QRZ"),
		),
		LookupHamQTH: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("gh", "HamQTH"),
		),
		LookupEQSL: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("ge", "eQSL"),
		),
	}
	for _, b := range []*key.Binding{
		&k.LookupPrefix, &k.LookupCancel, &k.LookupQRZ, &k.LookupHamQTH, &k.LookupEQSL,
	} {
		b.SetEnabled(lookup)
	}
	return k
}

func (k pagerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.NextTab, k.Close}
}

func (k pagerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Next, k.Prev, k.Close},
		{k.NextTab, k.PrevTab},
		{k.PageDown, k.PageUp, k.Top, k.Bottom},
		{k.LookupPrefix, k.LookupQRZ, k.LookupHamQTH, k.LookupEQSL},
	}
}

// globalKeys returns the global keybindings in effect: while the pager is
// open, q closes it, so only ctrl+c quits.
func (m Model) globalKeys() keyMap {
	k := m.keys
	if m.pager.open {
		k.Quit = m.pagerKeys.Quit
	}
	return k
}

// openPager opens the pager on the QSO under the index cursor, at the top
// of the tab last open. It does nothing until the index has a QSO to show.
func (m Model) openPager() Model {
	if m.idx.loaded && m.idx.total > 0 {
		m.pager.open = true
		m.pager.scroll = 0
	}
	return m
}

// closePager closes the pager. The fetched QSO is forgotten, so reopening
// shows any change made to it since.
func (m Model) closePager() Model {
	m.pager.open = false
	m.pager.lookupPrefix = false
	m.pager.fetched, m.pager.pending, m.pager.qso, m.pager.err = 0, 0, logbook.QSO{}, nil
	return m
}

// updatePager handles a key press in the pager. Model.layout fits the index
// to the strip or the whole body again, loads the rows the cursor moves to
// and fetches the QSO under it.
func (m Model) updatePager(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k, p := m.pagerKeys, &m.pager
	room := m.pagerRoom(m.bodyHeight())
	bottom := max(len(m.pagerContent(m.bodyWidth()))-room, 0)
	p.scroll = min(p.scroll, bottom)
	cursor := m.idx.cursor
	switch {
	case key.Matches(msg, k.Close):
		return m.closePager(), nil
	case key.Matches(msg, k.Next):
		m.idx.next()
	case key.Matches(msg, k.Prev):
		m.idx.prev()
	case key.Matches(msg, k.NextTab):
		p.tab, p.scroll = p.tab.next(), 0
	case key.Matches(msg, k.PrevTab):
		p.tab, p.scroll = p.tab.prev(), 0
	case key.Matches(msg, k.PageDown):
		p.scroll = min(p.scroll+max(room, 1), bottom)
	case key.Matches(msg, k.PageUp):
		p.scroll = max(p.scroll-max(room, 1), 0)
	case key.Matches(msg, k.Top):
		p.scroll = 0
	case key.Matches(msg, k.Bottom):
		p.scroll = bottom
	case key.Matches(msg, k.LookupPrefix):
		p.lookupPrefix = true
	}
	// Another QSO starts at the top; j or k at the end of the index leaves
	// the scroll alone.
	if m.idx.cursor != cursor {
		p.scroll = 0
	}
	return m, nil
}

// pagerRoom is the number of tab lines shown in a pager height lines high,
// below the index strip and tab bar and above the status line.
func (m Model) pagerRoom(height int) int {
	return max(height-m.idx.height-pagerChrome, 0)
}

// pagerContent returns the lines of the open tab for the QSO under the
// cursor, fitted to width cells.
func (m Model) pagerContent(width int) []string {
	st, p := m.styles, m.pager
	q, ok := m.idx.current()
	if !ok {
		return []string{st.muted.Render("Loading…")}
	}
	if p.tab == tabQSO {
		return strings.Split(qsoTab(st, q, m.idx.unit, width), "\n")
	}
	switch {
	case p.fetched != q.ID:
		return []string{st.muted.Render("Loading…")}
	case p.err != nil:
		return []string{st.err.Render(truncate(p.err.Error(), width, "…"))}
	}
	var tab string
	switch p.tab {
	case tabStation:
		var name string
		if p.stationName != nil {
			name = p.stationName(p.qso.StationCallsign, p.qso.MyGridsquare)
		}
		tab = stationTab(st, p.qso, name, width)
	case tabQSL:
		tab = qslTab(st, p.qso, width)
	case tabADIF:
		tab = adifTab(st, p.qso, width)
	}
	return strings.Split(tab, "\n")
}

// pagerStripHeight is the number of index rows shown above a pager in a
// body height lines high: lines, but no more than half the lines the chrome
// leaves, so a short window still shows the top of the QSO tab. A window too
// short for the whole tab cuts off its last rows.
func pagerStripHeight(lines, height int) int {
	return min(lines, max(height-pagerChrome, 0)/2)
}

// pagerView renders the pager in width by height cells: the index strip,
// with the cursor's row highlighted, the tab bar, the open tab scrolled to
// the pager's scroll and the index status line.
func (m Model) pagerView(width, height int) string {
	st, x := m.styles, m.idx
	lines := make([]string, 0, height)
	for i := x.top; i < x.top+min(x.visible(), x.height); i++ {
		lines = append(lines, x.rowView(st, i, width, 0))
	}
	// Keep the tab bar in place when there are fewer QSOs than strip rows.
	for len(lines) < x.height {
		lines = append(lines, "")
	}
	lines = append(lines, tabBar(st, m.pager.tab, width))
	content := m.pagerContent(width)
	room := m.pagerRoom(height)
	scroll := min(m.pager.scroll, max(len(content)-room, 0))
	lines = append(lines, content[scroll:]...)
	// Keep the status line at the bottom of the body, cutting off what
	// doesn't fit above it.
	lines = lines[:min(len(lines), max(height-1, 0))]
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	var armed []string
	if m.pager.lookupPrefix {
		armed = append(armed, "lookup-prefix")
	}
	return strings.Join(append(lines, x.statusView(st, width, armed...)), "\n")
}

// qsoTab renders the QSO tab: one labeled row per field, each fitted to
// width cells. An empty field shows pagerEmpty, so every QSO has the same
// rows. Distances are shown in unit; any unit other than Mi shows
// kilometers.
func qsoTab(st styles, q logbook.QSO, unit indexfmt.Unit, width int) string {
	mode := q.Mode
	switch {
	case q.Submode == "":
	case mode == "":
		mode = q.Submode
	default:
		mode += " (" + q.Submode + ")"
	}
	var freq, dist string
	if q.FreqHz.Valid {
		freq = logbook.FormatFreqMHz(q.FreqHz.V) + " MHz"
	}
	if q.DistanceKm.Valid {
		if unit != indexfmt.Mi {
			unit = indexfmt.Km
		}
		dist = indexfmt.FormatDistance(q.DistanceKm.V, unit) + " " + string(unit)
	}
	var date string
	if !q.StartUTC.IsZero() {
		date = q.StartUTC.UTC().Format("2006-01-02 15:04:05") + " UTC"
	}

	return pagerRows(st, []pagerRow{
		{label: "Date/time", value: date},
		{label: "Call", value: q.Call, suffix: lookupBadges(st, q.Call)},
		{label: "Band", value: q.Band},
		{label: "Frequency", value: freq},
		{label: "Mode", value: mode},
		{label: "RST sent", value: q.RSTSent},
		{label: "RST rcvd", value: q.RSTRcvd},
		{label: "Grid", value: q.Gridsquare},
		{label: "Distance", value: dist},
		{label: "State", value: q.State},
		{label: "County", value: q.Cnty},
		{label: "Name", value: q.Name},
		{label: "Comment", value: q.Comment},
		{label: "Country", value: q.Country},
		{label: "Continent", value: q.Cont},
	}, width)
}

// lookupBadges renders the QRZ, HamQTH and eQSL badges for call, each a
// hyperlink to the call's page on that site, or "" when call has no base
// callsign to look up.
func lookupBadges(st styles, call string) string {
	if callsign.Base(call) == "" {
		return ""
	}
	return strings.Join([]string{
		st.qrz.Hyperlink(callsign.QRZURL(call)).Render("[Q]"),
		st.hamQTH.Hyperlink(callsign.HamQTHURL(call)).Render("[H]"),
		st.eQSL.Hyperlink(callsign.EQSLURL(call)).Render("[E]"),
	}, " ")
}

// lookupTimeout bounds each call to the URL opener.
const lookupTimeout = 10 * time.Second

// msgNoLookupCall is the status message for a lookup on a QSO whose
// callsign has no base callsign to look up.
const msgNoLookupCall = "No callsign to look up."

// lookupResultMsg reports the outcome of opening a lookup site.
type lookupResultMsg struct {
	err error
}

// notLookupMsg is the status message for a key, such as "x", that does not
// follow the lookup-prefix.
func notLookupMsg(k string) string {
	return "g" + k + " is not a lookup."
}

// openURLCmd returns a command that opens url with open, with a timeout,
// and reports the result as a lookupResultMsg. desc describes the page in
// the error.
func openURLCmd(open func(ctx context.Context, url string) error, desc, url string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
		defer cancel()
		if err := open(ctx, url); err != nil {
			return lookupResultMsg{err: fmt.Errorf("open %s: %w", desc, err)}
		}
		return lookupResultMsg{}
	}
}

// updateLookup handles the key press that follows the lookup-prefix. Quit
// still quits and esc cancels the prefix silently. A lookup key opens the
// callsign of the QSO under the cursor on its site; any other key cancels
// the prefix without running its own function.
func (m Model) updateLookup(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := m.pagerKeys
	if key.Matches(msg, k.Quit) {
		return m, tea.Quit
	}
	m.pager.lookupPrefix = false
	var desc string
	var lookupURL func(call string) string
	switch {
	case key.Matches(msg, k.LookupCancel):
		return m, nil
	case key.Matches(msg, k.LookupQRZ):
		desc, lookupURL = "QRZ page", callsign.QRZURL
	case key.Matches(msg, k.LookupHamQTH):
		desc, lookupURL = "HamQTH page", callsign.HamQTHURL
	case key.Matches(msg, k.LookupEQSL):
		desc, lookupURL = "eQSL page", callsign.EQSLURL
	default:
		m.idx.msg = notLookupMsg(msg.String())
		return m, nil
	}
	q, ok := m.idx.current()
	if !ok {
		return m, nil
	}
	url := lookupURL(q.Call)
	if url == "" {
		m.idx.msg = msgNoLookupCall
		return m, nil
	}
	return m, openURLCmd(m.openURL, desc, url)
}
