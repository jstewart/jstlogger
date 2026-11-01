package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/logbook"
)

const (
	// indexBlockSize is how many rows the index loads at a time: the most
	// one logbook query returns, so a block is never cut short.
	indexBlockSize = logbook.MaxPageSize
	// indexPrefetch is how many rows beyond the visible ones are kept
	// loaded, so the next block arrives before the cursor reaches it.
	indexPrefetch = 100
	// indexMaxBlocks bounds the block cache. The blocks farthest from the
	// cursor are dropped first.
	indexMaxBlocks = 8
	// logbookTimeout bounds each logbook query the index runs.
	logbookTimeout = 5 * time.Second
)

// Messages shown on the index status line.
const (
	msgLastQSO       = "You are on the last QSO."
	msgFirstQSO      = "You are on the first QSO."
	msgLastPage      = "You are on the last page."
	msgFirstPage     = "You are on the first page."
	msgScrollDownEnd = "You can't scroll down farther."
	msgScrollUpEnd   = "You can't scroll up farther."
)

// index is the scrolling list of QSOs, newest first. Rows are identified by
// their position in that order; row 0 is the newest QSO. Only the rows near
// the view are cached, in blocks of indexBlockSize, so the logbook can be
// any size.
type index struct {
	// loaded reports whether total is known.
	loaded bool
	total  int
	// cursor is the selected row and top the first visible row.
	cursor int
	top    int
	// height is the number of rows visible.
	height int

	// blocks caches rows by block number: block b holds rows
	// b*indexBlockSize onward.
	blocks map[int][]logbook.QSO
	// pending holds the blocks being loaded, and blocks that failed to
	// load, which are not retried until the next reload.
	pending map[int]bool
	// gen counts reloads; results of a load started before the latest
	// reload are dropped.
	gen int

	// marks holds the IDs of the marked QSOs.
	marks map[int64]bool

	// msg is shown on the status line until the next key press.
	msg string
	// err is the last load error, shown until a reload succeeds.
	err error
}

func newIndex() index {
	return index{
		blocks:  map[int][]logbook.QSO{},
		pending: map[int]bool{},
		marks:   map[int64]bool{},
	}
}

// indexLoadedMsg reports a full (re)load: the row count and the block
// holding the cursor.
type indexLoadedMsg struct {
	gen    int
	total  int
	cursor int
	block  int
	rows   []logbook.QSO
	err    error
}

// indexBlockMsg reports one loaded block.
type indexBlockMsg struct {
	gen   int
	block int
	rows  []logbook.QSO
	err   error
}

// loadIndexCmd returns a command that counts the QSOs and loads the block
// holding the cursor. The cursor goes to the QSO with the given ID if it
// still exists, and otherwise to row fallback.
func loadIndexCmd(s logbook.Store, gen int, id int64, fallback int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		msg := indexLoadedMsg{gen: gen}
		msg.err = func() error {
			total, err := s.Count(ctx, logbook.Filter{})
			if err != nil {
				return fmt.Errorf("load index: %w", err)
			}
			cursor := fallback
			if id != 0 {
				n, err := s.IndexOf(ctx, logbook.Filter{}, id)
				switch {
				case err == nil:
					cursor = n
				case !errors.Is(err, logbook.ErrNotFound):
					return fmt.Errorf("load index: %w", err)
				}
			}
			cursor = max(min(cursor, total-1), 0)
			block := cursor / indexBlockSize
			rows, err := s.Index(ctx, logbook.Filter{}, block*indexBlockSize, indexBlockSize)
			if err != nil {
				return fmt.Errorf("load index: %w", err)
			}
			msg.total, msg.cursor, msg.block, msg.rows = total, cursor, block, rows
			return nil
		}()
		return msg
	}
}

// loadBlockCmd returns a command that loads one block of rows.
func loadBlockCmd(s logbook.Store, gen, block int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		rows, err := s.Index(ctx, logbook.Filter{}, block*indexBlockSize, indexBlockSize)
		if err != nil {
			err = fmt.Errorf("load rows %d–%d: %w", block*indexBlockSize+1, (block+1)*indexBlockSize, err)
		}
		return indexBlockMsg{gen: gen, block: block, rows: rows, err: err}
	}
}

// applyLoad replaces the cache with the result of a (re)load. The cursor
// keeps its place on screen when it can.
func (x *index) applyLoad(msg indexLoadedMsg) {
	if msg.gen != x.gen {
		return
	}
	if msg.err != nil {
		x.err = msg.err
		return
	}
	x.err = nil
	x.loaded = true
	x.total = msg.total
	x.blocks = map[int][]logbook.QSO{msg.block: msg.rows}
	x.pending = map[int]bool{}
	x.top += msg.cursor - x.cursor
	x.cursor = msg.cursor
	x.clampTop()
	x.scrollToCursor()
}

// applyBlock caches a loaded block.
func (x *index) applyBlock(msg indexBlockMsg) {
	if msg.gen != x.gen {
		return
	}
	if msg.err != nil {
		// Leave the block pending so it isn't retried on every update.
		x.err = msg.err
		return
	}
	delete(x.pending, msg.block)
	x.blocks[msg.block] = msg.rows
	x.evict()
}

// evict drops the cached blocks farthest from the cursor until at most
// indexMaxBlocks remain.
func (x *index) evict() {
	here := x.cursor / indexBlockSize
	for len(x.blocks) > indexMaxBlocks {
		far, dist := 0, -1
		for b := range x.blocks {
			if d := max(b-here, here-b); d > dist {
				far, dist = b, d
			}
		}
		delete(x.blocks, far)
	}
}

