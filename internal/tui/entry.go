package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/adif"
	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
	"jstlogger/internal/rig"
)

// entryTimes are the time fields, which the clock fills in live mode.
var entryTimes = []qsoentry.Field{qsoentry.Date, qsoentry.TimeOn, qsoentry.DateOff, qsoentry.TimeOff}

// entryRigFields are the fields the rig fills in live mode.
var entryRigFields = []qsoentry.Field{qsoentry.Mode, qsoentry.Submode, qsoentry.Freq, qsoentry.Band}

// The layouts of the dates and times the clock fills in.
const (
	entryDateLayout = "2006-01-02"
	entryTimeLayout = "15:04:05"
)

// msgDiscardEntry asks before closing the New QSO screen on unsaved changes.
const msgDiscardEntry = "Discard the unsaved QSO? (y/N)"

// entry is the New QSO screen, shown in the body in place of the index.
type entry struct {
	open bool
	// gen numbers the openings of the screen, so a clock tick from an
	// earlier one is dropped.
	gen int
	tab entryTab
	// live reports whether the screen is in live mode, where the clock
	// fills in the times and the rig the freq, band and mode, rather than
	// post mode, where every field is typed by hand.
	live bool
	// onLocked reports whether the time on has stopped following the
	// clock in live mode, as it does once the call is typed.
	onLocked bool
	// hand marks the rig fields edited by hand, which stop following the
	// rig until it is synced again.
	hand [qsoentry.FieldCount]bool
	// dataMode and dataSubmode are the data mode last in the form, which
	// a rig in a packet mode is taken to carry.
	dataMode, dataSubmode string
	// focus is the field with focus, on the open tab.
	focus qsoentry.Field
	// inputs holds a text input for each field typed on one line: every
	// field but the notes and those picked.
	inputs [qsoentry.FieldCount]textinput.Model
	// pickers holds a picker for each field in entryPicked.
	pickers [qsoentry.FieldCount]picker
	// notes is the notes field, which takes many lines.
	notes textarea.Model
	// stations are the station profiles the station picker picks from.
	stations []qsoentry.Station
	// base is the form as opened or last saved. The form has unsaved
	// changes when it differs.
	base qsoentry.Form
	// rst is the report last filled in by default. A report that still
	// equals it has not been typed, so the next default replaces it.
	rst string
	// band and bandRX are the bands last filled in from the freqs. A band
	// that still equals them has not been typed.
	band, bandRX string
	// errs holds the errors of the last save that failed validation, kept
	// up to date as the form changes, until a save succeeds.
	errs qsoentry.Errors
	// tags are the logbook's tags, which complete the tags field.
	tags []string
	// lotw is the LoTW lookup for the base callsign of the call.
	lotw lotwCache
	// saving reports whether a save is under way.
	saving bool
	// msg is shown on the status line until the next key press, and err,
	// the last save or tag load error, until the next save.
	msg string
	err error
}

// entryTagsMsg carries the logbook's tags, or the error loading them.
type entryTagsMsg struct {
	tags []string
	err  error
}

// entryTickMsg ticks the clock of the New QSO screen's opening gen.
type entryTickMsg struct {
	gen int
}

// entrySavedMsg reports a QSO saved, or the error saving it, and whether
// the screen closes.
type entrySavedMsg struct {
	qso   logbook.QSO
	close bool
	err   error
}

// entryKeyMap holds the keybindings handled while the New QSO screen is
// open; every other key goes to the field with focus.
type entryKeyMap struct {
	NextField key.Binding
	PrevField key.Binding
	NextTab   key.Binding
	PrevTab   key.Binding
	// GoTab opens the tab numbered by the key, from 1.
	GoTab     key.Binding
	Save      key.Binding
	SaveClose key.Binding
	Close     key.Binding
	// Live switches between live and post mode.
	Live key.Binding
	// ResetTime sets the time on to now in live mode.
	ResetTime key.Binding
	// SyncRig makes every rig field follow the rig again in live mode.
	SyncRig key.Binding
	// Quit replaces the global quit binding, since q is typed into fields.
	Quit key.Binding
}

