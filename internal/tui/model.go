// Package tui contains the Bubble Tea models that make up the jstlogger UI.
package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Model is the root model. It owns the screen layout (header, body, footer)
// and global keybindings; feature screens will be composed into the body.
type Model struct {
	keys   keyMap
	help   help.Model
	styles styles

	width  int
	height int
}

func New() Model {
	return Model{
		keys:   defaultKeys(),
		help:   help.New(),
		styles: defaultStyles(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	header := m.styles.header.Render("jstlogger")
	footer := m.styles.footer.Render(m.help.View(m.keys))

	bodyHeight := max(m.height-lipgloss.Height(header)-lipgloss.Height(footer), 0)
	body := m.styles.body.
		Width(m.width).
		Height(bodyHeight).
		Render(m.styles.muted.Render("No QSOs yet."))

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
	v.AltScreen = true
	return v
}
