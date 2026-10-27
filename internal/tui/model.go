// Package tui contains the Bubble Tea models that make up the jstlogger UI.
package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/rig"
)

// Model is the root model. It owns the screen layout (header, body, footer)
// and global keybindings; feature screens will be composed into the body.
type Model struct {
	keys       keyMap
	promptKeys promptKeyMap
	help       help.Model
	styles     styles

	// rig is nil when no rig is configured.
	rig      RigController
	rigState rigState
	// rigErr is the error from the last rig command, shown until the next
	// successful command or key press.
	rigErr error

	prompt    promptKind
	input     textinput.Model
	promptErr error

	width  int
	height int
}

// New returns the root model configured by opts.
func New(opts ...Option) Model {
	m := Model{
		promptKeys: defaultPromptKeys(),
		help:       help.New(),
		styles:     defaultStyles(),
		input:      newInput(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.keys = defaultKeys(m.rig != nil)
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.sizeInput()

	case tea.KeyPressMsg:
		if m.promptOpen() {
			return m.updatePrompt(msg)
		}
		m.rigErr = nil
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Freq):
			return m.openPrompt(promptFreq)
		case key.Matches(msg, m.keys.Mode):
			return m.openPrompt(promptMode)
		}

	case rig.StateChanged:
		m.rigState.state = msg.State
		m.rigState.haveState = true

	case rig.ConnectionChanged:
		m.rigState.conn = msg

	case rigResultMsg:
		m.rigErr = msg.err

	default:
		// Other input, such as pastes, goes to the open prompt.
		if m.promptOpen() {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	header := m.headerView()
	footer := m.footerView()

	bodyHeight := max(m.height-lipgloss.Height(header)-lipgloss.Height(footer), 0)
	body := m.styles.body.
		Width(m.width).
		Height(bodyHeight).
		Render(m.styles.muted.Render("No QSOs yet."))

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
	v.AltScreen = true
	return v
}

// headerView renders the title and, when a rig is configured, the rig status
// right-aligned and truncated to the terminal width.
func (m Model) headerView() string {
	title := m.styles.title.Render("jstlogger")
	if m.rig == nil {
		return m.styles.header.Render(title)
	}

	status := m.rigStatusView()
	inner := m.width - m.styles.header.GetHorizontalFrameSize()
	gap := max(inner-lipgloss.Width(title)-lipgloss.Width(status), 1)
	return m.styles.header.
		MaxWidth(m.width).
		Render(title + strings.Repeat(" ", gap) + status)
}

// footerView renders the help line, preceded by the open prompt, the last
// rig command error, or the reason the rig is disconnected.
func (m Model) footerView() string {
	var lines []string
	var keys help.KeyMap = m.keys
	conn := m.rigState.conn
	switch {
	case m.promptOpen():
		lines = append(lines, m.promptView())
		keys = m.promptKeys
	case m.rigErr != nil:
		lines = append(lines, m.styles.err.Render(m.rigErr.Error()))
	case conn.Status == rig.Disconnected && conn.Err != nil:
		lines = append(lines, m.styles.err.Render("rig: "+conn.Err.Error()))
	}
	lines = append(lines, m.help.View(keys))
	return m.styles.footer.
		MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}
