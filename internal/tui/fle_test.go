package tui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	cbmock "jstlogger/internal/callbook/mock"
	"jstlogger/internal/callinfo"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

var (
	fleKey     = keyPress('A', 0)
	fleSaveKey = keyPress('s', 0)
	fleEditKey = keyPress('e', 0)
	fleEscKey  = special(tea.KeyEscape)
)

// fleNow is the time the FLE tests run at.
var fleNow = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

// fleText logs two QSOs, K1ABC on 20m CW and W1AW on 40m FT4, and has a
// line the parser can't read, line 4.
const fleText = "date 2026-11-27\n20m cw\n1000 K1ABC 599 579\n1001 ???\n40m ft4\n1005 W1AW\n"

// newFLEModel returns a model on a logbook of n QSOs, its editor $VISUAL
// set to visual, and temporary files made in a directory of the test's
// own, which it returns.
func newFLEModel(t *testing.T, n int, visual string, opts ...Option) (Model, *logbook.SQLiteStore, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	s := newPatternStore(t, n)
	m := New(append([]Option{WithLogbook(s), WithClock(func() time.Time { return fleNow })}, opts...)...)
	m.getenv = func(k string) string {
		if k == "VISUAL" {
			return visual
		}
		return ""
	}
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return runIndex(t, m, cmd), s, dir
}

// openFLE presses A and returns the model, the command and the temporary
// file made for the editor.
func openFLE(t *testing.T, m Model, dir string) (Model, tea.Cmd, string) {
	t.Helper()
	m, cmd := update(t, m, fleKey)
	files := tempFiles(t, dir)
	require.Len(t, files, 1, "A makes a file for the editor")
	return m, cmd, files[0]
}

// enterFLE presses A, writes text into the editor's file and returns the
// model once the editor exits.
func enterFLE(t *testing.T, m Model, dir, text string) Model {
	t.Helper()
	m, _, path := openFLE(t, m, dir)
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	m, _ = update(t, m, fleEditedMsg{path: path})
	return m
}

// runFLESave runs cmd, the save it starts and the index loads that follow.
func runFLESave(t *testing.T, m Model, cmd tea.Cmd) Model {
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
		case fleSavedMsg, indexLoadedMsg, indexBlockMsg:
			var next tea.Cmd
			m, next = update(t, m, msg)
			queue = append(queue, next)
		}
	}
	return m
}

// fleDialogText returns the plain FLE dialog as drawn over the body.
func fleDialogText(t *testing.T, m Model) string {
	t.Helper()
	require.True(t, m.fle.open)
	return plain(m.fleView(m.bodyWidth(), m.bodyHeight()))
}

// storedCount returns the number of stored QSOs.
func storedCount(t *testing.T, s logbook.Store) int {
	t.Helper()
	qsos, err := s.Index(t.Context(), nil, logbook.Sort{}, 0, 1000)
	require.NoError(t, err)
	return len(qsos)
}

func TestFLEKeyNeedsLogbook(t *testing.T) {
	m := New()
	assert.NotContains(t, helpText(m), "enter QSOs as FLE")
	m, cmd := update(t, m, fleKey)
	assert.Nil(t, cmd)
	assert.False(t, m.fle.open)

	m, _, _ = newFLEModel(t, 1, "true")
	assert.Regexp(t, `QSOs\n(.*\n)*a +new QSO\nA +enter QSOs as FLE\n`, helpText(m))
}

func TestFLEOpensEmptyFile(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m, cmd, path := openFLE(t, m, dir)
	assert.NotNil(t, cmd, "A runs the editor")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Empty(t, string(data), "the editor starts on an empty buffer")
	assert.False(t, m.fle.open)
}

func TestFLEEditorBadCommand(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, `"unterminated`)
	m, cmd := update(t, m, fleKey)
	assert.Nil(t, cmd)
	assert.Empty(t, tempFiles(t, dir))
	assert.False(t, m.fle.open)
	assert.Contains(t, statusLine(t, m), "unterminated double quote")
}

func TestFLEEditorError(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m, _, path := openFLE(t, m, dir)
	require.NoError(t, os.WriteFile(path, []byte(fleText), 0o600))

	m, _ = update(t, m, fleEditedMsg{path: path, err: errors.New("exit status 1")})

	assert.False(t, m.fle.open)
	assert.Empty(t, tempFiles(t, dir), "the file is removed")
	assert.Contains(t, statusLine(t, m), "edit FLE: exit status 1")
}

