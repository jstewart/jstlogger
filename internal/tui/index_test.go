package tui

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// testQSOs returns n QSOs, newest first, with IDs n down to 1.
func testQSOs(n int) []logbook.QSO {
	t0 := time.Date(2026, 10, 31, 13, 34, 56, 0, time.UTC)
	out := make([]logbook.QSO, n)
	for i := range out {
		id := int64(n - i)
		out[i] = logbook.QSO{ID: id, StartUTC: t0.Add(-time.Duration(i) * time.Minute), Call: fmt.Sprintf("K%dA", id)}
	}
	return out
}

// loadedIndex returns an index of total rows, height rows high, with every
// row cached.
func loadedIndex(total, height int) index {
	x := newIndex()
	qs := testQSOs(total)
	x.applyLoad(indexLoadedMsg{total: total})
	for b := 0; b*indexBlockSize < total; b++ {
		x.blocks[b] = qs[b*indexBlockSize : min((b+1)*indexBlockSize, total)]
	}
	x.setHeight(height)
	return x
}

// pos is the cursor and top of an index.
type pos struct{ cursor, top int }

func posOf(x index) pos { return pos{x.cursor, x.top} }

func TestIndexNavigation(t *testing.T) {
	const total, height = 100, 10
	tests := []struct {
		name    string
		start   pos
		move    func(*index)
		want    pos
		wantMsg string
	}{
		{"next", pos{0, 0}, (*index).next, pos{1, 0}, ""},
		{"next scrolls one row", pos{9, 0}, (*index).next, pos{10, 1}, ""},
		{"next stops at last", pos{99, 90}, (*index).next, pos{99, 90}, msgLastQSO},
		{"prev", pos{5, 0}, (*index).prev, pos{4, 0}, ""},
		{"prev scrolls one row", pos{10, 10}, (*index).prev, pos{9, 9}, ""},
		{"prev stops at first", pos{0, 0}, (*index).prev, pos{0, 0}, msgFirstQSO},

		{"next page", pos{3, 0}, (*index).nextPage, pos{13, 10}, ""},
		{"next page near end", pos{85, 85}, (*index).nextPage, pos{95, 90}, ""},
		{"next page on last page", pos{93, 90}, (*index).nextPage, pos{99, 90}, ""},
		{"next page at last", pos{99, 90}, (*index).nextPage, pos{99, 90}, msgLastPage},
		{"prev page", pos{23, 20}, (*index).prevPage, pos{13, 10}, ""},
		{"prev page near start", pos{8, 5}, (*index).prevPage, pos{0, 0}, ""},
		{"prev page at first", pos{0, 0}, (*index).prevPage, pos{0, 0}, msgFirstPage},

		{"half down", pos{2, 0}, (*index).halfDown, pos{7, 5}, ""},
		{"half down at last", pos{99, 90}, (*index).halfDown, pos{99, 90}, msgLastPage},
		{"half up", pos{27, 20}, (*index).halfUp, pos{22, 15}, ""},
		{"half up at first", pos{0, 0}, (*index).halfUp, pos{0, 0}, msgFirstPage},

		{"next line keeps cursor", pos{5, 0}, (*index).nextLine, pos{5, 1}, ""},
		{"next line drags cursor", pos{0, 0}, (*index).nextLine, pos{1, 1}, ""},
		{"next line at end", pos{95, 90}, (*index).nextLine, pos{95, 90}, msgScrollDownEnd},
		{"prev line keeps cursor", pos{12, 10}, (*index).prevLine, pos{12, 9}, ""},
		{"prev line drags cursor", pos{19, 10}, (*index).prevLine, pos{18, 9}, ""},
		{"prev line at start", pos{3, 0}, (*index).prevLine, pos{3, 0}, msgScrollUpEnd},

		{"top of page", pos{17, 10}, (*index).topPage, pos{10, 10}, ""},
		{"middle of page", pos{10, 10}, (*index).middlePage, pos{14, 10}, ""},
		{"bottom of page", pos{10, 10}, (*index).bottomPage, pos{19, 10}, ""},
		{"first", pos{55, 50}, (*index).first, pos{0, 0}, ""},
		{"last", pos{5, 0}, (*index).last, pos{99, 90}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := loadedIndex(total, height)
			x.cursor, x.top = tt.start.cursor, tt.start.top

			tt.move(&x)

			assert.Equal(t, tt.want, posOf(x))
			assert.Equal(t, tt.wantMsg, x.msg)
		})
	}
}

