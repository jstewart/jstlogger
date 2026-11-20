package tui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

var (
	sortKey        = keyPress('o', 0)
	sortReverseKey = keyPress('O', 0)
)

// rowIDs returns the IDs of the index's first n rows, which must be loaded.
func rowIDs(t *testing.T, m Model, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range n {
		q, ok := m.idx.row(i)
		require.True(t, ok, "row %d is loaded", i)
		ids[i] = q.ID
	}
	return ids
}

func TestIndexSortPrompt(t *testing.T) {
	m, _ := newIndexModel(t, 10, 100, 24)

	m = press(t, m, sortKey)

	require.Equal(t, promptSort, m.prompt)
	assert.Contains(t, plain(m.View().Content), "Sort by: (d)ate (c)all (b)and (m)ode (C)ountry (s)tate c(o)ntinent")

	m = press(t, m, special(tea.KeyEscape))
	m = press(t, m, sortReverseKey)

	require.Equal(t, promptSortReverse, m.prompt)
	assert.Contains(t, plain(m.View().Content), "Reverse sort by: (d)ate (c)all (b)and (m)ode (C)ountry (s)tate c(o)ntinent")
}

func TestIndexSortPromptFooterShowsOnlyQuit(t *testing.T) {
	m, _ := newIndexModel(t, 10, 100, 24)
	m = press(t, m, keyPress('?', 0), sortKey)

	footer := plain(m.footerView())

	assert.Contains(t, footer, "quit")
	for _, help := range []string{"submit", "cancel", "prev pattern", "next"} {
		assert.NotContains(t, footer, help)
	}
}

func TestIndexSortKeys(t *testing.T) {
	tests := []struct {
		key  rune
		want logbook.SortField
	}{
		{'d', logbook.SortDate},
		{'c', logbook.SortCall},
		{'b', logbook.SortBand},
		{'m', logbook.SortMode},
		{'C', logbook.SortCountry},
		{'s', logbook.SortState},
		{'o', logbook.SortContinent},
	}
	for _, tt := range tests {
		for _, reverse := range []bool{false, true} {
			name := string(tt.key)
			open := sortKey
			if reverse {
				name, open = "O"+name, sortReverseKey
			}
			t.Run(name, func(t *testing.T) {
				m, s := newIndexModel(t, 10, 100, 24)

				m = press(t, m, open, keyPress(tt.key, 0))

				want := logbook.Sort{Field: tt.want, Reverse: reverse}
				assert.Equal(t, promptNone, m.prompt)
				assert.Equal(t, want, m.idx.sort)
				if want != logbook.DefaultSort {
					sorts := s.loadedSorts()
					assert.Equal(t, want, sorts[len(sorts)-1], "the index reloads in the new order")
				}
			})
		}
	}
}

func TestIndexSortCancel(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{special(tea.KeyEscape), keyPress('x', 0), enterKey} {
		t.Run(k.String(), func(t *testing.T) {
			m, s := newIndexModel(t, 10, 100, 24)
			before := len(s.loadedOffsets())

			m = press(t, m, sortKey, k)

			assert.Equal(t, promptNone, m.prompt)
			assert.Equal(t, logbook.DefaultSort, m.idx.sort)
			assert.Len(t, s.loadedOffsets(), before, "nothing reloads")
		})
	}
}

func TestIndexSortSameSortIsNoOp(t *testing.T) {
	m, s := newIndexModel(t, 10, 100, 24, WithIndexSort(logbook.Sort{Field: logbook.SortBand}))
	before := len(s.loadedOffsets())

	m = press(t, m, sortKey, keyPress('b', 0))

	assert.Equal(t, logbook.Sort{Field: logbook.SortBand}, m.idx.sort)
	assert.Len(t, s.loadedOffsets(), before, "nothing reloads")
}

func TestWithIndexSort(t *testing.T) {
	want := logbook.Sort{Field: logbook.SortMode, Reverse: true}

	m, s := newIndexModel(t, 10, 100, 24, WithIndexSort(want))

	assert.Equal(t, want, m.idx.sort)
	assert.Equal(t, []logbook.Sort{want}, s.loadedSorts(), "the first load uses the sort")
	assert.Equal(t, "QSO 1/10 · sort: reverse-mode", statusLine(t, m))
}

