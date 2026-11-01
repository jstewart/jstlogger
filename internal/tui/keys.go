package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// keyMap holds the global keybindings. It implements help.KeyMap so the
// footer and the full help overlay stay in sync with the actual bindings.
type keyMap struct {
	Freq key.Binding
	Mode key.Binding
	Help key.Binding
	Quit key.Binding
}

// defaultKeys returns the global keybindings. The rig keys are disabled, and
// so hidden from help, unless rigEnabled is true.
func defaultKeys(rigEnabled bool) keyMap {
	k := keyMap{
		Freq: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "freq"),
		),
		Mode: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "mode"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
	k.Freq.SetEnabled(rigEnabled)
	k.Mode.SetEnabled(rigEnabled)
	return k
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Freq, k.Mode, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Freq, k.Mode}, {k.Help, k.Quit}}
}

// promptKeyMap holds the keybindings handled while an input prompt is open;
// every other key goes to the prompt.
type promptKeyMap struct {
	Submit key.Binding
	Cancel key.Binding
	Quit   key.Binding
}

func defaultPromptKeys() promptKeyMap {
	return promptKeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "submit"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

func (k promptKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel}
}

func (k promptKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.Cancel, k.Quit}}
}

// indexKeyMap holds the QSO index keybindings, modeled on NeoMutt's generic
// menu.
type indexKeyMap struct {
	Next          key.Binding
	Prev          key.Binding
	NextPage      key.Binding
	PrevPage      key.Binding
	HalfDown      key.Binding
	HalfUp        key.Binding
	NextLine      key.Binding
	PrevLine      key.Binding
	TopPage       key.Binding
	MiddlePage    key.Binding
	BottomPage    key.Binding
	First         key.Binding
	Last          key.Binding
	Jump          key.Binding
	Mark          key.Binding
	MarkPattern   key.Binding
	UnmarkPattern key.Binding
	Limit         key.Binding
	Reload        key.Binding
}

// defaultIndexKeys returns the index keybindings, disabled, and so hidden
// from help, unless enabled is true.
func defaultIndexKeys(enabled bool) indexKeyMap {
	k := indexKeyMap{
		Next: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next"),
		),
		Prev: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev"),
		),
		NextPage: key.NewBinding(
			key.WithKeys("z", "pgdown", "right"),
			key.WithHelp("z/pgdn", "next page"),
		),
		PrevPage: key.NewBinding(
			key.WithKeys("Z", "pgup", "left"),
			key.WithHelp("Z/pgup", "prev page"),
		),
		HalfDown: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "half page down"),
		),
		HalfUp: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "half page up"),
		),
		NextLine: key.NewBinding(
			key.WithKeys(">"),
			key.WithHelp(">", "scroll down"),
		),
		PrevLine: key.NewBinding(
			key.WithKeys("<"),
			key.WithHelp("<", "scroll up"),
		),
		TopPage: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "top of page"),
		),
		MiddlePage: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "middle of page"),
		),
		BottomPage: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "bottom of page"),
		),
		First: key.NewBinding(
			key.WithKeys("=", "home"),
			key.WithHelp("=/home", "newest"),
		),
		Last: key.NewBinding(
			key.WithKeys("*", "end"),
			key.WithHelp("*/end", "oldest"),
		),
		Jump: key.NewBinding(
			key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"),
			key.WithHelp("1-9", "jump to QSO"),
		),
		Mark: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "mark"),
		),
		MarkPattern: key.NewBinding(
			key.WithKeys("T"),
			key.WithHelp("T", "mark pattern"),
		),
		UnmarkPattern: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "unmark pattern"),
		),
		Limit: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "limit"),
		),
		Reload: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "reload"),
		),
	}
	for _, b := range []*key.Binding{
		&k.Next, &k.Prev, &k.NextPage, &k.PrevPage, &k.HalfDown, &k.HalfUp,
		&k.NextLine, &k.PrevLine, &k.TopPage, &k.MiddlePage, &k.BottomPage,
		&k.First, &k.Last, &k.Jump, &k.Mark, &k.MarkPattern, &k.UnmarkPattern,
		&k.Limit, &k.Reload,
	} {
		b.SetEnabled(enabled)
	}
	return k
}

func (k indexKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.Mark, k.Jump}
}

func (k indexKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Next, k.Prev, k.NextLine, k.PrevLine},
		{k.NextPage, k.PrevPage, k.HalfDown, k.HalfUp},
		{k.TopPage, k.MiddlePage, k.BottomPage, k.Jump},
		{k.First, k.Last, k.Limit, k.Reload},
		{k.Mark, k.MarkPattern, k.UnmarkPattern},
	}
}

// joinedKeyMap shows several key maps' help as one, in order.
type joinedKeyMap []help.KeyMap

func (j joinedKeyMap) ShortHelp() []key.Binding {
	var out []key.Binding
	for _, k := range j {
		out = append(out, k.ShortHelp()...)
	}
	return out
}

func (j joinedKeyMap) FullHelp() [][]key.Binding {
	var out [][]key.Binding
	for _, k := range j {
		out = append(out, k.FullHelp()...)
	}
	return out
}
