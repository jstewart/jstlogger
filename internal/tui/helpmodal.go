package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	// helpTitle heads the help modal, beside helpBadge.
	helpTitle = "keybinds"
	helpBadge = " esc close "
	// helpPlaceholder fills the filter line until a filter is typed.
	helpPlaceholder = "press / to filter by command or shortcut"
	// helpHint and helpFilterHint end the help modal, while browsing and
	// while typing a filter.
	helpHint       = "search / · scroll j/k/↑↓/pgup/pgdn · close esc/enter"
	helpFilterHint = "apply enter · clear esc · scroll ↑↓/ctrl+p/ctrl+n"
	// helpChrome is the number of lines of the modal, inside its frame,
	// around the rows: the title, the filter line and a blank line above,
	// and a blank line and the hint below.
	helpChrome = 5
	// helpColumnGap separates the key and description columns.
	helpColumnGap = 2
	// helpBar is the width of the scroll bar and the space before it.
	helpBar = 2
)

// helpModalKeyMap holds the keybindings of the open help modal.
type helpModalKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	// Filter starts typing a filter.
	Filter key.Binding
	Close  key.Binding
	Quit   key.Binding
	// Apply and Clear stop typing a filter, keeping or clearing it.
	Apply key.Binding
	Clear key.Binding
	// FilterUp and FilterDown scroll while a filter is typed, in which j
	// and k are text.
	FilterUp   key.Binding
	FilterDown key.Binding
}

var helpModalKeys = helpModalKeyMap{
	Up:         key.NewBinding(key.WithKeys("k", "up", "ctrl+p"), key.WithHelp("j/k/↑/↓", "scroll")),
	Down:       key.NewBinding(key.WithKeys("j", "down", "ctrl+n")),
	PageUp:     key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup/pgdn", "page")),
	PageDown:   key.NewBinding(key.WithKeys("pgdown")),
	Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Close:      key.NewBinding(key.WithKeys("esc", "enter", "?", "f1"), key.WithHelp("esc/enter", "close")),
	Quit:       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Apply:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
	Clear:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
	FilterUp:   key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "scroll")),
	FilterDown: key.NewBinding(key.WithKeys("down", "ctrl+n")),
}

func (k helpModalKeyMap) ShortHelp() []key.Binding {
	// Close and quit come first so a narrow window still shows them.
	return []key.Binding{k.Close, k.Quit, k.Filter, k.Apply, k.Clear, k.Up, k.FilterUp, k.PageUp}
}

func (k helpModalKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// helpModal lists the keybindings of the context it was opened from, in
// titled sections, filtered by what is typed after /. It is read-only: no
// key it lists runs from it.
type helpModal struct {
	open     bool
	sections []helpSection
	// input holds the filter, which a binding's key or description
	// contains; filtering is true while it is typed.
	input     textinput.Model
	filtering bool
	// scroll is the index in rows of the first row shown.
	scroll int
}

// helpRow is a row of the help modal: a section's heading, one of its
// bindings, or, with neither, the blank row between two sections.
type helpRow struct {
	heading string
	binding key.Binding
}

// newHelpModal returns an open help modal listing the sections of keys.
func newHelpModal(keys helpKeyMap) helpModal {
	in := newInput()
	in.Prompt = ""
	return helpModal{open: true, sections: keys.sections(), input: in}
}

// filter returns the filter typed.
func (h helpModal) filter() string {
	return h.input.Value()
}

// keys returns the modal's keybindings, those for typing a filter enabled
// only while one is typed, and those for browsing only while not.
func (h helpModal) keys() helpModalKeyMap {
	k := helpModalKeys
	for _, b := range []*key.Binding{&k.Apply, &k.Clear, &k.FilterUp, &k.FilterDown} {
		b.SetEnabled(h.filtering)
	}
	for _, b := range []*key.Binding{&k.Filter, &k.Up, &k.Down, &k.Close} {
		b.SetEnabled(!h.filtering)
	}
	return k
}

// rows returns each section's heading and its bindings that are enabled
// and have help or, once a filter is typed, those whose key or description
// contains it, ignoring case. A section with no binding to show is left
// out.
func (h helpModal) rows() []helpRow {
	return h.rowsMatching(h.filter())
}

// rowsMatching returns the rows that filter matches, as rows does.
func (h helpModal) rowsMatching(filter string) []helpRow {
	f := strings.ToLower(filter)
	var out []helpRow
	for _, s := range h.sections {
		var rows []helpRow
		for _, b := range s.keys {
			hk := b.Help()
			if !b.Enabled() || hk.Key == "" && hk.Desc == "" {
				continue
			}
			if !strings.Contains(strings.ToLower(hk.Key), f) && !strings.Contains(strings.ToLower(hk.Desc), f) {
				continue
			}
			rows = append(rows, helpRow{binding: b})
		}
		if len(rows) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, helpRow{})
		}
		out = append(out, helpRow{heading: s.title})
		out = append(out, rows...)
	}
	return out
}