func TestFLEBlankEditThenEditorErrorOpensNothing(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, fleText)
	require.True(t, m.fle.open)

	// e, then the text is blanked.
	m, _ = update(t, m, tea.KeyPressMsg{Code: 'e', Text: "e"})
	paths := tempFiles(t, dir)
	require.Len(t, paths, 1)
	require.NoError(t, os.WriteFile(paths[0], []byte("\n"), 0o600))
	m, _ = update(t, m, fleEditedMsg{path: paths[0]})
	require.False(t, m.fle.open)

	// A, then the editor fails.
	m, _, path := openFLE(t, m, dir)
	m, _ = update(t, m, fleEditedMsg{path: path, err: errors.New("exit status 1")})

	assert.False(t, m.fle.open, "no dialog from the earlier text comes back")
	assert.Empty(t, m.fle.text)
}

func TestFLEEditorErrorOnEditKeepsText(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, fleText)
	require.True(t, m.fle.open)
	before := m.fle.result

	m, _ = update(t, m, tea.KeyPressMsg{Code: 'e', Text: "e"})
	paths := tempFiles(t, dir)
	require.Len(t, paths, 1)
	m, _ = update(t, m, fleEditedMsg{path: paths[0], err: errors.New("exit status 1")})

	assert.True(t, m.fle.open, "the dialog comes back on the text edited before")
	assert.Equal(t, fleText, m.fle.text)
	assert.Equal(t, before, m.fle.result)
	assert.Empty(t, tempFiles(t, dir), "the file is removed")
	assert.Contains(t, statusLine(t, m), "edit FLE: exit status 1")
}

func TestFLEReadBackError(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m, _, path := openFLE(t, m, dir)
	require.NoError(t, os.Remove(path))

	m, _ = update(t, m, fleEditedMsg{path: path})

	assert.False(t, m.fle.open)
	assert.Contains(t, statusLine(t, m), "read editor file")
}

func TestFLEEmptyText(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, "  \n\t\n\n")
	assert.False(t, m.fle.open)
	assert.Empty(t, tempFiles(t, dir), "the file is removed")
	assert.Contains(t, statusLine(t, m), "No QSOs entered.")
}

func TestFLEDialogListsQSOs(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, fleText)
	require.True(t, m.fle.open)
	assert.Empty(t, tempFiles(t, dir), "the file is removed")

	view := fleDialogText(t, m)
	assert.Contains(t, view, "FLE: 2 QSOs, 1 line ignored")
	assert.Regexp(t, `2026-11-27 +10:00 +K1ABC +20m +CW +599 +579`, view)
	assert.Regexp(t, `2026-11-27 +10:05 +W1AW +40m +FT4 +-10 +-10`, view)
	assert.Contains(t, view, "1 line ignored")
	assert.Regexp(t, `line 4: 1001 \?\?\? — \S`, view)
	assert.Contains(t, view, "s save · e edit · esc cancel")

	// The dialog is drawn over the body.
	assert.Contains(t, plain(m.View().Content), "FLE: 2 QSOs, 1 line ignored")
}

func TestFLEDialogTitleWithoutIgnored(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, "date 2026-11-27\n20m cw\n1000 K1ABC\n")
	view := fleDialogText(t, m)
	assert.Contains(t, view, "FLE: 1 QSO")
	assert.NotContains(t, view, "ignored")
}

func TestFLEOnlyIgnoredLines(t *testing.T) {
	m, s, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, "foo bar\n")
	require.True(t, m.fle.open)
	view := fleDialogText(t, m)
	assert.Contains(t, view, "FLE: 0 QSOs, 1 line ignored")
	assert.NotContains(t, view, "s save")
	assert.Contains(t, view, "e edit · esc cancel")
	assert.NotContains(t, helpText(m), "save")

	m, cmd := update(t, m, fleSaveKey)
	assert.Nil(t, cmd, "nothing to save")
	assert.True(t, m.fle.open)
	assert.Equal(t, 1, storedCount(t, s))
}

func TestFLEEditReopensWithText(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	text := "date 2026-11-27\n20m cw\n\n1000 K1ABC   \r\nfoo\n\n"
	m = enterFLE(t, m, dir, text)
	require.True(t, m.fle.open)

	m, cmd := update(t, m, fleEditKey)
	assert.NotNil(t, cmd, "e runs the editor")
	assert.False(t, m.fle.open, "the dialog closes while the editor runs")
	files := tempFiles(t, dir)
	require.Len(t, files, 1)
	data, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Equal(t, text, string(data), "the editor gets the text exactly as saved")

	// The edited text is parsed again.
	require.NoError(t, os.WriteFile(files[0], []byte("date 2026-11-27\n20m cw\n1000 K1ABC\n1001 K1ABD\n"), 0o600))
	m, _ = update(t, m, fleEditedMsg{path: files[0]})
	require.True(t, m.fle.open)
	assert.Contains(t, fleDialogText(t, m), "FLE: 2 QSOs")
}

