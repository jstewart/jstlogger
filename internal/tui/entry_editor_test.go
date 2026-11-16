package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

// editorKey opens the focused field in the editor.
var editorKey = keyPress('g', tea.ModCtrl)

// newEditorModel returns a model with the New QSO screen open, its editor
// $VISUAL set to visual, and temporary files made in a directory of the
// test's own, which it returns.
func newEditorModel(t *testing.T, visual string) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	m := newEntryModel(t, newEntryStore(t, nil))
	m.getenv = func(k string) string {
		if k == "VISUAL" {
			return visual
		}
		return ""
	}
	return m, dir
}

// tempFiles returns the files in dir.
func tempFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err)
	return files
}

// openEditor presses ctrl+g on field f holding value, and returns the
// model, the command and the temporary file made for the editor.
func openEditor(t *testing.T, m Model, dir string, f qsoentry.Field, value string) (Model, tea.Cmd, string) {
	t.Helper()
	m = fill(t, m, map[qsoentry.Field]string{f: value})
	m, cmd := update(t, m, editorKey)
	files := tempFiles(t, dir)
	require.Len(t, files, 1, "ctrl+g makes a file for the editor")
	return m, cmd, files[0]
}

// entryStatus returns the New QSO screen's status line.
func entryStatus(m Model) string {
	lines := strings.Split(plain(m.entryView(100, 20)), "\n")
	return lines[len(lines)-1]
}

func TestEntryEditorOpensFile(t *testing.T) {
	for _, f := range []qsoentry.Field{qsoentry.Comment, qsoentry.Notes, qsoentry.QSLMsg} {
		t.Run(entryLabels[f], func(t *testing.T) {
			m, dir := newEditorModel(t, "true")
			m, cmd, path := openEditor(t, m, dir, f, "hello")
			assert.NotNil(t, cmd, "ctrl+g runs the editor")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "hello", string(data), "the file holds the value")
			assert.Equal(t, "hello", entryValue(m, f), "ctrl+g doesn't select or change the text")
			assert.Equal(t, f, m.entry.focus)
		})
	}
}

func TestEntryEditorOtherFieldsDoNotOpen(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m = focusOn(t, m, qsoentry.Call)
	_, cmd := update(t, m, editorKey)
	assert.Nil(t, cmd)
	assert.Empty(t, tempFiles(t, dir), "no file for the editor")
}

func TestEntryEditorIgnoredWhileSaving(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m = focusOn(t, m, qsoentry.Comment)
	m.entry.saving = true
	_, cmd := update(t, m, editorKey)
	assert.Nil(t, cmd)
	assert.Empty(t, tempFiles(t, dir))
}

func TestEntryEditorBadCommand(t *testing.T) {
	m, dir := newEditorModel(t, `"unterminated`)
	m = focusOn(t, m, qsoentry.Comment)
	m, cmd := update(t, m, editorKey)
	assert.Nil(t, cmd)
	assert.Empty(t, tempFiles(t, dir))
	require.Error(t, m.entry.err)
	assert.Contains(t, entryStatus(m), "unterminated double quote")
}

func TestEntryEditorReadsBack(t *testing.T) {
	tests := []struct {
		field  qsoentry.Field
		edited string
		want   string
	}{
		{qsoentry.Comment, "  line one\nline   two\n", "line one line two"},
		{qsoentry.Notes, "line one\r\nline two\n\n", "line one\nline two"},
		{qsoentry.QSLMsg, "tnx\nfer QSO\n", "tnx\nfer QSO"},
	}
	for _, tt := range tests {
		t.Run(entryLabels[tt.field], func(t *testing.T) {
			m, dir := newEditorModel(t, "true")
			m, _, path := openEditor(t, m, dir, tt.field, "old")
			require.NoError(t, os.WriteFile(path, []byte(tt.edited), 0o600))

			m.entry.base = m.entry.form()
			require.False(t, m.entry.unsaved())
			m, _ = update(t, m, entryEditedMsg{gen: m.entry.gen, field: tt.field, path: path})

			assert.Equal(t, tt.want, entryValue(m, tt.field))
			assert.Empty(t, tempFiles(t, dir), "the file is removed")
			assert.True(t, m.entry.unsaved(), "the edit is an unsaved change")
			assert.Equal(t, tt.field, m.entry.focus, "the field keeps focus")
			assert.NoError(t, m.entry.err)
		})
	}
}

