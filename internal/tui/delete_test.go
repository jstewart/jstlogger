package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var deleteKey = keyPress('d', 0)

// deleteQuestion is the question d asks about QSO 3 of newPatternStore.
const deleteQuestion = "Delete QSO with K3A on 2026-10-31 12:03? [y/N]"

func TestDeleteConfirmation(t *testing.T) {
	m, _ := newBulkModel(t, 5)
	require.Equal(t, int64(3), currentID(t, m))

	m = press(t, m, deleteKey)

	assert.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), deleteQuestion)
}

func TestDeleteCancel(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyPress('n', 0), enterKey, escKey, keyPress('x', 0)} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newBulkModel(t, 5)
			m = press(t, m, deleteKey)

			m, cmd := update(t, m, k)

			assert.Equal(t, promptNone, m.prompt)
			assert.Empty(t, collectMsgs(cmd))
			assert.Equal(t, 5, countAll(t, s))
			assert.Equal(t, "QSO 3/5 · 2 marked", statusLine(t, m))
		})
	}
}

func TestDelete(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyPress('y', 0), keyPress('Y', 0)} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newBulkModel(t, 5)
			// A QSO logged behind the index's back shows the index reloads.
			addPatternQSO(t, s, 6)
			m = press(t, m, deleteKey)

			m = press(t, m, k)

			assert.Equal(t, promptNone, m.prompt)
			assert.Equal(t, []int64{6, 5, 4, 2, 1}, idsMatching(t, s, "~A"))
			assert.Equal(t, map[int64]bool{5: true, 4: true}, m.idx.marks, "the other marks stay")
			assert.Equal(t, 5, m.idx.total, "the index reloads")
			assert.Equal(t, 2, m.idx.cursor, "the cursor stays on the same row")
			assert.Equal(t, "QSO 3/5 · 2 marked · Deleted QSO with K3A on 2026-10-31 12:03.", statusLine(t, m))
		})
	}
}

func TestDeleteMarked(t *testing.T) {
	m, s := newBulkModel(t, 5)
	m = press(t, m, keyPress('k', 0))
	require.Equal(t, int64(4), currentID(t, m))

	m = press(t, m, deleteKey, keyPress('y', 0))

	assert.Equal(t, []int64{5, 3, 2, 1}, idsMatching(t, s, "~A"))
	assert.Equal(t, map[int64]bool{5: true}, m.idx.marks, "the deleted QSO's mark is dropped")
}

func TestDeleteNamesQuestionedQSO(t *testing.T) {
	m, s := newBulkModel(t, 5)
	m = press(t, m, deleteKey)
	// A search moves the cursor while the question is open.
	m, _ = update(t, m, indexSearchMsg{gen: m.idx.gen, row: 0})
	require.Equal(t, int64(5), currentID(t, m))

	m = press(t, m, keyPress('y', 0))

	assert.Equal(t, []int64{5, 4, 2, 1}, idsMatching(t, s, "~A"), "QSO 3, as asked, is deleted")
}

func TestDeleteError(t *testing.T) {
	m, s := newBulkModel(t, 5)
	// QSO 3 is deleted behind the index's back.
	require.NoError(t, s.Delete(t.Context(), 3))

	m = press(t, m, deleteKey, keyPress('y', 0))

	assert.Contains(t, m.idx.msg, "delete QSO with K3A on 2026-10-31 12:03:")
	assert.Equal(t, 4, countAll(t, s))
	assert.Len(t, m.idx.marks, 2)
}

func TestDeleteEmptyIndex(t *testing.T) {
	m := newPatternModel(t, newPatternStore(t, 0))

	m, cmd := update(t, m, deleteKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Empty(t, collectMsgs(cmd))
}

func TestDeleteCtrlCQuits(t *testing.T) {
	m, _ := newBulkModel(t, 5)
	m = press(t, m, deleteKey)

	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))

	require.NotNil(t, cmd)
	assert.Contains(t, collectMsgs(cmd), tea.QuitMsg{})
}

func TestDeleteHelp(t *testing.T) {
	m, _ := newIndexModel(t, 3, 200, 30)
	m = press(t, m, keyPress('?', 0))

	full := plain(m.footerView())

	assert.Contains(t, full, "d delete")
	assert.Contains(t, full, ";d delete marked")
}