func defaultEntryKeys() entryKeyMap {
	goTab := make([]string, entryTabCount)
	for i := range goTab {
		goTab[i] = "alt+" + strconv.Itoa(i+1)
	}
	return entryKeyMap{
		NextField: key.NewBinding(
			key.WithKeys("tab", "enter"),
			key.WithHelp("tab", "next field"),
		),
		PrevField: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev field"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("ctrl+pgdown"),
			key.WithHelp("ctrl+pgdn", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("ctrl+pgup"),
			key.WithHelp("ctrl+pgup", "prev tab"),
		),
		GoTab: key.NewBinding(
			key.WithKeys(goTab...),
			key.WithHelp(fmt.Sprintf("alt+1-%d", entryTabCount), "go to tab"),
		),
		Save: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "save"),
		),
		// Terminals send alt+enter for option+return; ctrl+enter needs the
		// kitty keyboard protocol.
		SaveClose: key.NewBinding(
			key.WithKeys("alt+enter", "ctrl+enter"),
			key.WithHelp("alt+enter", "save & close"),
		),
		Close: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close"),
		),
		Live: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "live/post"),
		),
		ResetTime: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "time on now"),
		),
		SyncRig: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "sync rig"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
	}
}

func (k entryKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Save, k.SaveClose, k.NextField, k.NextTab, k.Live, k.Close}
}

func (k entryKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Save, k.SaveClose, k.Close, k.Quit},
		{k.NextField, k.PrevField},
		{k.NextTab, k.PrevTab, k.GoTab},
		{k.Live, k.ResetTime, k.SyncRig},
	}
}

// newEntry returns the New QSO screen, closed, with an empty form.
func newEntry() entry {
	var e entry
	// The options picked from start with none.
	none := func(options []string) []string { return append([]string{""}, options...) }
	e.pickers[qsoentry.Mode] = newPicker(none(adif.Modes()))
	e.pickers[qsoentry.Submode] = newPicker(none(nil))
	e.pickers[qsoentry.PropMode] = newPicker(none(adif.PropModes()))
	e.pickers[qsoentry.AntPath] = newPicker(none(adif.AntPaths()))
	e.pickers[qsoentry.QSLSentVia] = newPicker(none(adif.QSLSentVias()))
	for _, f := range []qsoentry.Field{qsoentry.QSLSent, qsoentry.LoTWSent, qsoentry.EQSLSent, qsoentry.ClubLogSent} {
		p := newPicker(none(adif.QSLSentStatuses()))
		p.show = logbook.QSLSentText
		e.pickers[f] = p
	}
	e.notes = newNotes()
	for f := range qsoentry.FieldCount {
		in := newInput()
		in.Prompt = ""
		in.Placeholder = entryPlaceholders[f]
		e.inputs[f] = in
	}
	tags := &e.inputs[qsoentry.Tags]
	tags.ShowSuggestions = true
	// Tab moves to the next field, so right accepts a suggestion.
	tags.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("right"))
	return e
}

// newNotes returns the notes field: lines of text with no prompt or line
// numbers.
func newNotes() textarea.Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	s := textarea.DefaultDarkStyles()
	// A static cursor schedules no blink ticks, and the line with the
	// cursor looks like the others.
	s.Cursor.Blink = false
	s.Focused.CursorLine = s.Focused.Text
	ta.SetStyles(s)
	return ta
}

// value returns the value of field f: its text, or the option picked.
func (e entry) value(f qsoentry.Field) string {
	switch {
	case f == qsoentry.Notes:
		return e.notes.Value()
	case isPicked(f):
		return e.pickers[f].value()
	}
	return e.inputs[f].Value()
}

// setValue sets the value of field f to v. A picked field already showing v
// keeps its option, which matters when two share a name, as station
// profiles may; one with no option v picks none, or failing that keeps its
// option.
func (e *entry) setValue(f qsoentry.Field, v string) {
	switch {
	case f == qsoentry.Notes:
		e.notes.SetValue(v)
	case isPicked(f):
		if strings.EqualFold(e.pickers[f].value(), v) {
			return
		}
		if !e.pickers[f].set(v) {
			e.pickers[f].set("")
		}
	default:
		e.inputs[f].SetValue(v)
	}
}

// submodes returns the submodes offered for the mode picked, with none
// first.
func (e entry) submodes() []string {
	return append([]string{""}, adif.Submodes(e.pickers[qsoentry.Mode].value())...)
}

// form returns the values in the form.
func (e entry) form() qsoentry.Form {
	var f qsoentry.Form
	for k := range qsoentry.FieldCount {
		f[k] = e.value(k)
	}
	return f
}

