package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
	"jstlogger/internal/pattern"
)

var (
	bulkDeleteKey = keyPress('d', 0)
	bulkSaveKey   = keyPress('s', 0)
	bulkTagKey    = keyPress('y', 0)
	bulkUntagKey  = keyPress('Y', 0)
)

// newBulkModel returns a model showing a SQLite logbook of n QSOs, with the
// first two rows, QSOs n and n-1, marked and the cursor on row 2.
func newBulkModel(t *testing.T, n int) (Model, *logbook.SQLiteStore) {
	t.Helper()
	s := newPatternStore(t, n)
	m := newPatternModel(t, s)
	m = press(t, m, toggleKey, toggleKey)
	require.Len(t, m.idx.marks, 2)
	return m, s
}

// idsMatching returns the IDs of the QSOs in s matching the pattern p.
func idsMatching(t *testing.T, s logbook.Store, p string) []int64 {
	t.Helper()
	e, err := pattern.Parse(p)
	require.NoError(t, err)
	ids, err := s.IDs(t.Context(), e)
	require.NoError(t, err)
	return ids
}

// countAll returns the number of QSOs in s.
func countAll(t *testing.T, s logbook.Store) int {
	t.Helper()
	n, err := s.Count(t.Context(), nil)
	require.NoError(t, err)
	return n
}

// unmarkAllMsg is the result of an unmark pattern that matched every QSO
// in a logbook of n.
func unmarkAllMsg(n int) indexMarksMsg {
	var ids []int64
	for i := n; i >= 1; i-- {
		ids = append(ids, int64(i))
	}
	return indexMarksMsg{ids: ids, add: false}
}

// savedCalls returns the CALL of every record in the ADI file at path.
func savedCalls(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { assert.NoError(t, f.Close()) }()
	file, err := adif.Parse(f)
	require.NoError(t, err)
	var calls []string
	for _, r := range file.Records {
		c, _ := r.Get("CALL")
		calls = append(calls, c)
	}
	return calls
}

func TestBulkPromptLabels(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		kind  promptKind
		label string
	}{
		{"y", bulkTagKey, promptTag, "Tag marked QSOs with: "},
		{"Y", bulkUntagKey, promptUntag, "Remove tag from marked QSOs: "},
		{"s", bulkSaveKey, promptSave, "Save marked QSOs to: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newBulkModel(t, 5)

			m = press(t, m, prefixKey, tt.key)

			require.Equal(t, tt.kind, m.prompt)
			assert.Equal(t, tt.label, m.input.Prompt)
			assert.Empty(t, m.input.Value())
		})
	}
}

func TestBulkTag(t *testing.T) {
	m, s := newBulkModel(t, 5)
	// A QSO logged behind the index's back shows the index reloads.
	addPatternQSO(t, s, 6)

	m = press(t, m, prefixKey, bulkTagKey)
	m = typeText(t, m, " pota ")
	m = press(t, m, enterKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, []int64{5, 4}, idsMatching(t, s, "~y pota"))
	assert.Equal(t, 6, m.idx.total, "the index reloads")
	assert.Equal(t, int64(3), currentID(t, m), "the cursor stays on its QSO")
	assert.Len(t, m.idx.marks, 2, "the marks stay")
	assert.Equal(t, "QSO 4/6 · 2 marked · Tagged 2 QSOs.", statusLine(t, m))
}

func TestBulkTagOne(t *testing.T) {
	s := newPatternStore(t, 5)
	m := newPatternModel(t, s)
	m = press(t, m, toggleKey, prefixKey, bulkTagKey)
	m = typeText(t, m, "pota")

	m = press(t, m, enterKey)

	assert.Equal(t, []int64{5}, idsMatching(t, s, "~y pota"))
	assert.Equal(t, "QSO 2/5 · 1 marked · Tagged 1 QSO.", statusLine(t, m))
}

