package tui

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
)

func enter() tea.KeyPressMsg { return special(tea.KeyEnter) }
func esc() tea.KeyPressMsg   { return special(tea.KeyEscape) }

// fullQSO is a QSO with every field the pager's QSO tab shows set.
func fullQSO() logbook.QSO {
	return logbook.QSO{
		ID:         1,
		StartUTC:   time.Date(2026, 10, 31, 13, 34, 56, 0, time.FixedZone("EST", -5*3600)),
		Call:       "JA1ABC",
		Band:       "20m",
		FreqHz:     sql.Null[int64]{V: 14074000, Valid: true},
		Mode:       "MFSK",
		Submode:    "FT4",
		RSTSent:    "-10",
		RSTRcvd:    "+02",
		Gridsquare: "PM95",
		DistanceKm: sql.Null[float64]{V: 10_870.4, Valid: true},
		State:      "MA",
		Cnty:       "MA,MIDDLESEX",
		Name:       "Taro",
		Comment:    "Nice signal",
		Country:    "Japan",
		Cont:       "AS",
	}
}

func TestPagerOpensOnEnter(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	m = press(t, m, keyPress('j', 0), keyPress('j', 0))
	require.Equal(t, 2, m.idx.cursor)

	m = press(t, m, enter())

	require.True(t, m.pager.open)
	view := plain(m.View().Content)
	assert.Contains(t, view, "Call")
	assert.Contains(t, view, "K8A", "the pager shows the QSO under the cursor")
	assert.NotContains(t, view, "Date       Time  Call", "the index header is hidden")
	assert.Equal(t, "QSO 3/10", statusLine(t, m))

	v := m.View()
	assert.Equal(t, 24, lipgloss.Height(v.Content), "the pager fills the window")
	assert.LessOrEqual(t, lipgloss.Width(v.Content), 80)
}

func TestPagerKeepsIndexLinesAboveIt(t *testing.T) {
	m, _ := newIndexModel(t, 20, 80, 40)
	m = press(t, m, keyPress('z', 0), enter())

	lines := strings.Split(plain(m.View().Content), "\n")
	// The header is line 0; the index strip follows it.
	strip := lines[1 : 1+defaultPagerIndexLines]
	var cursor []int
	for i, l := range strip {
		if strings.Contains(l, " "+m.currentCall(t)+" ") {
			cursor = append(cursor, i)
		}
	}
	require.Len(t, cursor, 1, "the current QSO is in the strip: %q", strip)
	assert.Equal(t, defaultPagerIndexLines, m.idx.height, "the index is cut to the strip")

	raw := strings.Split(m.View().Content, "\n")
	assert.Contains(t, raw[1+cursor[0]], "\x1b[7m", "the strip highlights the cursor")
}

func TestPagerIndexLinesOption(t *testing.T) {
	m, _ := newIndexModel(t, 20, 80, 40, WithPagerIndexLines(3))
	m = press(t, m, enter())

	assert.Equal(t, 3, m.idx.height, "the index is cut to the configured strip")
	lines := strings.Split(plain(m.View().Content), "\n")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[1+3]), "QSO  Station  QSL  ADIF ─"), "the tab bar follows the strip: %q", lines[1+3])
}

func TestPagerIndexLinesShrinkInShortWindows(t *testing.T) {
	m, _ := newIndexModel(t, 40, 80, 24, WithPagerIndexLines(30))
	m = press(t, m, enter())

	assert.Less(t, m.idx.height, 30, "the strip shrinks to fit the window")
	assert.Positive(t, m.idx.height)
	body := plain(m.pagerBody(t))
	assert.Contains(t, body, " QSO ", "the tab bar is visible")
	assert.Contains(t, body, "Date/time", "the top of the QSO tab is visible")
}

func TestPagerStripHeight(t *testing.T) {
	tests := []struct {
		name          string
		lines, height int
		want          int
	}{
		{"room for every line", 10, 40, 10},
		{"no more than half the room", 10, 20, 9},
		{"zero lines", 0, 40, 0},
		{"no room", 10, 2, 0},
		{"shorter than the chrome", 10, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pagerStripHeight(tt.lines, tt.height))
		})
	}
}

func TestWithPagerIndexLinesNegative(t *testing.T) {
	m := New(WithPagerIndexLines(-3))
	assert.Equal(t, 0, m.pager.indexLines, "a negative count shows no strip")
}

// currentCall returns the callsign of the QSO under the index cursor.
func (m Model) currentCall(t *testing.T) string {
	t.Helper()
	q, ok := m.idx.current()
	require.True(t, ok, "the current QSO is not loaded")
	return q.Call
}