// setForm puts f's values in the form. Reports and bands that match what
// the mode and freqs would fill in are replaced as if filled in.
func (e *entry) setForm(f qsoentry.Form) {
	for k := range qsoentry.FieldCount {
		if k == qsoentry.Submode {
			// The submodes offered follow the mode, set before.
			e.pickers[k].setOptions(e.submodes())
		}
		e.setValue(k, f[k])
	}
	e.rst = qsoentry.DefaultRST(e.value(qsoentry.Mode), e.value(qsoentry.Submode))
	e.band = freqBand(f[qsoentry.Freq])
	e.bandRX = freqBand(f[qsoentry.FreqRX])
	e.setTagSuggestions()
}

// freqBand returns the band of the freq s, or "" when s is not a freq in a
// band.
func freqBand(s string) string {
	hz, err := qsoentry.ParseFreq(s)
	if err != nil {
		return ""
	}
	band, _ := logbook.BandForFreq(hz)
	return band
}

// readOnly reports whether field f can't be edited: the times in live mode.
func (e entry) readOnly(f qsoentry.Field) bool {
	return e.live && slices.Contains(entryTimes, f)
}

// setOn sets the date and time on to t.
func (e *entry) setOn(t time.Time) {
	t = t.UTC()
	e.inputs[qsoentry.Date].SetValue(t.Format(entryDateLayout))
	e.inputs[qsoentry.TimeOn].SetValue(t.Format(entryTimeLayout))
}

// tickClock sets the date and time on to now while they follow the clock.
func (e *entry) tickClock(now time.Time) {
	if e.live && !e.onLocked {
		e.setOn(now)
	}
}

// followCall locks the time on at now once a call is typed in live mode,
// and starts the timing over when the call is cleared.
func (e *entry) followCall(before, after qsoentry.Form, now time.Time) {
	had, has := strings.TrimSpace(before[qsoentry.Call]) != "", strings.TrimSpace(after[qsoentry.Call]) != ""
	if !e.live || had == has {
		return
	}
	e.onLocked = has
	e.setOn(now)
}

// followsRig reports whether field f follows the rig: a rig field not
// edited by hand, in live mode.
func (e entry) followsRig(f qsoentry.Field) bool {
	return e.live && slices.Contains(entryRigFields, f) && !e.hand[f]
}

// markHand marks field f, just edited by hand, as no longer following the
// rig. The mode and submode go together.
func (e *entry) markHand(f qsoentry.Field) {
	switch f {
	case qsoentry.Mode, qsoentry.Submode:
		e.hand[qsoentry.Mode], e.hand[qsoentry.Submode] = true, true
	case qsoentry.Freq, qsoentry.Band:
		e.hand[f] = true
	}
}

// syncRig fills in the rig fields that follow the rig from its state s, and
// what follows from them: the band and the default reports.
func (e *entry) syncRig(s rig.State) {
	before := e.form()
	if e.followsRig(qsoentry.Freq) {
		e.inputs[qsoentry.Freq].SetValue(logbook.FormatFreqMHz(s.Freq))
	}
	if e.followsRig(qsoentry.Mode) {
		mode, submode := qsoentry.RigMode(s.Mode, e.dataMode, e.dataSubmode)
		e.pickers[qsoentry.Mode].set(mode)
		e.pickers[qsoentry.Submode].setOptions(e.submodes())
		e.pickers[qsoentry.Submode].set(submode)
	}
	band := e.inputs[qsoentry.Band].Value()
	e.fillIn(before, e.form())
	if e.followsRig(qsoentry.Band) {
		// A band typed before a sync gives way even when the freq is
		// unchanged.
		band = freqBand(e.inputs[qsoentry.Freq].Value())
		e.band = band
	}
	e.inputs[qsoentry.Band].SetValue(band)
}

// unsaved reports whether the form holds values typed or picked since it
// was opened or last saved. In live mode, the values the clock and the rig
// fill in don't count, nor do the default reports.
func (e entry) unsaved() bool {
	f, b := e.form(), e.base
	for k := range qsoentry.FieldCount {
		if e.filledIn(k, f[k]) {
			f[k], b[k] = "", ""
		}
	}
	return f != b
}

// filledIn reports whether v, the value of field f, was filled in rather
// than typed in live mode: by the clock, by the rig or as the default
// report.
func (e entry) filledIn(f qsoentry.Field, v string) bool {
	isRST := f == qsoentry.RSTSent || f == qsoentry.RSTRcvd
	return e.readOnly(f) || e.followsRig(f) || e.live && isRST && v == e.rst
}

