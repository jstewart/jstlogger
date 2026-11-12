// Package tui contains the Bubble Tea models that make up the jstlogger UI.
package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/callsign"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
	"jstlogger/internal/rig"
)

// Model is the root model. It owns the screen layout (header, body, footer)
// and global keybindings; the QSO index fills the body.
type Model struct {
	keys       keyMap
	indexKeys  indexKeyMap
	pagerKeys  pagerKeyMap
	promptKeys promptKeyMap
	entryKeys  entryKeyMap
	help       help.Model
	styles     styles

	// rig is nil when no rig is configured.
	rig      RigController
	rigName  string
	rigState rigState
	// rigErr is the error from the last rig command, shown until the next
	// successful command or key press.
	rigErr error

	// openURL opens a URL in the browser; nil disables the pager's lookup
	// keys.
	openURL func(ctx context.Context, url string) error
	// lookupErr is the error from the last lookup, shown until the next
	// successful lookup or key press.
	lookupErr error

	// lotw looks up the LoTW use of the pager QSO and of the index rows
	// a %L column shows; nil shows none.
	lotw LoTWLookup
	// lotwErr is the error from the last LoTW sync or lookup, shown
	// until the next successful sync or key press.
	lotwErr error

	// eqsl looks up the pager QSO's eQSL AG membership; nil shows none.
	eqsl EQSLLookup
	// eqslErr is the error from the last eQSL sync or pager lookup, shown
	// until the next successful sync or key press.
	eqslErr error

	// now returns the current time.
	now func() time.Time
	// tick is tea.Tick, which the tests replace to send the ticks
	// themselves.
	tick func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd

	// logbook is nil when no logbook is open.
	logbook logbook.Store
	idx     index
	// pager is shown in the body, below a strip of the index, while open.
	pager pager
	// entry is the New QSO screen, shown in the body in place of the index
	// while open.
	entry entry
	// stations are the station profiles the New QSO screen picks our side
	// of each QSO from, the first by default.
	stations []qsoentry.Station

	// markedFuncs are the index functions that accept the mark-prefix. The
	// bulk operations register here.
	markedFuncs []markedFunc

	prompt    promptKind
	input     textinput.Model
	promptErr error
	// confirm is the y/N question open while prompt is promptConfirm.
	confirm *pendingConfirm
	// patterns is the history shared by the pattern prompts.
	patterns history

	// version is the program version written to exported files.
	version string

	width  int
	height int
}

