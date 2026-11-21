package tui

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/rig"
)

const (
	// rigModalTitle heads the rig modal, beside rigModalBadge.
	rigModalTitle = "rig"
	rigModalBadge = " esc cancel "
	// rigModalMinWidth is the narrowest the rig modal is drawn, in a body
	// at least that wide.
	rigModalMinWidth = 40
	// rigLabelWidth is the width of the rig modal's field labels.
	rigLabelWidth = len("Passband")
	// rigFocusMark marks the focused field of the rig modal.
	rigFocusMark = "▸ "
	// recentModesSize caps the modes the rig modal lists first.
	recentModesSize = 5
)

// rigField identifies a field of the rig modal.
type rigField int

const (
	rigFieldFreq rigField = iota
	rigFieldMode
	rigFieldPassband
	rigFieldCount
)

// rigModalKeyMap holds the keybindings of the open rig modal. Every other
// key goes to the focused field.
type rigModalKeyMap struct {
	Apply  key.Binding
	Cancel key.Binding
	// Next and Prev move to the next and previous field, round from the
	// last to the first.
	Next key.Binding
	Prev key.Binding
	// HistoryPrev and HistoryNext browse the frequency history. They are
	// enabled only while the frequency is focused.
	HistoryPrev key.Binding
	HistoryNext key.Binding
	// ModeUp and ModeDown move through the modes and Filter, which only
	// shows in the help, filters them by any text typed. They are enabled
	// only while the mode is focused.
	ModeUp   key.Binding
	ModeDown key.Binding
	Filter   key.Binding
	// Help opens the help modal; ? is typed into the field.
	Help key.Binding
	Quit key.Binding
}

var rigModalKeys = rigModalKeyMap{
	Apply:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
	Cancel:      key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Next:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab/shift+tab", "next field")),
	Prev:        key.NewBinding(key.WithKeys("shift+tab")),
	HistoryPrev: key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/ctrl+p", "prev freq")),
	HistoryNext: key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓/ctrl+n", "next freq")),
	ModeUp:      key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑/↓", "choose mode")),
	ModeDown:    key.NewBinding(key.WithKeys("down", "ctrl+n")),
	Filter:      key.NewBinding(key.WithKeys("type"), key.WithHelp("type", "filter modes")),
	Help:        helpKey(),
	Quit:        key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}

func (k rigModalKeyMap) ShortHelp() []key.Binding {
	// Apply and cancel come first so a narrow window still shows them.
	return []key.Binding{k.Apply, k.Cancel, k.Next, k.HistoryPrev, k.ModeUp, k.Filter, k.Help}
}

func (k rigModalKeyMap) FullHelp() [][]key.Binding {
	return sectionKeys(k.sections())
}

func (k rigModalKeyMap) sections() []helpSection {
	return []helpSection{
		{"rig", []key.Binding{k.Apply, k.Cancel, k.Next, k.Prev, k.Help, k.Quit}},
		{"freq history", []key.Binding{k.HistoryPrev, k.HistoryNext}},
		{"mode list", []key.Binding{k.ModeUp, k.ModeDown, k.Filter}},
	}
}

// rigModal sets the rig's frequency, mode and passband, each a field
// filled in from the rig's state when it opens. Applying it sends the rig
// only the fields changed.
type rigModal struct {
	open  bool
	focus rigField
	freq  textinput.Model
	// mode lists the modes, the one highlighted chosen; its cursor is -1
	// while none is.
	mode     pickList
	passband textinput.Model

	// haveState is true when the fields were filled in from the rig's
	// state, which was initHz, initMode and initPassband.
	haveState    bool
	initHz       int64
	initMode     string
	initPassband string

	// err is the last parse or rig error, shown until the next apply.
	err error
	// pending is true while the change applying, numbered gen, is sent to
	// the rig.
	pending  bool
	gen      int
	applying rigChange
}

