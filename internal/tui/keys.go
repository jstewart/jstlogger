package tui

import "charm.land/bubbles/v2/key"

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