func TestPagerCloses(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"q", keyPress('q', 0)},
		{"esc", esc()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 40, 80, 24)
			m = press(t, m, keyPress('z', 0), keyPress('j', 0))
			cursor, height := m.idx.cursor, m.idx.height

			m = press(t, m, enter(), keyPress('j', 0), keyPress('k', 0))
			var cmd tea.Cmd
			m, cmd = update(t, m, tt.key)

			assert.False(t, m.pager.open)
			if cmd != nil {
				_, quit := cmd().(tea.QuitMsg)
				assert.False(t, quit, "%s closes the pager, not the program", tt.name)
			}
			assert.Equal(t, cursor, m.idx.cursor, "the cursor is unchanged")
			assert.Equal(t, height, m.idx.height, "the index fills the body again")
			assert.Contains(t, plain(m.View().Content), "Date       Time  Call")
		})
	}
}

func TestPagerCtrlCQuits(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24)
	m = press(t, m, enter())

	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPagerMovesBetweenQSOs(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want int
	}{
		{"j", []tea.KeyPressMsg{keyPress('j', 0)}, 4},
		{"down", []tea.KeyPressMsg{special(tea.KeyDown)}, 4},
		{"k", []tea.KeyPressMsg{keyPress('k', 0)}, 2},
		{"up", []tea.KeyPressMsg{special(tea.KeyUp)}, 2},
		{"j j k", []tea.KeyPressMsg{keyPress('j', 0), keyPress('j', 0), keyPress('k', 0)}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 10, 80, 24)
			m = press(t, m, keyPress('j', 0), keyPress('j', 0), keyPress('j', 0), enter())
			m = press(t, m, tt.keys...)

			assert.True(t, m.pager.open, "moving stays in the pager")
			assert.Equal(t, tt.want, m.idx.cursor)
			call := m.currentCall(t)
			assert.Contains(t, plain(m.pagerBody(t)), call, "the pager follows the cursor")
		})
	}
}

// pagerBody returns the pager's QSO tab, below the index strip.
func (m Model) pagerBody(t *testing.T) string {
	t.Helper()
	lines := strings.Split(m.View().Content, "\n")
	require.Greater(t, len(lines), 1+m.idx.height)
	return strings.Join(lines[1+m.idx.height:], "\n")
}

func TestPagerStopsAtTheEnds(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, enter(), keyPress('k', 0))
	assert.Equal(t, 0, m.idx.cursor)
	assert.Contains(t, statusLine(t, m), msgFirstQSO)

	m = press(t, m, keyPress('j', 0), keyPress('j', 0), keyPress('j', 0))
	assert.Equal(t, 2, m.idx.cursor)
	assert.Contains(t, statusLine(t, m), msgLastQSO)
}

func TestPagerMovesAcrossABlock(t *testing.T) {
	n := indexBlockSize + indexPrefetch + 50
	m, s := newIndexModel(t, n, 80, 24)
	m = press(t, m, keyPress('*', 0), enter())
	require.Equal(t, n-1, m.idx.cursor)

	// Drop every block but the last, as eviction does far from the cursor.
	for b := range m.idx.blocks {
		if b != (n-1)/indexBlockSize {
			delete(m.idx.blocks, b)
		}
	}
	before := len(s.loadedOffsets())
	for range indexPrefetch + 60 {
		m = press(t, m, keyPress('k', 0))
	}

	want := n - 1 - (indexPrefetch + 60)
	require.Equal(t, want, m.idx.cursor)
	require.Less(t, want, indexBlockSize, "the cursor crossed into the first block")
	assert.Contains(t, s.loadedOffsets()[before:], 0, "the pager loads the block it moves into")
	assert.Contains(t, plain(m.pagerBody(t)), m.currentCall(t))
}

