package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var yesKey = keyPress('y', 0)

// tallPager returns m in an 80x50 window, tall enough to show the whole
// QSO tab.
func tallPager(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 50})
	return runPager(t, m, cmd)
}

// tagsValue returns the value of the plain QSO tab row labeled Tags.
func tagsValue(t *testing.T, m Model) string {
	t.Helper()
	for l := range strings.SplitSeq(plain(m.pagerBody(t)), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "Tags "); ok {
			return strings.TrimSpace(v)
		}
	}
	require.Fail(t, "no Tags row in the pager")
	return ""
}

func TestPagerDeleteConfirmation(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)

	m = pressPager(t, m, deleteKey)

	assert.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), deleteQuestion)
	assert.True(t, m.pager.open)
}

func TestPagerDelete(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	m.pager.scroll = 3

	m = pressPager(t, m, deleteKey, yesKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, []int64{5, 4, 2, 1}, idsMatching(t, s, "~A"))
	assert.True(t, m.pager.open, "the pager stays open")
	assert.Equal(t, 2, m.idx.cursor, "the cursor stays on the same row")
	assert.Equal(t, int64(2), m.mustCurrentID(t), "the pager moves on to the next QSO")
	assert.Equal(t, int64(2), m.pager.fetched, "the next QSO is fetched")
	assert.Equal(t, 0, m.pager.scroll, "the next QSO starts at the top")
	assert.Contains(t, plain(m.pagerBody(t)), "K2A")
	assert.Equal(t, "QSO 3/4 · sort: date · 2 marked · Deleted QSO with K3A on 2026-10-31 12:03.", statusLine(t, m))
}

func TestPagerDeleteCancel(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	m.pager.scroll = 3

	m = pressPager(t, m, deleteKey, keyPress('n', 0))

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, 5, countAll(t, s))
	assert.True(t, m.pager.open)
	assert.Equal(t, int64(3), m.mustCurrentID(t))
	assert.Equal(t, 3, m.pager.scroll)
	assert.Equal(t, "QSO 3/5 · sort: date · 2 marked", statusLine(t, m))
}

func TestPagerDeleteLastQSO(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	m = pressPager(t, m, keyPress('j', 0), keyPress('j', 0))
	require.Equal(t, int64(1), m.mustCurrentID(t))

	m = pressPager(t, m, deleteKey, yesKey)

	assert.Equal(t, []int64{5, 4, 3, 2}, idsMatching(t, s, "~A"))
	assert.True(t, m.pager.open)
	assert.Equal(t, 3, m.idx.cursor, "the cursor goes to the new last QSO")
	assert.Equal(t, int64(2), m.pager.fetched)
	assert.Contains(t, plain(m.pagerBody(t)), "K2A")
}

func TestPagerDeleteOnlyQSO(t *testing.T) {
	s := newPatternStore(t, 1)
	m := newPatternModel(t, s)
	m = pressPager(t, m, enter())
	require.True(t, m.pager.open)

	m = pressPager(t, m, deleteKey, yesKey)

	assert.Zero(t, countAll(t, s))
	assert.False(t, m.pager.open, "there is no QSO left to show")
	assert.Contains(t, m.View().Content, "No QSOs yet.")
}

func TestPagerBulkDelete(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)

	m = pressPager(t, m, prefixKey, bulkDeleteKey)
	require.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), "Delete 2 QSOs? [y/N]")
	m = pressPager(t, m, yesKey)

	assert.Equal(t, []int64{3, 2, 1}, idsMatching(t, s, "~A"))
	assert.Empty(t, m.idx.marks)
	assert.True(t, m.pager.open)
	assert.Equal(t, int64(3), m.mustCurrentID(t), "the pager stays on its QSO")
	assert.Equal(t, int64(3), m.pager.fetched)
	assert.Contains(t, plain(m.pagerBody(t)), "K3A")
	assert.Equal(t, "QSO 1/3 · sort: date · Deleted 2 QSOs.", statusLine(t, m))
}

func TestPagerBulkDeleteCancel(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)

	m = pressPager(t, m, prefixKey, bulkDeleteKey, keyPress('n', 0))

	assert.Equal(t, 5, countAll(t, s))
	assert.Len(t, m.idx.marks, 2)
	assert.True(t, m.pager.open)
}

func TestPagerBulkDeleteShownQSO(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	// Mark QSO 3, the one shown, too.
	m = pressPager(t, m, toggleKey, keyPress('k', 0))
	require.Equal(t, int64(3), m.mustCurrentID(t))
	m.pager.scroll = 3

	m = pressPager(t, m, prefixKey, bulkDeleteKey, yesKey)

	assert.Equal(t, []int64{2, 1}, idsMatching(t, s, "~A"))
	assert.True(t, m.pager.open)
	assert.Equal(t, int64(2), m.mustCurrentID(t), "the pager moves on to the next QSO, as d does")
	assert.Equal(t, int64(2), m.pager.fetched)
	assert.Equal(t, 0, m.pager.scroll)
	assert.Contains(t, plain(m.pagerBody(t)), "K2A")
}