// keyWidth is the width of the widest key of every row, filtered or not,
// so the column holds still while filtering.
func (h helpModal) keyWidth() int {
	return keyWidth(h.rowsMatching(""))
}

// keyWidth is the width of the widest key of rows.
func keyWidth(rows []helpRow) int {
	w := 0
	for _, r := range rows {
		w = max(w, lipgloss.Width(r.binding.Help().Key))
	}
	return w
}

// helpWidth is the width inside the frame that the title, filter line,
// hint and unfiltered rows all, at full length, need.
func helpWidth(all []helpRow) int {
	w := max(
		lipgloss.Width(helpTitle)+1+lipgloss.Width(helpBadge),
		1+lipgloss.Width("/ "+helpPlaceholder),
		1+max(lipgloss.Width(helpHint), lipgloss.Width(helpFilterHint)),
	)
	kw := keyWidth(all)
	for _, r := range all {
		w = max(w, 1+kw+helpColumnGap+lipgloss.Width(r.binding.Help().Desc)+helpBar)
	}
	return w
}

// lines renders every row in width cells: each heading, then its
// bindings, a key and what it does in columns, below. When the filter
// matches nothing, a line says so.
func (h helpModal) lines(st styles, width int) []string {
	rows := h.rows()
	if len(rows) == 0 {
		return []string{fit(st.muted.Render("no matches"), width)}
	}
	kw := min(h.keyWidth(), max(width/2-helpColumnGap, 1))
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		hk := r.binding.Help()
		switch {
		case r.heading != "":
			lines = append(lines, fit(st.helpHeading.Render(r.heading), width))
		case hk.Key == "" && hk.Desc == "":
			lines = append(lines, fit("", width))
		default:
			k := st.helpKey.Render(pad(truncate(hk.Key, kw, "…"), kw))
			lines = append(lines, fit(k+strings.Repeat(" ", helpColumnGap)+hk.Desc, width))
		}
	}
	return lines
}

// size returns the outer width and height of the modal drawn in a space w
// by ht cells, and the number of rows it shows at once, which is 0 when
// the space has no room for any. It takes four fifths of the space, or
// more for the width its unfiltered rows need, and no more height than
// they need, so it holds still while filtering.
func (h helpModal) size(st styles, w, ht int) (outerW, outerH, listHeight int) {
	all := h.rowsMatching("")
	fw, fh := st.helpBox.GetHorizontalFrameSize(), st.helpBox.GetVerticalFrameSize()
	outerW = min(max(w*4/5, helpWidth(all)+fw), w)
	outerH = min(len(all)+helpChrome+fh, max(ht*4/5, helpChrome+fh+1), ht)
	return outerW, outerH, max(outerH-fh-helpChrome, 0)
}

