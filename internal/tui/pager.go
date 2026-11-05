package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
)

// pagerIndexLines is the number of index rows kept above the pager, as
// NeoMutt's pager_index_lines.
const pagerIndexLines = 5

// pagerChrome is the number of body lines the pager uses besides the index
// strip and the tab: the tab bar and the index status line.
const pagerChrome = 2

// pagerEmpty stands in for an empty field, so the field keeps its row.
const pagerEmpty = "—"

// pager shows the details of the QSO under the index cursor, below a strip
// of index rows, in the style of NeoMutt's pager.
type pager struct {
	open bool
}

// pagerKeyMap holds the keybindings handled while the pager is open.
type pagerKeyMap struct {
	Next  key.Binding
	Prev  key.Binding
	Close key.Binding
	// Quit replaces the global quit binding, since q closes the pager.
	Quit key.Binding
}

func defaultPagerKeys() pagerKeyMap {
	return pagerKeyMap{
		Next: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next QSO"),
		),
		Prev: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev QSO"),
		),
		Close: key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q/esc", "close"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

func (k pagerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.Close}
}

func (k pagerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Next, k.Prev, k.Close}}
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

// openPager opens the pager on the QSO under the index cursor. It does
// nothing until the index has a QSO to show.
func (m Model) openPager() Model {
	if m.idx.loaded && m.idx.total > 0 {
		m.pager.open = true
	}
	return m
}

// updatePager handles a key press in the pager. Model.layout fits the index
// to the strip or the whole body again, and loads the rows the cursor moves
// to.
func (m Model) updatePager(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.pagerKeys.Close):
		m.pager.open = false
	case key.Matches(msg, m.pagerKeys.Next):
		m.idx.next()
	case key.Matches(msg, m.pagerKeys.Prev):
		m.idx.prev()
	}
	return m, nil
}

// pagerStripHeight is the number of index rows shown above a pager in a
// body height lines high: pagerIndexLines, but no more than half the lines
// the chrome leaves, so the QSO stays in view in a short window.
func pagerStripHeight(height int) int {
	return min(pagerIndexLines, max(height-pagerChrome, 0)/2)
}

// pagerView renders the pager in width by height cells: the index strip,
// with the cursor's row highlighted, the tab bar, the QSO under the cursor
// and the index status line.
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
	lines = append(lines, tabBar(st, "QSO", width))
	if q, ok := x.current(); ok {
		lines = append(lines, strings.Split(qsoTab(st, q, x.unit, width), "\n")...)
	} else {
		lines = append(lines, st.muted.Render("Loading…"))
	}
	// Keep the status line at the bottom of the body.
	lines = lines[:min(len(lines), max(height-1, 0))]
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, x.statusView(st, width))
	return strings.Join(lines[:min(len(lines), max(height, 1))], "\n")
}

// tabBar renders the pager's tab bar with the open tab's name, ruled to
// width cells.
func tabBar(st styles, name string, width int) string {
	tab := st.tab.Render(truncate(" "+name+" ", width, ""))
	rule := strings.Repeat("─", max(width-lipgloss.Width(tab), 0))
	return tab + st.muted.Render(rule)
}

// qsoTab renders the QSO tab: one labeled row per field, each fitted to
// width cells. An empty field shows pagerEmpty, so every QSO has the same
// rows. Distances are shown in unit; any unit other than Mi shows
// kilometers.
func qsoTab(st styles, q logbook.QSO, unit indexfmt.Unit, width int) string {
	mode := q.Mode
	if q.Submode != "" {
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

	fields := []struct{ label, value string }{
		{"Date/time", date},
		{"Call", q.Call},
		{"Band", q.Band},
		{"Frequency", freq},
		{"Mode", mode},
		{"RST sent", q.RSTSent},
		{"RST rcvd", q.RSTRcvd},
		{"Grid", q.Gridsquare},
		{"Distance", dist},
		{"State", q.State},
		{"County", q.Cnty},
		{"Name", q.Name},
		{"Comment", q.Comment},
		{"Country", q.Country},
		{"Continent", q.Cont},
	}
	lw := 0
	for _, f := range fields {
		lw = max(lw, lipgloss.Width(f.label))
	}
	lines := make([]string, len(fields))
	for i, f := range fields {
		label := st.label.Render(pad(truncate(f.label, width, ""), min(lw, width)))
		room := max(width-lw-2, 0)
		value := st.muted.Render(truncate(pagerEmpty, room, ""))
		if f.value != "" {
			value = truncate(f.value, room, "…")
		}
		if room == 0 {
			lines[i] = label
			continue
		}
		lines[i] = label + "  " + value
	}
	return strings.Join(lines, "\n")
}