func TestPagerBulkDeleteShownLastQSOs(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	// Unmark QSOs 5 and 4 and mark QSOs 3 to 1; t stays on the last QSO.
	m = pressPager(t, m, keyPress('k', 0), keyPress('k', 0))
	m = pressPager(t, m, toggleKey, toggleKey, toggleKey, toggleKey, toggleKey, keyPress('k', 0))
	require.Equal(t, map[int64]bool{3: true, 2: true, 1: true}, m.idx.marks)
	require.Equal(t, int64(2), m.mustCurrentID(t))

	m = pressPager(t, m, prefixKey, bulkDeleteKey, yesKey)

	assert.Equal(t, []int64{5, 4}, idsMatching(t, s, "~A"))
	assert.True(t, m.pager.open)
	assert.Equal(t, int64(4), m.mustCurrentID(t), "with no QSO after it left, the pager shows the new last QSO")
	assert.Contains(t, plain(m.pagerBody(t)), "K4A")
}

func TestPagerBulkDeleteEverything(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	m = pressPager(t, m, toggleKey, toggleKey, toggleKey)
	require.Len(t, m.idx.marks, 5)

	m = pressPager(t, m, prefixKey, bulkDeleteKey, yesKey)

	assert.Zero(t, countAll(t, s))
	assert.False(t, m.pager.open, "there is no QSO left to show")
	assert.Contains(t, m.View().Content, "No QSOs yet.")
}

func TestPagerBulkSave(t *testing.T) {
	m, _ := newPagerBulkModel(t, 5)
	path := filepath.Join(t.TempDir(), "marked.adi")

	m = pressPager(t, m, prefixKey, bulkSaveKey)
	require.Equal(t, promptSave, m.prompt)
	m = typeText(t, m, path)
	m = pressPager(t, m, enterKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Equal(t, []string{"K4A", "K5A"}, savedCalls(t, path))
	assert.True(t, m.pager.open)
	assert.Equal(t, "Saved 2 QSOs to "+path+".", m.idx.msg)
}

func TestPagerBulkTag(t *testing.T) {
	m, s := newPagerBulkModel(t, 5)
	m = tallPager(t, m)
	// Mark QSO 3, the one shown, too.
	m = pressPager(t, m, toggleKey, keyPress('k', 0))
	require.Equal(t, pagerEmpty, tagsValue(t, m))

	m = pressPager(t, m, prefixKey, bulkTagKey)
	require.Equal(t, promptTag, m.prompt)
	m = typeText(t, m, "pota")
	m = pressPager(t, m, enterKey)

	assert.Equal(t, []int64{5, 4, 3}, idsMatching(t, s, "~y pota"))
	assert.True(t, m.pager.open)
	assert.Equal(t, int64(3), m.mustCurrentID(t))
	assert.Equal(t, "pota", tagsValue(t, m), "the pager shows the new tag")
	assert.Equal(t, "QSO 3/5 · sort: date · 3 marked · Tagged 3 QSOs.", statusLine(t, m))
}

func TestPagerBulkUntag(t *testing.T) {
	s := newPatternStore(t, 5)
	require.NoError(t, s.Tag(t.Context(), []int64{1, 2, 3, 4, 5}, "pota"))
	require.NoError(t, s.Tag(t.Context(), []int64{3}, "dx"))
	m := newPatternModel(t, s)
	// Mark QSO 3 and open the pager on it.
	m = press(t, m, keyPress('j', 0), keyPress('j', 0), toggleKey, keyPress('k', 0))
	m = pressPager(t, m, enter())
	m = tallPager(t, m)
	require.Equal(t, int64(3), m.mustCurrentID(t))
	require.Equal(t, "dx, pota", tagsValue(t, m))

	m = pressPager(t, m, prefixKey, bulkUntagKey)
	require.Equal(t, promptUntag, m.prompt)
	m = typeText(t, m, "pota")
	m = pressPager(t, m, enterKey)

	assert.Equal(t, []int64{5, 4, 2, 1}, idsMatching(t, s, "~y pota"))
	assert.Equal(t, "dx", tagsValue(t, m), "the pager drops the removed tag")
}

// The index's ;d moves on to the next QSO as well.
func TestBulkDeleteMovesOnPastDeletedQSOs(t *testing.T) {
	m, s := newBulkModel(t, 5)
	// Mark QSO 3, under the cursor, too.
	m = press(t, m, toggleKey, keyPress('k', 0))
	require.Equal(t, int64(3), currentID(t, m))

	m = press(t, m, prefixKey, bulkDeleteKey, yesKey)

	assert.Equal(t, []int64{2, 1}, idsMatching(t, s, "~A"))
	assert.Equal(t, int64(2), currentID(t, m), "the cursor goes to the next QSO left")
}