// scrollBy scrolls the rows n rows down, or up for a negative n, stopping
// at the first and last, for a window of listHeight rows.
func (h *helpModal) scrollBy(n, listHeight int) {
	bottom := max(len(h.rows())-listHeight, 0)
	h.scroll = min(max(min(h.scroll, bottom)+n, 0), bottom)
}

// update handles a key press. / starts typing a filter, which enter keeps
// and esc clears; a change to the filter scrolls back to the top.
func (h *helpModal) update(msg tea.KeyPressMsg, listHeight int) tea.Cmd {
	k := h.keys()
	switch {
	case key.Matches(msg, k.Close):
		h.open = false
	case key.Matches(msg, k.Filter):
		h.filtering = true
		return h.input.Focus()
	case key.Matches(msg, k.Apply):
		h.filtering = false
		h.input.Blur()
	case key.Matches(msg, k.Clear):
		h.filtering = false
		h.input.Blur()
		h.input.Reset()
		h.scroll = 0
	case key.Matches(msg, k.Up, k.FilterUp):
		h.scrollBy(-1, listHeight)
	case key.Matches(msg, k.Down, k.FilterDown):
		h.scrollBy(1, listHeight)
	case key.Matches(msg, k.PageUp):
		h.scrollBy(-listHeight, listHeight)
	case key.Matches(msg, k.PageDown):
		h.scrollBy(listHeight, listHeight)
	case h.filtering:
		return h.edit(msg)
	}
	return nil
}

// paste adds a paste to the filter, starting typing one if need be. The
// input puts each line break as a space, so the filter stays on one line.
func (h *helpModal) paste(msg tea.PasteMsg) tea.Cmd {
	if !h.filtering {
		h.filtering = true
		h.input.Focus()
	}
	return h.edit(msg)
}

// edit passes msg to the filter's input, scrolling back to the top if the
// filter changes.
func (h *helpModal) edit(msg tea.Msg) tea.Cmd {
	before := h.filter()
	var cmd tea.Cmd
	h.input, cmd = h.input.Update(msg)
	if h.filter() != before {
		h.scroll = 0
	}
	return cmd
}

// filterLine renders the line with the filter, or how to type one, in
// width cells.
func (h helpModal) filterLine(st styles, width int) string {
	if !h.filtering && h.filter() == "" {
		return fit(" "+st.muted.Render("/ "+helpPlaceholder), width)
	}
	in := h.input
	in.SetWidth(max(width-3, 1))
	return fit(" "+st.muted.Render("/ ")+in.View(), width)
}

// scrollBar returns a scroll bar listHeight lines tall for total rows
// scrolled to scroll, or nothing when they all show.
func scrollBar(st styles, total, listHeight, scroll int) []string {
	if total <= listHeight || listHeight == 0 {
		return nil
	}
	thumb := max(listHeight*listHeight/total, 1)
	top := scroll * (listHeight - thumb) / (total - listHeight)
	bar := make([]string, listHeight)
	for i := range bar {
		bar[i] = st.muted.Render("│")
		if i >= top && i < top+thumb {
			bar[i] = "┃"
		}
	}
	return bar
}

// view renders the modal in a box fitting a space width by height cells:
// the title and badge, the filter line, a window of the rows from scroll
// with a scroll bar beside it, and the hint.
func (h helpModal) view(st styles, width, height int) string {
	w, _, listHeight := h.size(st, width, height)
	inner := max(w-st.helpBox.GetHorizontalFrameSize(), 1)
	rowWidth := max(inner-1-helpBar, 1)

	badge := st.helpBadge.Render(helpBadge)
	title := st.label.Render(helpTitle)
	gap := max(inner-lipgloss.Width(title)-lipgloss.Width(badge), 1)
	hint := helpHint
	if h.filtering {
		hint = helpFilterHint
	}

	rows := h.lines(st, rowWidth)
	start := min(h.scroll, max(len(rows)-listHeight, 0))
	shown := rows[start:min(start+listHeight, len(rows))]
	bar := scrollBar(st, len(rows), listHeight, start)
	lines := []string{fit(title+strings.Repeat(" ", gap)+badge, inner), h.filterLine(st, inner), ""}
	for i := range listHeight {
		row, b := "", " "
		if i < len(shown) {
			row = shown[i]
		}
		if bar != nil {
			b = bar[i]
		}
		lines = append(lines, fit(" "+pad(row, rowWidth)+" "+b, inner))
	}
	lines = append(lines, "", fit(" "+st.muted.Render(hint), inner))
	return st.helpBox.Render(strings.Join(lines, "\n"))
}