func TestPagerNeedsAQSO(t *testing.T) {
	m, _ := newIndexModel(t, 0, 80, 24)
	m = press(t, m, enter())
	assert.False(t, m.pager.open, "there is no QSO to show")

	m, _ = update(t, New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(t, m, enter())
	assert.False(t, m.pager.open, "there is no logbook")
}

func TestPagerHelp(t *testing.T) {
	m, _ := newIndexModel(t, 5, 120, 24)
	m = press(t, m, enter())

	footer := plain(m.footerView())
	assert.Contains(t, footer, "close")
	assert.Contains(t, footer, "next")
	assert.NotContains(t, footer, "q quit", "q closes the pager")
	assert.NotContains(t, footer, "mark", "the index keys are not active")

	m.help.ShowAll = true
	assert.Contains(t, plain(m.footerView()), "ctrl+c")
}

func TestIndexHelpShowsEnter(t *testing.T) {
	m, _ := newIndexModel(t, 5, 120, 24)
	assert.Contains(t, plain(m.footerView()), "enter")
}

func TestQSOTab(t *testing.T) {
	labels := []string{
		"Date/time", "Call", "Band", "Frequency", "Mode", "RST sent", "RST rcvd",
		"Grid", "Distance", "State", "County", "Name", "Comment", "Country", "Continent",
	}

	full := plain(qsoTab(defaultStyles(), fullQSO(), indexfmt.Km, lotwStatus{}, 80))
	for _, want := range append(labels,
		"2026-10-31 18:34:56 UTC", "JA1ABC", "20m", "14.074 MHz", "MFSK (FT4)",
		"-10", "+02", "PM95", "10870 km", "MA,MIDDLESEX", "Taro", "Nice signal",
		"Japan", "AS",
	) {
		assert.Contains(t, full, want)
	}

	empty := plain(qsoTab(defaultStyles(), logbook.QSO{}, indexfmt.Mi, lotwStatus{}, 80))
	for _, want := range labels {
		assert.Contains(t, empty, want, "an empty field keeps its row")
	}
	assert.Equal(t, lipgloss.Height(full), lipgloss.Height(empty), "the layout doesn't jump between QSOs")
	assert.Contains(t, empty, pagerEmpty)
}

func TestQSOTabDetails(t *testing.T) {
	q := logbook.QSO{Mode: "SSB", DistanceKm: sql.Null[float64]{V: 160.9344, Valid: true}}
	tab := plain(qsoTab(defaultStyles(), q, indexfmt.Mi, lotwStatus{}, 80))
	assert.Contains(t, tab, "SSB\n", "a mode without a submode is shown alone")
	assert.Contains(t, tab, "100 mi")

	for _, line := range strings.Split(plain(qsoTab(defaultStyles(), fullQSO(), indexfmt.Km, lotwStatus{}, 20)), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 20, "lines fit the width: %q", line)
	}
}

func TestPagerShowsLoadingRow(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	m = press(t, m, enter())
	clear(m.idx.blocks)
	assert.Contains(t, plain(m.pagerBody(t)), "Loading…")
}

func TestPagerFitsSmallWindows(t *testing.T) {
	for _, h := range []int{3, 6, 10, 14} {
		m, _ := newIndexModel(t, 10, 40, h)
		m = press(t, m, enter())
		v := m.View()
		assert.Equal(t, h, lipgloss.Height(v.Content), "height %d", h)
		assert.LessOrEqual(t, lipgloss.Width(v.Content), 40, "height %d", h)
	}
}

func TestPagerClosesWhenTheIndexEmpties(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, enter())
	require.True(t, m.pager.open)

	m, _ = update(t, m, indexLoadedMsg{gen: m.idx.gen})
	assert.False(t, m.pager.open, "there is no QSO left to show")
	assert.Contains(t, m.View().Content, "No QSOs yet.")
}

func TestQSOTabKeepsMultilineValuesOnOneRow(t *testing.T) {
	q := fullQSO()
	q.Comment = "line one\r\nline two\n"
	tab := plain(qsoTab(defaultStyles(), q, indexfmt.Km, lotwStatus{}, 80))
	assert.Equal(t, lipgloss.Height(plain(qsoTab(defaultStyles(), fullQSO(), indexfmt.Km, lotwStatus{}, 80))), lipgloss.Height(tab))
	assert.Contains(t, tab, "line one line two")
	assert.NotContains(t, tab, "\r")
}

func TestQSOTabTooNarrowForValues(t *testing.T) {
	tab := plain(qsoTab(defaultStyles(), fullQSO(), indexfmt.Km, lotwStatus{}, 10))
	assert.NotContains(t, tab, "JA1ABC", "there is no room for values")
	for _, line := range strings.Split(tab, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 10, "lines fit the width: %q", line)
	}
}

func TestQSOTabMode(t *testing.T) {
	tests := []struct {
		name          string
		mode, submode string
		want          string
	}{
		{"mode and submode", "MFSK", "FT4", "Mode       MFSK (FT4)"},
		{"mode only", "SSB", "", "Mode       SSB"},
		{"submode only", "", "FT4", "Mode       FT4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := logbook.QSO{Mode: tt.mode, Submode: tt.submode}
			tab := plain(qsoTab(defaultStyles(), q, indexfmt.Km, lotwStatus{}, 80))
			assert.Contains(t, strings.Split(tab, "\n"), tt.want)
		})
	}
}
