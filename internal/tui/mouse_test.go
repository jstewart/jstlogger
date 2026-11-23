package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

// screenLine returns line y of the screen, unstyled.
func (m Model) screenLine(t *testing.T, y int) string {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	require.Greater(t, len(lines), y)
	return plain(lines[y])
}

// clickOn returns a left click on the first cell of text where it is
// drawn on line y of the screen.
func clickOn(t *testing.T, m Model, y int, text string) tea.MouseClickMsg {
	t.Helper()
	x := strings.Index(m.screenLine(t, y), text)
	require.GreaterOrEqual(t, x, 0, "%q is on line %d: %q", text, y, m.screenLine(t, y))
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func wheelDown() tea.MouseWheelMsg { return tea.MouseWheelMsg{Button: tea.MouseWheelDown} }
func wheelUp() tea.MouseWheelMsg   { return tea.MouseWheelMsg{Button: tea.MouseWheelUp} }

// send sends each message to m and runs the commands that follow.
func send(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = update(t, m, msg)
		m = runPager(t, m, cmd)
	}
	return m
}

// pagerTabBarY is the screen line of the pager's tab bar, below the header
// and the index strip.
func (m Model) pagerTabBarY() int {
	return 1 + m.idx.height
}

func TestPagerClickOpensTab(t *testing.T) {
	for tb := range pagerTabCount {
		t.Run(tb.String(), func(t *testing.T) {
			m, _ := newIndexModel(t, 5, 80, 24)
			m = pressPager(t, m, enter())
			// Start elsewhere, scrolled, so the click visibly moves.
			m.pager.tab, m.pager.scroll = tabADIF, 2
			if tb == tabADIF {
				m.pager.tab = tabQSO
			}
			for _, x := range []int{0, len(tb.String()) + 1} {
				click := clickOn(t, m, m.pagerTabBarY(), " "+tb.String()+" ")
				click.X += x
				got := send(t, m, click)
				assert.Equal(t, tb, got.pager.tab, "click %d cells into the tab", x)
				assert.Zero(t, got.pager.scroll, "the tab opens at the top")
			}
		})
	}
}

func TestPagerClickOffTabsDoesNothing(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	m = pressPager(t, m, enter())
	y := m.pagerTabBarY()
	rule := clickOn(t, m, y, "─")
	pad := tea.MouseClickMsg{X: 0, Y: y, Button: tea.MouseLeft}
	right := clickOn(t, m, y, " QSL ")
	right.Button = tea.MouseRight
	tests := []struct {
		name  string
		click tea.MouseClickMsg
	}{
		{"rule after the tabs", rule},
		{"far end of the rule", tea.MouseClickMsg{X: 78, Y: y, Button: tea.MouseLeft}},
		{"body padding", pad},
		{"right button", right},
		{"blank strip row", tea.MouseClickMsg{X: 7, Y: y - 1, Button: tea.MouseLeft}},
		{"open tab", tea.MouseClickMsg{X: 7, Y: y + 1, Button: tea.MouseLeft}},
		{"header", tea.MouseClickMsg{X: 7, Y: 0, Button: tea.MouseLeft}},
		{"footer", tea.MouseClickMsg{X: 7, Y: 23, Button: tea.MouseLeft}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := send(t, m, tt.click)
			assert.Equal(t, tabQSO, got.pager.tab)
			assert.True(t, got.pager.open)
			assert.Equal(t, m.idx.cursor, got.idx.cursor)
		})
	}
}

func TestPagerClickOnCutOffTab(t *testing.T) {
	// A body 16 cells wide fits " QSO  Station " and the first two cells
	// of " QSL ".
	m, _ := newIndexModel(t, 5, 18, 24)
	m = pressPager(t, m, enter())
	y := m.pagerTabBarY()
	require.Equal(t, " QSO  Station  Q", m.screenLine(t, y)[1:17])

	got := send(t, m, clickOn(t, m, y, " QSO "))
	assert.Equal(t, tabQSO, got.pager.tab, "the first cell is QSO's")

	click := clickOn(t, m, y, "Station")
	click.X += len("Station  ")
	got = send(t, m, click)
	assert.Equal(t, tabQSL, got.pager.tab, "the cut-off QSL tab opens from the part shown")
}