func TestIndexSortKeepsCursorOnQSO(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, keyPress('j', 0), keyPress('j', 0))
	require.Equal(t, int64(8), currentID(t, m))

	m = press(t, m, sortKey, keyPress('b', 0))

	// 40m, below 20m in frequency, comes first; each band newest first.
	assert.Equal(t, []int64{9, 7, 5, 3, 1, 10, 8, 6, 4, 2}, rowIDs(t, m, 10))
	assert.Equal(t, int64(8), currentID(t, m), "the cursor stays on the QSO")
	assert.Equal(t, 6, m.idx.cursor)
	assert.Equal(t, "QSO 7/10 · sort: band", statusLine(t, m))

	m = press(t, m, sortReverseKey, keyPress('b', 0))

	assert.Equal(t, []int64{10, 8, 6, 4, 2, 9, 7, 5, 3, 1}, rowIDs(t, m, 10))
	assert.Equal(t, int64(8), currentID(t, m))
	assert.Equal(t, "QSO 2/10 · sort: reverse-band", statusLine(t, m))

	m = press(t, m, sortKey, keyPress('d', 0))

	// Date, like every field, sorts oldest first.
	assert.Equal(t, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, rowIDs(t, m, 10))
	assert.Equal(t, int64(8), currentID(t, m))
	assert.Equal(t, "QSO 8/10 · sort: date", statusLine(t, m))

	m = press(t, m, sortReverseKey, keyPress('d', 0))

	assert.Equal(t, []int64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}, rowIDs(t, m, 10))
	assert.Equal(t, int64(8), currentID(t, m))
	assert.Equal(t, "QSO 3/10", statusLine(t, m))
}

func TestIndexSortJump(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, sortKey, keyPress('b', 0))

	m = press(t, m, keyPress('6', 0), enterKey)

	// Row 6 under the band sort is the newest 20m QSO.
	assert.Equal(t, int64(10), currentID(t, m))
	assert.Equal(t, "QSO 6/10 · sort: band", statusLine(t, m))
}

func TestIndexSortDropsStaleRows(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)

	m, cmd := update(t, m, sortKey)
	m = runIndex(t, m, cmd)
	m, cmd = update(t, m, keyPress('c', 0))

	_, ok := m.idx.row(1)
	assert.False(t, ok, "rows in the old order are not shown while the new order loads")
	m = runIndex(t, m, cmd)
	assert.Equal(t, []int64{10, 1, 2, 3, 4, 5, 6, 7, 8, 9}, rowIDs(t, m, 10))
}

func TestIndexSortKeepsMarks(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, keyPress('t', 0), keyPress('t', 0))
	require.Equal(t, map[int64]bool{10: true, 9: true}, m.idx.marks)

	m = press(t, m, sortKey, keyPress('m', 0))
	assert.Equal(t, map[int64]bool{10: true, 9: true}, m.idx.marks)

	m = press(t, m, reloadKey)
	assert.Equal(t, map[int64]bool{10: true, 9: true}, m.idx.marks)
	assert.Equal(t, logbook.Sort{Field: logbook.SortMode}, m.idx.sort, "a reload keeps the sort")
	// FT8 comes before SSB.
	assert.Equal(t, []int64{9, 6, 3, 10, 8, 7, 5, 4, 2, 1}, rowIDs(t, m, 10))
	assert.Equal(t, int64(8), currentID(t, m))
	assert.Equal(t, "QSO 5/10 · sort: mode · 2 marked", statusLine(t, m))
}

func TestIndexSortWithLimit(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = submitPattern(t, m, promptLimit, "~b 20m")

	m = press(t, m, sortKey, keyPress('m', 0))

	assert.Equal(t, 5, m.idx.total)
	assert.Equal(t, []int64{6, 10, 8, 4, 2}, rowIDs(t, m, 5))
	assert.Equal(t, int64(10), currentID(t, m))
	assert.Equal(t, "QSO 2/5 · sort: mode · limit: ~b 20m", statusLine(t, m))

	m = submitPattern(t, m, promptLimit, "~m SSB")

	assert.Equal(t, logbook.Sort{Field: logbook.SortMode}, m.idx.sort, "a new limit keeps the sort")
	assert.Equal(t, []int64{10, 8, 7, 5, 4, 2, 1}, rowIDs(t, m, 7))
}