// focusField gives field f focus, with the cursor at the end of its value.
func (e *entry) focusField(f qsoentry.Field) tea.Cmd {
	e.inputs[e.focus].Blur()
	e.notes.Blur()
	e.focus = f
	switch {
	case isPicked(f):
		return nil
	case f == qsoentry.Notes:
		return e.notes.Focus()
	}
	e.inputs[f].CursorEnd()
	return e.inputs[f].Focus()
}

// openEntry opens the New QSO screen on a form empty but for the Station
// tab, filled in from the first station profile, starts its clock and loads
// the logbook's tags. The screen opens in live mode when the rig is
// connected, and in post mode otherwise.
func (m Model) openEntry() (Model, tea.Cmd) {
	e := newEntry()
	e.setStations(m.stations)
	// The profile's values are the form's own, not unsaved changes.
	e.base = e.form()
	e.open = true
	e.gen = m.entry.gen + 1
	e.tags = m.entry.tags
	e.dataMode, e.dataSubmode = m.entry.dataMode, m.entry.dataSubmode
	e.live = m.rig != nil && m.rigState.conn.Status == rig.Connected
	focus := qsoentry.Date
	if e.live {
		focus = qsoentry.Call
		e.tickClock(m.now())
		if m.rigState.haveState {
			e.syncRig(m.rigState.state)
		}
	}
	cmd := e.focusField(focus)
	m.entry = e
	return m, tea.Batch(cmd, loadTagsCmd(m.logbook), m.entryTickCmd())
}

// closeEntry closes the New QSO screen, discarding the form. The tags are
// kept to complete the tags field when it opens again, and the last data
// mode for a rig in a packet mode.
func (m Model) closeEntry() Model {
	old := m.entry
	m.entry = newEntry()
	m.entry.gen, m.entry.tags = old.gen, old.tags
	m.entry.dataMode, m.entry.dataSubmode = old.dataMode, old.dataSubmode
	return m
}

// applyRigState fills in the New QSO screen's rig fields from the rig's
// state s, when the screen is open in live mode.
func (m *Model) applyRigState(s rig.State) {
	if !m.entry.open || !m.entry.live {
		return
	}
	m.entry.syncRig(s)
	m.revalidateEntry()
}

// entryTickCmd returns a command that ticks the New QSO screen's clock at
// the start of the next second.
func (m Model) entryTickCmd() tea.Cmd {
	gen, now := m.entry.gen, m.now()
	return m.tick(now.Truncate(time.Second).Add(time.Second).Sub(now), func(time.Time) tea.Msg {
		return entryTickMsg{gen: gen}
	})
}

// tickEntry moves the New QSO screen's clock on and schedules the next
// tick. A tick for a closed screen, or an earlier opening, stops.
func (m Model) tickEntry(msg entryTickMsg) (Model, tea.Cmd) {
	if !m.entry.open || msg.gen != m.entry.gen {
		return m, nil
	}
	m.entry.tickClock(m.now())
	m.revalidateEntry()
	return m, m.entryTickCmd()
}

// toggleLive switches the New QSO screen between live and post mode. Post
// mode keeps the times and the rig's values for editing. Live mode starts
// the clock again, or with a call typed, locks the time on, filling it in
// if empty, and fills in the rig fields from the rig's last state.
func (m Model) toggleLive() (Model, tea.Cmd) {
	e := &m.entry
	if e.live {
		// The values filled in become the form's own, not unsaved
		// changes, as post mode takes them over.
		f := e.form()
		for k := range qsoentry.FieldCount {
			if e.filledIn(k, f[k]) {
				e.base[k] = f[k]
			}
		}
	}
	e.live = !e.live
	if !e.live {
		return m, nil
	}
	e.onLocked = strings.TrimSpace(e.inputs[qsoentry.Call].Value()) != ""
	if !e.onLocked || e.inputs[qsoentry.Date].Value() == "" || e.inputs[qsoentry.TimeOn].Value() == "" {
		e.setOn(m.now())
	}
	if m.rigState.haveState {
		e.syncRig(m.rigState.state)
	}
	m.revalidateEntry()
	if e.readOnly(e.focus) {
		return m, e.focusField(qsoentry.Call)
	}
	return m, nil
}

// loadTagsCmd returns a command that loads the logbook's tag names.
func loadTagsCmd(s logbook.Store) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		tags, err := s.Tags(ctx)
		if err != nil {
			return entryTagsMsg{err: fmt.Errorf("load tags: %w", err)}
		}
		names := make([]string, len(tags))
		for i, t := range tags {
			names[i] = t.Name
		}
		return entryTagsMsg{tags: names}
	}
}