func TestBulkUntag(t *testing.T) {
	m, s := newBulkModel(t, 5)
	require.NoError(t, s.Tag(t.Context(), []int64{1, 2, 3, 4, 5}, "pota"))

	m = press(t, m, prefixKey, bulkUntagKey)
	m = typeText(t, m, "pota")
	m = press(t, m, enterKey)

	assert.Equal(t, []int64{3, 2, 1}, idsMatching(t, s, "~y pota"))
	assert.Equal(t, "QSO 3/5 · 2 marked · Untagged 2 QSOs.", statusLine(t, m))
}

func TestBulkTagEmptyDoesNothing(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{bulkTagKey, bulkUntagKey} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newBulkModel(t, 5)
			require.NoError(t, s.Tag(t.Context(), []int64{5}, "pota"))
			m = press(t, m, prefixKey, k)
			m = typeText(t, m, "  ")

			m, cmd := update(t, m, enterKey)

			assert.Equal(t, promptNone, m.prompt)
			assert.Empty(t, collectMsgs(cmd))
			assert.Equal(t, []int64{5}, idsMatching(t, s, "~y pota"))
			assert.Equal(t, "QSO 3/5 · 2 marked", statusLine(t, m))
		})
	}
}

func TestBulkTagError(t *testing.T) {
	m, s := newBulkModel(t, 5)
	require.NoError(t, s.Delete(t.Context(), 5))

	m = press(t, m, prefixKey, bulkTagKey)
	m = typeText(t, m, "pota")
	m = press(t, m, enterKey)

	assert.Contains(t, m.idx.msg, "tag QSOs:")
	assert.Empty(t, idsMatching(t, s, "~y pota"), "nothing is tagged")
	assert.Len(t, m.idx.marks, 2)
}

func TestBulkDeleteConfirmation(t *testing.T) {
	m, _ := newBulkModel(t, 5)

	m = press(t, m, prefixKey, bulkDeleteKey)

	assert.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), "Delete 2 QSOs? [y/N]")
}

func TestBulkDeleteConfirmationOne(t *testing.T) {
	s := newPatternStore(t, 5)
	m := newPatternModel(t, s)

	m = press(t, m, toggleKey, prefixKey, bulkDeleteKey)

	assert.Contains(t, plain(m.footerView()), "Delete 1 QSO? [y/N]")
}

func TestBulkDeleteCancel(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyPress('n', 0), enterKey, escKey, keyPress('x', 0)} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newBulkModel(t, 5)
			m = press(t, m, prefixKey, bulkDeleteKey)

			m, cmd := update(t, m, k)

			assert.Equal(t, promptNone, m.prompt)
			assert.Empty(t, collectMsgs(cmd))
			assert.Equal(t, 5, countAll(t, s))
			assert.Len(t, m.idx.marks, 2)
			assert.Equal(t, "QSO 3/5 · 2 marked", statusLine(t, m))
		})
	}
}

func TestBulkDelete(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyPress('y', 0), keyPress('Y', 0)} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newBulkModel(t, 5)
			require.Equal(t, int64(3), currentID(t, m))
			m = press(t, m, prefixKey, bulkDeleteKey)

			m = press(t, m, k)

			assert.Equal(t, promptNone, m.prompt)
			assert.Equal(t, 3, countAll(t, s))
			assert.Equal(t, []int64{3, 2, 1}, idsMatching(t, s, "~A"))
			assert.Empty(t, m.idx.marks, "the deleted QSOs' marks are dropped")
			assert.Equal(t, 3, m.idx.total)
			assert.Equal(t, int64(3), currentID(t, m), "the cursor stays on its QSO")
			assert.Equal(t, "QSO 1/3 · Deleted 2 QSOs.", statusLine(t, m))
		})
	}
}

func TestBulkDeleteKeepsOtherMarks(t *testing.T) {
	m, s := newBulkModel(t, 5)
	m = press(t, m, prefixKey, bulkDeleteKey)
	m, cmd := update(t, m, keyPress('y', 0))
	// QSO 1 is marked while the delete runs; it is not deleted.
	m.idx.marks[1] = true

	m = runIndex(t, m, cmd)

	assert.Equal(t, 3, countAll(t, s))
	assert.Equal(t, map[int64]bool{1: true}, m.idx.marks)
}