func TestPagerTabBarLayoutMatchesClicks(t *testing.T) {
	st := defaultStyles(true)
	names := pagerTabNames[:]
	// Cutting the bar short only drops cells from its end, so each tab
	// starts where it does in a wide bar.
	wide := plain(tabBar(st, names, 0, 200))
	for _, width := range []int{0, 3, 5, 12, 16, 25, 40} {
		bar := plain(tabBar(st, names, 0, width))
		for x := range width + 2 {
			i := tabAt(names, width, x)
			if x >= width || x >= len(strings.TrimRight(bar, "─")) {
				assert.Equal(t, -1, i, "width %d, cell %d is off the tabs", width, x)
				continue
			}
			require.GreaterOrEqual(t, i, 0, "width %d, cell %d", width, x)
			// The cell lies within the drawn label of tab i.
			start := strings.Index(wide, " "+names[i]+" ")
			assert.GreaterOrEqual(t, x, start, "width %d, cell %d", width, x)
			assert.Less(t, x, start+len(names[i])+2, "width %d, cell %d", width, x)
		}
	}
	assert.Equal(t, -1, tabAt(names, 40, -1))
}

func TestPagerWheelScrollsTab(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 12)
	m = pressPager(t, m, enter())
	bottom := m.pagerBottom()
	require.Greater(t, bottom, wheelLines, "the QSO tab is taller than the pager")
	cursor := m.idx.cursor

	m = send(t, m, wheelDown())
	assert.Equal(t, wheelLines, m.pager.scroll)
	assert.Equal(t, cursor, m.idx.cursor, "the wheel scrolls the tab, not the index")
	m = send(t, m, wheelUp())
	assert.Zero(t, m.pager.scroll)
	m = send(t, m, wheelUp())
	assert.Zero(t, m.pager.scroll, "the wheel stops at the top")

	for range bottom {
		m = send(t, m, wheelDown())
	}
	assert.Equal(t, bottom, m.pager.scroll, "the wheel stops at the bottom")
	m = send(t, m, wheelUp())
	assert.Equal(t, bottom-wheelLines, m.pager.scroll)
}

func TestPagerWheelDoesNothingOnShortTab(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 40)
	m = pressPager(t, m, enter())
	m = send(t, m, wheelDown())
	assert.Zero(t, m.pager.scroll)
}

// entryTabBarY is the screen line of the New QSO screen's tab bar, below
// the header.
const entryTabBarY = 1

func TestEntryClickOpensTab(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.PropMode: "sat"})
	m = press(t, m, altKey('1'))
	require.Contains(t, tabNames(m), "Satellite")
	for _, tb := range m.entry.tabs() {
		t.Run(entryTabNames[tb], func(t *testing.T) {
			from := m
			if tb == entryTabQSO {
				from = press(t, m, altKey('2'))
			}
			got := send(t, from, clickOn(t, from, entryTabBarY, " "+entryTabNames[tb]+" "))
			assert.Equal(t, tb, got.entry.tab)
			assert.Equal(t, entryTabOf(got.entry.focus), tb, "the tab's first field has focus")
		})
	}
}

func TestEntryClickOffTabsDoesNothing(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	right := clickOn(t, m, entryTabBarY, " Station ")
	right.Button = tea.MouseRight
	tests := []struct {
		name  string
		click tea.MouseClickMsg
	}{
		{"rule after the tabs", clickOn(t, m, entryTabBarY, "─")},
		{"body padding", tea.MouseClickMsg{X: 0, Y: entryTabBarY, Button: tea.MouseLeft}},
		{"right button", right},
		{"form row", clickOn(t, m, entryTabBarY+3, "Call")},
		{"header", tea.MouseClickMsg{X: 7, Y: 0, Button: tea.MouseLeft}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := send(t, m, tt.click)
			assert.Equal(t, entryTabQSO, got.entry.tab)
			assert.Equal(t, m.entry.focus, got.entry.focus)
		})
	}
}