// applyTags keeps the logbook's tags for completing the tags field.
func (e *entry) applyTags(msg entryTagsMsg) {
	if msg.err != nil {
		e.err = msg.err
		return
	}
	e.tags = msg.tags
	e.setTagSuggestions()
}

// setTagSuggestions offers each tag not yet in the tags field to complete
// the tag being typed after the last comma.
func (e *entry) setTagSuggestions() {
	in := &e.inputs[qsoentry.Tags]
	v := in.Value()
	head := v[:strings.LastIndex(v, ",")+1]
	tail := v[len(head):]
	head += tail[:len(tail)-len(strings.TrimLeft(tail, " "))]
	var given []string
	for t := range strings.SplitSeq(head, ",") {
		given = append(given, strings.TrimSpace(t))
	}
	var sugs []string
	for _, t := range e.tags {
		if !slices.ContainsFunc(given, func(g string) bool { return strings.EqualFold(g, t) }) {
			sugs = append(sugs, head+t)
		}
	}
	in.SetSuggestions(sugs)
}

// updateEntry handles a key press on the New QSO screen.
func (m Model) updateEntry(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k, e := m.entryKeys, &m.entry
	e.msg = ""
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case e.saving:
		// Other keys wait for the save, so it lands on the form it saved.
	case key.Matches(msg, k.Save):
		return m.saveEntry(false)
	case key.Matches(msg, k.SaveClose):
		return m.saveEntry(true)
	case key.Matches(msg, k.Close):
		if !e.unsaved() {
			return m.closeEntry(), nil
		}
		return m.askConfirm(msgDiscardEntry, func(m Model) (Model, tea.Cmd) {
			return m.closeEntry(), nil
		}), nil
	case key.Matches(msg, k.NextTab):
		return m, e.stepTab(1)
	case key.Matches(msg, k.PrevTab):
		return m, e.stepTab(-1)
	case key.Matches(msg, k.GoTab):
		// The keys are alt+1 onward, numbering the tabs shown.
		n, _ := strconv.Atoi(strings.TrimPrefix(msg.String(), "alt+"))
		if ts := e.tabs(); n >= 1 && n <= len(ts) {
			return m, e.goTab(ts[n-1])
		}
	case key.Matches(msg, k.Live):
		return m.toggleLive()
	case key.Matches(msg, k.ResetTime):
		if e.live {
			e.setOn(m.now())
			m.revalidateEntry()
		}
	case key.Matches(msg, k.SyncRig):
		if e.live {
			e.hand = [qsoentry.FieldCount]bool{}
			if m.rigState.haveState {
				m.applyRigState(m.rigState.state)
			}
		}
	case e.focus == qsoentry.Notes && key.Matches(msg, e.notes.KeyMap.InsertNewline):
		// Enter starts a new line of the notes rather than moving on.
		return m.editEntry(msg)
	case key.Matches(msg, k.NextField):
		return m, e.focusField(e.stepFocus(1))
	case key.Matches(msg, k.PrevField):
		return m, e.focusField(e.stepFocus(-1))
	default:
		return m.editEntry(msg)
	}
	return m, nil
}

// editEntry sends msg, a key press or other input such as a paste, to the
// field with focus, then fills in what follows from the change.
func (m Model) editEntry(msg tea.Msg) (Model, tea.Cmd) {
	e := &m.entry
	if e.saving || e.readOnly(e.focus) {
		return m, nil
	}
	before := e.form()
	var cmd tea.Cmd
	switch {
	case isPicked(e.focus):
		k, ok := msg.(tea.KeyPressMsg)
		if ok && e.pickers[e.focus].update(k) && e.focus == qsoentry.StationName {
			e.fillStation()
		}
	case e.focus == qsoentry.Notes:
		e.notes, cmd = e.notes.Update(msg)
	default:
		in := &e.inputs[e.focus]
		if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, in.KeyMap.AcceptSuggestion) && in.CurrentSuggestion() != "" {
			// The suggestion replaces what was typed, so a tag keeps the
			// logbook's spelling.
			in.SetValue(in.CurrentSuggestion())
			in.CursorEnd()
			break
		}
		e.inputs[e.focus], cmd = e.inputs[e.focus].Update(msg)
	}
	if after := e.form(); after != before {
		e.markHand(e.focus)
		e.fillIn(before, after)
		e.followCall(before, after, m.now())
		cmd = tea.Batch(cmd, e.keepTab())
		m.revalidateEntry()
	}
	return m, cmd
}

