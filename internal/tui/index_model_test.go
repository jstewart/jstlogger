package tui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// fakeIndexStore serves the index queries from qsos, newest first, and
// records the offset of each Index call.
type fakeIndexStore struct {
	logbook.Store

	mu      sync.Mutex
	qsos    []logbook.QSO
	offsets []int
	err     error
}

func (s *fakeIndexStore) Count(_ context.Context, _ logbook.Filter) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.qsos), s.err
}

func (s *fakeIndexStore) Index(_ context.Context, _ logbook.Filter, offset, limit int) ([]logbook.QSO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offsets = append(s.offsets, offset)
	if s.err != nil {
		return nil, s.err
	}
	if offset >= len(s.qsos) {
		return nil, nil
	}
	return slices.Clone(s.qsos[offset:min(offset+limit, len(s.qsos))]), nil
}

func (s *fakeIndexStore) IndexOf(_ context.Context, _ logbook.Filter, id int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.IndexFunc(s.qsos, func(q logbook.QSO) bool { return q.ID == id }); i >= 0 {
		return i, nil
	}
	return 0, logbook.ErrNotFound
}

func (s *fakeIndexStore) loadedOffsets() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.offsets)
}

// runIndex runs cmd and feeds the index messages it produces back into m, until
// no commands are left. Other messages are dropped.
func runIndex(t *testing.T, m Model, cmd tea.Cmd) Model {
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
		case indexLoadedMsg, indexBlockMsg:
			var next tea.Cmd
			m, next = update(t, m, msg)
			queue = append(queue, next)
		}
	}
	return m
}

// newIndexModel returns a Model showing an index of n QSOs in a width by
// height window, with the initial load done.
func newIndexModel(t *testing.T, n, width, height int) (Model, *fakeIndexStore) {
	t.Helper()
	s := &fakeIndexStore{qsos: testQSOs(n)}
	m := New(WithLogbook(s))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: width, Height: height})
	return runIndex(t, m, cmd), s
}

// press sends each key to m and runs the commands that follow.
func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, k)
		m = runIndex(t, m, cmd)
	}
	return m
}

func special(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// statusLine returns the index status line, the last line of the body.
func statusLine(t *testing.T, m Model) string {
	t.Helper()
	lines := strings.Split(plain(m.View().Content), "\n")
	footer := lipgloss.Height(m.footerView())
	require.Greater(t, len(lines), footer)
	return strings.TrimSpace(lines[len(lines)-footer-1])
}

func TestIndexIsTheStartScreen(t *testing.T) {
	m, s := newIndexModel(t, 10, 80, 24)

	view := plain(m.View().Content)
	lines := strings.Split(view, "\n")

	assert.Contains(t, lines[1], "Date       Time  Call")
	assert.Contains(t, lines[2], "K10A", "the newest QSO comes first")
	assert.Contains(t, lines[11], "K1A")
	assert.Equal(t, "QSO 1/10", statusLine(t, m))
	assert.Equal(t, []int{0}, s.loadedOffsets(), "one block loads the whole log")

	v := m.View()
	assert.Equal(t, 24, lipgloss.Height(v.Content), "the index fills the window")
	assert.LessOrEqual(t, lipgloss.Width(v.Content), 80)
}

func TestIndexEmptyLogbook(t *testing.T) {
	m, _ := newIndexModel(t, 0, 80, 24)
	assert.Contains(t, m.View().Content, "No QSOs yet.")
}

func TestIndexLoadErrorIsShown(t *testing.T) {
	s := &fakeIndexStore{err: assert.AnError}
	m := New(WithLogbook(s))
	m = runIndex(t, m, m.Init())
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 24})

	assert.Contains(t, plain(m.View().Content), assert.AnError.Error())
}

func TestIndexKeys(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
		move func(*index)
	}{
		{"j", keyPress('j', 0), (*index).next},
		{"down", special(tea.KeyDown), (*index).next},
		{"k", keyPress('k', 0), (*index).prev},
		{"up", special(tea.KeyUp), (*index).prev},
		{"z", keyPress('z', 0), (*index).nextPage},
		{"pgdown", special(tea.KeyPgDown), (*index).nextPage},
		{"right", special(tea.KeyRight), (*index).nextPage},
		{"Z", keyPress('Z', 0), (*index).prevPage},
		{"pgup", special(tea.KeyPgUp), (*index).prevPage},
		{"left", special(tea.KeyLeft), (*index).prevPage},
		{"]", keyPress(']', 0), (*index).halfDown},
		{"[", keyPress('[', 0), (*index).halfUp},
		{">", keyPress('>', 0), (*index).nextLine},
		{"<", keyPress('<', 0), (*index).prevLine},
		{"H", keyPress('H', 0), (*index).topPage},
		{"M", keyPress('M', 0), (*index).middlePage},
		{"L", keyPress('L', 0), (*index).bottomPage},
		{"=", keyPress('=', 0), (*index).first},
		{"home", special(tea.KeyHome), (*index).first},
		{"*", keyPress('*', 0), (*index).last},
		{"end", special(tea.KeyEnd), (*index).last},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 200, 80, 24)
			m.idx.cursor, m.idx.top = 50, 45

			want := m.idx
			tt.move(&want)
			m = press(t, m, tt.key)

			assert.Equal(t, posOf(want), posOf(m.idx))
			assert.NotEqual(t, pos{50, 45}, posOf(m.idx), "the key moves the cursor or the view")
		})
	}
}

