package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// newPagerBulkModel returns a model showing a SQLite logbook of n QSOs, at
// least three, in an 80x24 window, with QSOs n and n-1 marked, as
// newBulkModel does, and the pager open on row 2, QSO n-2, fetched. opts
// are added to the model's options.
func newPagerBulkModel(t *testing.T, n int, opts ...Option) (Model, *logbook.SQLiteStore) {
	t.Helper()
	require.GreaterOrEqual(t, n, 3)
	s := newPatternStore(t, n)
	m := New(append([]Option{WithLogbook(s)}, opts...)...)
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)
	m = press(t, m, toggleKey, toggleKey)
	require.Len(t, m.idx.marks, 2)
	m = pressPager(t, m, enter())
	require.True(t, m.pager.open)
	require.Equal(t, int64(n-2), m.pager.fetched)
	return m, s
}

// stripLine returns the plain index row above the pager showing call.
func stripLine(t *testing.T, m Model, call string) string {
	t.Helper()
	lines := strings.Split(plain(m.View().Content), "\n")
	require.Greater(t, len(lines), m.idx.height)
	for _, l := range lines[1 : 1+m.idx.height] {
		if strings.Contains(l, " "+call+" ") {
			return l
		}
	}
	require.Fail(t, "no strip row for "+call)
	return ""
}

// isMarkedRow reports whether a plain index row shows the mark.
func isMarkedRow(row string) bool {
	return strings.HasPrefix(strings.TrimSpace(row), "* ")
}

func TestPagerMark(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	m.pager.scroll = 3
	require.False(t, isMarkedRow(stripLine(t, m, "K3A")))

	m = pressPager(t, m, toggleKey)

	assert.True(t, m.pager.open, "marking stays in the pager")
	assert.Equal(t, map[int64]bool{5: true, 4: true, 3: true}, m.idx.marks)
	assert.Equal(t, 3, m.idx.cursor, "the cursor moves on to the next QSO")
	assert.Equal(t, 0, m.pager.scroll, "the next QSO starts at the top")
	assert.Equal(t, int64(2), m.pager.fetched)
	assert.Contains(t, plain(m.pagerBody(t)), "K2A")
	assert.True(t, isMarkedRow(stripLine(t, m, "K3A")), "the strip shows the mark")
	assert.Equal(t, "QSO 4/5 · sort: date · 3 marked", statusLine(t, m))
}

func TestPagerUnmark(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	m = pressPager(t, m, keyPress('k', 0), keyPress('k', 0))
	require.Equal(t, int64(5), m.mustCurrentID(t))

	m = pressPager(t, m, toggleKey)

	assert.Equal(t, map[int64]bool{4: true}, m.idx.marks)
	assert.Equal(t, int64(4), m.mustCurrentID(t))
	assert.False(t, isMarkedRow(stripLine(t, m, "K5A")), "the strip shows the mark is gone")
}

func TestPagerMarkLastQSO(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	m = pressPager(t, m, keyPress('j', 0), keyPress('j', 0))
	require.Equal(t, 4, m.idx.cursor)
	m.pager.scroll = 3

	m = pressPager(t, m, toggleKey)

	assert.True(t, m.idx.marks[1], "the last QSO is marked")
	assert.Equal(t, 4, m.idx.cursor, "the cursor stays on the last QSO")
	assert.Equal(t, 3, m.pager.scroll, "the scroll stays, as j does at the end")

	m = pressPager(t, m, toggleKey)
	assert.False(t, m.idx.marks[1], "a second t unmarks it")
	assert.Equal(t, 4, m.idx.cursor)
}

func TestPagerMarkPrefixArms(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)

	m = pressPager(t, m, prefixKey)

	assert.True(t, m.idx.prefix)
	assert.True(t, m.pager.open)
	assert.Equal(t, "QSO 3/5 · sort: date · 2 marked · mark-prefix", statusLine(t, m))
}

func TestPagerMarkPrefixWithoutMarks(t *testing.T) {
	s := newPatternStore(t, 3)
	m := newPatternModel(t, s)
	m = pressPager(t, m, enter())

	m = pressPager(t, m, prefixKey)

	assert.False(t, m.idx.prefix)
	assert.Equal(t, "QSO 1/3 · sort: date · No marked QSOs.", statusLine(t, m))
}

func TestPagerMarkPrefixEscCancels(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)

	m = pressPager(t, m, prefixKey, esc())

	assert.False(t, m.idx.prefix)
	assert.True(t, m.pager.open, "esc cancels the prefix, not the pager")
	assert.Equal(t, "QSO 3/5 · sort: date · 2 marked", statusLine(t, m))
}

func TestPagerMarkPrefixQDoesNotQuit(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	m = pressPager(t, m, prefixKey)

	m, cmd := update(t, m, keyPress('q', 0))

	assert.NotContains(t, collectMsgs(cmd), tea.QuitMsg{})
	assert.False(t, m.idx.prefix)
	assert.True(t, m.pager.open, "q does not close the pager either")
	assert.Equal(t, "QSO 3/5 · sort: date · 2 marked · q does not apply to marked QSOs.", statusLine(t, m))
}

func TestPagerMarkPrefixCtrlCQuits(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	m = pressPager(t, m, prefixKey)

	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))

	assert.Contains(t, collectMsgs(cmd), tea.QuitMsg{})
}

func TestPagerMarkPrefixThenLookupPrefix(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5, WithURLOpener((&fakeOpener{}).open))

	m = pressPager(t, m, prefixKey, keyPress('g', 0))

	assert.False(t, m.idx.prefix)
	assert.False(t, m.pager.lookupPrefix, "g does not start a lookup")
	assert.Equal(t, "QSO 3/5 · sort: date · 2 marked · g does not apply to marked QSOs.", statusLine(t, m))
}

func TestPagerLookupPrefixCancelsMarkKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"t", toggleKey},
		{"d", deleteKey},
		{";", prefixKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, s := newPagerBulkModel(t, 5, WithURLOpener((&fakeOpener{}).open))

			m = pressPager(t, m, keyPress('g', 0), tt.key)

			assert.False(t, m.pager.lookupPrefix)
			assert.False(t, m.idx.prefix)
			assert.Equal(t, promptNone, m.prompt, "no question is asked")
			assert.Len(t, m.idx.marks, 2, "the marks are unchanged")
			assert.Equal(t, 2, m.idx.cursor)
			assert.Equal(t, 5, countAll(t, s))
			assert.Equal(t, "QSO 3/5 · sort: date · 2 marked · g"+tt.key.String()+" is not a lookup.", statusLine(t, m))
		})
	}
}

func TestPagerHelpListsMarkKeys(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	// Wide enough for every help entry.
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 240, Height: 24})

	footer := plain(m.footerView())
	for _, want := range []string{"d delete", "t mark", "; mark-prefix"} {
		assert.Contains(t, footer, want)
	}

	m.help.ShowAll = true
	full := plain(m.footerView())
	// Full help aligns the keys and descriptions in columns of their own.
	for _, want := range []string{
		"delete", "mark", "mark-prefix",
		";d", "delete marked", ";s", "save marked", ";y", "tag marked", ";Y", "untag marked",
	} {
		assert.Contains(t, full, want)
	}

	m = pressPager(t, m, prefixKey)
	assert.Equal(t, full, plain(m.footerView()), "arming the prefix leaves the help alone, as in the index")
}
