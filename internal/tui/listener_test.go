package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogbookChangedReloadsIndex(t *testing.T) {
	m, s := newIndexModel(t, 100, 80, 24)
	m = press(t, m, keyPress('j', 0), keyPress('t', 0))
	m = press(t, m, keyPress('k', 0))
	require.Equal(t, 1, m.idx.cursor)
	cur := currentID(t, m)
	top := m.idx.top

	// Three newer QSOs are logged in the background.
	s.mu.Lock()
	s.qsos = append(testQSOs(103)[:3], s.qsos...)
	s.mu.Unlock()
	m, cmd := update(t, m, LogbookChanged{})
	m = runIndex(t, m, cmd)

	assert.Equal(t, cur, currentID(t, m), "the cursor stays on the same QSO")
	assert.Equal(t, 4, m.idx.cursor)
	assert.Equal(t, top, m.idx.top, "the view stays put, so the new QSOs show")
	assert.Equal(t, 103, m.idx.total)
	assert.Equal(t, map[int64]bool{cur: true}, m.idx.marks, "marks survive the reload")
	assert.Equal(t, "QSO 5/103 · 1 marked", statusLine(t, m), "no message is shown")
}

func TestLogbookChangedShowsNewQSOsAtTheTop(t *testing.T) {
	m, s := newIndexModel(t, 100, 80, 24)
	require.Equal(t, 0, m.idx.top)
	cur := currentID(t, m)

	// A newer QSO is logged in the background, sorting first.
	s.mu.Lock()
	s.qsos = append(testQSOs(101)[:1], s.qsos...)
	s.mu.Unlock()
	m, cmd := update(t, m, LogbookChanged{})
	m = runIndex(t, m, cmd)

	assert.Equal(t, 0, m.idx.top, "the new QSO is on screen, in the first row")
	assert.Equal(t, 1, m.idx.cursor, "the cursor moves down with its QSO")
	assert.Equal(t, cur, currentID(t, m))
}

func TestLogbookChangedKeepsCursorOnScreen(t *testing.T) {
	m, s := newIndexModel(t, 100, 80, 24)
	m = press(t, m, keyPress('L', 0))
	require.Equal(t, m.idx.pageSize()-1, m.idx.cursor-m.idx.top, "the cursor is on the last visible row")
	cur := currentID(t, m)

	s.mu.Lock()
	s.qsos = append(testQSOs(102)[:2], s.qsos...)
	s.mu.Unlock()
	m, cmd := update(t, m, LogbookChanged{})
	m = runIndex(t, m, cmd)

	assert.Equal(t, cur, currentID(t, m))
	assert.Equal(t, m.idx.pageSize()-1, m.idx.cursor-m.idx.top, "the view scrolls just enough to keep the cursor")
}

func TestLogbookChangedDoesNotRedrawScreen(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)

	_, cmd := m.update(LogbookChanged{})

	require.NotNil(t, cmd)
	assert.IsType(t, indexLoadedMsg{}, cmd(), "a plain reload, without tea.ClearScreen")
}

func TestLogbookChangedWithoutLogbook(t *testing.T) {
	m := New()

	got, cmd := m.update(LogbookChanged{})

	assert.Nil(t, cmd)
	assert.Equal(t, m.idx.gen, got.idx.gen, "no load is restarted")
	assert.Empty(t, got.idx.msg)
}

// indexMsg returns the message of type T among those cmd produces.
func indexMsg[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	for _, msg := range collectMsgs(cmd) {
		if got, ok := msg.(T); ok {
			return got
		}
	}
	var zero T
	require.Failf(t, "message not produced", "want %T", zero)
	return zero
}

func TestLogbookChangedWaitsForLimitInFlight(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, limitKey)
	m.input.SetValue("~b 20m")
	m, limitCmd := update(t, m, enterKey)
	// The limit loads before the QSOs are added, but lands after.
	limitMsg := indexMsg[indexLimitMsg](t, limitCmd)
	addPatternQSO(t, s, 11)
	addPatternQSO(t, s, 12)

	m, changedCmd := update(t, m, LogbookChanged{})
	m, limitNext := update(t, m, limitMsg)
	m = runIndex(t, m, limitNext)
	m = runIndex(t, m, changedCmd)

	require.NotNil(t, m.idx.limit.expr, "the limit is adopted")
	assert.Equal(t, 6, m.idx.total, "the new 20m QSO is counted under the limit")
	assert.Equal(t, int64(10), currentID(t, m))
	assert.Equal(t, "QSO 2/6 · limit: ~b 20m", statusLine(t, m))
}

func TestLogbookChangedWaitsForSortInFlight(t *testing.T) {
	m, s := newIndexModel(t, 100, 80, 24)
	m = press(t, m, keyPress('j', 0), keyPress('j', 0))
	cur := currentID(t, m)
	m = press(t, m, sortKey)
	m, sortCmd := update(t, m, keyPress('c', 0))
	sortMsg := indexMsg[indexLoadedMsg](t, sortCmd)
	s.mu.Lock()
	s.qsos = append(testQSOs(103)[:3], s.qsos...)
	s.mu.Unlock()

	m, changedCmd := update(t, m, LogbookChanged{})
	m, sortNext := update(t, m, sortMsg)
	m = runIndex(t, m, sortNext)
	m = runIndex(t, m, changedCmd)

	assert.Equal(t, 103, m.idx.total)
	assert.Equal(t, cur, currentID(t, m), "the cursor stays on the same QSO")
	assert.Equal(t, 5, m.idx.cursor)
}

func TestLogbookChangedUnderOtherScreens(t *testing.T) {
	tests := []struct {
		name string
		open tea.KeyPressMsg
		on   func(Model) bool
	}{
		{"new QSO", newQSOKey, func(m Model) bool { return m.entry.open }},
		{"pager", enter(), func(m Model) bool { return m.pager.open }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newPatternStore(t, 10)
			m := press(t, newPatternModel(t, s), keyPress('j', 0), tt.open)
			require.True(t, tt.on(m))
			cur := currentID(t, m)

			addPatternQSO(t, s, 11)
			addPatternQSO(t, s, 12)
			m, cmd := update(t, m, LogbookChanged{})
			m = runIndex(t, m, cmd)

			assert.True(t, tt.on(m), "the screen stays open")
			assert.Equal(t, 12, m.idx.total)
			assert.Equal(t, cur, currentID(t, m))
		})
	}
}