func TestIndexStopsAtTheEnds(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)

	m = press(t, m, keyPress('k', 0))
	assert.Equal(t, "QSO 1/3 · You are on the first QSO.", statusLine(t, m))

	m = press(t, m, special(tea.KeyEnd), keyPress('j', 0))
	assert.Equal(t, "QSO 3/3 · You are on the last QSO.", statusLine(t, m))

	m = press(t, m, keyPress('k', 0))
	assert.Equal(t, "QSO 2/3", statusLine(t, m), "the next key clears the message")
}

func TestIndexGlobalKeys(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)

	m = press(t, m, keyPress('?', 0))
	assert.True(t, m.help.ShowAll)

	_, cmd := update(t, m, keyPress('q', 0))
	require.NotNil(t, cmd)
	assert.Contains(t, runQuit(cmd), tea.QuitMsg{})
}

// runQuit runs cmd, expanding batches, and returns the messages that are
// not index loads.
func runQuit(cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
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
		default:
			out = append(out, msg)
		}
	}
	return out
}

func TestIndexMarkKey(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)

	m = press(t, m, keyPress('t', 0), keyPress('j', 0), keyPress('t', 0))

	assert.Equal(t, map[int64]bool{10: true, 8: true}, m.idx.marks)
	assert.Equal(t, "QSO 4/10 · 2 marked", statusLine(t, m))
	lines := strings.Split(plain(m.View().Content), "\n")
	assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[2]), "* "), "the first QSO is marked")
	assert.False(t, strings.HasPrefix(strings.TrimSpace(lines[3]), "* "))
}

func TestIndexLoadsBlocksInBackground(t *testing.T) {
	m, s := newIndexModel(t, 2_000, 80, 24)
	require.Equal(t, []int{0}, s.loadedOffsets())

	m, cmd := update(t, m, special(tea.KeyEnd))

	require.NotNil(t, cmd, "the last block loads in a command")
	assert.Equal(t, []int{0}, s.loadedOffsets(), "the key press does not wait for the query")
	assert.Contains(t, plain(m.View().Content), "  …", "rows not yet loaded show a placeholder")
	assert.Equal(t, "QSO 2,000/2,000", statusLine(t, m))

	m = runIndex(t, m, cmd)
	assert.Equal(t, []int{0, 3 * indexBlockSize}, s.loadedOffsets(), "the rows in between are not loaded")
	q, ok := m.idx.current()
	require.True(t, ok)
	assert.Equal(t, int64(1), q.ID, "the oldest QSO")
	assert.NotContains(t, plain(m.View().Content), "  …")
}

func TestIndexJumpPrompt(t *testing.T) {
	m, s := newIndexModel(t, 2_000, 80, 24)

	m, _ = update(t, m, keyPress('1', 0))
	require.Equal(t, promptJump, m.prompt)
	assert.Equal(t, "1", m.input.Value(), "the prompt is seeded with the digit")
	assert.Contains(t, plain(m.View().Content), "Jump to: 1")

	for _, r := range "750" {
		m, _ = update(t, m, keyPress(r, 0))
	}
	m = press(t, m, special(tea.KeyEnter))

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, 1749, m.idx.cursor)
	assert.Equal(t, []int{0, 3 * indexBlockSize}, s.loadedOffsets(), "the rows in between are not loaded")
	assert.Equal(t, "QSO 1,750/2,000", statusLine(t, m))
}

func TestIndexJumpPromptErrors(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"9999", "no QSO 9999: enter 1–2,000"},
		{"9x", `invalid QSO number "9x": invalid syntax`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m, _ := newIndexModel(t, 2_000, 80, 24)
			m.idx.cursor = 5
			for _, r := range tt.input {
				m, _ = update(t, m, keyPress(r, 0))
			}

			m, _ = update(t, m, special(tea.KeyEnter))

			assert.Equal(t, promptJump, m.prompt, "the prompt stays open")
			require.Error(t, m.promptErr)
			assert.Equal(t, tt.want, m.promptErr.Error())
			assert.Equal(t, 5, m.idx.cursor)
		})
	}
}