func TestFLECancelStoresNothing(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{fleEscKey, keyPress('q', 0)} {
		t.Run(k.String(), func(t *testing.T) {
			m, s, dir := newFLEModel(t, 1, "true")
			m = enterFLE(t, m, dir, fleText)
			m, cmd := update(t, m, k)
			assert.False(t, m.fle.open)
			if cmd != nil {
				for _, msg := range collectMsgs(cmd) {
					assert.NotEqual(t, tea.QuitMsg{}, msg, "q cancels rather than quits")
				}
			}
			assert.Contains(t, statusLine(t, m), "FLE discarded.")
			assert.Equal(t, 1, storedCount(t, s))
		})
	}
}

func TestFLECtrlCQuits(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, fleText)
	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestFLEKeysDoNotReachIndex(t *testing.T) {
	m, _, dir := newFLEModel(t, 5, "true")
	m = enterFLE(t, m, dir, fleText)
	cursor := m.idx.cursor
	m = press(t, m, keyPress('j', 0), keyPress('k', 0), keyPress('t', 0), keyPress('d', 0), special(tea.KeyEnter), keyPress('a', 0))
	assert.True(t, m.fle.open)
	assert.Equal(t, cursor, m.idx.cursor)
	assert.Empty(t, m.idx.marks)
	assert.False(t, m.pager.open)
	assert.False(t, m.entry.open)
	assert.False(t, m.promptOpen())

	m, _ = update(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: 4})
	assert.False(t, m.pager.open, "a click behind the dialog does nothing")
	m, _ = update(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 2, Y: 4})
	assert.Equal(t, cursor, m.idx.cursor, "the wheel doesn't move the index behind the dialog")
}

func TestFLEContextKeys(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, fleText)
	help := helpText(m)
	for _, want := range []string{"save", "edit", "cancel", "scroll"} {
		assert.Contains(t, help, want)
	}
	assert.NotContains(t, help, "new QSO")
	assert.Contains(t, plain(m.footerView()), "s save")

	m = press(t, m, keyPress('?', 0))
	require.True(t, m.helpModal.open, "? opens help over the dialog")
	m = press(t, m, fleEscKey)
	assert.False(t, m.helpModal.open)
	assert.True(t, m.fle.open, "esc closes the help, not the dialog")
}

func TestFLEDialogScrolls(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	var b strings.Builder
	b.WriteString("date 2026-11-27\n20m cw\n")
	for i := range 40 {
		b.WriteString("10" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + " K1A" + string(rune('A'+i%26)) + "\n")
	}
	m = enterFLE(t, m, dir, b.String())
	require.True(t, m.fle.open)
	assert.Contains(t, fleDialogText(t, m), "10:00")
	assert.NotContains(t, fleDialogText(t, m), "10:39")

	m = press(t, m, keyPress('j', 0))
	assert.Equal(t, 1, m.fle.scroll)
	m = press(t, m, keyPress('k', 0), keyPress('k', 0))
	assert.Equal(t, 0, m.fle.scroll, "the scroll stops at the top")
	m = press(t, m, special(tea.KeyPgDown), special(tea.KeyPgDown), special(tea.KeyPgDown))
	assert.Contains(t, fleDialogText(t, m), "10:39", "the scroll reaches the bottom")
	assert.NotContains(t, fleDialogText(t, m), "10:00")

	m, _ = update(t, m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Less(t, m.fle.scroll, 40, "the wheel scrolls the dialog")
}

func TestFLESave(t *testing.T) {
	station := qsoentry.Station{Name: "home", Callsign: "N0CALL", Grid: "FN31pr"}
	l := newLooker(t, map[string]callbook.Result{"W1AW": w1awResult})
	m, s, dir := newFLEModel(t, 3, "true", WithStations([]qsoentry.Station{station}), WithCallbook(l))
	require.Equal(t, 3, m.idx.total)
	m = enterFLE(t, m, dir, fleText)

	m, cmd := update(t, m, fleSaveKey)
	assert.False(t, m.fle.open)
	require.NotNil(t, cmd)
	assert.Contains(t, statusLine(t, m), "Saving 2 QSOs…")

	m = runFLESave(t, m, cmd)
	assert.Contains(t, statusLine(t, m), "Added 2 QSOs.")
	assert.NotContains(t, statusLine(t, m), "Saving")
	assert.Equal(t, 5, m.idx.total, "the index reloads")
	assert.Equal(t, 5, storedCount(t, s))

	got := savedQSO(t, s, "W1AW")
	assert.Equal(t, "N0CALL", got.StationCallsign)
	assert.Equal(t, "FN31PR", got.MyGridsquare)
	assert.Equal(t, "Hiram Maxim", got.Name, "the callbook fills in the name")
	assert.Equal(t, "FT4", got.Submode)
	assert.NotContains(t, got.Tags, callinfo.PendingTag)
}

func TestFLESaveSkipsDuplicates(t *testing.T) {
	m, s, dir := newFLEModel(t, 2, "true")
	// QSO 1 of newPatternStore, and the same QSO again.
	m = enterFLE(t, m, dir, "date 2026-10-31\n40m ssb\n1201 K1A\n1201 K1A\n1210 K9Z\n")
	m, cmd := update(t, m, fleSaveKey)
	m = runFLESave(t, m, cmd)
	assert.Contains(t, statusLine(t, m), "Added 1 QSO, skipped 2 duplicates.")
	assert.Equal(t, 3, storedCount(t, s))
}

func TestFLESaveKeepsEnteredValues(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"W1AW": w1awResult})
	m, s, dir := newFLEModel(t, 1, "true", WithCallbook(l))
	m = enterFLE(t, m, dir, "date 2026-11-27\n20m cw\n1000 W1AW FN42 @Bob\n")
	m, cmd := update(t, m, fleSaveKey)
	runFLESave(t, m, cmd)

	got := savedQSO(t, s, "W1AW")
	assert.Equal(t, "Bob", got.Name, "a name entered is kept")
	assert.Equal(t, "FN42", got.Gridsquare, "a grid entered is kept")
	assert.Equal(t, "Newington", got.QTH, "an empty field is filled in")
}