func TestEntryClickOnCutOffTab(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	// A body 20 cells wide fits " QSO  Station " and the first six cells
	// of " General ".
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 22, Height: 30})
	require.Equal(t, "Gener", strings.Fields(m.screenLine(t, entryTabBarY))[2])

	got := send(t, m, clickOn(t, m, entryTabBarY, "Gener"))
	assert.Equal(t, entryTabGeneral, got.entry.tab)
}

func TestEntryClickWaitsForSave(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m.entry.saving = true
	got := send(t, m, clickOn(t, m, entryTabBarY, " Station "))
	assert.Equal(t, entryTabQSO, got.entry.tab)
}

func TestMouseIgnoredWhilePromptOpen(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "W1AW"})
	m = press(t, m, esc())
	require.True(t, m.promptOpen(), "closing with changes asks first")
	got := send(t, m, clickOn(t, m, entryTabBarY, " Station "))
	assert.Equal(t, entryTabQSO, got.entry.tab)
	assert.True(t, got.promptOpen())
}

func TestEntryWheelScrollsForm(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 10})
	require.Equal(t, qsoentry.Date, m.entry.focus)
	first := func(m Model) string { return m.screenLine(t, entryTabBarY+1) }
	require.Contains(t, first(m), "Date")

	m = send(t, m, wheelDown())
	assert.Contains(t, first(m), "Mode", "the wheel scrolls three rows down")
	assert.Equal(t, qsoentry.Date, m.entry.focus, "focus stays put")
	m = send(t, m, wheelUp(), wheelUp())
	assert.Contains(t, first(m), "Date", "the wheel stops at the top")

	for range 50 {
		m = send(t, m, wheelDown())
	}
	body := strings.Split(m.entryView(m.bodyWidth(), m.bodyHeight()), "\n")
	all, _ := m.entryBody(m.bodyWidth())
	assert.Equal(t, plain(all[len(all)-1]), plain(body[len(body)-2]), "the wheel stops at the last line")

	m = press(t, m, tab())
	assert.Equal(t, qsoentry.TimeOn, m.entry.focus)
	assert.Contains(t, first(m), "Date", "a key press scrolls back to the focused field")
}

func TestMouseReleaseAndMotionDoNothing(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Notes)
	for _, msg := range []tea.Msg{
		tea.MouseReleaseMsg{X: 7, Y: 3, Button: tea.MouseLeft},
		tea.MouseMotionMsg{X: 8, Y: 4, Button: tea.MouseLeft},
	} {
		got, cmd := m.update(msg)
		assert.Nil(t, cmd, "%T", msg)
		assert.Equal(t, m.entry.form(), got.entry.form(), "%T", msg)
		assert.Equal(t, m.entry.focus, got.entry.focus, "%T", msg)
	}
}

func TestEntryWheelDoesNothingOnShortForm(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = press(t, m, altKey('2'))
	before := m.View().Content
	m = send(t, m, wheelDown())
	assert.Equal(t, before, m.View().Content)
}

func TestMouseMode(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	assert.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode, "index")
	m = pressPager(t, m, enter())
	assert.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode, "pager")
	m = pressPager(t, m, keyPress('q', 0))
	assert.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode, "index after the pager")

	e := newEntryModel(t, newEntryStore(t, nil))
	assert.Equal(t, tea.MouseModeCellMotion, e.View().MouseMode, "New QSO")
	e = press(t, e, esc())
	require.False(t, e.entry.open)
	assert.Equal(t, tea.MouseModeCellMotion, e.View().MouseMode, "index after New QSO")

	assert.Equal(t, tea.MouseModeNone, New().View().MouseMode, "no logbook")
}

