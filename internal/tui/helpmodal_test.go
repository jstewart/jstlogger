package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

var (
	f1Key        = special(tea.KeyF1)
	helpDownKey  = special(tea.KeyDown)
	helpUpKey    = special(tea.KeyUp)
	backspaceKey = special(tea.KeyBackspace)
)

// helpText returns every line the help modal lists for the context m is
// in, unfiltered and unwindowed, as plain text.
func helpText(m Model) string {
	h := newHelpModal(m.contextKeys(), true)
	lines := h.lines(m.styles, 200)
	for i, l := range lines {
		lines[i] = strings.TrimRight(plain(l), " ")
	}
	return strings.Join(lines, "\n")
}

// modalText returns the plain help modal as drawn over the body.
func modalText(t *testing.T, m Model) string {
	t.Helper()
	require.True(t, m.helpModal.open)
	return plain(m.helpView(m.bodyWidth(), m.bodyHeight()))
}

func TestHelpOpensAndCloses(t *testing.T) {
	tests := []struct {
		name        string
		open, close tea.KeyPressMsg
	}{
		{"? and esc", keyPress('?', 0), esc()},
		{"? and ?", keyPress('?', 0), keyPress('?', 0)},
		{"f1 and f1", f1Key, f1Key},
		{"f1 and esc", f1Key, esc()},
		{"? and enter", keyPress('?', 0), enter()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newIndexModel(t, 3, 80, 24)
			m = press(t, m, tt.open)
			require.True(t, m.helpModal.open)

			m = press(t, m, tt.close)
			assert.False(t, m.helpModal.open)
		})
	}
}

func TestHelpIsReadOnly(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, keyPress('?', 0), keyPress('j', 0), keyPress('d', 0), keyPress('a', 0), keyPress('q', 0))

	assert.True(t, m.helpModal.open, "q doesn't close it")
	assert.Equal(t, 0, m.idx.cursor, "j scrolls the help rather than moves")
	assert.Equal(t, promptNone, m.prompt, "d doesn't ask to delete")
	assert.False(t, m.entry.open, "a doesn't open the New QSO screen")
	assert.Empty(t, m.helpModal.filter(), "typing doesn't filter until /")

	m = press(t, m, enter())
	assert.False(t, m.helpModal.open, "enter closes it")
	assert.False(t, m.pager.open, "enter doesn't act on the index")
}

func TestHelpDrawsOverTheBody(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, keyPress('?', 0))

	v := m.View()
	assert.Equal(t, 24, lipgloss.Height(v.Content), "the view still fills the terminal")
	screen := plain(v.Content)
	assert.Contains(t, screen, "navigation")
	assert.Regexp(t, `keybinds +esc close`, screen)
	assert.Contains(t, screen, "/ press / to filter by command or shortcut")
	assert.Contains(t, screen, "search / · scroll j/k/↑↓/pgup/pgdn · close esc/enter")
	assert.Contains(t, screen, "jstlogger", "the header still shows")

	lines := strings.Split(screen, "\n")
	topY := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "┌") })
	require.Positive(t, topY, "the box starts below the header")
	top := lines[topY]
	left := lipgloss.Width(top[:strings.Index(top, "┌")])
	right := 80 - lipgloss.Width(top[:strings.Index(top, "┐")]) - 1
	assert.InDelta(t, right, left, 2, "the box is centered: %q", top)
	assert.GreaterOrEqual(t, 80-left-right, m.bodyWidth()*4/5, "the box takes four fifths of the body's width")
	assert.Greater(t, topY, 1, "the box is shorter than the body")
	assert.Contains(t, lines[1], "Date", "the index shows above the box")
}

func TestHelpScrollBar(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, keyPress('?', 0))
	assert.Contains(t, modalText(t, m), "┃", "the rows overflow, so a bar shows")

	m = press(t, m, keyPress('/', 0))
	m = typeText(t, m, "quit")
	assert.NotContains(t, modalText(t, m), "┃", "a filter that fits needs no bar")
}