// New returns the root model configured by opts.
func New(opts ...Option) Model {
	m := Model{
		promptKeys: defaultPromptKeys(),
		entryKeys:  defaultEntryKeys(),
		help:       help.New(),
		styles:     defaultStyles(),
		input:      newInput(),
		idx:        newIndex(),
		pager:      pager{indexLines: defaultPagerIndexLines},
		entry:      newEntry(),
		now:        time.Now,
		tick:       tea.Tick,
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.idx.lotw.now = m.now
	m.keys = defaultKeys(m.rig != nil)
	m.pagerKeys = defaultPagerKeys(m.openURL != nil)
	m.indexKeys = defaultIndexKeys(m.logbook != nil)
	if m.logbook != nil {
		m.markedFuncs = m.bulkFuncs()
	}
	return m
}

// WithVersion sets the program version written to exported files.
func WithVersion(v string) Option {
	return func(m *Model) {
		m.version = v
	}
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
		m.lookupErr = nil
		m.lotwErr = nil
		m.eqslErr = nil
		m.idx.msg = ""
		if m.entry.open {
			return m.updateEntry(msg)
		}
		if m.idx.prefix {
			return m.updatePrefix(msg)
		}
		// The lookup-prefix comes before the global keys, so gq opens QRZ
		// rather than closing the pager.
		if m.pager.lookupPrefix {
			return m.updateLookup(msg)
		}
		keys := m.globalKeys()
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Help):
			m.help.ShowAll = !m.help.ShowAll
		case key.Matches(msg, keys.Freq):
			return m.openPrompt(promptFreq)
		case key.Matches(msg, keys.Mode):
			return m.openPrompt(promptMode)
		case m.pager.open:
			return m.updatePager(msg)
		default:
			return m.updateIndex(msg)
		}

	case indexLoadedMsg:
		if m.idx.applyLoad(msg) {
			return m, m.lookupIndexLoTW(msg.rows)
		}

	case indexBlockMsg:
		if m.idx.applyBlock(msg) {
			return m, m.lookupIndexLoTW(msg.rows)
		}

	case indexLimitMsg:
		if m.idx.applyLimit(msg) {
			return m, m.lookupIndexLoTW(msg.rows)
		}

	case indexMarksMsg:
		m.idx.applyMarks(msg)

	case indexSearchMsg:
		m.idx.applySearch(msg)

	case indexBulkMsg:
		return m.applyBulk(msg)

	case entryTagsMsg:
		m.entry.applyTags(msg)

	case entrySavedMsg:
		return m.applySaved(msg)

	case entryTickMsg:
		return m.tickEntry(msg)

	case pagerQSOMsg:
		q, _ := m.idx.current()
		m.pager.applyQSO(msg, q.ID)

	case lotwLookupMsg:
		q, _ := m.idx.current()
		if m.pager.applyLoTW(msg, callsign.Base(q.Call)) && msg.err != nil {
			m.lotwErr = msg.err
		}

	case entryLoTWMsg:
		m.applyEntryLoTW(msg)

	case lotwUsersMsg:
		if err := m.idx.lotw.apply(msg); err != nil {
			m.lotwErr = err
		}

	case LoTWSynced:
		return m, m.applyLoTWSynced(msg)

	case eqslLookupMsg:
		q, _ := m.idx.current()
		if m.pager.applyEQSL(msg, callsign.Base(q.Call)) && msg.err != nil {
			m.eqslErr = msg.err
		}

	case EQSLSynced:
		m.applyEQSLSynced(msg)

	case rig.StateChanged:
		m.rigState.state = msg.State
		m.rigState.haveState = true
		m.applyRigState(msg.State)

	case rig.ConnectionChanged:
		m.rigState.conn = msg

	case rigResultMsg:
		m.rigErr = msg.err

	case lookupResultMsg:
		m.lookupErr = msg.err

	default:
		// Other input, such as pastes, goes to the open prompt, but not to
		// a y/N question, or else to the New QSO screen.
		if m.promptOpen() && m.prompt != promptConfirm {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.entry.open && !m.promptOpen() {
			return m.editEntry(msg)
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	header := m.headerView()
	footer := m.footerView()

	bodyHeight := m.bodyHeight()
	content := m.styles.muted.Render("No QSOs yet.")
	switch {
	case m.logbook != nil && m.entry.open:
		content = m.entryView(m.bodyWidth(), bodyHeight)
	case m.logbook != nil && m.pager.open:
		content = m.pagerView(m.bodyWidth(), bodyHeight)
	case m.logbook != nil:
		content = m.idx.view(m.styles, m.bodyWidth(), bodyHeight, m.prompt == promptJump)
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
// rig command, lookup, LoTW sync or eQSL sync error.
func (m Model) footerView() string {
	var lines []string
	// The global keys come first so a narrow window still shows them.
	var keys help.KeyMap = joinedKeyMap{m.keys, m.indexKeys}
	if m.pager.open {
		keys = joinedKeyMap{m.globalKeys(), m.pagerKeys}
	}
	if m.entry.open {
		keys = m.entryKeys
	}
	switch {
	case m.promptOpen():
		lines = append(lines, m.promptView())
		keys = m.promptKeys
		if m.prompt == promptConfirm {
			// Only y answers yes; enter and esc are no like any other key.
			keys = promptKeyMap{Quit: m.promptKeys.Quit}
		}
	case m.rigErr != nil:
		lines = append(lines, m.styles.err.Render(m.rigErr.Error()))
	case m.lookupErr != nil:
		lines = append(lines, m.styles.err.Render(m.lookupErr.Error()))
	default:
		// Both background syncs can fail at once, as when offline, so
		// neither error hides the other.
		for _, err := range []error{m.lotwErr, m.eqslErr} {
			if err != nil {
				lines = append(lines, m.styles.err.Render(err.Error()))
			}
		}
	}
	lines = append(lines, m.help.View(keys))
	return m.styles.footer.
		MaxWidth(m.width).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// bodyHeight is the height of the body, between the header and footer.
func (m Model) bodyHeight() int {
	return max(m.height-lipgloss.Height(m.headerView())-lipgloss.Height(m.footerView()), 0)
}

// bodyWidth is the width inside the body's padding.
func (m Model) bodyWidth() int {
	return max(m.width-m.styles.body.GetHorizontalFrameSize(), 0)
}

// layout fits the index to the space between the header and footer, which
// change height with the prompt and help, or to the strip above the open
// pager, and returns a command loading any rows that come into view and
// the whole QSO the pager shows, and its LoTW use and eQSL AG membership,
// and the LoTW use of the call typed into the New QSO screen.
func (m *Model) layout() tea.Cmd {
	// A reload or bulk delete may have left no QSO for the pager to show.
	if m.idx.total == 0 && m.pager.open {
		*m = m.closePager()
	}
	bodyHeight := m.height - lipgloss.Height(m.headerView()) - lipgloss.Height(m.footerView())
	if m.pager.open {
		m.idx.setHeight(pagerStripHeight(m.pager.indexLines, bodyHeight))
	} else {
		m.idx.setHeight(bodyHeight - indexChrome)
	}
	m.sizeEntryNotes(bodyHeight)
	if m.logbook == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, b := range m.idx.missing() {
		cmds = append(cmds, loadBlockCmd(m.logbook, m.idx.gen, m.idx.limit.expr, b))
	}
	return tea.Batch(append(cmds, m.fetch(), m.fetchLoTW(), m.fetchEQSL(), m.fetchEntryLoTW())...)
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
	case key.Matches(msg, k.Open):
		return m.openPager(), nil
	case key.Matches(msg, k.NewQSO):
		return m.openEntry()
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
	case key.Matches(msg, k.Search, k.SearchReverse, k.SearchNext, k.SearchOpposite) && !x.loaded:
		// There is nothing to search until the index loads.
	case key.Matches(msg, k.Search):
		return m.openPrompt(promptSearch)
	case key.Matches(msg, k.SearchReverse):
		return m.openPrompt(promptSearchReverse)
	case key.Matches(msg, k.SearchNext):
		return m, m.searchAgain(false)
	case key.Matches(msg, k.SearchOpposite):
		return m, m.searchAgain(true)
	case key.Matches(msg, k.Delete):
		return m.askDeleteCurrent(), nil
	case key.Matches(msg, k.MarkPrefix) && x.loaded:
		x.armPrefix()
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
