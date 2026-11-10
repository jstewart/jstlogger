package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
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
	options []string
	i       int
	// typed is what has been typed since the last step.
	typed string
}

func newPicker(options []string) picker {
	return picker{options: options}
}

// value returns the chosen option, or "" when there are none.
func (p picker) value() string {
	if p.i >= len(p.options) {
		return ""
	}
	return p.options[p.i]
}

// set chooses the option equal to v, ignoring case, reporting false, and
// leaving the choice alone, when there is none.
func (p *picker) set(v string) bool {
	i := slices.IndexFunc(p.options, func(o string) bool { return strings.EqualFold(o, v) })
	if i < 0 {
		return false
	}
	p.i, p.typed = i, ""
	return true
}

// setOptions replaces the options, keeping the chosen value if it is still
// offered and choosing the first option otherwise.
func (p *picker) setOptions(options []string) {
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
		p.typed = p.typed[:len(p.typed)-1]
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
	i := slices.IndexFunc(p.options, func(o string) bool {
		return strings.HasPrefix(strings.ToUpper(o), strings.ToUpper(prefix))
	})
	if i < 0 {
		return false
	}
	p.i, p.typed = i, prefix
	return true
}

// view renders the chosen value, or pagerEmpty for none, between arrows
// while the picker has focus.
func (p picker) view(st styles, focused bool) string {
	v := p.value()
	if v == "" {
		v = st.muted.Render(pagerEmpty)
	}
	if !focused {
		return v
	}
	return st.muted.Render("‹ ") + v + st.muted.Render(" ›")
}