func TestIndexSortSearch(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, sortKey, keyPress('b', 0), special(tea.KeyHome))
	require.Equal(t, int64(9), currentID(t, m))

	m = submitPattern(t, m, promptSearch, "~b 20m")

	assert.Equal(t, int64(10), currentID(t, m), "the search goes down the rows in the sort's order")
	assert.Equal(t, 5, m.idx.cursor)
}

func TestPagerNextPrevFollowSort(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, sortKey, keyPress('b', 0), special(tea.KeyHome), enter())
	require.True(t, m.pager.open)

	var ids []int64
	for range 10 {
		ids = append(ids, currentID(t, m))
		m = press(t, m, keyPress('j', 0))
	}
	assert.Equal(t, []int64{9, 7, 5, 3, 1, 10, 8, 6, 4, 2}, ids)

	m = press(t, m, keyPress('k', 0), keyPress('k', 0))
	assert.Equal(t, int64(6), currentID(t, m))
}

func TestIndexSortAfterSaveKeepsCursorOnQSO(t *testing.T) {
	s := newPatternStore(t, 10)
	m := newPatternModel(t, s)
	m = press(t, m, sortKey, keyPress('c', 0))
	require.Equal(t, int64(10), currentID(t, m), "K10A sorts first")

	q, err := s.Add(t.Context(), logbook.QSO{
		StartUTC: time.Date(2026, 10, 31, 13, 0, 0, 0, time.UTC),
		Call:     "K5B",
		Band:     "20m",
		Mode:     "SSB",
	})
	require.NoError(t, err)
	m, cmd := update(t, m, entrySavedMsg{qso: q, close: true})
	m = runIndex(t, m, cmd)

	assert.Equal(t, 11, m.idx.total)
	assert.Equal(t, q.ID, currentID(t, m), "the cursor goes to the saved QSO")
	assert.Equal(t, []int64{10, 1, 2, 3, 4, 5, q.ID, 6, 7, 8, 9}, rowIDs(t, m, 11))
	assert.Equal(t, 6, m.idx.cursor)
}

func TestIndexSortPagesAcrossBlocks(t *testing.T) {
	const n = indexBlockSize*2 + 50
	s := newPatternStore(t, 0)
	t0 := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	qsos := make([]logbook.QSO, n)
	for i := range qsos {
		band := "40m"
		if i%2 == 0 {
			band = "20m"
		}
		qsos[i] = logbook.QSO{
			StartUTC: t0.Add(time.Duration(i) * time.Minute),
			Call:     fmt.Sprintf("K%dA", i),
			Band:     band,
			Mode:     "SSB",
		}
	}
	_, err := s.Import(t.Context(), qsos, nil)
	require.NoError(t, err)
	m := newPatternModel(t, s)
	m = press(t, m, sortKey, keyPress('b', 0))

	seen := make([]int64, 0, n)
	for range n / m.idx.pageSize() {
		for i := m.idx.top; i < m.idx.top+m.idx.visible(); i++ {
			q, ok := m.idx.row(i)
			require.True(t, ok, "row %d is loaded", i)
			if len(seen) <= i {
				seen = append(seen, q.ID)
			}
		}
		m = press(t, m, keyPress('z', 0))
	}
	m = press(t, m, special(tea.KeyEnd))
	for i := len(seen); i < n; i++ {
		q, ok := m.idx.row(i)
		require.True(t, ok, "row %d is loaded", i)
		seen = append(seen, q.ID)
	}

	// IDs follow the import order, so QSO i has ID i+1: 40m are the even
	// IDs, newest first, then 20m, the odd IDs.
	var want []int64
	for id := int64(n); id > 0; id-- {
		if id%2 == 0 {
			want = append(want, id)
		}
	}
	for id := int64(n); id > 0; id-- {
		if id%2 == 1 {
			want = append(want, id)
		}
	}
	require.Len(t, seen, n)
	assert.Equal(t, want, seen, "no row is repeated or skipped")
	assert.Len(t, slices.Compact(slices.Sorted(slices.Values(seen))), n)
}