// fillIn fills in what follows from a change to the form from before to
// after: the submodes and default reports from the mode, the bands from the
// freqs and the tag completions from the tags. A data mode is kept for a
// rig in a packet mode.
func (e *entry) fillIn(before, after qsoentry.Form) {
	changed := func(f qsoentry.Field) bool { return after[f] != before[f] }
	if changed(qsoentry.Mode) {
		e.pickers[qsoentry.Submode].setOptions(e.submodes())
	}
	if changed(qsoentry.Mode) || changed(qsoentry.Submode) {
		e.fillRST()
		if mode := e.value(qsoentry.Mode); qsoentry.IsDataMode(mode) {
			e.dataMode, e.dataSubmode = mode, e.value(qsoentry.Submode)
		}
	}
	if changed(qsoentry.Freq) {
		e.band = e.fillBand(qsoentry.Freq, qsoentry.Band, e.band)
	}
	if changed(qsoentry.FreqRX) {
		e.bandRX = e.fillBand(qsoentry.FreqRX, qsoentry.BandRX, e.bandRX)
	}
	if changed(qsoentry.Tags) {
		e.setTagSuggestions()
	}
}

// fillRST fills in the default report for the mode and submode, in each
// report field that is empty or holds the last default.
func (e *entry) fillRST() {
	rst := qsoentry.DefaultRST(e.value(qsoentry.Mode), e.value(qsoentry.Submode))
	for _, f := range []qsoentry.Field{qsoentry.RSTSent, qsoentry.RSTRcvd} {
		if v := e.inputs[f].Value(); v == "" || v == e.rst {
			e.inputs[f].SetValue(rst)
		}
	}
	e.rst = rst
}

// fillBand fills in the band of the freq in freq, in the band field if it
// is empty or holds last, the band last filled in. It returns the band
// filled in.
func (e *entry) fillBand(freq, band qsoentry.Field, last string) string {
	derived := freqBand(e.inputs[freq].Value())
	if v := e.inputs[band].Value(); v == "" || v == last {
		e.inputs[band].SetValue(derived)
	}
	return derived
}

// validateEntry checks the form, keeping its errors for the view.
func (m *Model) validateEntry() {
	_, err := qsoentry.Build(m.entry.form(), m.now())
	m.entry.errs, _ = errors.AsType[qsoentry.Errors](err)
}

// revalidateEntry checks the form again after it changes, if the last save
// failed validation, so the errors shown stay up to date.
func (m *Model) revalidateEntry() {
	if m.entry.errs != nil {
		m.validateEntry()
	}
}

// saveEntry saves the form as a new QSO, then clears it for the next QSO or,
// when close is true, closes the screen. A form that fails validation shows
// its errors, with focus on the first field in error, on its tab, and is
// not saved.
func (m Model) saveEntry(close bool) (Model, tea.Cmd) {
	e := &m.entry
	if e.saving {
		return m, nil
	}
	now := m.now()
	e.tickClock(now)
	f := e.form()
	if e.live {
		// The QSO ends as it is saved. The form keeps no time off, so a
		// save that fails leaves none behind.
		f[qsoentry.DateOff] = now.UTC().Format(entryDateLayout)
		f[qsoentry.TimeOff] = now.UTC().Format(entryTimeLayout)
	}
	q, err := qsoentry.Build(f, now)
	if errs, ok := errors.AsType[qsoentry.Errors](err); ok {
		e.errs = errs
		fs := errs.Fields()
		i := slices.IndexFunc(fs, e.focusable)
		if i < 0 {
			return m, nil
		}
		e.tab = entryTabOf(fs[i])
		return m, e.focusField(fs[i])
	}
	if err != nil {
		e.err = err
		return m, nil
	}
	e.errs, e.err, e.saving = nil, nil, true
	return m, addQSOCmd(m.logbook, q, close)
}

// addQSOCmd returns a command that adds q to the logbook.
func addQSOCmd(s logbook.Store, q logbook.QSO, close bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		saved, err := s.Add(ctx, q)
		if err != nil {
			return entrySavedMsg{close: close, err: fmt.Errorf("save QSO with %s: %w", q.Call, err)}
		}
		return entrySavedMsg{qso: saved, close: close}
	}
}

