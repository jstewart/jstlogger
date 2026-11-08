package tui

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// Get returns the stored QSO with the given ID, as the pager fetches it.
func (s *fakeIndexStore) Get(_ context.Context, id int64) (logbook.QSO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.IndexFunc(s.qsos, func(q logbook.QSO) bool { return q.ID == id }); i >= 0 {
		return s.qsos[i], nil
	}
	return logbook.QSO{}, logbook.ErrNotFound
}

// failingGetStore fails every Get with err.
type failingGetStore struct {
	*fakeIndexStore
	err error
}

func (s failingGetStore) Get(_ context.Context, _ int64) (logbook.QSO, error) {
	return logbook.QSO{}, s.err
}

func shiftTab() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift} }
func tab() tea.KeyPressMsg      { return special(tea.KeyTab) }

// runPager runs cmd as runIndex does, also feeding the pager's QSO fetches
// back into m.
func runPager(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case indexLoadedMsg, indexBlockMsg, indexLimitMsg, indexMarksMsg, indexSearchMsg, indexBulkMsg, pagerQSOMsg, lotwLookupMsg:
			var next tea.Cmd
			m, next = update(t, m, msg)
			queue = append(queue, next)
		}
	}
	return m
}

// pressPager sends each key to m and runs the commands that follow,
// including the pager's QSO fetches.
func pressPager(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, k)
		m = runPager(t, m, cmd)
	}
	return m
}

// tabBarLine returns the pager's tab bar, the body line below the strip.
func (m Model) tabBarLine(t *testing.T) string {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	require.Greater(t, len(lines), 1+m.idx.height)
	return lines[1+m.idx.height]
}

// stationQSO is a QSO with the Station and QSL tab details set.
func stationQSO() logbook.QSO {
	q := fullQSO()
	q.StationCallsign = "W1AW"
	q.Operator = "K1ABC"
	q.MyGridsquare = "FN31pr"
	q.TxPwr = sql.Null[float64]{V: 100, Valid: true}
	q.Fields = map[string]string{
		"MY_RIG":     "IC-7300",
		"MY_ANTENNA": "Dipole",
		"MY_CITY":    "Newington",
		"APP_X":      "other",
	}
	return q
}

func TestPagerTabCycling(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want pagerTab
	}{
		{"tab", []tea.KeyPressMsg{tab()}, tabStation},
		{"tab tab", []tea.KeyPressMsg{tab(), tab()}, tabQSL},
		{"tab wraps", []tea.KeyPressMsg{tab(), tab(), tab(), tab()}, tabQSO},
		{"shift+tab wraps", []tea.KeyPressMsg{shiftTab()}, tabADIF},
		{"shift+tab back", []tea.KeyPressMsg{tab(), tab(), shiftTab()}, tabStation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 5, 80, 24)
			m = press(t, m, enter())
			m = press(t, m, tt.keys...)
			assert.True(t, m.pager.open)
			assert.Equal(t, tt.want, m.pager.tab)
		})
	}
}

func TestPagerTabPersistsAcrossQSOs(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	m = pressPager(t, m, enter(), tab(), tab(), tab())
	require.Equal(t, tabADIF, m.pager.tab)

	m = pressPager(t, m, keyPress('j', 0))
	assert.Equal(t, tabADIF, m.pager.tab)
	m = pressPager(t, m, keyPress('k', 0))
	assert.Equal(t, tabADIF, m.pager.tab)
	assert.Contains(t, plain(m.pagerBody(t)), "CALL")
	assert.Contains(t, plain(m.pagerBody(t)), m.currentCall(t))
}

func TestTabBarMarksActiveTab(t *testing.T) {
	st := defaultStyles()
	for tb := range pagerTabCount {
		bar := tabBar(st, tb, 80)
		assert.Equal(t, 80, lipgloss.Width(bar))
		assert.Contains(t, bar, st.tab.Render(" "+tb.String()+" "), "%s is active", tb)
		for other := range pagerTabCount {
			assert.Contains(t, plain(bar), " "+other.String()+" ", "every tab is listed")
			if other != tb {
				assert.NotContains(t, bar, st.tab.Render(" "+other.String()+" "))
			}
		}
	}
	for _, w := range []int{0, 1, 5, 12, 30} {
		assert.LessOrEqual(t, lipgloss.Width(tabBar(st, tabADIF, w)), w, "width %d", w)
	}
}