// indexRowY is the screen line of row i of the index, counting from the
// first visible row, below the header, the column header and its rule.
func indexRowY(i int) int {
	return 3 + i
}

// stripRowY is the screen line of row i of the index strip above the
// pager, below the header.
func stripRowY(i int) int {
	return 1 + i
}

func TestIndexWheelMovesCursor(t *testing.T) {
	m, _ := newIndexModel(t, 50, 80, 24)
	require.Greater(t, 50, m.idx.height, "the index is taller than the window")

	m = send(t, m, wheelDown())
	assert.Equal(t, wheelLines, m.idx.cursor)
	assert.Zero(t, m.idx.top, "the cursor is still on screen")
	m = send(t, m, wheelUp())
	assert.Zero(t, m.idx.cursor)
	m = send(t, m, wheelUp())
	assert.Zero(t, m.idx.cursor, "the wheel stops at the first QSO")
	assert.Empty(t, m.idx.msg, "silently")

	for range 50 {
		m = send(t, m, wheelDown())
	}
	assert.Equal(t, 49, m.idx.cursor, "the wheel stops at the last QSO")
	assert.Equal(t, m.idx.maxTop(), m.idx.top, "the view follows the cursor")
	assert.Empty(t, m.idx.msg, "silently")
	assert.False(t, m.pager.open)

	top := m.idx.top
	for range 10 {
		m = send(t, m, wheelUp())
	}
	assert.Equal(t, 49-10*wheelLines, m.idx.cursor)
	assert.Less(t, m.idx.top, top, "the view follows the cursor up")
}

func TestIndexWheelLoadsRows(t *testing.T) {
	n := 2 * indexBlockSize
	m, s := newIndexModel(t, n, 80, 24)
	require.Equal(t, []int{0}, s.loadedOffsets())

	// Wheel until the rows near the view reach into the next block,
	// giving up should the wheel not move the view.
	for i := 0; i < n && m.idx.top+m.idx.height+indexPrefetch < indexBlockSize+wheelLines; i++ {
		m = send(t, m, wheelDown())
	}
	assert.Contains(t, s.loadedOffsets(), indexBlockSize, "the next block loads")
	_, ok := m.idx.row(indexBlockSize)
	assert.True(t, ok, "the next block's rows are cached")
	_, ok = m.idx.current()
	assert.True(t, ok)
}

func TestIndexClickOpensQSO(t *testing.T) {
	tests := []struct {
		name   string
		wheels int
		row    int
	}{
		{"first row", 0, 0},
		{"third row", 0, 2},
		{"last row", 0, -1},
		{"scrolled", 15, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 50, 80, 24)
			for range tt.wheels {
				m = send(t, m, wheelDown())
			}
			if tt.wheels > 0 {
				require.Positive(t, m.idx.top, "the view has scrolled")
			}
			row := tt.row
			if row < 0 {
				row = m.idx.visible() - 1
			}
			want := m.idx.top + row
			q, ok := m.idx.row(want)
			require.True(t, ok)
			click := clickOn(t, m, indexRowY(row), q.Call+" ")

			got := send(t, m, click)
			assert.True(t, got.pager.open)
			assert.Equal(t, want, got.idx.cursor)
			assert.Zero(t, got.pager.scroll)
			moved := m
			moved.idx.moveTo(want)
			byKey := pressPager(t, moved, enter())
			assert.Equal(t, byKey.pager.qso, got.pager.qso, "as moving to the row and pressing enter")
			assert.Equal(t, q, got.pager.qso, "the pager shows the clicked QSO")
		})
	}
}

