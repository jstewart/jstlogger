package tui

import "charm.land/bubbles/v2/key"

// keyMap holds the global keybindings. It implements help.KeyMap so the
// footer and the full help overlay stay in sync with the actual bindings.
type keyMap struct {
	Help key.Binding
	Quit key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Help, k.Quit}}
}