func TestPagerTabBarFollowsTab(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	m = press(t, m, enter(), tab())
	assert.Contains(t, m.tabBarLine(t), m.styles.tab.Render(" Station "))
}

func TestStationTab(t *testing.T) {
	st := defaultStyles()
	tab := plain(stationTab(st, stationQSO(), "Home QTH", 80))
	lines := strings.Split(tab, "\n")
	for _, want := range []string{
		"Station callsign  W1AW",
		"Operator          K1ABC",
		"Station grid      FN31pr",
		"TX power          100 W",
		"Station name      Home QTH",
	} {
		assert.Contains(t, lines, want)
	}
	assert.NotContains(t, tab, "APP_X", "only MY_ fields are shown")
	antenna := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "MY_ANTENNA") })
	city := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "MY_CITY") })
	rig := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, "MY_RIG") })
	require.Positive(t, antenna)
	assert.Less(t, antenna, city, "MY_ fields are sorted")
	assert.Less(t, city, rig, "MY_ fields are sorted")
	assert.Contains(t, lines[rig], "IC-7300")

	q := stationQSO()
	q.TxPwr = sql.Null[float64]{V: 7.5, Valid: true}
	assert.Contains(t, plain(stationTab(st, q, "", 80)), "7.5 W")

	empty := plain(stationTab(st, logbook.QSO{}, "", 80))
	assert.Contains(t, strings.Split(empty, "\n"), "Station name      "+pagerEmpty)
	assert.Contains(t, strings.Split(empty, "\n"), "TX power          "+pagerEmpty)

	for _, line := range strings.Split(plain(stationTab(st, stationQSO(), "Home QTH", 12)), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 12, "lines fit the width: %q", line)
	}
}

func TestPagerStationName(t *testing.T) {
	var gotCall, gotGrid string
	name := func(call, grid string) string {
		gotCall, gotGrid = call, grid
		return "Portable"
	}
	m, s := newIndexModel(t, 3, 80, 40, WithStationName(name))
	s.qsos[0].StationCallsign, s.qsos[0].MyGridsquare = "W1AW", "FN31"
	m = pressPager(t, m, enter(), tab())

	body := plain(m.pagerBody(t))
	assert.Contains(t, body, "Portable")
	assert.Equal(t, "W1AW", gotCall)
	assert.Equal(t, "FN31", gotGrid)

	m, _ = newIndexModel(t, 3, 80, 40)
	m = pressPager(t, m, enter(), tab())
	assert.Contains(t, plain(m.pagerBody(t)), "Station name      "+pagerEmpty, "no station name without the option")
}

func TestQSLTab(t *testing.T) {
	day := func(d int) sql.Null[time.Time] {
		return sql.Null[time.Time]{V: time.Date(2024, 1, d, 12, 0, 0, 0, time.UTC), Valid: true}
	}
	q := logbook.QSO{Confirmations: []logbook.Confirmation{
		{Service: logbook.ServiceEQSL, Sent: "Y", SentDate: day(3)},
		{Service: "qrz", Sent: "Q"},
		{Service: logbook.ServiceLoTW, Sent: "Y", SentDate: day(2), Rcvd: "V", RcvdDate: day(5)},
		{Service: "clublog", Rcvd: "R"},
		{Service: logbook.ServiceCard, Sent: "R"},
	}}
	tab := plain(qslTab(defaultStyles(), q, 80))
	lines := strings.Split(tab, "\n")

	assert.Contains(t, lines, "LoTW sent         Yes 2024-01-02")
	assert.Contains(t, lines, "LoTW received     Verified 2024-01-05")
	assert.Contains(t, lines, "eQSL sent         Yes 2024-01-03")
	assert.Contains(t, lines, "eQSL received     "+pagerEmpty)
	assert.Contains(t, lines, "Card sent         Requested")
	assert.Contains(t, lines, "qrz sent          Queued")
	assert.Contains(t, lines, "clublog received  Requested")

	var order []string
	for _, l := range lines {
		if f := strings.Fields(l); len(f) > 1 && f[1] == "sent" {
			order = append(order, f[0])
		}
	}
	assert.Equal(t, []string{"Card", "LoTW", "eQSL", "clublog", "qrz"}, order)

	none := plain(qslTab(defaultStyles(), logbook.QSO{}, 80))
	assert.Equal(t, "No confirmations", none)

	for _, line := range strings.Split(plain(qslTab(defaultStyles(), q, 15)), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 15, "lines fit the width: %q", line)
	}
}

