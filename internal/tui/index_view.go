package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"jstlogger/internal/indexfmt"
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
	for i, it := range f.Items {
		w := it.Width
		switch {
		case it.Flex:
			w = max(width-used, minFlexWidth)
		case it.IsLiteral() || w == 0:
			b.WriteString(cells[i])
			continue
		}
		v := truncate(cells[i], w, "…")
		if it.Left {
			b.WriteString(pad(v, w))
		} else {
			b.WriteString(strings.Repeat(" ", max(w-lipgloss.Width(v), 0)) + v)
		}
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
	line := numberLine(num, nw, width, func(w int) string {
		return formatLine(x.format, x.format.Values(q, x.marks[q.ID], x.unit), w)
	})
	if i == x.cursor {
		return st.cursor.Render(line)
	}
	return line
}

// statusView renders the position, the limit, the mark count, the armed
// mark-prefix and any message or error, e.g. "QSO 12/4,567 · limit: ~b 20m ·
// 3 marked · mark-prefix".
func (x index) statusView(st styles, width int) string {
	position := "QSO " + formatCount(x.cursor+1) + "/" + formatCount(x.total)
	var rest []string
	if n := len(x.marks); n > 0 {
		rest = append(rest, formatCount(n)+" marked")
	}
	if x.prefix {
		rest = append(rest, "mark-prefix")
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
