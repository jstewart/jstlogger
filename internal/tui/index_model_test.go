package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
	"jstlogger/internal/pattern"
)

// fakeIndexStore serves the index queries from qsos, newest first, and
// records the offset of each Index call.
type fakeIndexStore struct {
	logbook.Store

	mu      sync.Mutex
	qsos    []logbook.QSO
	offsets []int
	err     error
	// countErr fails only Count.
	countErr error
	// idsCalls counts the IDs calls.
	idsCalls int
}

// IDs returns the ID of every QSO, ignoring e, and counts the call.
func (s *fakeIndexStore) IDs(_ context.Context, _ logbook.Expr) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idsCalls++
	if s.err != nil {
		return nil, s.err
	}
	var ids []int64
	for _, q := range s.qsos {
		ids = append(ids, q.ID)
	}
	return ids, nil
}

func (s *fakeIndexStore) Count(_ context.Context, _ logbook.Expr) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.countErr != nil {
		return 0, s.countErr
	}
	return len(s.qsos), s.err
}

func (s *fakeIndexStore) Index(_ context.Context, _ logbook.Expr, offset, limit int) ([]logbook.QSO, error) {
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

func (s *fakeIndexStore) IndexOf(_ context.Context, _ logbook.Expr, id int64) (int, error) {
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
		case indexLoadedMsg, indexBlockMsg, indexLimitMsg, indexMarksMsg:
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
	assert.Contains(t, collectMsgs(cmd), tea.QuitMsg{})
}

// collectMsgs runs cmd, expanding batches, and returns every message it
// produces without feeding any of them back into the model.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
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

func TestIndexReloadRedrawsScreen(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)

	_, cmd := update(t, m, keyPress('l', tea.ModCtrl))

	require.NotNil(t, cmd)
	assert.Contains(t, collectMsgs(cmd), tea.ClearScreen(), "ctrl+l clears the screen so it is drawn again in full")
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

func TestIndexFailedReloadRetriesDroppedLoads(t *testing.T) {
	m, s := newIndexModel(t, 2_000, 80, 24)
	m, blockCmd := update(t, m, special(tea.KeyEnd))
	require.NotNil(t, blockCmd)

	s.mu.Lock()
	s.countErr = assert.AnError
	s.mu.Unlock()
	m = press(t, m, keyPress('l', tea.ModCtrl))
	m = runIndex(t, m, blockCmd)
	require.ErrorIs(t, m.idx.err, assert.AnError, "the reload failed")

	_, ok := m.idx.current()
	assert.True(t, ok, "the block dropped by the reload is loaded again")
}

func TestIndexHelp(t *testing.T) {
	m, _ := newIndexModel(t, 3, 200, 30)
	short := plain(m.footerView())
	for _, want := range []string{"next", "prev", "mark", "jump to QSO", "help", "quit"} {
		assert.Contains(t, short, want)
	}

	m = press(t, m, keyPress('?', 0))
	full := plain(m.footerView())
	for _, want := range []string{"next page", "half page down", "scroll up", "middle of page", "oldest", "reload", "limit", "mark pattern", "unmark pattern"} {
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

// newPatternStore returns a SQLite logbook of n QSOs with IDs 1 to n, oldest
// first, so row 0 is QSO n. Even IDs are on 20m and odd IDs on 40m; IDs
// divisible by three are FT8 and the rest SSB. Calls are K<ID>A.
func newPatternStore(t *testing.T, n int) *logbook.SQLiteStore {
	t.Helper()
	s, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	for i := 1; i <= n; i++ {
		addPatternQSO(t, s, i)
	}
	return s
}

// addPatternQSO adds the QSO newPatternStore describes for ID i, logged i
// minutes after a fixed time.
func addPatternQSO(t *testing.T, s logbook.Store, i int) {
	t.Helper()
	t0 := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	band, mode := "40m", "SSB"
	if i%2 == 0 {
		band = "20m"
	}
	if i%3 == 0 {
		mode = "FT8"
	}
	q, err := s.Add(t.Context(), logbook.QSO{
		StartUTC: t0.Add(time.Duration(i) * time.Minute),
		Call:     fmt.Sprintf("K%dA", i),
		Band:     band,
		Mode:     mode,
	})
	require.NoError(t, err)
	require.Equal(t, int64(i), q.ID)
}

// newPatternModel returns a Model showing s's index in an 80x24 window,
// with the initial load done.
func newPatternModel(t *testing.T, s logbook.Store) Model {
	t.Helper()
	m := New(WithLogbook(s))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return runIndex(t, m, cmd)
}

var (
	limitKey   = keyPress('l', 0)
	markKey    = keyPress('T', 0)
	unmarkKey  = keyPress('t', tea.ModCtrl)
	reloadKey  = keyPress('l', tea.ModCtrl)
	patternFor = map[promptKind]tea.KeyPressMsg{promptLimit: limitKey, promptMark: markKey, promptUnmark: unmarkKey}
)

// submitPattern opens the prompt for kind, replaces its input with p,
// submits it and runs the commands that follow.
func submitPattern(t *testing.T, m Model, kind promptKind, p string) Model {
	t.Helper()
	m = press(t, m, patternFor[kind])
	require.Equal(t, kind, m.prompt)
	m.input.SetValue(p)
	return press(t, m, enterKey)
}

// currentID returns the ID of the QSO under the cursor.
func currentID(t *testing.T, m Model) int64 {
	t.Helper()
	q, ok := m.idx.current()
	require.True(t, ok, "the current row is loaded")
	return q.ID
}

func TestIndexPatternPrompts(t *testing.T) {
	tests := []struct {
		name  string
		kind  promptKind
		label string
	}{
		{"l", promptLimit, "Limit to QSOs matching: "},
		{"T", promptMark, "Mark QSOs matching: "},
		{"ctrl+t", promptUnmark, "Unmark QSOs matching: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 10, 80, 24)

			m = press(t, m, patternFor[tt.kind])

			require.Equal(t, tt.kind, m.prompt)
			assert.Empty(t, m.input.Value())
			assert.Contains(t, plain(m.View().Content), strings.TrimSpace(tt.label))
			assert.Equal(t, tt.label, m.input.Prompt)
		})
	}
}

func TestIndexPatternPromptParseError(t *testing.T) {
	for _, kind := range []promptKind{promptLimit, promptMark, promptUnmark} {
		t.Run(patternFor[kind].String(), func(t *testing.T) {
			m, s := newIndexModel(t, 10, 80, 24)
			m = press(t, m, patternFor[kind])
			m = typeText(t, m, "~b 20m ~q")

			m = press(t, m, enterKey)

			assert.Equal(t, kind, m.prompt, "the prompt stays open")
			require.ErrorIs(t, m.promptErr, pattern.ErrSyntax)
			assert.Contains(t, plain(m.View().Content), "unknown term ~q at column 8")
			assert.Zero(t, s.idsCalls)
		})
	}
}

func TestIndexLimit(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, keyPress('j', 0), keyPress('j', 0))
	require.Equal(t, int64(8), currentID(t, m))

	m = press(t, m, limitKey)
	m = typeText(t, m, "~b 20m")
	m = press(t, m, enterKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, 5, m.idx.total)
	assert.Equal(t, int64(8), currentID(t, m), "the cursor stays on the QSO, which matches")
	assert.Equal(t, 1, m.idx.cursor)
	assert.Equal(t, "QSO 2/5 · limit: ~b 20m", statusLine(t, m))
	view := plain(m.View().Content)
	assert.Contains(t, view, "K10A")
	assert.NotContains(t, view, "K9A")

	m = press(t, m, limitKey)
	assert.Equal(t, "~b 20m", m.input.Value(), "the prompt shows the limit to edit")
	m = typeText(t, m, " ~m FT8")
	m = press(t, m, enterKey)
	assert.Equal(t, 1, m.idx.total)
	assert.Equal(t, int64(6), currentID(t, m), "the cursor goes to the first row when its QSO does not match")
	assert.Equal(t, "QSO 1/1 · limit: ~b 20m ~m FT8", statusLine(t, m))
}

func TestIndexLimitNoMatchesKeepsLimit(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = submitPattern(t, m, promptLimit, "~b 20m")
	m = press(t, m, keyPress('j', 0))
	before := m.idx.cursor

	m = submitPattern(t, m, promptLimit, "~b 6m")

	assert.Equal(t, 5, m.idx.total)
	assert.Equal(t, before, m.idx.cursor)
	assert.Equal(t, "QSO 2/5 · limit: ~b 20m · No QSOs match the pattern.", statusLine(t, m))
}

func TestIndexLimitClear(t *testing.T) {
	for _, input := range []string{"", "  ", "~A"} {
		t.Run(input, func(t *testing.T) {
			s := newPatternStore(t, 10)
			m := newPatternModel(t, s)
			m = submitPattern(t, m, promptLimit, "~b 40m")
			m = press(t, m, keyPress('j', 0))
			require.Equal(t, int64(7), currentID(t, m))

			m = submitPattern(t, m, promptLimit, input)

			assert.Equal(t, promptNone, m.prompt)
			assert.Equal(t, 10, m.idx.total)
			assert.Equal(t, int64(7), currentID(t, m), "the cursor stays on the QSO")
			assert.Equal(t, "QSO 4/10", statusLine(t, m))
		})
	}
}

func TestIndexLimitClearWithoutLimitJustCloses(t *testing.T) {
	m, s := newIndexModel(t, 10, 80, 24)
	m = press(t, m, limitKey)

	m, cmd := update(t, m, enterKey)

	assert.Equal(t, promptNone, m.prompt)
	m = runIndex(t, m, cmd)
	assert.Equal(t, []int{0}, s.loadedOffsets(), "nothing reloads")
}

func TestIndexReloadKeepsLimit(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = submitPattern(t, m, promptLimit, "~b 20m")

	addPatternQSO(t, s, 11)
	addPatternQSO(t, s, 12)
	m = press(t, m, reloadKey)

	assert.Equal(t, 6, m.idx.total)
	assert.Equal(t, int64(10), currentID(t, m))
	assert.Equal(t, "QSO 2/6 · limit: ~b 20m", statusLine(t, m))
}

func TestIndexReloadDropsLimitInFlight(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, limitKey)
	m.input.SetValue("~b 20m")
	m, limitCmd := update(t, m, enterKey)
	require.NotNil(t, limitCmd)

	m, reloadCmd := update(t, m, reloadKey)
	m = runIndex(t, m, limitCmd)
	m = runIndex(t, m, reloadCmd)

	assert.Equal(t, 10, m.idx.total, "the limit loaded before the reload is dropped")
	assert.Equal(t, "QSO 1/10", statusLine(t, m))
}