// newRigModal returns an open rig modal filled in from s, listing the
// recent modes first.
func newRigModal(s rigState, recent []string) rigModal {
	r := rigModal{open: true, freq: newInput(), passband: newInput()}
	r.freq.Prompt, r.freq.Placeholder = "", "14.074, 7074 kHz…"
	r.passband.Prompt, r.passband.Placeholder = "", "rig default"
	if s.haveState {
		r.haveState, r.initHz = true, s.state.Freq
		r.initMode = strings.ToUpper(s.state.Mode)
		r.freq.SetValue(formatFreq(s.state.Freq))
		r.freq.CursorEnd()
		if s.state.Passband > 0 {
			r.initPassband = strconv.Itoa(s.state.Passband)
			r.passband.SetValue(r.initPassband)
			r.passband.CursorEnd()
		}
	}
	common := codeOptions(recent)
	// A mode the rig reports that is not a Hamlib one is still offered.
	if r.initMode != "" && !slices.Contains(rig.Modes, r.initMode) && !slices.Contains(recent, r.initMode) {
		common = append(codeOptions([]string{r.initMode}), common...)
	}
	r.mode = newPickList(common, codeOptions(rig.Modes), r.initMode)
	if r.initMode == "" {
		r.mode.cursor = -1
	}
	return r
}

// modeValue returns the mode chosen, or "" when none is.
func (r rigModal) modeValue() string {
	items := r.mode.items()
	if r.mode.cursor < 0 || r.mode.cursor >= len(items) {
		return ""
	}
	return items[r.mode.cursor].Code
}

// setFocus focuses field f.
func (r *rigModal) setFocus(f rigField) tea.Cmd {
	r.focus = f
	r.freq.Blur()
	r.passband.Blur()
	switch f {
	case rigFieldFreq:
		return r.freq.Focus()
	case rigFieldPassband:
		return r.passband.Focus()
	}
	return nil
}

// keys returns the modal's keybindings, those of a field enabled only while
// it is focused, and only cancel and quit while a change is applying.
func (r rigModal) keys() rigModalKeyMap {
	k := rigModalKeys
	for _, b := range []*key.Binding{&k.HistoryPrev, &k.HistoryNext} {
		b.SetEnabled(r.focus == rigFieldFreq && !r.pending)
	}
	for _, b := range []*key.Binding{&k.ModeUp, &k.ModeDown, &k.Filter} {
		b.SetEnabled(r.focus == rigFieldMode && !r.pending)
	}
	for _, b := range []*key.Binding{&k.Apply, &k.Next, &k.Prev, &k.Help} {
		b.SetEnabled(!r.pending)
	}
	return k
}

// change returns the change to the rig the fields make. A blank frequency
// is no change when there was none to start with, and the frequency filled
// in is none when left as it is, even one parseFreq rejects such as 0 Hz.
// A new mode takes the rig's default passband unless the passband was
// edited too, since the one filled in is the old mode's; a new passband
// alone is sent with the mode.
func (r rigModal) change() (rigChange, error) {
	var ch rigChange
	switch freq := strings.TrimSpace(r.freq.Value()); {
	case r.haveState && freq == formatFreq(r.initHz):
	case freq != "" || r.haveState:
		hz, err := parseFreq(freq)
		if err != nil {
			return rigChange{}, err
		}
		if !r.haveState || hz != r.initHz {
			ch.setFreq, ch.hz = true, hz
		}
	}

	pbText := strings.TrimSpace(r.passband.Value())
	passband, err := parsePassband(pbText)
	if err != nil {
		return rigChange{}, err
	}
	pbEdited := pbText != r.initPassband
	mode := r.modeValue()
	if mode != r.initMode || pbEdited {
		if mode == "" {
			return rigChange{}, errors.New("choose a mode")
		}
		ch.setMode, ch.mode = true, mode
		if pbEdited {
			ch.passband = passband
		}
	}
	return ch, nil
}

// input sends msg, a key press or other input such as a paste, to the
// focused field. A paste into the mode filters the modes.
func (r *rigModal) input(msg tea.Msg) tea.Cmd {
	if r.pending {
		return nil
	}
	var cmd tea.Cmd
	switch r.focus {
	case rigFieldFreq:
		r.freq, cmd = r.freq.Update(msg)
	case rigFieldPassband:
		r.passband, cmd = r.passband.Update(msg)
	case rigFieldMode:
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			r.mode.update(msg)
		case tea.PasteMsg:
			if text := strings.Join(strings.Fields(msg.Content), " "); text != "" {
				r.mode.filter, r.mode.cursor = r.mode.filter+text, 0
			}
		}
	}
	return cmd
}

// addRecentMode returns recent with mode moved, or added, to the front,
// capped at recentModesSize.
func addRecentMode(recent []string, mode string) []string {
	out := []string{mode}
	for _, r := range recent {
		if r != mode && len(out) < recentModesSize {
			out = append(out, r)
		}
	}
	return out
}