func TestIndexShortLastPage(t *testing.T) {
	x := loadedIndex(5, 10)

	x.middlePage()
	assert.Equal(t, pos{2, 0}, posOf(x))
	x.bottomPage()
	assert.Equal(t, pos{4, 0}, posOf(x))
	x.nextPage()
	assert.Equal(t, pos{4, 0}, posOf(x))
	assert.Equal(t, msgLastPage, x.msg)
	x.msg = ""
	x.nextLine()
	assert.Equal(t, msgScrollDownEnd, x.msg, "nothing to scroll when every row fits")
}

func TestIndexEmptyNavigation(t *testing.T) {
	x := loadedIndex(0, 10)
	for _, move := range []func(*index){
		(*index).next, (*index).prev, (*index).nextPage, (*index).prevPage,
		(*index).halfDown, (*index).halfUp, (*index).nextLine, (*index).prevLine,
		(*index).topPage, (*index).middlePage, (*index).bottomPage,
		(*index).first, (*index).last, (*index).toggleMark,
	} {
		move(&x)
		assert.Equal(t, pos{0, 0}, posOf(x))
	}
	assert.EqualError(t, x.jump(1), "no QSOs to jump to")
}

func TestIndexJump(t *testing.T) {
	x := loadedIndex(100, 10)

	require.NoError(t, x.jump(50))
	assert.Equal(t, pos{49, 44}, posOf(x), "row 50 is centered")

	require.NoError(t, x.jump(100))
	assert.Equal(t, pos{99, 90}, posOf(x))

	require.NoError(t, x.jump(1))
	assert.Equal(t, pos{0, 0}, posOf(x))

	for _, n := range []int{0, -3, 101} {
		err := x.jump(n)
		require.Error(t, err, "row %d", n)
		assert.Contains(t, err.Error(), "1–100")
		assert.Equal(t, pos{0, 0}, posOf(x), "a bad row number does not move the cursor")
	}
}

func TestIndexResize(t *testing.T) {
	tests := []struct {
		name   string
		start  pos
		height int
		want   pos
	}{
		{"shrink keeps visible cursor", pos{12, 10}, 5, pos{12, 10}},
		{"shrink scrolls to cursor", pos{19, 10}, 5, pos{19, 15}},
		{"grow keeps top", pos{19, 10}, 20, pos{19, 10}},
		{"grow at end pulls top back", pos{99, 90}, 20, pos{99, 80}},
		{"grow past every row", pos{50, 45}, 200, pos{50, 0}},
		{"zero height", pos{50, 45}, 0, pos{50, 50}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := loadedIndex(100, 10)
			x.cursor, x.top = tt.start.cursor, tt.start.top
			q, ok := x.current()
			require.True(t, ok)

			x.setHeight(tt.height)

			assert.Equal(t, tt.want, posOf(x))
			after, ok := x.current()
			require.True(t, ok)
			assert.Equal(t, q.ID, after.ID, "the cursor stays on the same QSO")
		})
	}
}

func TestIndexResizeKeepsCache(t *testing.T) {
	x := loadedIndex(100, 10)
	blocks := x.blocks

	x.setHeight(30)

	assert.Equal(t, blocks, x.blocks)
	assert.Empty(t, x.missing(), "every visible row is cached")
}

func TestIndexMarks(t *testing.T) {
	x := loadedIndex(3, 10)

	x.toggleMark()
	assert.Equal(t, map[int64]bool{3: true}, x.marks)
	assert.Equal(t, 1, x.cursor, "marking moves to the next QSO")

	x.toggleMark()
	x.toggleMark()
	assert.Equal(t, map[int64]bool{3: true, 2: true, 1: true}, x.marks)
	assert.Equal(t, 2, x.cursor, "the last QSO stays under the cursor")
	assert.Empty(t, x.msg)

	x.toggleMark()
	assert.Equal(t, map[int64]bool{3: true, 2: true}, x.marks, "a second t unmarks")

	x.first()
	x.setHeight(1)
	x.applyLoad(indexLoadedMsg{gen: x.gen, total: 3, cursor: 0, rows: testQSOs(3)})
	assert.Equal(t, map[int64]bool{3: true, 2: true}, x.marks, "marks survive resizing and reloading")
}

func TestIndexMarkLoadingRow(t *testing.T) {
	x := loadedIndex(1000, 10)
	delete(x.blocks, 1)
	require.NoError(t, x.jump(700))

	x.toggleMark()

	assert.Empty(t, x.marks)
	assert.Equal(t, 699, x.cursor, "the cursor waits for the row to load")
}