func TestADIFTab(t *testing.T) {
	q := stationQSO()
	tab := plain(adifTab(defaultStyles(), q, 80))
	lines := strings.Split(tab, "\n")

	var names []string
	for _, l := range lines {
		names = append(names, strings.Fields(l)[0])
	}
	assert.True(t, slices.IsSorted(names), "fields are sorted by name: %v", names)
	assert.Contains(t, names, "CALL")
	assert.Contains(t, names, "MY_RIG", "Fields are included")
	assert.Contains(t, names, "APP_X", "Fields are included")
	callRow := lines[slices.Index(names, "CALL")]
	assert.True(t, strings.HasSuffix(callRow, " JA1ABC"), "%q", callRow)

	for _, line := range strings.Split(plain(adifTab(defaultStyles(), q, 12)), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 12, "lines fit the width: %q", line)
	}
}

func TestPagerLoadsWholeQSO(t *testing.T) {
	m, s := newIndexModel(t, 3, 80, 40)
	// Only the whole QSO has Fields; the cached index row does not.
	s.qsos[0].Fields = map[string]string{"MY_RIG": "IC-7300"}

	// press drops the fetch result, so the QSO stays loading.
	m = press(t, m, enter(), tab())
	body := plain(m.pagerBody(t))
	assert.Contains(t, body, "Loading…")
	assert.NotContains(t, body, "Station callsign")

	m = runPager(t, m, fetchQSOCmd(m.logbook, s.qsos[0].ID))
	body = plain(m.pagerBody(t))
	assert.NotContains(t, body, "Loading…")
	assert.Contains(t, body, "MY_RIG")
	assert.Contains(t, body, "IC-7300")
}

func TestPagerQSOTabNeedsNoFetch(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, enter())
	body := plain(m.pagerBody(t))
	assert.NotContains(t, body, "Loading…")
	assert.Contains(t, body, m.currentCall(t))
}

func TestPagerDropsStaleFetch(t *testing.T) {
	m, s := newIndexModel(t, 3, 80, 40)
	m = press(t, m, enter(), tab(), keyPress('j', 0))
	first, second := s.qsos[0], s.qsos[1]
	require.Equal(t, second.ID, m.mustCurrentID(t))

	first.StationCallsign = "STALE1"
	m, _ = update(t, m, pagerQSOMsg{id: first.ID, qso: first})
	assert.Contains(t, plain(m.pagerBody(t)), "Loading…", "a fetch for another QSO is dropped")
	assert.NotContains(t, plain(m.pagerBody(t)), "STALE1")

	second.StationCallsign = "FRESH2"
	m, _ = update(t, m, pagerQSOMsg{id: second.ID, qso: second})
	assert.Contains(t, plain(m.pagerBody(t)), "FRESH2")
}

func TestPagerDropsFetchAfterClose(t *testing.T) {
	m, s := newIndexModel(t, 3, 80, 40)
	m = press(t, m, enter(), tab(), esc())
	q := s.qsos[0]
	require.Equal(t, q.ID, m.mustCurrentID(t))

	m, _ = update(t, m, pagerQSOMsg{id: q.ID, qso: q})
	assert.NotEqual(t, q.ID, m.pager.fetched, "a fetch landing after close is dropped, so reopening fetches afresh")
}

// mustCurrentID returns the ID of the QSO under the index cursor.
func (m Model) mustCurrentID(t *testing.T) int64 {
	t.Helper()
	q, ok := m.idx.current()
	require.True(t, ok)
	return q.ID
}