func TestIndexLimitLoadErrorKeepsView(t *testing.T) {
	m, s := newIndexModel(t, 10, 80, 24)
	s.mu.Lock()
	s.countErr = assert.AnError
	s.mu.Unlock()

	m = submitPattern(t, m, promptLimit, "~b 20m")

	require.NoError(t, m.idx.err, "the old rows stay on screen")
	assert.Equal(t, 10, m.idx.total)
	assert.Contains(t, m.View().Content, "K10A")
	assert.Contains(t, statusLine(t, m), assert.AnError.Error())
	assert.NotContains(t, statusLine(t, m), "limit:")
}

func TestIndexMarkPattern(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)

	m = submitPattern(t, m, promptMark, "~m FT8")
	assert.Equal(t, map[int64]bool{3: true, 6: true, 9: true}, m.idx.marks)
	assert.Equal(t, "QSO 1/10 · 3 marked · Marked 3 QSOs.", statusLine(t, m))

	m = submitPattern(t, m, promptLimit, "~b 20m")
	assert.Equal(t, map[int64]bool{3: true, 6: true, 9: true}, m.idx.marks, "limiting keeps the marks")
	m = submitPattern(t, m, promptMark, "~c K1")
	assert.Equal(t, map[int64]bool{3: true, 6: true, 9: true, 10: true}, m.idx.marks, "only QSOs within the limit are marked")
	assert.Equal(t, "QSO 1/5 · limit: ~b 20m · 4 marked · Marked 1 QSO.", statusLine(t, m))

	m = submitPattern(t, m, promptMark, "~b 20m")
	assert.Len(t, m.idx.marks, 7)
	assert.Contains(t, statusLine(t, m), "Marked 3 QSOs.", "QSOs already marked are not counted")

	m = submitPattern(t, m, promptMark, "~c ZZZ")
	assert.Len(t, m.idx.marks, 7)
	assert.Contains(t, statusLine(t, m), "No QSOs match the pattern.")
}