func TestPagerClickOnStripMovesCursor(t *testing.T) {
	m, _ := newIndexModel(t, 50, 80, 24)
	m = pressPager(t, m, enter())
	require.Greater(t, m.idx.height, 2)
	m.pager.scroll = 2

	got := send(t, m, tea.MouseClickMsg{X: 7, Y: stripRowY(2), Button: tea.MouseLeft})
	assert.Equal(t, m.idx.top+2, got.idx.cursor)
	assert.True(t, got.pager.open)
	assert.Zero(t, got.pager.scroll, "another QSO starts at the top")
	assert.Equal(t, tabQSO, got.pager.tab)

	got.pager.scroll = 2
	again := send(t, got, tea.MouseClickMsg{X: 7, Y: stripRowY(2), Button: tea.MouseLeft})
	assert.Equal(t, got.idx.cursor, again.idx.cursor)
	assert.Equal(t, 2, again.pager.scroll, "the cursor's own row keeps the scroll")
	assert.True(t, again.pager.open)
}

func TestIndexClickDoesNothing(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	m = send(t, m, wheelDown())
	require.Equal(t, wheelLines, m.idx.cursor)
	// The status line is the body's last line, below the 1-line header.
	status := m.bodyHeight()
	require.Contains(t, m.screenLine(t, status), "QSO 4/5")
	row := tea.MouseClickMsg{X: 7, Y: indexRowY(1), Button: tea.MouseLeft}
	right := row
	right.Button = tea.MouseRight
	pad := row
	pad.X = 0
	tests := []struct {
		name  string
		setup func(Model) Model
		click tea.MouseClickMsg
	}{
		{"column header", nil, tea.MouseClickMsg{X: 7, Y: 1, Button: tea.MouseLeft}},
		{"rule", nil, tea.MouseClickMsg{X: 7, Y: 2, Button: tea.MouseLeft}},
		{"empty row", nil, tea.MouseClickMsg{X: 7, Y: indexRowY(5), Button: tea.MouseLeft}},
		{"status line", nil, tea.MouseClickMsg{X: 7, Y: status, Button: tea.MouseLeft}},
		{"header", nil, tea.MouseClickMsg{X: 7, Y: 0, Button: tea.MouseLeft}},
		{"body padding", nil, pad},
		{"right button", nil, right},
		{"prompt open", func(m Model) Model {
			m = press(t, m, keyPress('/', 0))
			require.True(t, m.promptOpen())
			return m
		}, row},
		{"mark-prefix armed", func(m Model) Model {
			m.idx.prefix = true
			return m
		}, row},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := m
			if tt.setup != nil {
				from = tt.setup(m)
			}
			got := send(t, from, tt.click)
			assert.False(t, got.pager.open)
			assert.Equal(t, wheelLines, got.idx.cursor)
		})
	}
}

func TestIndexClickOnEmptyIndexDoesNothing(t *testing.T) {
	m, _ := newIndexModel(t, 0, 80, 24)
	got := send(t, m, tea.MouseClickMsg{X: 7, Y: indexRowY(0), Button: tea.MouseLeft})
	assert.False(t, got.pager.open)
	got = send(t, m, wheelDown())
	assert.Zero(t, got.idx.cursor)
}

func TestPagerClickWhilePrefixArmedDoesNothing(t *testing.T) {
	m, _ := newIndexModel(t, 50, 80, 30)
	m = pressPager(t, m, enter())
	tests := []struct {
		name string
		arm  func(*Model)
	}{
		{"mark-prefix", func(m *Model) { m.idx.prefix = true }},
		{"lookup-prefix", func(m *Model) { m.pager.lookupPrefix = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := m
			tt.arm(&from)
			got := send(t, from, tea.MouseClickMsg{X: 7, Y: stripRowY(2), Button: tea.MouseLeft})
			assert.Equal(t, m.idx.cursor, got.idx.cursor, "strip row")
			got = send(t, from, clickOn(t, from, from.pagerTabBarY(), " QSL "))
			assert.Equal(t, tabQSO, got.pager.tab, "tab")
		})
	}
}