// openRigModal opens the rig modal filled in from the rig's state, with
// field focus focused.
func (m Model) openRigModal(focus rigField) (Model, tea.Cmd) {
	m.freqs.reset()
	m.rigModal = newRigModal(m.rigState, m.recentModes)
	return m, m.rigModal.setFocus(focus)
}

// updateRigModal handles a key press while the rig modal is open. Esc
// cancels and ctrl+c quits even while a change is applying, which ignores
// every other key.
func (m Model) updateRigModal(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	r := &m.rigModal
	k := r.keys()
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Cancel):
		m.rigModal = rigModal{}
		return m, nil
	case r.pending:
		return m, nil
	case key.Matches(msg, k.Help):
		return m.openHelp(), nil
	case key.Matches(msg, k.Apply):
		return m.applyRigModal()
	case key.Matches(msg, k.Next):
		return m, r.setFocus((r.focus + 1) % rigFieldCount)
	case key.Matches(msg, k.Prev):
		return m, r.setFocus((r.focus + rigFieldCount - 1) % rigFieldCount)
	case key.Matches(msg, k.HistoryPrev):
		if s, ok := m.freqs.prev(r.freq.Value()); ok {
			r.freq.SetValue(s)
			r.freq.CursorEnd()
		}
		return m, nil
	case key.Matches(msg, k.HistoryNext):
		if s, ok := m.freqs.next(); ok {
			r.freq.SetValue(s)
			r.freq.CursorEnd()
		}
		return m, nil
	}
	return m, r.input(msg)
}

// applyRigModal parses the rig modal's fields. A change is sent to the rig
// while the modal waits; no change closes it; a parse error shows in it.
func (m Model) applyRigModal() (Model, tea.Cmd) {
	r := &m.rigModal
	ch, err := r.change()
	if err != nil {
		r.err = err
		return m, nil
	}
	if !ch.changes() {
		m.rigModal = rigModal{}
		return m, nil
	}
	m.rigGen++
	r.gen, r.pending, r.applying, r.err = m.rigGen, true, ch, nil
	return m, rigApplyCmd(m.rig, m.rigGen, ch)
}

// applyRigResult handles the result of a change sent to the rig. The rig
// modal waiting for it closes on success, recording the frequency and mode
// sent in their histories, or shows the error. A result the modal is not
// waiting for, from before it was cancelled or reopened, is ignored, or
// shows in the footer while the modal is closed.
func (m Model) applyRigResult(msg rigResultMsg) Model {
	r := &m.rigModal
	switch {
	case !r.open:
		m.rigErr = msg.err
		return m
	case !r.pending || msg.gen != r.gen:
		return m
	}
	r.pending = false
	if msg.err != nil {
		r.err = msg.err
		return m
	}
	if r.applying.setFreq {
		m.freqs.add(formatFreq(r.applying.hz))
	}
	if r.applying.setMode {
		m.recentModes = addRecentMode(m.recentModes, r.applying.mode)
	}
	m.rigModal = rigModal{}
	return m
}

// rigConnLines renders the rig's connection status and, while it is not
// connected, the reason and when it retries, wrapped to width cells.
func (m Model) rigConnLines(width int) (status string, reason []string) {
	st, c := m.styles, m.rigState.conn
	switch c.Status {
	case rig.Connected:
		return fit(st.connected.Render(c.Status.String()), width), nil
	case rig.Disconnected:
		var parts []string
		if c.Err != nil {
			parts = append(parts, c.Err.Error())
		}
		if c.RetryIn > 0 {
			parts = append(parts, "retry in "+c.RetryIn.String())
		}
		return fit(st.err.Render(m.rigName+" not connected"), width), wrapLines(st.muted, strings.Join(parts, " · "), width)
	}
	return fit(st.muted.Render(c.Status.String()), width), nil
}

// wrapLines wraps s to width cells, each line rendered in style, or returns
// nothing for an empty s.
func wrapLines(style lipgloss.Style, s string, width int) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(lipgloss.NewStyle().Width(width).Render(s), "\n")
	for i, l := range lines {
		lines[i] = fit(style.Render(strings.TrimRight(l, " ")), width)
	}
	return lines
}