func TestIndexUnmarkPattern(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = submitPattern(t, m, promptMark, "~A")
	require.Len(t, m.idx.marks, 10)

	m = submitPattern(t, m, promptUnmark, "~b 40m")
	assert.Equal(t, map[int64]bool{2: true, 4: true, 6: true, 8: true, 10: true}, m.idx.marks)
	assert.Equal(t, "QSO 1/10 · 5 marked · Unmarked 5 QSOs.", statusLine(t, m))

	m = submitPattern(t, m, promptLimit, "~m FT8")
	m = submitPattern(t, m, promptUnmark, "~A")
	assert.Equal(t, map[int64]bool{2: true, 4: true, 8: true, 10: true}, m.idx.marks, "~A unmarks only the QSOs within the limit")
	assert.Equal(t, "QSO 1/3 · limit: ~m FT8 · 4 marked · Unmarked 1 QSO.", statusLine(t, m))
}

func TestIndexUnmarkAllWithoutLimit(t *testing.T) {
	m, s := newIndexModel(t, 10, 80, 24)
	m = press(t, m, keyPress('t', 0), keyPress('t', 0))
	require.Len(t, m.idx.marks, 2)

	m = press(t, m, unmarkKey)
	m.input.SetValue("~A")
	m, cmd := update(t, m, enterKey)

	assert.Empty(t, collectMsgs(cmd), "nothing runs in the background")
	assert.Empty(t, m.idx.marks)
	assert.Zero(t, s.idsCalls, "the logbook is not queried")
	assert.Equal(t, "QSO 3/10 · Unmarked 2 QSOs.", statusLine(t, m))
}

func TestIndexMarkPatternEmptyInputCloses(t *testing.T) {
	for _, kind := range []promptKind{promptMark, promptUnmark} {
		t.Run(patternFor[kind].String(), func(t *testing.T) {
			m, s := newIndexModel(t, 10, 80, 24)

			m = submitPattern(t, m, kind, " ")

			assert.Equal(t, promptNone, m.prompt)
			assert.Zero(t, s.idsCalls)
			assert.Equal(t, "QSO 1/10", statusLine(t, m))
		})
	}
}

func TestIndexMarkPatternError(t *testing.T) {
	m, s := newIndexModel(t, 10, 80, 24)
	s.mu.Lock()
	s.err = assert.AnError
	s.mu.Unlock()

	m = submitPattern(t, m, promptMark, "~b 20m")

	assert.Empty(t, m.idx.marks)
	assert.Contains(t, statusLine(t, m), "mark QSOs: "+assert.AnError.Error())
}