func TestEntryEditorErrorKeepsValue(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m, _, path := openEditor(t, m, dir, qsoentry.Notes, "old")
	require.NoError(t, os.WriteFile(path, []byte("new"), 0o600))

	m, _ = update(t, m, entryEditedMsg{gen: m.entry.gen, field: qsoentry.Notes, path: path, err: errors.New("exit status 1")})

	assert.Equal(t, "old", entryValue(m, qsoentry.Notes))
	assert.Empty(t, tempFiles(t, dir), "the file is removed")
	require.Error(t, m.entry.err)
	assert.Contains(t, entryStatus(m), "edit Notes: exit status 1")
}

func TestEntryEditorReadBackError(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m, _, path := openEditor(t, m, dir, qsoentry.Comment, "old")
	require.NoError(t, os.Remove(path))

	m, _ = update(t, m, entryEditedMsg{gen: m.entry.gen, field: qsoentry.Comment, path: path})

	assert.Equal(t, "old", entryValue(m, qsoentry.Comment))
	require.Error(t, m.entry.err)
	assert.Contains(t, entryStatus(m), "read editor file")
}

func TestEntryEditorStaleIgnored(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m, _, path := openEditor(t, m, dir, qsoentry.Comment, "old")
	require.NoError(t, os.WriteFile(path, []byte("new"), 0o600))

	stale := entryEditedMsg{gen: m.entry.gen - 1, field: qsoentry.Comment, path: path}
	m, _ = update(t, m, stale)
	assert.Equal(t, "old", entryValue(m, qsoentry.Comment), "an earlier opening's edit is dropped")
	assert.Empty(t, tempFiles(t, dir), "the file is removed")
	assert.NoError(t, m.entry.err)
}

func TestEntryEditorClosedIgnored(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m, _, path := openEditor(t, m, dir, qsoentry.Comment, "old")
	gen := m.entry.gen
	m = m.closeEntry()

	m, _ = update(t, m, entryEditedMsg{gen: gen, field: qsoentry.Comment, path: path})
	assert.False(t, m.entry.open)
	assert.Empty(t, entryValue(m, qsoentry.Comment))
	assert.Empty(t, tempFiles(t, dir), "the file is removed")
}

func TestEntryEditorDroppedReadBackError(t *testing.T) {
	m, dir := newEditorModel(t, "true")
	m, _, path := openEditor(t, m, dir, qsoentry.Comment, "old")
	gen := m.entry.gen
	require.NoError(t, os.Remove(path))

	stale, _ := update(t, m, entryEditedMsg{gen: gen - 1, field: qsoentry.Comment, path: path})
	require.Error(t, stale.entry.err, "an earlier opening's read error shows on the open screen")
	assert.Contains(t, entryStatus(stale), "read editor file")

	closed := m.closeEntry()
	closed, _ = update(t, closed, entryEditedMsg{gen: gen, field: qsoentry.Comment, path: path})
	assert.Contains(t, closed.idx.msg, "read editor file", "a closed screen's read error shows on the index")
}

func TestEntryEditorHelp(t *testing.T) {
	m, _ := newEditorModel(t, "true")
	m.help.ShowAll = true
	m = focusOn(t, m, qsoentry.Call)
	assert.NotRegexp(t, `ctrl\+g`, plain(m.footerView()), "no editor for the call")

	for _, f := range []qsoentry.Field{qsoentry.Comment, qsoentry.Notes, qsoentry.QSLMsg} {
		m = focusOn(t, m, f)
		assert.Regexp(t, `ctrl\+g\s+edit in \$EDITOR`, plain(m.footerView()), entryLabels[f])
	}
	// The short help is cut to the window, and ctrl+g follows the tab keys.
	m.help.ShowAll = false
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	assert.Regexp(t, `ctrl\+g\s+edit in \$EDITOR`, plain(m.footerView()), "the short help too")
}