// missing returns the blocks that the visible rows, give or take
// indexPrefetch rows, need and that are neither cached nor pending, and
// marks them pending.
func (x *index) missing() []int {
	if !x.loaded || x.total == 0 {
		return nil
	}
	lo := max(x.top-indexPrefetch, 0)
	hi := min(x.top+x.height+indexPrefetch, x.total) - 1
	var out []int
	for b := lo / indexBlockSize; b <= hi/indexBlockSize; b++ {
		if _, ok := x.blocks[b]; ok || x.pending[b] {
			continue
		}
		x.pending[b] = true
		out = append(out, b)
	}
	return out
}

// row returns row i if it is cached.
func (x index) row(i int) (logbook.QSO, bool) {
	rows := x.blocks[i/indexBlockSize]
	if i < 0 || i%indexBlockSize >= len(rows) {
		return logbook.QSO{}, false
	}
	return rows[i%indexBlockSize], true
}

// current returns the QSO under the cursor if it is cached.
func (x index) current() (logbook.QSO, bool) {
	if x.total == 0 {
		return logbook.QSO{}, false
	}
	return x.row(x.cursor)
}

// pageSize is the number of rows a page moves, at least one.
func (x index) pageSize() int {
	return max(x.height, 1)
}

// maxTop is the largest top that still fills the view.
func (x index) maxTop() int {
	return max(x.total-x.pageSize(), 0)
}

func (x *index) clampTop() {
	x.top = max(min(x.top, x.maxTop()), 0)
}

// scrollToCursor scrolls the view as little as possible to show the cursor.
func (x *index) scrollToCursor() {
	if x.cursor < x.top {
		x.top = x.cursor
	}
	if x.cursor >= x.top+x.pageSize() {
		x.top = x.cursor - x.pageSize() + 1
	}
	x.clampTop()
}

// setHeight changes the number of visible rows, keeping the cursor on the
// same row.
func (x *index) setHeight(h int) {
	x.height = max(h, 0)
	x.clampTop()
	x.scrollToCursor()
}

// moveTo moves the cursor to row i, clamped to the rows that exist.
func (x *index) moveTo(i int) {
	x.cursor = max(min(i, x.total-1), 0)
	x.scrollToCursor()
}

// next moves the cursor down one row.
func (x *index) next() {
	if x.cursor >= x.total-1 {
		x.msg = msgLastQSO
		return
	}
	x.moveTo(x.cursor + 1)
}

// prev moves the cursor up one row.
func (x *index) prev() {
	if x.cursor <= 0 {
		x.msg = msgFirstQSO
		return
	}
	x.moveTo(x.cursor - 1)
}

// scroll moves the view and the cursor n rows, down when n is positive.
func (x *index) scroll(n int) {
	switch {
	case n > 0 && x.cursor >= x.total-1:
		x.msg = msgLastPage
		return
	case n < 0 && x.cursor <= 0:
		x.msg = msgFirstPage
		return
	}
	x.top += n
	x.clampTop()
	x.moveTo(x.cursor + n)
}

func (x *index) nextPage() { x.scroll(x.pageSize()) }
func (x *index) prevPage() { x.scroll(-x.pageSize()) }
func (x *index) halfDown() { x.scroll(max(x.pageSize()/2, 1)) }
func (x *index) halfUp()   { x.scroll(-max(x.pageSize()/2, 1)) }

// nextLine scrolls the view down one row, keeping the cursor on screen.
func (x *index) nextLine() {
	if x.top >= x.maxTop() {
		x.msg = msgScrollDownEnd
		return
	}
	x.top++
	x.cursor = max(x.cursor, x.top)
}

// prevLine scrolls the view up one row, keeping the cursor on screen.
func (x *index) prevLine() {
	if x.top <= 0 {
		x.msg = msgScrollUpEnd
		return
	}
	x.top--
	x.cursor = min(x.cursor, x.top+x.pageSize()-1)
}

// visible is the number of rows on screen.
func (x index) visible() int {
	return max(min(x.pageSize(), x.total-x.top), 0)
}

func (x *index) topPage()    { x.moveTo(x.top) }
func (x *index) middlePage() { x.moveTo(x.top + (x.visible()-1)/2) }
func (x *index) bottomPage() { x.moveTo(x.top + x.visible() - 1) }
func (x *index) first()      { x.moveTo(0) }
func (x *index) last()       { x.moveTo(x.total - 1) }

// jump moves the cursor to row number n, counting from 1, and centers it.
func (x *index) jump(n int) error {
	if x.total == 0 {
		return errors.New("no QSOs to jump to")
	}
	if n < 1 || n > x.total {
		return fmt.Errorf("no QSO %d: enter 1–%s", n, formatCount(x.total))
	}
	x.cursor = n - 1
	x.top = x.cursor - x.pageSize()/2
	x.clampTop()
	return nil
}

// toggleMark marks or unmarks the QSO under the cursor, then moves to the
// next QSO. It does nothing while the row is loading.
func (x *index) toggleMark() {
	q, ok := x.current()
	if !ok {
		return
	}
	if x.marks[q.ID] {
		delete(x.marks, q.ID)
	} else {
		x.marks[q.ID] = true
	}
	if x.cursor < x.total-1 {
		x.moveTo(x.cursor + 1)
	}
}

// jump moves the index cursor to the row number typed in the jump prompt.
func (m *Model) jump(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("invalid QSO number %q: %w", s, numErr(err))
	}
	return m.idx.jump(n)
}