// applySaved reloads the index on the saved QSO and clears the form for the
// next QSO, keeping the values that carry over, or closes the screen. A
// failed save keeps the form as it is.
func (m Model) applySaved(msg entrySavedMsg) (Model, tea.Cmd) {
	e := &m.entry
	e.saving = false
	if msg.err != nil {
		e.err = msg.err
		return m, nil
	}
	saved := "Saved QSO with " + msg.qso.Call + "."
	for _, t := range msg.qso.Tags {
		if !slices.ContainsFunc(e.tags, func(s string) bool { return strings.EqualFold(s, t) }) {
			e.tags = append(e.tags, t)
		}
	}
	m.idx.restart()
	reload := loadIndexCmd(m.logbook, m.idx.gen, m.idx.limit.expr, msg.qso.ID, m.idx.cursor)
	if msg.close {
		m = m.closeEntry()
		m.idx.msg = saved
		return m, reload
	}
	next := e.form().Carry()
	e.setForm(next)
	e.base = next
	e.msg = saved
	e.tab = entryTabQSO
	focus := qsoentry.TimeOn
	if e.live {
		// The next QSO's clock starts now.
		focus, e.onLocked = qsoentry.Call, false
		e.tickClock(m.now())
	}
	return m, tea.Batch(e.focusField(focus), reload)
}

// entryView renders the New QSO screen in width by height cells: the tab
// bar, the open tab and a status line.
func (m Model) entryView(width, height int) string {
	st, e := m.styles, m.entry
	tabs := e.tabs()
	names := make([]string, len(tabs))
	for i, t := range tabs {
		names[i] = entryTabNames[t]
	}
	lines := []string{tabBar(st, names, slices.Index(tabs, e.tab), width)}
	fields, row, rows := m.entryFields(width)
	// Scroll a window too short for every row to show the focused row and
	// its errors, keeping the tab bar. end is the line after them.
	end := 1 + row + rows
	lines = append(lines, fields...)
	if skip := end - (height - 1); skip > 0 {
		lines = append(lines[:1], lines[1+skip:]...)
	}
	lines = lines[:min(len(lines), max(height-1, 0))]
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	status := "New QSO · post"
	if e.live {
		status = "New QSO · live"
	}
	switch {
	case e.saving:
		status += " · Saving…"
	case e.err != nil:
		status = st.err.Render(truncate(status+" · "+e.err.Error(), width, "…"))
	case e.msg != "":
		status += " · " + e.msg
	}
	return strings.Join(append(lines, truncate(status, width, "…")), "\n")
}

// entryFields renders the open tab's rows, each fitted to width cells and
// followed by a line with the errors of its fields, if any. It also returns
// the line of the focused field's row and the number of lines that row
// takes.
func (m Model) entryFields(width int) (lines []string, focus, rows int) {
	e := m.entry
	lw := entryLayouts[e.tab].labelWidth
	for _, row := range e.rows(e.tab) {
		colWidth := width
		if len(row) > 1 {
			colWidth = max(width-2, 0) / 2
		}
		cells := make([]string, len(row))
		var errs []string
		for i, f := range row {
			cells[i] = m.entryCell(f, colWidth, lw)
			var msg string
			if err := e.errs[f]; err != nil {
				msg = m.styles.err.Render(truncate("✗ "+err.Error(), max(colWidth-lw-2, 0), "…"))
			}
			errs = append(errs, fit(strings.Repeat(" ", lw+2)+msg, colWidth))
		}
		start := len(lines)
		lines = append(lines, joinCells(cells, colWidth)...)
		if slices.ContainsFunc(row, func(f qsoentry.Field) bool { return e.errs[f] != nil }) {
			lines = append(lines, strings.TrimRight(strings.Join(errs, "  "), " "))
		}
		if slices.Contains(row, e.focus) {
			focus, rows = start, len(lines)-start
		}
	}
	return lines, focus, rows
}

// joinCells lays out cells side by side, each fitted to width cells, two
// apart. A cell such as the notes takes more than one line, and the cells
// beside it are blank below their own lines.
func joinCells(cells []string, width int) []string {
	split := make([][]string, len(cells))
	height := 1
	for i, c := range cells {
		split[i] = strings.Split(c, "\n")
		height = max(height, len(split[i]))
	}
	lines := make([]string, height)
	for l := range lines {
		parts := make([]string, len(split))
		for i, cell := range split {
			var part string
			if l < len(cell) {
				part = cell[l]
			}
			parts[i] = fit(part, width)
		}
		lines[l] = strings.TrimRight(strings.Join(parts, "  "), " ")
	}
	return lines
}