func TestIndexResizeKeepsCursorAndCache(t *testing.T) {
	m, s := newIndexModel(t, 300, 80, 40)
	m = press(t, m, special(tea.KeyEnd))
	before, ok := m.idx.current()
	require.True(t, ok)

	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 10}, {Width: 120, Height: 60}, {Width: 80, Height: 24}} {
		var cmd tea.Cmd
		m, cmd = update(t, m, size)
		assert.Nil(t, cmd, "no rows to load at %dx%d", size.Width, size.Height)

		after, ok := m.idx.current()
		require.True(t, ok)
		assert.Equal(t, before.ID, after.ID, "the cursor stays on the same QSO at %dx%d", size.Width, size.Height)
		v := m.View()
		assert.Equal(t, size.Height, lipgloss.Height(v.Content))
		assert.LessOrEqual(t, lipgloss.Width(v.Content), size.Width)
	}
	assert.Equal(t, []int{0}, s.loadedOffsets(), "resizing does not reload")
}

func TestIndexSmallWindow(t *testing.T) {
	// The header, one body line and the footer need at least three lines.
	const minHeight = 3
	for _, height := range []int{1, 2, 3, 4, 5} {
		m, _ := newIndexModel(t, 10, 20, height)
		m = press(t, m, keyPress('j', 0), keyPress('z', 0))
		v := m.View()
		assert.Equal(t, max(height, minHeight), lipgloss.Height(v.Content), "height %d", height)
		assert.LessOrEqual(t, lipgloss.Width(v.Content), 20)
	}
}

func TestIndexReloadKeepsCursorAndMarks(t *testing.T) {
	m, s := newIndexModel(t, 100, 80, 24)
	m = press(t, m, keyPress('j', 0), keyPress('t', 0))
	m = press(t, m, keyPress('k', 0))
	require.Equal(t, 1, m.idx.cursor)
	cur, ok := m.idx.current()
	require.True(t, ok)

	// Three newer QSOs are logged.
	s.mu.Lock()
	newer := testQSOs(103)[:3]
	s.qsos = append(newer, s.qsos...)
	s.mu.Unlock()
	m = press(t, m, keyPress('l', tea.ModCtrl))

	after, ok := m.idx.current()
	require.True(t, ok)
	assert.Equal(t, cur.ID, after.ID, "the cursor stays on the same QSO")
	assert.Equal(t, 4, m.idx.cursor)
	assert.Equal(t, 103, m.idx.total)
	assert.Equal(t, map[int64]bool{cur.ID: true}, m.idx.marks, "marks survive a reload")
	assert.Equal(t, "QSO 5/103 · 1 marked", statusLine(t, m))
}

func TestIndexReloadAfterDelete(t *testing.T) {
	m, s := newIndexModel(t, 10, 80, 24)
	m = press(t, m, special(tea.KeyEnd))

	s.mu.Lock()
	s.qsos = s.qsos[:5]
	s.mu.Unlock()
	m = press(t, m, keyPress('l', tea.ModCtrl))

	assert.Equal(t, 5, m.idx.total)
	assert.Equal(t, 4, m.idx.cursor, "the cursor moves to the last QSO left")
}

func TestIndexReloadDropsLoadsInFlight(t *testing.T) {
	m, _ := newIndexModel(t, 2_000, 80, 24)
	m, blockCmd := update(t, m, special(tea.KeyEnd))
	require.NotNil(t, blockCmd)

	m, reloadCmd := update(t, m, keyPress('l', tea.ModCtrl))
	m = runIndex(t, m, blockCmd)
	assert.Empty(t, m.idx.blocks[3], "a block loaded before the reload is dropped")

	m = runIndex(t, m, reloadCmd)
	q, ok := m.idx.current()
	require.True(t, ok)
	assert.Equal(t, int64(1), q.ID)
}

func TestIndexHelp(t *testing.T) {
	m, _ := newIndexModel(t, 3, 200, 30)
	short := plain(m.footerView())
	for _, want := range []string{"next", "prev", "mark", "jump to QSO", "help", "quit"} {
		assert.Contains(t, short, want)
	}

	m = press(t, m, keyPress('?', 0))
	full := plain(m.footerView())
	for _, want := range []string{"next page", "half page down", "scroll up", "middle of page", "oldest", "reload"} {
		assert.Contains(t, full, want)
	}
}

func TestIndexHelpNarrowWindowKeepsGlobalKeys(t *testing.T) {
	m, _ := newIndexModel(t, 3, 60, 30)
	assert.Contains(t, plain(m.footerView()), "q quit")

	m = press(t, m, keyPress('?', 0))
	assert.Contains(t, plain(m.footerView()), "quit")
}

func TestHelpWithoutLogbook(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 200, Height: 30})
	m.help.ShowAll = true

	full := plain(m.footerView())

	assert.NotContains(t, full, "mark")
	assert.Contains(t, full, "quit")
}