func TestIndexMissing(t *testing.T) {
	x := newIndex()
	x.applyLoad(indexLoadedMsg{total: 10_000, rows: testQSOs(indexBlockSize)})
	x.setHeight(20)

	assert.Empty(t, x.missing(), "the first block covers the first page")

	x.moveTo(indexBlockSize - indexPrefetch - 1)
	assert.Empty(t, x.missing(), "the next block is not needed yet")
	x.next()
	assert.Equal(t, []int{1}, x.missing(), "the next block loads before the cursor reaches it")
	assert.Empty(t, x.missing(), "a pending block is not requested again")

	x.last()
	assert.Equal(t, []int{19}, x.missing(), "only the last block, not the rows in between")

	require.NoError(t, x.jump(4_700))
	assert.Equal(t, []int{9}, x.missing(), "only the block around the row")
}

func TestIndexMissingNotLoaded(t *testing.T) {
	x := newIndex()
	x.setHeight(20)
	assert.Empty(t, x.missing())
}

func TestIndexApplyBlock(t *testing.T) {
	x := newIndex()
	x.applyLoad(indexLoadedMsg{total: 1000, rows: testQSOs(indexBlockSize)})
	x.setHeight(20)
	x.last()
	require.Equal(t, []int{1}, x.missing())

	_, ok := x.current()
	assert.False(t, ok, "the row shows a placeholder while it loads")

	rows := testQSOs(1000)[indexBlockSize:]
	x.applyBlock(indexBlockMsg{gen: x.gen, block: 1, rows: rows})

	q, ok := x.current()
	require.True(t, ok)
	assert.Equal(t, int64(1), q.ID, "the oldest QSO")
	assert.Empty(t, x.pending)
}

func TestIndexDropsStaleLoads(t *testing.T) {
	x := loadedIndex(10, 5)
	x.gen = 2

	x.applyLoad(indexLoadedMsg{gen: 1, total: 99})
	x.applyBlock(indexBlockMsg{gen: 1, block: 3, rows: testQSOs(1)})

	assert.Equal(t, 10, x.total)
	assert.NotContains(t, x.blocks, 3)
}

func TestIndexBlockError(t *testing.T) {
	x := newIndex()
	x.applyLoad(indexLoadedMsg{total: 1000, rows: testQSOs(indexBlockSize)})
	x.setHeight(20)
	x.last()
	require.Equal(t, []int{1}, x.missing())

	boom := errors.New("boom")
	x.applyBlock(indexBlockMsg{gen: x.gen, block: 1, err: boom})

	assert.ErrorIs(t, x.err, boom)
	assert.Empty(t, x.missing(), "a failed block is not retried until a reload")

	x.applyLoad(indexLoadedMsg{gen: x.gen, total: 1000, cursor: 999, block: 1, rows: testQSOs(1000)[indexBlockSize:]})
	assert.NoError(t, x.err, "a successful reload clears the error")
}

func TestIndexLoadError(t *testing.T) {
	x := newIndex()
	boom := errors.New("boom")

	x.applyLoad(indexLoadedMsg{err: boom})

	assert.False(t, x.loaded)
	assert.ErrorIs(t, x.err, boom)
}

func TestIndexApplyLoadKeepsScreenPosition(t *testing.T) {
	x := loadedIndex(100, 10)
	x.cursor, x.top = 54, 50

	// Three QSOs were added above the cursor.
	x.applyLoad(indexLoadedMsg{gen: x.gen, total: 103, cursor: 57, rows: testQSOs(103)})

	assert.Equal(t, pos{57, 53}, posOf(x), "the cursor stays on the same line of the screen")
}

func TestIndexEvict(t *testing.T) {
	x := newIndex()
	x.applyLoad(indexLoadedMsg{total: 20 * indexBlockSize})
	x.setHeight(10)
	for b := range indexMaxBlocks + 2 {
		x.moveTo(b * indexBlockSize)
		x.applyBlock(indexBlockMsg{gen: x.gen, block: b, rows: testQSOs(1)})
	}

	assert.Len(t, x.blocks, indexMaxBlocks)
	assert.NotContains(t, x.blocks, 0, "the block farthest from the cursor goes first")
	assert.NotContains(t, x.blocks, 1)
	assert.Contains(t, x.blocks, indexMaxBlocks+1, "the cursor's block stays")
}
