package tui

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

var (
	prefixKey = keyPress(';', 0)
	toggleKey = keyPress('t', 0)
)

// stubMsg is the message the stand-in marked function's command produces.
type stubMsg struct{}

// stubMarkedFunc returns a stand-in marked function on x that records the
// IDs it ran on in got and reports them on the status line.
func stubMarkedFunc(got *[]int64) markedFunc {
	return markedFunc{
		keys: key.NewBinding(key.WithKeys("x")),
		run: func(m Model, ids []int64) (Model, tea.Cmd) {
			*got = ids
			m.idx.msg = "ran"
			return m, func() tea.Msg { return stubMsg{} }
		},
	}
}

func TestPrefixWithoutMarks(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)

	m = press(t, m, prefixKey)

	assert.False(t, m.idx.prefix)
	assert.Equal(t, "QSO 1/10 · No marked QSOs.", statusLine(t, m))
}

func TestPrefixArms(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)

	m = press(t, m, toggleKey, toggleKey, prefixKey)

	assert.True(t, m.idx.prefix)
	assert.Equal(t, "QSO 3/10 · 2 marked · mark-prefix", statusLine(t, m))
}

func TestPrefixEscCancelsSilently(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	m = press(t, m, toggleKey, toggleKey, prefixKey)

	m = press(t, m, special(tea.KeyEscape))

	assert.False(t, m.idx.prefix)
	assert.Equal(t, "QSO 3/10 · 2 marked", statusLine(t, m))
}

func TestPrefixOtherKeysCancel(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"next", keyPress('j', 0)},
		{"help", keyPress('?', 0)},
		{"prefix again", prefixKey},
		{"mark", toggleKey},
		{"limit", limitKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 10, 80, 24)
			m = press(t, m, toggleKey, toggleKey, prefixKey)
			marks := len(m.idx.marks)

			m = press(t, m, tt.key)

			assert.False(t, m.idx.prefix)
			assert.Equal(t, "QSO 3/10 · 2 marked · "+tt.key.String()+" does not apply to marked QSOs.", statusLine(t, m))
			assert.False(t, m.help.ShowAll, "the key's own function does not run")
			assert.Equal(t, promptNone, m.prompt)
			assert.Len(t, m.idx.marks, marks)
		})
	}
}

func TestPrefixQuitStillQuits(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	m = press(t, m, toggleKey, prefixKey)

	_, cmd := update(t, m, keyPress('q', 0))

	require.NotNil(t, cmd)
	assert.Contains(t, collectMsgs(cmd), tea.QuitMsg{})
}

func TestPrefixRunsMarkedFunc(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	var got []int64
	m.markedFuncs = []markedFunc{stubMarkedFunc(&got)}
	// Mark rows 0 and 2, QSOs 10 and 8, out of ID order.
	m = press(t, m, toggleKey, keyPress('j', 0), toggleKey, prefixKey)
	before := m.idx.cursor

	m, cmd := update(t, m, keyPress('x', 0))

	assert.Equal(t, []int64{8, 10}, got, "the marked IDs, ascending")
	assert.False(t, m.idx.prefix)
	assert.Equal(t, before, m.idx.cursor)
	assert.Contains(t, statusLine(t, m), "ran")
	assert.Contains(t, collectMsgs(cmd), stubMsg{})
}

func TestPrefixMarksClearedWhileArmed(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	var got []int64
	m.markedFuncs = []markedFunc{stubMarkedFunc(&got)}
	m = press(t, m, toggleKey, prefixKey)
	require.True(t, m.idx.prefix)
	// An unmark pattern submitted earlier finishes while the prefix is armed.
	m, _ = update(t, m, indexMarksMsg{ids: []int64{10}, add: false})

	m, cmd := update(t, m, keyPress('x', 0))

	assert.Nil(t, got, "the function does not run on no QSOs")
	assert.Nil(t, cmd)
	assert.False(t, m.idx.prefix)
	assert.Equal(t, "QSO 2/10 · "+msgNoMarks, statusLine(t, m))
}

func TestPrefixKeyWithoutPrefixDoesNotRunMarkedFunc(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	var got []int64
	m.markedFuncs = []markedFunc{stubMarkedFunc(&got)}
	m = press(t, m, toggleKey)

	m = press(t, m, keyPress('x', 0))

	assert.Nil(t, got)
}

func TestPrefixBeforeLoadDoesNothing(t *testing.T) {
	m, _ := update(t, New(WithLogbook(&fakeIndexStore{qsos: testQSOs(10)})), tea.WindowSizeMsg{Width: 80, Height: 24})
	m.idx.marks[1] = true

	m, _ = update(t, m, prefixKey)

	assert.False(t, m.idx.prefix)
	assert.Empty(t, m.idx.msg)
}

func TestPrefixSurvivesLoads(t *testing.T) {
	m, _ := newIndexModel(t, 10, 80, 24)
	m = press(t, m, toggleKey, prefixKey)

	m, cmd := update(t, m, reloadKey)
	require.False(t, m.idx.prefix, "ctrl+l while armed cancels the prefix")
	m = runIndex(t, m, cmd)

	m = press(t, m, prefixKey)
	m = runIndex(t, m, loadIndexCmd(m.logbook, m.idx.gen, nil, 0, 0))

	assert.True(t, m.idx.prefix, "a reload arriving while armed keeps the prefix")
}

func TestPrefixHelp(t *testing.T) {
	m, _ := newIndexModel(t, 3, 200, 30)
	m = press(t, m, keyPress('?', 0))

	full := plain(m.footerView())

	assert.Contains(t, full, "mark-prefix")
	assert.NotContains(t, full, "esc")
}

func TestPrefixHelpWithoutLogbook(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 200, Height: 30})
	m.help.ShowAll = true

	assert.NotContains(t, plain(m.footerView()), "mark-prefix")
}

func TestStatusViewPrefixWithLimit(t *testing.T) {
	st := defaultStyles()
	x := loadedIndex(1234, 8)
	x.marks[1] = true
	x.prefix = true
	x.limit = indexLimit{expr: logbook.All{}, text: "~c W1 (~b 20m | ~b 40m | ~b 15m) !~m FT8 ~d 2026-01-01-"}

	got := plain(x.statusView(st, 80))

	assert.Equal(t, "QSO 1/1,234 · limit: ~c W1 (~b 20m | ~b 40m | ~b 15m) … · 1 marked · mark-prefix", got)
	assert.Equal(t, 80, lipgloss.Width(got), "the limit is shortened to leave room for the mark-prefix")
}