func TestBulkDeleteCursorOnDeletedQSO(t *testing.T) {
	s := newPatternStore(t, 5)
	m := newPatternModel(t, s)
	// Mark rows 1 and 2, QSOs 4 and 3, and go back to row 1.
	m = press(t, m, keyPress('j', 0), toggleKey, toggleKey, keyPress('k', 0), keyPress('k', 0))
	require.Equal(t, int64(4), currentID(t, m))

	m = press(t, m, prefixKey, bulkDeleteKey, keyPress('y', 0))

	assert.Equal(t, 3, m.idx.total)
	assert.Equal(t, 1, m.idx.cursor, "the cursor stays on the same row")
	assert.Equal(t, int64(2), currentID(t, m))
}

func TestBulkDeleteError(t *testing.T) {
	m, s := newBulkModel(t, 5)
	// QSO 5 is deleted behind the index's back.
	require.NoError(t, s.Delete(t.Context(), 5))

	m = press(t, m, prefixKey, bulkDeleteKey, keyPress('y', 0))

	assert.Contains(t, m.idx.msg, "delete QSOs:")
	assert.Equal(t, 4, countAll(t, s), "nothing is deleted")
	assert.Len(t, m.idx.marks, 2)
}

func TestBulkDeleteCtrlCQuits(t *testing.T) {
	m, _ := newBulkModel(t, 5)
	m = press(t, m, prefixKey, bulkDeleteKey)

	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))

	require.NotNil(t, cmd)
	assert.Contains(t, collectMsgs(cmd), tea.QuitMsg{})
}

func TestBulkSave(t *testing.T) {
	m, _ := newBulkModel(t, 5)
	path := filepath.Join(t.TempDir(), "marked.adi")

	m = press(t, m, prefixKey, bulkSaveKey)
	m = typeText(t, m, path)
	m = press(t, m, enterKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, []string{"K4A", "K5A"}, savedCalls(t, path), "the marked QSOs, oldest first")
	assert.Equal(t, "Saved 2 QSOs to "+path+".", m.idx.msg)
	assert.Len(t, m.idx.marks, 2)
}

func TestBulkSaveVersion(t *testing.T) {
	s := newPatternStore(t, 3)
	m := New(WithLogbook(s), WithVersion("1.2.3"))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)
	path := filepath.Join(t.TempDir(), "marked.adi")

	m = press(t, m, toggleKey, prefixKey, bulkSaveKey)
	m.input.SetValue(path)
	press(t, m, enterKey)

	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { assert.NoError(t, f.Close()) }()
	file, err := adif.Parse(f)
	require.NoError(t, err)
	require.NotNil(t, file.Header)
	v, _ := file.Header.Fields.Get("PROGRAMVERSION")
	assert.Equal(t, "1.2.3", v)
}

func TestBulkSaveHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, _ := newBulkModel(t, 5)

	m = press(t, m, prefixKey, bulkSaveKey)
	m = typeText(t, m, "~/marked.adi")
	m = press(t, m, enterKey)

	path := filepath.Join(home, "marked.adi")
	assert.Equal(t, []string{"K4A", "K5A"}, savedCalls(t, path))
	assert.Equal(t, "Saved 2 QSOs to "+path+".", m.idx.msg)
}

func TestBulkSaveEmptyDoesNothing(t *testing.T) {
	m, _ := newBulkModel(t, 5)
	m = press(t, m, prefixKey, bulkSaveKey)

	m, cmd := update(t, m, enterKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Empty(t, collectMsgs(cmd))
	assert.Empty(t, m.idx.msg)
}

func TestBulkSaveOverwrite(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		calls []string
	}{
		{"n", keyPress('n', 0), nil},
		{"enter", enterKey, nil},
		{"y", keyPress('y', 0), []string{"K4A", "K5A"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newBulkModel(t, 5)
			path := filepath.Join(t.TempDir(), "marked.adi")
			require.NoError(t, os.WriteFile(path, []byte("original"), 0o644))
			m = press(t, m, prefixKey, bulkSaveKey)
			m.input.SetValue(path)

			m = press(t, m, enterKey)
			require.Equal(t, promptConfirm, m.prompt)
			require.NotNil(t, m.confirm)
			assert.Equal(t, "Overwrite "+path+"? [y/N]", m.confirm.question)

			m = press(t, m, tt.key)

			assert.Equal(t, promptNone, m.prompt)
			if tt.calls == nil {
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "original", string(got))
				assert.Empty(t, m.idx.msg)
				return
			}
			assert.Equal(t, tt.calls, savedCalls(t, path))
			assert.Equal(t, "Saved 2 QSOs to "+path+".", m.idx.msg)
		})
	}
}

