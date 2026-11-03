package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	historyPrevKeys = []tea.KeyPressMsg{special(tea.KeyUp), keyPress('p', tea.ModCtrl)}
	historyNextKeys = []tea.KeyPressMsg{special(tea.KeyDown), keyPress('n', tea.ModCtrl)}
)

func TestHistoryAdd(t *testing.T) {
	var h history
	for _, s := range []string{"a", "", "b", "b", "a"} {
		h.add(s)
	}
	assert.Equal(t, []string{"a", "b", "a"}, h.entries, "empty and repeated entries are skipped")
}

func TestHistoryAddCaps(t *testing.T) {
	var h history
	for i := range patternHistorySize + 2 {
		h.add(fmt.Sprint(i))
	}
	require.Len(t, h.entries, patternHistorySize)
	assert.Equal(t, "2", h.entries[0], "the oldest entries drop off")
	assert.Equal(t, fmt.Sprint(patternHistorySize+1), h.entries[len(h.entries)-1])
}

func TestHistoryAddDoesNotShareEntries(t *testing.T) {
	var h history
	h.add("a")
	h.add("b")
	c := h
	c.add("c")
	h.add("d")
	assert.Equal(t, []string{"a", "b", "d"}, h.entries)
	assert.Equal(t, []string{"a", "b", "c"}, c.entries)
}

func TestHistoryBrowse(t *testing.T) {
	var h history
	h.add("a")
	h.add("b")
	h.reset()

	_, ok := h.next()
	assert.False(t, ok, "there is nothing newer than the input")

	s, ok := h.prev("typed")
	assert.True(t, ok)
	assert.Equal(t, "b", s)
	s, ok = h.prev("b")
	assert.True(t, ok)
	assert.Equal(t, "a", s)
	_, ok = h.prev("a")
	assert.False(t, ok, "there is nothing older than the oldest entry")

	s, _ = h.next()
	assert.Equal(t, "b", s)
	s, ok = h.next()
	assert.True(t, ok)
	assert.Equal(t, "typed", s, "moving past the newest entry restores the input")
	_, ok = h.next()
	assert.False(t, ok)
}

func TestHistoryBrowseSkipsInputEqualToNewest(t *testing.T) {
	var h history
	h.add("a")
	h.add("b")
	h.reset()

	s, ok := h.prev("b")
	assert.True(t, ok)
	assert.Equal(t, "a", s, "the newest entry is already in the prompt")
	s, ok = h.next()
	assert.True(t, ok)
	assert.Equal(t, "b", s)
	_, ok = h.next()
	assert.False(t, ok, "the input is the newest entry")
	s, _ = h.prev("b")
	assert.Equal(t, "a", s)

	var one history
	one.add("a")
	one.reset()
	_, ok = one.prev("a")
	assert.False(t, ok, "the only entry is already in the prompt")
}

func TestPatternHistoryAfterPrefilledPrompt(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 10))
	m = submitPattern(t, m, promptMark, "~b 20m")
	m = submitPattern(t, m, promptSearch, "~m FT8")

	m = press(t, m, searchKey)
	require.Equal(t, "~m FT8", m.input.Value(), "the prompt shows the last search")
	m = press(t, m, special(tea.KeyUp))

	assert.Equal(t, "~b 20m", m.input.Value(), "up recalls the pattern before the one shown")
}

func TestHistoryEmpty(t *testing.T) {
	var h history
	h.reset()
	_, ok := h.prev("typed")
	assert.False(t, ok)
	_, ok = h.next()
	assert.False(t, ok)
}

func TestPatternHistoryKeys(t *testing.T) {
	for _, kind := range []promptKind{promptLimit, promptMark, promptUnmark, promptSearch, promptSearchReverse} {
		for i := range historyPrevKeys {
			prev, next := historyPrevKeys[i], historyNextKeys[i]
			t.Run(patternFor[kind].String()+"/"+prev.String(), func(t *testing.T) {
				m := newPatternModel(t, newPatternStore(t, 10))
				m = submitPattern(t, m, promptMark, "~b 20m")
				m = submitPattern(t, m, promptUnmark, "~m FT8")
				m = submitPattern(t, m, promptSearch, "")
				m = press(t, m, patternFor[kind])
				m.input.SetValue("typed")

				m = press(t, m, prev)
				assert.Equal(t, "~m FT8", m.input.Value())
				assert.Equal(t, len("~m FT8"), m.input.Position(), "the cursor is at the end")
				m = press(t, m, prev)
				assert.Equal(t, "~b 20m", m.input.Value())
				m = press(t, m, prev)
				assert.Equal(t, "~b 20m", m.input.Value(), "the oldest pattern stays")

				m = press(t, m, next)
				assert.Equal(t, "~m FT8", m.input.Value())
				m = press(t, m, next)
				assert.Equal(t, "typed", m.input.Value(), "moving past the newest pattern restores the input")
				m = press(t, m, next)
				assert.Equal(t, "typed", m.input.Value())
				assert.Equal(t, kind, m.prompt)
			})
		}
	}
}

func TestPatternHistoryIsShared(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 10))
	m = submitPattern(t, m, promptSearch, " ~b 40m ")

	m = press(t, m, limitKey, special(tea.KeyUp))

	assert.Equal(t, "~b 40m", m.input.Value(), "a search is there to limit by")
}

func TestPatternHistorySkipsCancelledAndBadPatterns(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 10))
	m = submitPattern(t, m, promptMark, "~b 20m")

	m = press(t, m, markKey)
	m = typeText(t, m, "~b 40m")
	m = press(t, m, escKey)
	m = press(t, m, markKey)
	m = typeText(t, m, "~q")
	m = press(t, m, enterKey)
	require.Equal(t, promptMark, m.prompt, "the bad pattern keeps the prompt open")
	m = press(t, m, escKey, markKey, special(tea.KeyUp))

	assert.Equal(t, "~b 20m", m.input.Value())
	assert.Equal(t, []string{"~b 20m"}, m.patterns.entries)
}

func TestPatternHistoryRestartsEachPrompt(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 10))
	m = submitPattern(t, m, promptMark, "~b 20m")
	m = submitPattern(t, m, promptMark, "~b 40m")
	m = press(t, m, markKey, special(tea.KeyUp), special(tea.KeyUp), escKey)

	m = press(t, m, markKey, special(tea.KeyUp))

	assert.Equal(t, "~b 40m", m.input.Value(), "browsing starts again from the newest pattern")
}

func TestPatternHistoryNotInOtherPrompts(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 10))
	m = submitPattern(t, m, promptMark, "~b 20m")

	m = press(t, m, keyPress('5', 0), special(tea.KeyUp))

	assert.Equal(t, promptJump, m.prompt)
	assert.Equal(t, "5", m.input.Value())
	assert.NotContains(t, plain(m.View().Content), "prev pattern")
}

func TestPatternHistoryHelp(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 10))

	m = press(t, m, limitKey)

	view := plain(m.View().Content)
	assert.Contains(t, view, "prev pattern")
	assert.Contains(t, view, "next pattern")
}
