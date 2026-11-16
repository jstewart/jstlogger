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
		{"index strip", tea.MouseClickMsg{X: 7, Y: y - 1, Button: tea.MouseLeft}},
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
	st := defaultStyles()
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

func TestMouseModeOnlyOnPagerAndEntry(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	assert.Equal(t, tea.MouseModeNone, m.View().MouseMode, "index")
	m = pressPager(t, m, enter())
	assert.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode, "pager")
	m = pressPager(t, m, keyPress('q', 0))
	assert.Equal(t, tea.MouseModeNone, m.View().MouseMode, "index after the pager")

	e := newEntryModel(t, newEntryStore(t, nil))
	assert.Equal(t, tea.MouseModeCellMotion, e.View().MouseMode, "New QSO")
	e = press(t, e, esc())
	require.False(t, e.entry.open)
	assert.Equal(t, tea.MouseModeNone, e.View().MouseMode, "index after New QSO")

	assert.Equal(t, tea.MouseModeNone, New().View().MouseMode, "no logbook")
}
