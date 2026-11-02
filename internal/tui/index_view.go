package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"jstlogger/internal/logbook"
)

// indexChrome is the number of body lines the index uses besides its rows:
// the column header and the status line.
const indexChrome = 2

// indexColumn is one column of the index. A width of 0 marks the column
// that takes the remaining width.
type indexColumn struct {
	title string
	width int
}

// indexColumns are the index columns, in order. The first is the mark.
var indexColumns = []indexColumn{
	{"", 1},
	{"Date", 10},
	{"Time", 5},
	{"Call", 10},
	{"Mode", 6},
	{"RST S", 5},
	{"RST R", 5},
	{"Band", 5},
	{"Country", 0},
	{"Station", 10},
}

// minFlexWidth is the narrowest the remaining-width column gets; below it
// the line is truncated instead.
const minFlexWidth = 8

// minLimitWidth is the narrowest the limit gets on the status line; below
// it the line is truncated instead.
const minLimitWidth = 8

// indexTitles are the column header cells.
var indexTitles = func() []string {
	out := make([]string, len(indexColumns))
	for i, c := range indexColumns {
		out[i] = c.title
	}
	return out
}()

// indexCells returns the cells of q's row; marked sets the mark column.
func indexCells(q logbook.QSO, marked bool) []string {
	mark := " "
	if marked {
		mark = "*"
	}
	mode := q.Submode
	if mode == "" {
		mode = q.Mode
	}
	start := q.StartUTC.UTC()
	return []string{
		mark,
		start.Format("2006-01-02"),
		start.Format("15:04"),
		q.Call,
		mode,
		q.RSTSent,
		q.RSTRcvd,
		q.Band,
		q.Country,
		q.StationCallsign,
	}
}

// formatCells lays cells out in indexColumns, one space apart, and fits the
// line to exactly width cells.
func formatCells(cells []string, width int) string {
	fixed := len(indexColumns) - 1
	for _, c := range indexColumns {
		fixed += c.width
	}
	flex := max(width-fixed, minFlexWidth)

	var b strings.Builder
	for i, c := range indexColumns {
		if i > 0 {
			b.WriteByte(' ')
		}
		w := c.width
		if w == 0 {
			w = flex
		}
		b.WriteString(pad(truncate(cells[i], w, "…"), w))
	}
	return pad(truncate(b.String(), width, ""), width)
}

// truncate shortens s to at most width cells, ending it with tail when it
// is cut and tail fits.
func truncate(s string, width int, tail string) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	tw := lipgloss.Width(tail)
	if tw > width {
		tail, tw = "", 0
	}
	var b strings.Builder
	n := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if n+rw > width-tw {
			break
		}
		b.WriteRune(r)
		n += rw
	}
	return b.String() + tail
}

// pad pads s with spaces to width cells.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}

// formatCount formats n with comma thousands separators, e.g. 4,567.
func formatCount(n int) string {
	s := strconv.Itoa(n)
	start := 0
	if n < 0 {
		start = 1
	}
	for i := len(s) - 3; i > start; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// view renders the index in width by height cells: the column header, the
// visible rows and the status line. Only the visible rows are formatted.
func (x index) view(st styles, width, height int) string {
	switch {
	case !x.loaded && x.err != nil:
		return st.err.Render(truncate(x.err.Error(), width, "…"))
	case !x.loaded:
		return st.muted.Render("Loading…")
	case x.total == 0 && x.limit.expr != nil:
		return st.muted.Render("No QSOs match the limit " + x.limit.text + ".")
	case x.total == 0:
		return st.muted.Render("No QSOs yet.")
	}

	lines := make([]string, 0, height)
	lines = append(lines, st.indexHeader.Render(formatCells(indexTitles, width)))
	for i := x.top; i < x.top+x.visible(); i++ {
		lines = append(lines, x.rowView(st, i, width))
	}
	// Keep the status line at the bottom of the body.
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, x.statusView(st, width))
	// A window too short for the header and status line shows what fits.
	return strings.Join(lines[:min(len(lines), max(height, 1))], "\n")
}

// rowView renders row i, or a placeholder while it loads.
func (x index) rowView(st styles, i, width int) string {
	q, ok := x.row(i)
	if !ok {
		line := pad(truncate("  …", width, ""), width)
		if i == x.cursor {
			return st.cursor.Render(line)
		}
		return st.muted.Render(line)
	}
	line := formatCells(indexCells(q, x.marks[q.ID]), width)
	if i == x.cursor {
		return st.cursor.Render(line)
	}
	return line
}

// statusView renders the position, the limit, the mark count and any
// message or error, e.g. "QSO 12/4,567 · limit: ~b 20m · 3 marked".
func (x index) statusView(st styles, width int) string {
	position := "QSO " + formatCount(x.cursor+1) + "/" + formatCount(x.total)
	var rest []string
	if n := len(x.marks); n > 0 {
		rest = append(rest, formatCount(n)+" marked")
	}
	if x.msg != "" {
		rest = append(rest, x.msg)
	}
	if x.err != nil {
		rest = append(rest, st.err.Render(x.err.Error()))
	}
	parts := []string{position}
	if x.limit.expr != nil {
		// The limit prompt shows the whole pattern, so shorten it here to
		// leave room for the marks, message and error.
		const label = "limit: "
		room := width - lipgloss.Width(strings.Join(append([]string{position, label}, rest...), " · "))
		parts = append(parts, label+truncate(x.limit.text, max(room, minLimitWidth), "…"))
	}
	line := strings.Join(append(parts, rest...), " · ")
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}