func TestHelpGroupsHaveHeadings(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 40)
	help := helpText(m)
	for _, heading := range []string{"general", "navigation", "QSOs", "view", "search", "marks"} {
		assert.Regexp(t, `(?m)^`+heading+`$`, help)
	}
	assert.NotRegexp(t, `(?m)^rig$`, help, "no rig, so no rig keys")

	m, _ = newIndexModel(t, 3, 80, 40, WithRig("test", &fakeRig{}))
	assert.Regexp(t, `(?m)^rig$`, helpText(m))
}

func TestHelpFilter(t *testing.T) {
	m, _ := newIndexModel(t, 3, 100, 40)
	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/search")

	got := modalText(t, m)
	assert.Contains(t, got, "/ search")
	assert.Contains(t, got, "apply enter · clear esc", "the hint is for typing a filter")
	assert.Contains(t, got, "search back")
	assert.NotContains(t, got, "next page")
	assert.NotContains(t, got, "navigation", "a section with no match has no heading")

	m = press(t, m, enter())
	assert.True(t, m.helpModal.open, "enter keeps the filter")
	assert.False(t, m.helpModal.filtering)
	assert.Equal(t, "search", m.helpModal.filter())
	assert.NotContains(t, modalText(t, m), "next page")

	m = press(t, m, keyPress('/', 0), esc())
	assert.True(t, m.helpModal.open, "esc clears the filter")
	assert.Empty(t, m.helpModal.filter())
	assert.Contains(t, modalText(t, m), "press / to filter")
}

func TestHelpFilterTypesBrowsingKeys(t *testing.T) {
	m, _ := newIndexModel(t, 3, 100, 40)
	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/jk?")

	assert.True(t, m.helpModal.open, "? is text in the filter")
	assert.Equal(t, "jk?", m.helpModal.filter())
	assert.Zero(t, m.helpModal.scroll)
}

func TestHelpFilterByKey(t *testing.T) {
	m, _ := newIndexModel(t, 3, 100, 40)
	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/CTRL+T")

	got := modalText(t, m)
	assert.Contains(t, got, "unmark pattern", "case doesn't matter")
	assert.NotContains(t, got, "mark-prefix")
}

func TestHelpFilterErase(t *testing.T) {
	m, _ := newIndexModel(t, 3, 100, 40)
	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/zz")
	m = press(t, m, backspaceKey)
	assert.Equal(t, "z", m.helpModal.filter())
	m = press(t, m, backspaceKey, backspaceKey)
	assert.Empty(t, m.helpModal.filter())
	assert.True(t, m.helpModal.open, "backspace on an empty filter does nothing")
}

func TestHelpFilterNoMatches(t *testing.T) {
	m, _ := newIndexModel(t, 3, 100, 40)
	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/xyzzy")

	assert.Contains(t, modalText(t, m), "no matches")
}

func TestHelpFilterResetsOnOpen(t *testing.T) {
	m, _ := newIndexModel(t, 3, 100, 40)
	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/xyz")
	m = press(t, m, enter(), esc(), keyPress('?', 0))

	assert.Empty(t, m.helpModal.filter())
	assert.False(t, m.helpModal.filtering)
	assert.Zero(t, m.helpModal.scroll)
}

func TestHelpScrolls(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 16)
	m = press(t, m, keyPress('?', 0))
	top := modalText(t, m)
	require.Contains(t, top, "general")
	require.NotContains(t, top, "untag marked", "the window is too short for every key")

	m = press(t, m, helpDownKey)
	assert.Equal(t, 1, m.helpModal.scroll)
	m = press(t, m, keyPress('n', tea.ModCtrl))
	assert.Equal(t, 2, m.helpModal.scroll)
	m = press(t, m, keyPress('j', 0))
	assert.Equal(t, 3, m.helpModal.scroll)
	m = press(t, m, keyPress('k', 0), keyPress('p', tea.ModCtrl), helpUpKey, helpUpKey)
	assert.Equal(t, 0, m.helpModal.scroll, "up stops at the top")

	listHeight := m.helpListHeight()
	m = press(t, m, special(tea.KeyPgDown))
	assert.Equal(t, listHeight, m.helpModal.scroll, "pgdn scrolls a page")
	m = press(t, m, special(tea.KeyPgUp))
	assert.Zero(t, m.helpModal.scroll)

	for range 100 {
		m = press(t, m, helpDownKey)
	}
	assert.Contains(t, modalText(t, m), "untag marked", "down stops at the bottom")
	bottom := m.helpModal.scroll
	m = press(t, m, helpDownKey)
	assert.Equal(t, bottom, m.helpModal.scroll)
}