func TestBulkSaveError(t *testing.T) {
	m, _ := newBulkModel(t, 5)
	path := filepath.Join(t.TempDir(), "missing", "marked.adi")

	m = press(t, m, prefixKey, bulkSaveKey)
	m.input.SetValue(path)
	m = press(t, m, enterKey)

	assert.Contains(t, m.idx.msg, "save QSOs to "+path+":")
	assert.Len(t, m.idx.marks, 2)
}

func TestBulkMarksClearedBeforeSubmit(t *testing.T) {
	tests := []struct {
		name   string
		key    tea.KeyPressMsg
		submit func(t *testing.T, m Model, dir string) Model
	}{
		{"tag", bulkTagKey, func(t *testing.T, m Model, _ string) Model {
			return press(t, typeText(t, m, "pota"), enterKey)
		}},
		{"untag", bulkUntagKey, func(t *testing.T, m Model, _ string) Model {
			return press(t, typeText(t, m, "pota"), enterKey)
		}},
		{"delete", bulkDeleteKey, func(t *testing.T, m Model, _ string) Model {
			return press(t, m, keyPress('y', 0))
		}},
		{"save", bulkSaveKey, func(t *testing.T, m Model, dir string) Model {
			m.input.SetValue(filepath.Join(dir, "marked.adi"))
			return press(t, m, enterKey)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, s := newBulkModel(t, 5)
			require.NoError(t, s.Tag(t.Context(), []int64{5}, "pota"))
			dir := t.TempDir()
			m = press(t, m, prefixKey, tt.key)
			require.NotEqual(t, promptNone, m.prompt)
			m, _ = update(t, m, unmarkAllMsg(5))

			m = tt.submit(t, m, dir)

			assert.Equal(t, promptNone, m.prompt)
			assert.Equal(t, "QSO 3/5 · "+msgNoMarks, statusLine(t, m))
			assert.Equal(t, 5, countAll(t, s))
			assert.Equal(t, []int64{5}, idsMatching(t, s, "~y pota"))
			assert.NoFileExists(t, filepath.Join(dir, "marked.adi"))
		})
	}
}

func TestBulkKeysNeedPrefix(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{bulkDeleteKey, bulkSaveKey, bulkTagKey, bulkUntagKey} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newBulkModel(t, 5)

			m, cmd := update(t, m, k)

			assert.Equal(t, promptNone, m.prompt)
			assert.Empty(t, collectMsgs(cmd))
			assert.Equal(t, 5, countAll(t, s))
			assert.Equal(t, "QSO 3/5 · 2 marked", statusLine(t, m))
		})
	}
}

func TestBulkHelp(t *testing.T) {
	m, _ := newIndexModel(t, 3, 200, 30)
	m = press(t, m, keyPress('?', 0))

	full := plain(m.footerView())

	for _, want := range []string{";d delete marked", ";s save marked", ";y tag marked", ";Y untag marked"} {
		assert.Contains(t, full, want)
	}
}

func TestBulkHelpWithoutLogbook(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 200, Height: 30})
	m.help.ShowAll = true

	full := plain(m.footerView())

	for _, hidden := range []string{"delete marked", "save marked", "tag marked"} {
		assert.NotContains(t, full, hidden)
	}
}

func TestWithVersion(t *testing.T) {
	assert.Equal(t, "1.2.3", New(WithVersion("1.2.3")).version)
}
