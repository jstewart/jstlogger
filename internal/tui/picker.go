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

// pickerKeyMap holds the keybindings of a picker. Typed text jumps to the
// first option starting with it.
type pickerKeyMap struct {
	Prev  key.Binding
	Next  key.Binding
	First key.Binding
	Last  key.Binding
	// Erase drops the last character typed.
	Erase key.Binding
}

var pickerKeys = pickerKeyMap{
	Prev:  key.NewBinding(key.WithKeys("left", "up")),
	Next:  key.NewBinding(key.WithKeys("right", "down")),
	First: key.NewBinding(key.WithKeys("home")),
	Last:  key.NewBinding(key.WithKeys("end")),
	Erase: key.NewBinding(key.WithKeys("backspace")),
}

// picker chooses one of a fixed list of options, such as the ADIF modes.
// The arrow keys step through the options and typing jumps to the first
// option that starts with what was typed, ignoring case.
type picker struct {
	// options are the codes picked from, each with what it means, if
	// anything.
	options []adif.Enum
	i       int
	// typed is what has been typed since the last step.
	typed string
}

func newPicker(options []adif.Enum) picker {
	return picker{options: options}
}

// codeOptions returns codes as options that need no description, such as
// station names.
func codeOptions(codes []string) []adif.Enum {
	out := make([]adif.Enum, len(codes))
	for i, c := range codes {
		out[i] = adif.Enum{Code: c}
	}
	return out
}

// value returns the chosen option, or "" when there are none.
func (p picker) value() string {
	if p.i >= len(p.options) {
		return ""
	}
	return p.options[p.i].Code
}

// set chooses the option equal to v, ignoring case, reporting false, and
// leaving the choice alone, when there is none.
func (p *picker) set(v string) bool {
	i := slices.IndexFunc(p.options, func(o adif.Enum) bool { return strings.EqualFold(o.Code, v) })
	if i < 0 {
		return false
	}
	p.i, p.typed = i, ""
	return true
}

// offer adds v as the last option and chooses it, as for a stored value
// that is none of the options, so it shows and saves as it is.
func (p *picker) offer(v string) {
	p.options = append(slices.Clip(p.options), adif.Enum{Code: v})
	p.i, p.typed = len(p.options)-1, ""
}

// setOptions replaces the options, keeping the chosen value if it is still
// offered and choosing the first option otherwise.
func (p *picker) setOptions(options []adif.Enum) {
	v := p.value()
	p.options, p.i, p.typed = options, 0, ""
	p.set(v)
}

// update handles a key press, reporting whether the chosen value changed.
func (p *picker) update(msg tea.KeyPressMsg) bool {
	if len(p.options) == 0 {
		return false
	}
	old := p.i
	k := pickerKeys
	switch {
	case key.Matches(msg, k.Prev):
		p.i, p.typed = max(p.i-1, 0), ""
	case key.Matches(msg, k.Next):
		p.i, p.typed = min(p.i+1, len(p.options)-1), ""
	case key.Matches(msg, k.First):
		p.i, p.typed = 0, ""
	case key.Matches(msg, k.Last):
		p.i, p.typed = len(p.options)-1, ""
	case key.Matches(msg, k.Erase):
		if p.typed == "" {
			break
		}
		_, n := utf8.DecodeLastRuneInString(p.typed)
		p.typed = p.typed[:len(p.typed)-n]
		if p.typed == "" {
			p.i = 0
		} else {
			p.jump(p.typed)
		}
	case msg.Text != "":
		// What was typed so far and this key, or failing that a fresh start
		// from this key.
		if !p.jump(p.typed + msg.Text) {
			p.jump(msg.Text)
		}
	}
	return p.i != old
}

// jump chooses the first option starting with prefix and remembers it as
// typed, reporting false, and changing nothing, when no option does.
func (p *picker) jump(prefix string) bool {
	i := slices.IndexFunc(p.options, func(o adif.Enum) bool {
		return strings.HasPrefix(strings.ToUpper(o.Code), strings.ToUpper(prefix))
	})
	if i < 0 {
		return false
	}
	p.i, p.typed = i, prefix
	return true
}

// view renders the chosen value, or pagerEmpty for none, between arrows
// while the picker has focus, followed by what it means, cut short to fit
// width cells. The value is never cut.
func (p picker) view(st styles, focused bool, width int) string {
	var o adif.Enum
	if p.i < len(p.options) {
		o = p.options[p.i]
	}
	v := o.Code
	if v == "" {
		v = st.muted.Render(pagerEmpty)
	}
	if focused {
		v = st.muted.Render("‹ ") + v + st.muted.Render(" ›")
	}
	// A description needs a cell after the space, and one for the "…".
	if room := width - lipgloss.Width(v) - 1; o.Description != "" && room > 1 {
		v += " " + st.muted.Render(truncate(o.Description, room, "…"))
	}
	return v
}