func TestHelpFilterScrollsToTheTop(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 16)
	m = press(t, m, keyPress('?', 0), helpDownKey, helpDownKey)
	m = typeText(t, m, "/e")
	assert.Zero(t, m.helpModal.scroll)

	m = press(t, m, helpDownKey)
	assert.Equal(t, 1, m.helpModal.scroll, "↓ scrolls while typing a filter")
	m = press(t, m, keyPress('n', tea.ModCtrl))
	assert.Equal(t, 2, m.helpModal.scroll, "and so does ctrl+n")
}

func TestHelpWheelScrolls(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 16)
	m = press(t, m, keyPress('?', 0))
	cursor := m.idx.cursor

	m = send(t, m, wheelDown())
	assert.Equal(t, wheelLines, m.helpModal.scroll)
	assert.Equal(t, cursor, m.idx.cursor, "the index behind doesn't move")
	m = send(t, m, wheelUp())
	assert.Zero(t, m.helpModal.scroll)
}

func TestHelpClickDoesNothing(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	click := clickOn(t, m, 2, "K3A")
	m = press(t, m, keyPress('?', 0))

	m = send(t, m, click)
	assert.True(t, m.helpModal.open)
	assert.False(t, m.pager.open)
}

func TestHelpMatchesTheIndex(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 40)
	help := helpText(m)
	for _, want := range []string{
		"next page", "half page down", "scroll up", "middle of page", "first", "last", "reload", "limit",
		"reverse sort", "mark pattern", "unmark pattern", "search back", "search next", "search opposite",
		"?/f1", "quit",
	} {
		assert.Contains(t, help, want)
	}
	assert.Regexp(t, `\bo +sort\b`, help)
	assert.NotContains(t, help, "QRZ")
	assert.NotContains(t, help, "save & close")
}

func TestHelpMatchesThePager(t *testing.T) {
	m := newLookupModel(t, "W1AW", 120, WithURLOpener((&fakeOpener{}).open))
	help := helpText(m)
	for _, want := range []string{"lookup", "gq", "QRZ", "next tab", "page down", "ctrl+c"} {
		assert.Contains(t, help, want)
	}
	assert.NotContains(t, help, "half page down", "the index keys aren't the pager's")

	m = press(t, m, keyPress('?', 0))
	m = typeText(t, m, "/qrz")
	assert.Contains(t, modalText(t, m), "QRZ")
	assert.True(t, m.pager.open)
	m = press(t, m, esc(), esc())
	assert.False(t, m.helpModal.open)
	assert.True(t, m.pager.open, "esc closes help, not the pager")
}

func TestHelpMatchesTheEntry(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)
	m = press(t, m, f1Key)
	require.True(t, m.helpModal.open)

	got := modalText(t, m)
	assert.Contains(t, got, "save & close")
	assert.Contains(t, got, "tabs")
	assert.NotContains(t, got, "show QSO")

	m = press(t, m, esc())
	assert.False(t, m.helpModal.open)
	assert.True(t, m.entry.open, "esc closes help, not the form")
	m = typeText(t, m, "w")
	assert.Equal(t, "w", entryValue(m, qsoentry.Call))
}

func TestHelpEntryQuestionMarkIsTyped(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)
	m = typeText(t, m, "?")

	assert.False(t, m.helpModal.open)
	assert.Equal(t, "?", entryValue(m, qsoentry.Call))
}

func TestHelpEntryPasteIsNotTyped(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)
	m = press(t, m, f1Key)

	m, _ = update(t, m, tea.PasteMsg{Content: "save"})
	assert.Empty(t, entryValue(m, qsoentry.Call), "the form behind gets no input")
	assert.Equal(t, "save", m.helpModal.filter(), "the paste filters")
	assert.True(t, m.helpModal.filtering)
	assert.Contains(t, modalText(t, m), "save & close")
}

func TestHelpMultiLinePasteStaysOnOneLine(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, keyPress('?', 0))
	m, _ = update(t, m, tea.PasteMsg{Content: "a\nb\r\nc"})

	assert.NotContains(t, m.helpModal.filter(), "\n")
	assert.Equal(t, 24, lipgloss.Height(m.View().Content), "the view still fills the terminal")
}

