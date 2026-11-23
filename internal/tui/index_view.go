package tui

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
	"jstlogger/internal/lotw"
)

// indexChrome is the number of body lines the index uses besides its rows:
// the column header and the status line.
const indexChrome = 2

// minFlexWidth is the narrowest the remaining-width column gets; below it
// the line is truncated instead.
const minFlexWidth = 8

// minLimitWidth is the narrowest the limit gets on the status line; below
// it the line is truncated instead.
const minLimitWidth = 8

// formatLine lays out cells, one per item of f, and fits the line to
// exactly width cells. A literal is written as is and a natural-width
// expando at its value's width. A fixed-width expando is truncated and
// padded to its width, aligned as the item says, and the flexible one takes
// the width the others leave, but at least minFlexWidth.
func formatLine(f indexfmt.Format, cells []string, width int) string {
	return formatStyledLine(f, cells, width, nil)
}

// formatStyledLine lays out cells as formatLine does, then passes each
// cell, padded and cut to fit, and its item number to style, unless style
// is nil, and writes what style returns. Styling after the cells are
// measured keeps escape sequences out of the widths.
func formatStyledLine(f indexfmt.Format, cells []string, width int, style func(i int, s string) string) string {
	used := 0
	for i, it := range f.Items {
		switch {
		case it.Flex:
		case it.IsLiteral() || it.Width == 0:
			used += lipgloss.Width(cells[i])
		default:
			used += it.Width
		}
	}

	var b strings.Builder
	room := width
	for i, it := range f.Items {
		v := cells[i]
		switch w := it.Width; {
		case it.Flex:
			w = max(width-used, minFlexWidth)
			fallthrough
		case !it.IsLiteral() && w > 0:
			v = truncate(v, w, "…")
			if it.Left {
				v = pad(v, w)
			} else {
				v = strings.Repeat(" ", max(w-lipgloss.Width(v), 0)) + v
			}
		}
		// Cut the line at width, as truncating it whole would.
		cut := truncate(v, room, "")
		n := lipgloss.Width(cut)
		if style != nil && cut != "" {
			cut = style(i, cut)
		}
		b.WriteString(cut)
		room -= n
		if n < lipgloss.Width(v) {
			break
		}
	}
	return b.String() + strings.Repeat(" ", room)
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
// numbered adds a column at the left with each row's number, counting from
// 1 as jump does.
func (x index) view(st styles, width, height int, numbered bool) string {
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

	nw := 0
	if numbered {
		nw = len(strconv.Itoa(x.total))
	}
	lines := make([]string, 0, height)
	lines = append(lines, st.indexHeader.Render(numberLine("#", nw, width, func(w int) string {
		return formatLine(x.format, x.format.Titles(), w)
	})))
	for i := x.top; i < x.top+x.visible(); i++ {
		lines = append(lines, x.rowView(st, i, width, nw))
	}
	// Keep the status line at the bottom of the body.
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	lines = append(lines, x.statusView(st, width))
	// A window too short for the header and status line shows what fits.
	return strings.Join(lines[:min(len(lines), max(height, 1))], "\n")
}

// numberLine prefixes the line rest renders, in the width left over, with
// num right-aligned in an nw-cell column, and fits it to exactly width
// cells. An nw of 0 leaves out the column.
func numberLine(num string, nw, width int, rest func(width int) string) string {
	if nw == 0 {
		return rest(width)
	}
	num = truncate(num, nw, "")
	line := strings.Repeat(" ", nw-lipgloss.Width(num)) + num + " " + rest(max(width-nw-1, 0))
	return pad(truncate(line, width, ""), width)
}

// rowView renders row i, or a placeholder while it loads, numbered in an
// nw-cell column unless nw is 0.
func (x index) rowView(st styles, i, width, nw int) string {
	num := strconv.Itoa(i + 1)
	q, ok := x.row(i)
	if !ok {
		line := numberLine(num, nw, width, func(w int) string {
			return pad(truncate("  …", w, ""), w)
		})
		if i == x.cursor {
			return st.cursor.Render(line)
		}
		return st.muted.Render(line)
	}
	last, user := x.lotw.lastUpload(q.Call)
	row := indexfmt.Row{Marked: x.marks[q.ID], LoTW: user}
	cells := x.format.Values(q, row, x.unit)
	recency := lotw.RecencyOf(last, x.lotw.now())
	style := func(j int, v string) string {
		s, ok := cellStyle(st, x.format.Items[j], cells[j], row, recency)
		switch {
		case !ok:
			return v
		case i == x.cursor:
			return onBar(st.cursor, s).Render(v)
		default:
			return s.Render(v)
		}
	}
	line := numberLine(num, nw, width, func(w int) string {
		return formatStyledLine(x.format, cells, w, style)
	})
	if i == x.cursor {
		return fillBar(st.cursor, line)
	}
	return line
}

// cellStyle returns the style of the cell of item it whose value is v, in
// a row with the marks and LoTW status of row and a last LoTW upload of
// recency r, and reports whether the cell is styled. The style follows the
// item's field, so custom formats get it too: the call is bold accent, the
// date, time and RST are muted, a marked row's mark is bold yellow, a band
// has its own color and the LoTW cell shows the upload's recency.
func cellStyle(st styles, it indexfmt.Item, v string, row indexfmt.Row, r lotw.Recency) (lipgloss.Style, bool) {
	switch it.Field {
	case 'c':
		return st.indexCall, true
	case indexfmt.FieldTime, 's', 'r':
		return st.indexDim, true
	case 'Z':
		return st.indexMark, row.Marked
	case 'b':
		s, ok := st.bands[strings.ToLower(v)]
		return s, ok
	case indexfmt.FieldLoTW:
		return lotwStyle(st, r)
	default:
		return lipgloss.Style{}, false
	}
}

// onBar returns cell style s for a cell on the cursor bar: s with the
// bar's background and whatever else of the bar s leaves unset. A
// foreground that matches the bar would hide the cell, so it gives way to
// the bar's, and a faint cell stays faint rather than taking the bar's
// bold.
func onBar(bar, s lipgloss.Style) lipgloss.Style {
	if s.GetForeground() == bar.GetBackground() {
		s = s.UnsetForeground()
	}
	if s.GetFaint() {
		s = s.Bold(false)
	}
	return s.Inherit(bar)
}

// fillBar puts the unstyled runs of line, such as the number column, the
// plain cells and the padding, on the cursor bar, so the bar spans the line
// unbroken around the cells styled with onBar. Each styled cell is one SGR
// sequence ended by a reset, as Style.Render writes it.
func fillBar(bar lipgloss.Style, line string) string {
	const reset = "\x1b[m"
	var b strings.Builder
	parts := strings.Split(line, reset)
	for i, p := range parts {
		text, styled, ok := strings.Cut(p, "\x1b[")
		if text != "" {
			b.WriteString(bar.Render(text))
		}
		if ok {
			b.WriteString("\x1b[" + styled)
			if i < len(parts)-1 {
				b.WriteString(reset)
			}
		}
	}
	return b.String()
}

// statusView renders the position, the sort unless it is the default, the
// limit, the mark count, the armed mark-prefix, any other armed prefixes
// named by armed, and any message or error, e.g. " QSO 12/4,567  · sort:
// band · limit: ~b 20m · 3 marked · mark-prefix". The position is a chip,
// the sort and limit have muted labels and purple values, the mark count is
// bold yellow and the separators are muted.
func (x index) statusView(st styles, width int, armed ...string) string {
	position := st.statusChip.Render(" QSO " + formatCount(x.cursor+1) + "/" + formatCount(x.total) + " ")
	var rest []string
	if n := len(x.marks); n > 0 {
		rest = append(rest, st.statusMarked.Render(formatCount(n)+" marked"))
	}
	if x.prefix {
		rest = append(rest, "mark-prefix")
	}
	rest = append(rest, armed...)
	if x.msg != "" {
		rest = append(rest, x.msg)
	}
	if x.err != nil {
		rest = append(rest, st.err.Render(x.err.Error()))
	}
	sep := st.muted.Render(" · ")
	parts := []string{position}
	if x.sort != logbook.DefaultSort {
		parts = append(parts, st.muted.Render("sort: ")+st.statusValue.Render(x.sort.String()))
	}
	if x.limit.expr != nil {
		// The limit prompt shows the whole pattern, so shorten it here to
		// leave room for the marks, message and error. The widths are of
		// the styled segments, padding and all.
		label := st.muted.Render("limit: ")
		room := width - lipgloss.Width(strings.Join(slices.Concat(parts, []string{label}, rest), sep))
		parts = append(parts, label+st.statusValue.Render(truncate(x.limit.text, max(room, minLimitWidth), "…")))
	}
	line := strings.Join(append(parts, rest...), sep)
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}