// entryCell renders field f, labeled with a label lw cells wide, in width
// cells. The call is followed by its LoTW status and the grid by the
// bearing and distance to it. The notes take a line for each line of the
// field.
func (m Model) entryCell(f qsoentry.Field, width, lw int) string {
	st, e := m.styles, m.entry
	focused := f == e.focus
	label := st.label.Render(pad(entryLabels[f], lw))
	if focused {
		label = st.title.Render(pad(entryLabels[f], lw))
	}
	room := max(width-lw-2, 1)
	var value string
	switch {
	case f == qsoentry.Notes:
		// The lines after the first line up under it.
		value = strings.ReplaceAll(e.notes.View(), "\n", "\n"+strings.Repeat(" ", lw+2))
	case isPicked(f):
		value = e.pickers[f].view(st, focused)
		if mark := m.entryHandMark(f); mark != "" {
			value += "  " + mark
		}
	default:
		suffix, suffixAt := m.entrySuffix(f)
		in := e.inputs[f]
		inWidth := room
		if suffix != "" {
			// The suffix sits next to the value rather than at the far side.
			inWidth = max(min(room-lipgloss.Width(suffix)-2, suffixAt), 1)
		}
		// The input needs a cell for the cursor past the end of its value.
		in.SetWidth(max(inWidth-1, 1))
		value = in.View()
		if e.readOnly(f) {
			value = st.muted.Render(in.Value())
		}
		if suffix != "" {
			value = pad(value, inWidth) + "  " + suffix
		}
	}
	return label + "  " + value
}

// entrySuffix returns what follows field f's value, styled, and the width
// of the input before it: the LoTW status after the call, the path after
// the grid and the hand mark after the freq and band. It returns "" for
// other fields.
func (m Model) entrySuffix(f qsoentry.Field) (string, int) {
	switch f {
	case qsoentry.Freq, qsoentry.Band:
		// A freq to the hertz in MHz, or the longest band, and the cursor.
		width := 11
		if f == qsoentry.Band {
			width = 6
		}
		if mark := m.entryHandMark(f); mark != "" {
			return mark, width
		}
	case qsoentry.Call:
		// A long portable call, such as VE3/W1ABC/P, and the cursor.
		return m.entryLoTW(), 14
	case qsoentry.Gridsquare:
		// An 8-character locator and the cursor.
		if path := m.entryPath(); path != "" {
			return m.styles.muted.Render(path), 9
		}
	}
	return "", 0
}

// entryHandMark returns the mark, styled, of a rig field edited by hand
// that no longer follows the rig, or "" for any other field.
func (m Model) entryHandMark(f qsoentry.Field) string {
	e := m.entry
	if m.rig == nil || !e.live || !slices.Contains(entryRigFields, f) || !e.hand[f] {
		return ""
	}
	return m.styles.muted.Render("manual")
}

// entryPath returns the bearing and distance from our grid, on the Station
// tab, to the grid in the form, in the index's distance unit, or "" when
// either grid is missing or invalid.
func (m Model) entryPath() string {
	myGrid := strings.TrimSpace(m.entry.value(qsoentry.MyGridsquare))
	grid := strings.TrimSpace(m.entry.value(qsoentry.Gridsquare))
	km, bearing, err := qsoentry.Path(myGrid, grid)
	if err != nil {
		return ""
	}
	unit := m.idx.unit
	if unit != indexfmt.Mi {
		unit = indexfmt.Km
	}
	// A bearing that rounds up to 360° is due north.
	return fmt.Sprintf("%d° · %s %s", int(math.Round(bearing))%360, indexfmt.FormatDistance(km, unit), unit)
}

// fit cuts the styled s to width cells, or pads it out to width.
func fit(s string, width int) string {
	if width <= 0 {
		// MaxWidth(0) is no limit.
		return ""
	}
	return pad(lipgloss.NewStyle().MaxWidth(width).Render(s), width)
}

// sizeEntryNotes fits the notes field to the Notes tab of a body height
// lines tall, below the tab bar and above the status line.
func (m *Model) sizeEntryNotes(height int) {
	lw := entryLayouts[entryTabNotes].labelWidth
	m.entry.notes.SetWidth(max(m.bodyWidth()-lw-2, 1))
	m.entry.notes.SetHeight(max(height-2, 1))
}