func TestHelpTinyTerminal(t *testing.T) {
	for _, height := range []int{3, 4, 5, 6} {
		m, _ := newIndexModel(t, 3, 40, height)
		m = press(t, m, keyPress('?', 0), helpDownKey)
		m = send(t, m, wheelDown())
		assert.Equal(t, height, lipgloss.Height(m.View().Content), "height %d", height)
	}
}

func TestHelpMatchesThePickList(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Mode)
	m = press(t, m, enterKey)
	require.True(t, m.entry.list.open)

	m = press(t, m, f1Key)
	got := modalText(t, m)
	assert.Contains(t, got, "pick list")
	assert.Regexp(t, `enter +pick`, got)

	m = press(t, m, esc())
	assert.True(t, m.entry.list.open, "esc closes help, not the list")
}

func TestHelpMatchesThePrompt(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, keyPress('l', 0))
	require.Equal(t, promptLimit, m.prompt)

	m = typeText(t, m, "?")
	assert.False(t, m.helpModal.open, "? is typed into the prompt")
	assert.Equal(t, "?", m.input.Value())

	m = press(t, m, f1Key)
	got := modalText(t, m)
	assert.Contains(t, got, "submit")
	assert.Contains(t, got, "prev pattern")
	assert.NotContains(t, got, "show QSO")

	m = press(t, m, esc())
	assert.Equal(t, promptLimit, m.prompt, "esc closes help, not the prompt")
	assert.Equal(t, "?", m.input.Value())
}

func TestHelpFooterWhileOpen(t *testing.T) {
	m, _ := newIndexModel(t, 3, 120, 24)
	m = press(t, m, keyPress('?', 0))
	footer := plain(m.footerView())
	assert.Contains(t, footer, "esc/enter close")
	assert.Contains(t, footer, "/ filter")
	assert.NotContains(t, footer, "jump to QSO")

	m = press(t, m, keyPress('/', 0))
	footer = plain(m.footerView())
	assert.Contains(t, footer, "enter apply")
	assert.Contains(t, footer, "esc clear")
	assert.NotContains(t, footer, "/ filter")
	m = press(t, m, esc())

	m = press(t, m, esc())
	footer = plain(m.footerView())
	assert.Contains(t, footer, "jump to QSO", "the short help is back")
	assert.Contains(t, footer, "?/f1 help")
	assert.Equal(t, 1, lipgloss.Height(footer), "the footer is one line")
}

func TestHelpModalWithoutLogbook(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	m = press(t, m, keyPress('?', 0))

	got := modalText(t, m)
	assert.Contains(t, got, "quit")
	assert.NotContains(t, got, "navigation", "the index keys are off")
}

func TestHelpCtrlCQuits(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, keyPress('?', 0))
	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
	require.NotNil(t, cmd)
	assert.Contains(t, collectMsgs(cmd), tea.QuitMsg{})
}

func TestFullHelpIsTheSections(t *testing.T) {
	for _, k := range []helpKeyMap{
		defaultKeys(true), defaultPromptKeys(), defaultIndexKeys(true),
		defaultPagerKeys(true, defaultIndexKeys(true)), defaultEntryKeys(), pickListKeys,
		joinedKeyMap{defaultKeys(true), defaultIndexKeys(true)},
	} {
		sections := k.sections()
		full := k.FullHelp()
		require.Len(t, full, len(sections), "%T", k)
		for i, s := range sections {
			assert.NotEmpty(t, s.title, "%T", k)
			assert.Equal(t, s.keys, full[i], "%T %s", k, s.title)
		}
	}
	assert.Equal(t, [][]key.Binding{helpModalKeys.ShortHelp()}, helpModalKeys.FullHelp())
}

func TestHelpF1InThePager(t *testing.T) {
	m := newLookupModel(t, "W1AW", 120)
	m = press(t, m, f1Key)
	require.True(t, m.helpModal.open)
	assert.Contains(t, modalText(t, m), "next QSO")

	m = press(t, m, keyPress('?', 0))
	assert.False(t, m.helpModal.open)
	assert.True(t, m.pager.open)
}
