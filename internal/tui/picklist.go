package tui

import (
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/adif"
)

const (
	// pickListHeight is the number of lines of items a pick list shows at
	// once.
	pickListHeight = 8
	// entryListMinWidth is the narrowest a list lines up under its value
	// in.
	entryListMinWidth = 20
)

// pickListKeyMap holds the keybindings of an open pick list. Typed text
// filters it.
type pickListKeyMap struct {
	Up    key.Binding
	Down  key.Binding
	Pick  key.Binding
	Close key.Binding
	// Erase drops the last character of the filter.
	Erase key.Binding
	// Filter only shows in the help; any text typed filters.
	Filter key.Binding
	// Leave closes the list, without picking, and moves to another field.
	Leave key.Binding
}

var pickListKeys = pickListKeyMap{
	Up:     key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "move")),
	Down:   key.NewBinding(key.WithKeys("down", "ctrl+n")),
	Pick:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "pick")),
	Close:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	Erase:  key.NewBinding(key.WithKeys("backspace")),
	Filter: key.NewBinding(key.WithKeys("type"), key.WithHelp("type", "filter")),
	Leave:  key.NewBinding(key.WithKeys("tab", "shift+tab")),
}

func (k pickListKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Pick, k.Close, k.Filter}
}

func (k pickListKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

// pickList is the list a picker opens to choose from: each option with
// what it means, below any common ones, filtered by what is typed.
type pickList struct {
	open bool
	// common are offered above the options, such as the most used modes,
	// and may include values none of the options are.
	common  []adif.Enum
	options []adif.Enum
	// filter is what has been typed, which an item's code or description
	// contains.
	filter string
	// cursor is the index in items of the item highlighted.
	cursor int
}

// pickItem is an item of a pick list, with its index in the options, or -1
// for a common item none of the options is.
type pickItem struct {
	adif.Enum
	i int
}

// newPickList returns an open list of common and then options, with the
// first item equal to value, ignoring case, highlighted, or the first item
// when value is "", so the common ones show.
func newPickList(common, options []adif.Enum, value string) pickList {
	l := pickList{open: true, common: common, options: options}
	if value != "" {
		l.cursor = max(slices.IndexFunc(l.items(), func(it pickItem) bool { return strings.EqualFold(it.Code, value) }), 0)
	}
	return l
}

// items returns the common items and then the options or, once something
// is typed, those whose code or description contains it, ignoring case,
// each once.
func (l pickList) items() []pickItem {
	var all []pickItem
	for _, c := range l.common {
		i := slices.IndexFunc(l.options, func(o adif.Enum) bool { return strings.EqualFold(o.Code, c.Code) })
		all = append(all, pickItem{c, i})
	}
	for i, o := range l.options {
		all = append(all, pickItem{o, i})
	}
	if l.filter == "" {
		return all
	}
	f := strings.ToLower(l.filter)
	var out []pickItem
	for _, it := range all {
		if !strings.Contains(strings.ToLower(it.Code), f) && !strings.Contains(strings.ToLower(it.Description), f) {
			continue
		}
		if !slices.ContainsFunc(out, func(o pickItem) bool { return strings.EqualFold(o.Code, it.Code) }) {
			out = append(out, it)
		}
	}
	return out
}

// sep returns the index in items of the first option, after the common
// items, or -1 when no line separates them.
func (l pickList) sep() int {
	if l.filter != "" || len(l.common) == 0 {
		return -1
	}
	return len(l.common)
}

// update handles a key press, returning the item picked and true when one
// is. Picking an item or cancelling closes the list.
func (l *pickList) update(msg tea.KeyPressMsg) (pickItem, bool) {
	k := pickListKeys
	n := len(l.items())
	switch {
	case key.Matches(msg, k.Up):
		l.cursor = max(l.cursor-1, 0)
	case key.Matches(msg, k.Down):
		l.cursor = max(min(l.cursor+1, n-1), 0)
	case key.Matches(msg, k.Pick):
		if l.cursor >= n {
			break
		}
		l.open = false
		return l.items()[l.cursor], true
	case key.Matches(msg, k.Close):
		l.open = false
	case key.Matches(msg, k.Erase):
		if l.filter != "" {
			_, size := utf8.DecodeLastRuneInString(l.filter)
			l.filter, l.cursor = l.filter[:len(l.filter)-size], 0
		}
	case msg.Text != "":
		l.filter, l.cursor = l.filter+msg.Text, 0
	}
	return pickItem{}, false
}

// view renders the list in width cells: a line with the filter, then a
// window of up to pickListHeight lines of items around the one
// highlighted, each a code and what it means, with a line between the
// common items and the rest.
func (l pickList) view(st styles, width int) []string {
	head := st.muted.Render("type to filter")
	if l.filter != "" {
		head = st.muted.Render("filter: ") + l.filter
	}
	items := l.items()
	if len(items) == 0 {
		return []string{fit(head, width), fit(st.muted.Render("no matches"), width)}
	}
	codeWidth := 0
	for _, it := range items {
		codeWidth = max(codeWidth, lipgloss.Width(it.Code))
	}
	// A long code is cut, leaving at least half the width to say what it
	// means.
	codeWidth = min(max(codeWidth, lipgloss.Width(pagerEmpty)), max(width/2-2, 1))
	var lines []string
	cursorLine := 0
	for i, it := range items {
		if i == l.sep() {
			lines = append(lines, fit(st.muted.Render(strings.Repeat("─", width)), width))
		}
		code := it.Code
		if code == "" {
			code = pagerEmpty
		}
		code = pad(truncate(code, codeWidth, "…"), codeWidth)
		if i == l.cursor {
			cursorLine = len(lines)
			lines = append(lines, st.cursor.Render(fit(code+"  "+it.Description, width)))
			continue
		}
		lines = append(lines, fit(code+"  "+st.muted.Render(it.Description), width))
	}
	start := max(min(cursorLine-pickListHeight/2, len(lines)-pickListHeight), 0)
	end := min(start+pickListHeight, len(lines))
	return append([]string{fit(head, width)}, lines[start:end]...)
}
