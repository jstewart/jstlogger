// Package tui contains the Bubble Tea models that make up the jstlogger UI.
package tui

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/logbook"
	"jstlogger/internal/rig"
)

// Model is the root model. It owns the screen layout (header, body, footer)
// and global keybindings; the QSO index fills the body.
type Model struct {
	keys       keyMap
	indexKeys  indexKeyMap
	promptKeys promptKeyMap
	help       help.Model
	styles     styles

	// rig is nil when no rig is configured.
	rig      RigController
	rigName  string
	rigState rigState
	// rigErr is the error from the last rig command, shown until the next
	// successful command or key press.
	rigErr error

	// logbook is nil when no logbook is open.
	logbook logbook.Store
	idx     index

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
		idx:        newIndex(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.keys = defaultKeys(m.rig != nil)
	m.indexKeys = defaultIndexKeys(m.logbook != nil)
	return m
}

// Init loads the QSO index when a logbook is open.
func (m Model) Init() tea.Cmd {
	if m.logbook == nil {
		return nil
	}
	return loadIndexCmd(m.logbook, m.idx.gen, m.idx.limit.expr, 0, 0)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := m.update(msg)
	return m, tea.Batch(cmd, m.layout())
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
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
		m.idx.msg = ""
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, m.keys.Freq):
			return m.openPrompt(promptFreq)
		case key.Matches(msg, m.keys.Mode):
			return m.openPrompt(promptMode)
		default:
			return m.updateIndex(msg)
		}

	case indexLoadedMsg:
		m.idx.applyLoad(msg)

	case indexBlockMsg:
		m.idx.applyBlock(msg)

	case indexLimitMsg:
		m.idx.applyLimit(msg)

	case indexMarksMsg:
		m.idx.applyMarks(msg)

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
	content := m.styles.muted.Render("No QSOs yet.")
	if m.logbook != nil {
		content = m.idx.view(m.styles, m.bodyWidth(), bodyHeight)
	}
	body := m.styles.body.
		Width(m.width).
		Height(bodyHeight).
		Render(content)

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

// footerView renders the help line, preceded by the open prompt or the last
// rig command error.
func (m Model) footerView() string {
	var lines []string
	// The global keys come first so a narrow window still shows them.
	var keys help.KeyMap = joinedKeyMap{m.keys, m.indexKeys}
	switch {
	case m.promptOpen():
		lines = append(lines, m.promptView())
		keys = m.promptKeys
	case m.rigErr != nil:
		lines = append(lines, m.styles.err.Render(m.rigErr.Error()))
	}
	lines = append(lines, m.help.View(keys))
	return m.styles.footer.
		MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// bodyWidth is the width inside the body's padding.
func (m Model) bodyWidth() int {
	return max(m.width-m.styles.body.GetHorizontalFrameSize(), 0)
}

// layout fits the index to the space between the header and footer, which
// change height with the prompt and help, and returns a command loading any
// rows that come into view.
func (m *Model) layout() tea.Cmd {
	bodyHeight := m.height - lipgloss.Height(m.headerView()) - lipgloss.Height(m.footerView())
	m.idx.setHeight(bodyHeight - indexChrome)
	if m.logbook == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, b := range m.idx.missing() {
		cmds = append(cmds, loadBlockCmd(m.logbook, m.idx.gen, m.idx.limit.expr, b))
	}
	return tea.Batch(cmds...)
}

// updateIndex handles a key press on the QSO index.
func (m Model) updateIndex(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k, x := m.indexKeys, &m.idx
	switch {
	case key.Matches(msg, k.Next):
		x.next()
	case key.Matches(msg, k.Prev):
		x.prev()
	case key.Matches(msg, k.NextPage):
		x.nextPage()
	case key.Matches(msg, k.PrevPage):
		x.prevPage()
	case key.Matches(msg, k.HalfDown):
		x.halfDown()
	case key.Matches(msg, k.HalfUp):
		x.halfUp()
	case key.Matches(msg, k.NextLine):
		x.nextLine()
	case key.Matches(msg, k.PrevLine):
		x.prevLine()
	case key.Matches(msg, k.TopPage):
		x.topPage()
	case key.Matches(msg, k.MiddlePage):
		x.middlePage()
	case key.Matches(msg, k.BottomPage):
		x.bottomPage()
	case key.Matches(msg, k.First):
		x.first()
	case key.Matches(msg, k.Last):
		x.last()
	case key.Matches(msg, k.Mark):
		x.toggleMark()
	case key.Matches(msg, k.Jump):
		next, cmd := m.openPrompt(promptJump)
		next.input.SetValue(msg.String())
		next.input.CursorEnd()
		return next, cmd
	case key.Matches(msg, k.Limit):
		return m.openPrompt(promptLimit)
	case key.Matches(msg, k.MarkPattern):
		return m.openPrompt(promptMark)
	case key.Matches(msg, k.UnmarkPattern):
		return m.openPrompt(promptUnmark)
	case key.Matches(msg, k.Reload):
		return m.reloadIndex()
	}
	return m, nil
}

// reloadIndex clears and redraws the screen, and reloads the QSO count and
// rows under the current limit, keeping the cursor on the same QSO when it
// still matches. Loads already under way, including a new limit, are
// dropped. The redraw repairs a screen the terminal has drawn wrong.
func (m Model) reloadIndex() (Model, tea.Cmd) {
	id := m.idx.restart()
	return m, tea.Batch(tea.ClearScreen, loadIndexCmd(m.logbook, m.idx.gen, m.idx.limit.expr, id, m.idx.cursor))
}
