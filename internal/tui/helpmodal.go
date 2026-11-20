package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// helpModalKeyMap holds the keybindings of the open help modal. Typed text
// filters it.
type helpModalKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Close key.Binding
	Quit  key.Binding
	// Erase drops the last character of the filter.
	Erase key.Binding
	// Filter only shows in the help; any text typed filters.
	Filter key.Binding
}

var helpModalKeys = helpModalKeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "scroll")),
	Down:   key.NewBinding(key.WithKeys("down", "ctrl+n")),
	Close:  key.NewBinding(key.WithKeys("esc", "?", "f1"), key.WithHelp("esc", "close")),
	Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Erase:  key.NewBinding(key.WithKeys("backspace")),
	Filter: key.NewBinding(key.WithKeys("type"), key.WithHelp("type", "filter")),
}

func (k helpModalKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Close, k.Filter, k.Quit}
}

func (k helpModalKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// helpModal lists the keybindings of the context it was opened from, in
// titled sections, filtered by what is typed. It is read-only: no key it
// lists runs from it.
type helpModal struct {
	open     bool
	sections []helpSection
	// filter is what has been typed, which a binding's key or description
	// contains.
	filter string
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
	return helpModal{open: true, sections: keys.sections()}
}

// rows returns each section's heading and its bindings that are enabled
// and have help or, once something is typed, those whose key or
// description contains it, ignoring case. A section with no binding to
// show is left out.
func (h helpModal) rows() []helpRow {
	return h.rowsMatching(h.filter)
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

// width is the width the unfiltered rows and the head line, before
// anything is typed, need.
func width(st styles, all []helpRow) int {
	w := lipgloss.Width(helpModal{}.head(st))
	kw := keyWidth(all)
	for _, r := range all {
		if r.heading != "" {
			w = max(w, lipgloss.Width(r.heading))
			continue
		}
		w = max(w, 2+kw+2+lipgloss.Width(r.binding.Help().Desc))
	}
	return w
}

// head renders the line above the rows, with the filter.
func (h helpModal) head(st styles) string {
	if h.filter == "" {
		return st.title.Render("Help") + "  " + st.muted.Render("type to filter")
	}
	return st.title.Render("Help") + "  " + st.muted.Render("filter: ") + h.filter
}

// lines renders every row in width cells: each heading, then its
// bindings, a key and what it does, below. When the filter matches
// nothing, a line says so.
func (h helpModal) lines(st styles, width int) []string {
	rows := h.rows()
	if len(rows) == 0 {
		return []string{fit(st.muted.Render("no matches"), width)}
	}
	kw := min(h.keyWidth(), max(width/2-2, 1))
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		hk := r.binding.Help()
		switch {
		case r.heading != "":
			lines = append(lines, fit(st.label.Render(r.heading), width))
		case hk.Key == "" && hk.Desc == "":
			lines = append(lines, fit("", width))
		default:
			lines = append(lines, fit("  "+pad(truncate(hk.Key, kw, "…"), kw)+"  "+st.muted.Render(hk.Desc), width))
		}
	}
	return lines
}

// size returns the outer width and height of the modal drawn in a space
// width by height cells, and the number of rows it shows at once, which is
// 0 when the space has no room for any. It takes the size its unfiltered
// rows need, so it holds still while filtering.
func (h helpModal) size(st styles, w, ht int) (outerW, outerH, listHeight int) {
	all := h.rowsMatching("")
	fw, fh := st.helpBox.GetHorizontalFrameSize(), st.helpBox.GetVerticalFrameSize()
	outerW = min(width(st, all)+fw, w)
	outerH = min(len(all)+1+fh, ht)
	return outerW, outerH, max(outerH-fh-1, 0)
}

// scrollBy scrolls the rows n rows down, or up for a negative n, stopping
// at the first and last, for a window of listHeight rows.
func (h *helpModal) scrollBy(n, listHeight int) {
	bottom := max(len(h.rows())-listHeight, 0)
	h.scroll = min(max(min(h.scroll, bottom)+n, 0), bottom)
}

// update handles a key press. Typing filters, and scrolls back to the top.
func (h *helpModal) update(msg tea.KeyPressMsg, listHeight int) {
	k := helpModalKeys
	switch {
	case key.Matches(msg, k.Close):
		h.open = false
	case key.Matches(msg, k.Up):
		h.scrollBy(-1, listHeight)
	case key.Matches(msg, k.Down):
		h.scrollBy(1, listHeight)
	case key.Matches(msg, k.Erase):
		if h.filter != "" {
			_, size := utf8.DecodeLastRuneInString(h.filter)
			h.filter, h.scroll = h.filter[:len(h.filter)-size], 0
		}
	case msg.Text != "":
		h.typeText(msg.Text)
	}
}

// typeText adds s, typed or pasted, to the filter, with each control
// character, such as a pasted line break, as a space, so the filter stays
// on one line. It scrolls back to the top.
func (h *helpModal) typeText(s string) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	h.filter, h.scroll = h.filter+s, 0
}

// view renders the modal in a box fitting a space width by height cells:
// the head line, then a window of the rows from scroll.
func (h helpModal) view(st styles, width, height int) string {
	w, _, listHeight := h.size(st, width, height)
	inner := max(w-st.helpBox.GetHorizontalFrameSize(), 1)
	lines := h.lines(st, inner)
	start := min(h.scroll, max(len(lines)-listHeight, 0))
	shown := slices.Clone(lines[start:min(start+listHeight, len(lines))])
	for len(shown) < listHeight {
		shown = append(shown, fit("", inner))
	}
	return st.helpBox.Render(strings.Join(append([]string{fit(h.head(st), inner)}, shown...), "\n"))
}

// contextKeys returns the keybindings in effect, which the footer and the
// help modal show: the open prompt's, or else the New QSO screen's or its
// open pick list's, or else the global keys and the pager's or the
// index's.
func (m Model) contextKeys() helpKeyMap {
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
	m.helpModal.update(msg, m.helpListHeight())
	return m, nil
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
	box := m.helpView(width, height)
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