func TestFLESaveUnreachableCallbook(t *testing.T) {
	netErr := &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("connection refused")}
	l := cbmock.NewMockLooker(gomock.NewController(t))
	// Only the first QSO is looked up in the callbook.
	l.EXPECT().Lookup(gomock.Any(), "K1ABC").Return(callbook.Result{}, netErr).Times(1)
	m, s, dir := newFLEModel(t, 1, "true", WithCallbook(l))
	m = enterFLE(t, m, dir, "date 2026-11-27\n20m cw\n1000 K1ABC\n1001 K1ABD\n1002 K1ABE\n")
	m, cmd := update(t, m, fleSaveKey)
	m = runFLESave(t, m, cmd)

	assert.Contains(t, statusLine(t, m), "Added 3 QSOs. Callbook unreachable: tagged callbook-pending.")
	for _, call := range []string{"K1ABC", "K1ABD", "K1ABE"} {
		assert.Contains(t, savedQSO(t, s, call).Tags, callinfo.PendingTag, call)
	}
}

func TestFLESaveOtherLookupErrorNotTagged(t *testing.T) {
	m, s, dir := newFLEModel(t, 1, "true", WithCallbook(failingLooker(t, callbook.ErrAuth)))
	m = enterFLE(t, m, dir, "date 2026-11-27\n20m cw\n1000 K1ABC\n")
	m, cmd := update(t, m, fleSaveKey)
	m = runFLESave(t, m, cmd)
	assert.Contains(t, statusLine(t, m), "Added 1 QSO.")
	assert.NotContains(t, savedQSO(t, s, "K1ABC").Tags, callinfo.PendingTag)
}

// failingImportStore is a logbook whose imports fail.
type failingImportStore struct {
	logbook.Store
}

func (failingImportStore) Import(context.Context, []logbook.QSO, []string) (logbook.ImportResult, error) {
	return logbook.ImportResult{}, errors.New("disk full")
}

func TestFLESaveError(t *testing.T) {
	m, s, dir := newFLEModel(t, 1, "true")
	m.logbook = failingImportStore{Store: s}
	m = enterFLE(t, m, dir, fleText)
	m, cmd := update(t, m, fleSaveKey)
	m = runFLESave(t, m, cmd)
	assert.Contains(t, statusLine(t, m), "save FLE QSOs: disk full")
	assert.NotContains(t, statusLine(t, m), "Saving")
	assert.False(t, m.fleSaving)
}

func TestFLENotWhileSaving(t *testing.T) {
	m, _, dir := newFLEModel(t, 1, "true")
	m = enterFLE(t, m, dir, fleText)
	m, _ = update(t, m, fleSaveKey)
	require.True(t, m.fleSaving)

	m, cmd := update(t, m, fleKey)
	assert.Nil(t, cmd)
	assert.Empty(t, tempFiles(t, dir))
	assert.Contains(t, statusLine(t, m), "FLE QSOs are being saved.")
}