// rigModeLines renders the modes, the one chosen highlighted, with a line
// between the recent modes and the rest, in width cells.
func (r rigModal) rigModeLines(st styles, width int) (lines []string, cursorLine int) {
	items := r.mode.items()
	if len(items) == 0 {
		return []string{fit(st.muted.Render("no matches"), width)}, 0
	}
	for i, it := range items {
		if i == r.mode.sep() {
			lines = append(lines, fit(st.muted.Render(strings.Repeat("─", width)), width))
		}
		if i == r.mode.cursor {
			cursorLine = len(lines)
			lines = append(lines, st.cursor.Render(fit(it.Code, width)))
			continue
		}
		lines = append(lines, fit(it.Code, width))
	}
	return lines, cursorLine
}

// rigModalView renders the rig modal in a box fitting a body width by
// height cells: the title and badge, the connection status and why it is
// not connected, the fields, the focused one marked, with the modes below
// the mode while it is focused, and any error or that a change is applying.
// In a short body the modes, the reason and the error take fewer lines.
func (m Model) rigModalView(width, height int) string {
	st, r := m.styles, m.rigModal
	fw, fh := st.helpBox.GetHorizontalFrameSize(), st.helpBox.GetVerticalFrameSize()
	inner := max(min(max(width*3/5, rigModalMinWidth), width)-fw, 1)
	valueWidth := max(inner-lipgloss.Width(rigFocusMark)-rigLabelWidth-2, 1)

	badge := st.helpBadge.Render(rigModalBadge)
	title := st.label.Render(rigModalTitle) + st.muted.Render(" "+m.rigName)
	head := fit(title+strings.Repeat(" ", max(inner-lipgloss.Width(title)-lipgloss.Width(badge), 1))+badge, inner)
	status, reason := m.rigConnLines(inner)

	var msg []string
	switch {
	case r.pending:
		msg = []string{fit(st.muted.Render("applying…"), inner)}
	case r.err != nil:
		msg = wrapLines(st.err, r.err.Error(), inner)
	}

	var modes []string
	modesCursor := 0
	indent := lipgloss.Width(rigFocusMark) + rigLabelWidth + 2
	if r.focus == rigFieldMode {
		modes, modesCursor = r.rigModeLines(st, valueWidth)
	}

	// The title, status, fields and first line of any message always
	// show; the rest share what room is left.
	room := height - fh - 5 - min(len(msg), 1)
	extraMsg := min(max(len(msg)-1, 0), max(room, 0))
	room -= extraMsg
	extraReason := min(len(reason), max(room, 0))
	room -= extraReason
	modeRows := min(len(modes), pickListHeight, max(room, 0))
	room -= modeRows
	gapAbove := room > 0
	if gapAbove {
		room--
	}
	gapBelow := len(msg) > 0 && room > 0

	row := func(f rigField, label, value string) string {
		mark, lab := strings.Repeat(" ", lipgloss.Width(rigFocusMark)), st.muted.Render(pad(label, rigLabelWidth))
		if r.focus == f {
			mark, lab = st.helpKey.Render(rigFocusMark), st.label.Render(pad(label, rigLabelWidth))
		}
		return fit(mark+lab+"  "+value, inner)
	}
	inputView := func(in textinput.Model) string {
		in.SetWidth(max(valueWidth-1, 1))
		return in.View()
	}

	mode := r.modeValue()
	modeValue := st.muted.Render("—")
	if mode != "" {
		modeValue = mode
	}
	if r.focus == rigFieldMode {
		hint := "  type to filter"
		if r.mode.filter != "" {
			hint = "  filter: "
		}
		modeValue += st.muted.Render(hint) + r.mode.filter
	}

	lines := append([]string{head, status}, reason[:extraReason]...)
	if gapAbove {
		lines = append(lines, "")
	}
	lines = append(lines, row(rigFieldFreq, "Freq", inputView(r.freq)), row(rigFieldMode, "Mode", modeValue))
	if modeRows > 0 {
		start := max(min(modesCursor-modeRows/2, len(modes)-modeRows), 0)
		for _, l := range modes[start : start+modeRows] {
			lines = append(lines, strings.Repeat(" ", indent)+l)
		}
	}
	lines = append(lines, row(rigFieldPassband, "Passband", inputView(r.passband)))
	if len(msg) > 0 {
		if gapBelow {
			lines = append(lines, "")
		}
		lines = append(lines, msg[:1+extraMsg]...)
	}
	for i, l := range lines {
		lines[i] = fit(l, inner)
	}
	return st.helpBox.Render(strings.Join(lines, "\n"))
}