func TestPagerShowsFetchError(t *testing.T) {
	m, s := newIndexModel(t, 3, 80, 40)
	m.logbook = failingGetStore{fakeIndexStore: s, err: errors.New("disk on fire")}
	m = pressPager(t, m, enter(), tab())

	body := plain(m.pagerBody(t))
	assert.Contains(t, body, "disk on fire")
	assert.NotContains(t, body, "Loading…")

	m = pressPager(t, m, shiftTab())
	assert.Contains(t, plain(m.pagerBody(t)), m.currentCall(t), "the QSO tab still shows the index row")
}

func TestPagerScrollsADIFTab(t *testing.T) {
	m, s := newIndexModel(t, 3, 80, 14)
	fields := map[string]string{}
	for i := range 30 {
		fields[fmt.Sprintf("APP_F%02d", i)] = fmt.Sprintf("v%02d", i)
	}
	s.qsos[0].Fields = fields
	m = pressPager(t, m, enter(), shiftTab())
	require.Equal(t, tabADIF, m.pager.tab)

	content := func() []string {
		lines := strings.Split(plain(m.pagerBody(t)), "\n")
		// Drop the tab bar and the status line, and the help footer.
		footer := lipgloss.Height(m.footerView())
		lines = lines[1 : len(lines)-footer-1]
		for i, l := range lines {
			lines[i] = strings.TrimSpace(l)
		}
		return lines
	}
	room := len(content())
	require.Positive(t, room)
	assert.True(t, strings.HasPrefix(content()[0], "APP_F00"), "%q", content()[0])

	m = pressPager(t, m, keyPress(' ', 0))
	assert.True(t, strings.HasPrefix(content()[0], fmt.Sprintf("APP_F%02d", room)), "space pages down: %q", content()[0])

	m = pressPager(t, m, keyPress('-', 0))
	assert.True(t, strings.HasPrefix(content()[0], "APP_F00"), "- pages up: %q", content()[0])

	m = pressPager(t, m, special(tea.KeyPgDown), special(tea.KeyPgUp))
	assert.True(t, strings.HasPrefix(content()[0], "APP_F00"), "pgdown/pgup: %q", content()[0])

	m = pressPager(t, m, special(tea.KeyEnd))
	last := content()[room-1]
	assert.True(t, strings.HasPrefix(last, "TIME_ON"), "end shows the last row: %q", last)

	for range 10 {
		m = pressPager(t, m, keyPress(' ', 0))
	}
	assert.Equal(t, last, content()[room-1], "scrolling stops at the bottom")
	m = pressPager(t, m, keyPress('-', 0))
	assert.NotEqual(t, last, content()[room-1], "page up works right after overscrolling")

	m = pressPager(t, m, special(tea.KeyHome))
	assert.True(t, strings.HasPrefix(content()[0], "APP_F00"), "home: %q", content()[0])

	for range 3 {
		m = pressPager(t, m, keyPress('-', 0))
	}
	assert.True(t, strings.HasPrefix(content()[0], "APP_F00"), "scrolling stops at the top")

	m = pressPager(t, m, keyPress(' ', 0), tab())
	assert.Equal(t, 0, m.pager.scroll, "changing tab resets the scroll")
	m = pressPager(t, m, shiftTab(), keyPress(' ', 0), keyPress('j', 0))
	assert.Equal(t, 0, m.pager.scroll, "moving QSO resets the scroll")
}

func TestPagerKeepsScrollAtIndexEnds(t *testing.T) {
	m, s := newIndexModel(t, 1, 80, 14)
	fields := map[string]string{}
	for i := range 30 {
		fields[fmt.Sprintf("APP_F%02d", i)] = fmt.Sprintf("v%02d", i)
	}
	s.qsos[0].Fields = fields
	m = pressPager(t, m, enter(), shiftTab(), keyPress(' ', 0))
	scroll := m.pager.scroll
	require.Positive(t, scroll)

	m = pressPager(t, m, keyPress('j', 0), keyPress('k', 0))
	assert.Equal(t, scroll, m.pager.scroll, "j/k that can't move keep the scroll")
}

func TestPagerHelpListsTabKeys(t *testing.T) {
	m, _ := newIndexModel(t, 5, 120, 24)
	m = press(t, m, enter())
	m.help.ShowAll = true
	footer := plain(m.footerView())
	for _, want := range []string{"tab", "shift+tab", "next tab", "prev tab", "space", "home", "end"} {
		assert.Contains(t, footer, want)
	}
}