// contextKeys returns the keybindings in effect, which the footer and the
// help modal show: the open rig modal's, or else the open prompt's, or
// else the New QSO screen's or its open pick list's, or else the global
// keys and the pager's or the index's.
func (m Model) contextKeys() helpKeyMap {
	if m.rigModal.open {
		return m.rigModal.keys()
	}
	if m.promptOpen() {
		if isKeyPrompt(m.prompt) {
			// Only y answers yes, and only a field's key sorts; enter and
			// esc cancel like any other key.
			return promptKeyMap{Quit: m.promptKeys.Quit, Help: m.promptKeys.Help}
		}
		return m.promptKeys
	}
	if m.entry.open {
		if m.entry.list.open {
			return pickListKeys
		}
		ek := m.entryKeys
		// The keys number the tabs shown, which the Satellite tab is not
		// always among.
		ek.GoTab.SetHelp(fmt.Sprintf("alt+1-%d", len(m.entry.tabs())), ek.GoTab.Help().Desc)
		ek.Editor.SetEnabled(slices.Contains(entryEditorFields, m.entry.focus))
		picked := isPicked(m.entry.focus) && !m.entry.readOnly(m.entry.focus)
		ek.Change.SetEnabled(picked)
		ek.List.SetEnabled(picked)
		// An edited QSO never goes live, so the rig keys do nothing.
		for _, b := range []*key.Binding{&ek.Live, &ek.ResetTime, &ek.SyncRig} {
			b.SetEnabled(!m.entry.editing)
		}
		return ek
	}
	// The global keys come first so a narrow window still shows them.
	if m.pager.open {
		return joinedKeyMap{m.globalKeys(), m.pagerKeys}
	}
	return joinedKeyMap{m.keys, m.indexKeys}
}

// openHelp opens the help modal on the keybindings in effect.
func (m Model) openHelp() Model {
	m.helpModal = newHelpModal(m.contextKeys())
	return m
}

// updateHelp handles a key press while the help modal is open. No key
// reaches the screen behind it, but ctrl+c still quits.
func (m Model) updateHelp(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if key.Matches(msg, helpModalKeys.Quit) {
		return m, tea.Quit
	}
	return m, m.helpModal.update(msg, m.helpListHeight())
}

// helpListHeight is the number of rows the open help modal shows at once.
func (m Model) helpListHeight() int {
	_, _, listHeight := m.helpModal.size(m.styles, m.bodyWidth(), m.bodyHeight())
	return listHeight
}

// helpView renders the help modal for a body width by height cells.
func (m Model) helpView(width, height int) string {
	return m.helpModal.view(m.styles, width, height)
}

// overlayHelp draws the help modal centered over content, a body width by
// height cells. A body too short for the modal's frame and head shows
// content alone.
func (m Model) overlayHelp(content string, width, height int) string {
	return overlay(content, m.helpView(width, height), width, height)
}

// overlay draws box centered over content, a body width by height cells. A
// box taller than the body is not drawn, leaving content alone.
func overlay(content, box string, width, height int) string {
	if lipgloss.Height(box) > height {
		return content
	}
	x := max((width-lipgloss.Width(box))/2, 0)
	y := max((height-lipgloss.Height(box))/2, 0)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(content),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}
