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
	"jstlogger/internal/editor"
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

// msgDiscardEdit asks before closing the screen on unsaved changes to an
// edited QSO.
const msgDiscardEdit = "Discard changes to this QSO? (y/N)"

// entry is the New QSO screen, shown in the body in place of the index.
type entry struct {
	open bool
	// gen numbers the openings of the screen, so a clock tick from an
	// earlier one is dropped.
	gen int
	tab entryTab
	// wheeled reports whether the wheel has scrolled the form, to show
	// scroll lines down from its top, since the last key press. Otherwise
	// the form scrolls to show the focused field.
	wheeled bool
	scroll  int
	// list is the list the picker with focus opened, while it is open.
	list pickList
	// editing reports whether the screen edits orig, a stored QSO, rather
	// than logging a new one. An edit is always in post mode: the rig is
	// never applied to it.
	editing bool
	orig    logbook.QSO
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
	// w holds the form's widgets. Copies of the Model share them, which
	// keeps a copy small; newEntry gives a fresh set, so a form opened
	// again never shares them with one held from before.
	w *entryWidgets
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
	// dxccLookup looks up the DXCC entity of the call; nil fills in none.
	dxccLookup DXCCLookup
	// dxcc holds the DXCC entity, continent and zones last filled in from
	// the call, in the order of dxccFields. A value that still equals
	// them has not been typed.
	dxcc [4]string
	// errs holds the errors of the last save that failed validation, kept
	// up to date as the form changes, until a save succeeds.
	errs qsoentry.Errors
	// tags are the logbook's tags, which complete the tags field.
	tags []string
	// lotw is the LoTW lookup for the base callsign of the call.
	lotw lotwCache
	// worked is the worked-before panel for the call.
	worked workedBefore
	// callbook is the lookup of the call's name, QTH, grid, state, county
	// and email.
	callbook callbookLookup
	// saving reports whether a save is under way.
	saving bool
	// now returns the current time, which the short forms of the dates
	// and times, such as t and n, count from.
	now func() time.Time
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

// editLoadedMsg carries the stored QSO to open in the form for editing, or
// the error loading it.
type editLoadedMsg struct {
	qso logbook.QSO
	err error
}

// entryEditedMsg reports that the editor opened on field f of the New QSO
// screen's opening gen has exited, with the error running it, if any. The
// edited text is in the file at path.
type entryEditedMsg struct {
	gen   int
	field qsoentry.Field
	path  string
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
	// Editor opens the field with focus in the user's editor, for the
	// fields in entryEditorFields.
	Editor key.Binding
	// Quit replaces the global quit binding, since q is typed into fields.
	Quit key.Binding
	// Change only shows in the help, for the arrows a picker steps with.
	Change key.Binding
	// List opens the list of a picker's options.
	List key.Binding
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
		// alt+. and alt+, sit on > and <, for keyboards without PgUp/PgDn.
		NextTab: key.NewBinding(
			key.WithKeys("alt+.", "ctrl+pgdown"),
			key.WithHelp("alt+./ctrl+pgdn", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("alt+,", "ctrl+pgup"),
			key.WithHelp("alt+,/ctrl+pgup", "prev tab"),
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
		// The text areas select all on ctrl+g, which this overrides.
		Editor: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "edit in $EDITOR"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Change: key.NewBinding(
			key.WithKeys("left", "right"),
			key.WithHelp("←/→", "change"),
		),
		List: key.NewBinding(
			key.WithKeys("enter", "space"),
			key.WithHelp("enter", "list"),
		),
	}
}

func (k entryKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Change, k.List, k.Save, k.SaveClose, k.NextField, k.NextTab, k.Editor, k.Live, k.Close}
}

func (k entryKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Save, k.SaveClose, k.Close, k.Quit},
		{k.NextField, k.PrevField, k.Editor, k.Change, k.List},
		{k.NextTab, k.PrevTab, k.GoTab},
		{k.Live, k.ResetTime, k.SyncRig},
	}
}

// entryWidgets holds the New QSO form's widgets, one for each field, of the
// kind the field uses: a text input for each field typed on one line, a
// text area for each in entryMultiLine and a picker for each in
// entryPicked. Each is at the field's entrySlots index in its slice.
type entryWidgets struct {
	inputs  []textinput.Model
	areas   []textarea.Model
	pickers []picker
}

// entrySlots maps each field to the index of its widget in the slice for
// its kind.
var entrySlots = func() (slots [qsoentry.FieldCount]int) {
	var inputs, areas, pickers int
	for f := range qsoentry.FieldCount {
		switch {
		case isMultiLine(f):
			slots[f], areas = areas, areas+1
		case isPicked(f):
			slots[f], pickers = pickers, pickers+1
		default:
			slots[f], inputs = inputs, inputs+1
		}
	}
	return slots
}()

// input returns the text input of field f, which must be typed on one line.
func (e *entry) input(f qsoentry.Field) *textinput.Model {
	if isMultiLine(f) || isPicked(f) {
		panic(fmt.Sprintf("tui: %s has no text input", entryLabels[f]))
	}
	return &e.w.inputs[entrySlots[f]]
}

// area returns the text area of field f, which must be in entryMultiLine.
func (e *entry) area(f qsoentry.Field) *textarea.Model {
	if !isMultiLine(f) {
		panic(fmt.Sprintf("tui: %s has no text area", entryLabels[f]))
	}
	return &e.w.areas[entrySlots[f]]
}

// picker returns the picker of field f, which must be in entryPicked.
func (e *entry) picker(f qsoentry.Field) *picker {
	if !isPicked(f) {
		panic(fmt.Sprintf("tui: %s has no picker", entryLabels[f]))
	}
	return &e.w.pickers[entrySlots[f]]
}

// newEntry returns the New QSO screen, closed, with an empty form and a
// fresh set of widgets.
func newEntry() entry {
	e := entry{now: time.Now, w: &entryWidgets{
		inputs:  make([]textinput.Model, int(qsoentry.FieldCount)-len(entryMultiLine)-len(entryPicked)),
		areas:   make([]textarea.Model, len(entryMultiLine)),
		pickers: make([]picker, len(entryPicked)),
	}}
	// The options picked from start with none.
	none := func(options []adif.Enum) []adif.Enum { return append([]adif.Enum{{}}, options...) }
	*e.picker(qsoentry.Mode) = newPicker(none(modeOptions(adif.Modes())))
	*e.picker(qsoentry.Submode) = newPicker(none(nil))
	*e.picker(qsoentry.PropMode) = newPicker(none(adif.PropModes()))
	*e.picker(qsoentry.AntPath) = newPicker(none(adif.AntPaths()))
	*e.picker(qsoentry.QSLSentVia) = newPicker(none(adif.QSLSentVias()))
	for _, f := range []qsoentry.Field{qsoentry.QSLSent, qsoentry.LoTWSent, qsoentry.EQSLSent, qsoentry.ClubLogSent} {
		*e.picker(f) = newPicker(none(adif.QSLSentStatuses()))
	}
	for _, f := range entryMultiLine {
		*e.area(f) = newArea()
	}
	e.area(qsoentry.QSLMsg).SetHeight(entryQSLMsgHeight)
	for f := range qsoentry.FieldCount {
		if isMultiLine(f) || isPicked(f) {
			continue
		}
		in := newInput()
		in.Prompt = ""
		in.Placeholder = entryPlaceholders[f]
		*e.input(f) = in
	}
	tags := e.input(qsoentry.Tags)
	tags.ShowSuggestions = true
	// Tab moves to the next field, so right accepts a suggestion.
	tags.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("right"))
	return e
}

// entryMultiLine are the fields typed on many lines rather than one.
var entryMultiLine = []qsoentry.Field{qsoentry.Notes, qsoentry.QSLMsg}

// entryEditorFields are the fields that ctrl+g opens in the user's editor:
// the long text ones.
var entryEditorFields = []qsoentry.Field{qsoentry.Comment, qsoentry.Notes, qsoentry.QSLMsg}

// entryQSLMsgHeight is the height of the QSL message field, in lines. The
// notes take the height of the Notes tab instead.
const entryQSLMsgHeight = 3

// isMultiLine reports whether field f is typed on many lines.
func isMultiLine(f qsoentry.Field) bool {
	return slices.Contains(entryMultiLine, f)
}

// newArea returns a multi-line field: lines of text with no prompt or line
// numbers.
func newArea() textarea.Model {
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
func (e *entry) value(f qsoentry.Field) string {
	switch {
	case isMultiLine(f):
		return e.area(f).Value()
	case isPicked(f):
		return e.picker(f).value()
	}
	return e.input(f).Value()
}

// setValue sets the value of field f to v. A picked field already showing v
// keeps its option, which matters when two share a name, as station
// profiles may; one with no option v picks none, or failing that keeps its
// option.
func (e *entry) setValue(f qsoentry.Field, v string) {
	switch {
	case isMultiLine(f):
		e.area(f).SetValue(v)
	case isPicked(f):
		if strings.EqualFold(e.picker(f).value(), v) {
			return
		}
		if !e.picker(f).set(v) {
			e.picker(f).set("")
		}
	default:
		e.input(f).SetValue(v)
	}
}

// submodes returns the submodes offered for the mode picked, with none
// first.
func (e *entry) submodes() []adif.Enum {
	return append([]adif.Enum{{}}, modeOptions(adif.Submodes(e.picker(qsoentry.Mode).value()))...)
}

// modeOptions returns the modes or submodes named, each with what it is
// when its name doesn't say.
func modeOptions(names []string) []adif.Enum {
	out := make([]adif.Enum, len(names))
	for i, n := range names {
		out[i] = adif.Enum{Code: n, Description: adif.DescribeMode(n)}
	}
	return out
}

// form returns the values in the form.
func (e *entry) form() qsoentry.Form {
	var f qsoentry.Form
	for k := range qsoentry.FieldCount {
		f[k] = e.value(k)
	}
	return f
}

// setForm puts f's values in the form. Reports, bands and DXCC values that
// match what the mode, freqs and call would fill in are replaced as if
// filled in. While editing, a picked value that is none of its picker's
// options is added to them. The callbook lookup under way is canceled and
// the form's values are its own, not filled in from the callbook.
func (e *entry) setForm(f qsoentry.Form) {
	for k := range qsoentry.FieldCount {
		if k == qsoentry.Submode {
			// The submodes offered follow the mode, set before.
			e.picker(k).setOptions(e.submodes())
		}
		e.setValue(k, f[k])
		if e.editing && isPicked(k) && !strings.EqualFold(e.value(k), f[k]) {
			// A stored value that is none of the options, such as an old
			// mode or a station no longer configured, is offered as it is
			// rather than dropped.
			e.picker(k).offer(f[k])
		}
	}
	e.rst = qsoentry.DefaultRST(e.value(qsoentry.Mode), e.value(qsoentry.Submode))
	e.band = freqBand(f[qsoentry.Freq])
	e.bandRX = freqBand(f[qsoentry.FreqRX])
	e.dxcc = entityValues(e.dxccLookup, f[qsoentry.Call])
	e.callbook.reset()
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
func (e *entry) readOnly(f qsoentry.Field) bool {
	return e.live && slices.Contains(entryTimes, f)
}

// setOn sets the date and time on to t.
func (e *entry) setOn(t time.Time) {
	t = t.UTC()
	e.input(qsoentry.Date).SetValue(t.Format(entryDateLayout))
	e.input(qsoentry.TimeOn).SetValue(t.Format(entryTimeLayout))
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
func (e *entry) followsRig(f qsoentry.Field) bool {
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
		e.input(qsoentry.Freq).SetValue(logbook.FormatFreqMHz(s.Freq))
	}
	if e.followsRig(qsoentry.Mode) {
		mode, submode := qsoentry.RigMode(s.Mode, e.dataMode, e.dataSubmode)
		e.picker(qsoentry.Mode).set(mode)
		e.picker(qsoentry.Submode).setOptions(e.submodes())
		e.picker(qsoentry.Submode).set(submode)
	}
	band := e.input(qsoentry.Band).Value()
	e.fillIn(before, e.form())
	if e.followsRig(qsoentry.Band) {
		// A band typed before a sync gives way even when the freq is
		// unchanged.
		band = freqBand(e.input(qsoentry.Freq).Value())
		e.band = band
	}
	e.input(qsoentry.Band).SetValue(band)
}

// unsaved reports whether the form holds values typed or picked since it
// was opened or last saved. In live mode, the values the clock and the rig
// fill in don't count, nor do the default reports. Dates and times are
// compared as read, so t is no change from today's date.
func (e *entry) unsaved() bool {
	f, b := e.form(), e.base
	for k := range qsoentry.FieldCount {
		if e.filledIn(k, f[k]) {
			f[k], b[k] = "", ""
		}
		if slices.Contains(entryTimes, k) {
			f[k], b[k] = qsoentry.Canonical(k, f[k], e.now()), qsoentry.Canonical(k, b[k], e.now())
		}
	}
	return f != b
}

// filledIn reports whether v, the value of field f, was filled in rather
// than typed in live mode: by the clock, by the rig or as the default
// report.
func (e *entry) filledIn(f qsoentry.Field, v string) bool {
	isRST := f == qsoentry.RSTSent || f == qsoentry.RSTRcvd
	return e.readOnly(f) || e.followsRig(f) || e.live && isRST && v == e.rst
}

// copies reports whether the blank off fields copy the on fields: in post
// mode, for a new QSO. Live mode fills in the times from the clock, and an
// edited QSO keeps its own.
func (e *entry) copies() bool {
	return !e.live && !e.editing
}

// withOff returns f with its blank off fields filled in from the on fields
// while they copy them. The date off is filled in only with a time off,
// which it needs.
func (e *entry) withOff(f qsoentry.Form) qsoentry.Form {
	if !e.copies() {
		return f
	}
	dateOff, timeOff := f.OffDefaults(e.now())
	if strings.TrimSpace(f[qsoentry.TimeOff]) == "" {
		f[qsoentry.TimeOff] = timeOff
	}
	if strings.TrimSpace(f[qsoentry.DateOff]) == "" && strings.TrimSpace(f[qsoentry.TimeOff]) != "" {
		f[qsoentry.DateOff] = dateOff
	}
	return f
}

// offCopy returns what off field f copies from the on fields, shown muted
// in its place while it is blank, or "" when it copies nothing.
func (e *entry) offCopy(f qsoentry.Field) string {
	if !e.copies() || strings.TrimSpace(e.value(f)) != "" {
		return ""
	}
	dateOff, timeOff := e.form().OffDefaults(e.now())
	switch f {
	case qsoentry.DateOff:
		return dateOff
	case qsoentry.TimeOff:
		return timeOff
	}
	return ""
}

// defaultDate fills in today's date in an empty date while the off fields
// copy, as the form's own value rather than an unsaved change. The time on
// is left for the user, so no QSO is saved at a time nobody entered.
func (e *entry) defaultDate() {
	if !e.copies() || strings.TrimSpace(e.value(qsoentry.Date)) != "" {
		return
	}
	today := e.now().UTC().Format(qsoentry.DateLayout)
	e.input(qsoentry.Date).SetValue(today)
	e.base[qsoentry.Date] = today
}

// canonicalize rewrites the dates and times that can be edited in their
// canonical layouts.
func (e *entry) canonicalize() {
	for _, f := range entryTimes {
		if !e.readOnly(f) {
			in := e.input(f)
			in.SetValue(qsoentry.Canonical(f, in.Value(), e.now()))
		}
	}
}

// focusField gives field f focus, with the cursor at the end of its value,
// closing any list open.
func (e *entry) focusField(f qsoentry.Field) tea.Cmd {
	e.list.open = false
	if f != e.focus && slices.Contains(entryTimes, e.focus) && !e.readOnly(e.focus) {
		// A date or time left shows what it was read as, so a mistake
		// shows before the save.
		in := e.input(e.focus)
		in.SetValue(qsoentry.Canonical(e.focus, in.Value(), e.now()))
	}
	switch {
	case isMultiLine(e.focus):
		e.area(e.focus).Blur()
	case !isPicked(e.focus):
		e.input(e.focus).Blur()
	}
	e.focus = f
	switch {
	case isPicked(f):
		return nil
	case isMultiLine(f):
		return e.area(f).Focus()
	}
	e.input(f).CursorEnd()
	return e.input(f).Focus()
}

// openEntry opens the New QSO screen on a form empty but for the Station
// tab, filled in from the first station profile, starts its clock and loads
// the logbook's tags. The screen opens in live mode when the rig is
// connected, and in post mode otherwise.
func (m Model) openEntry() (Model, tea.Cmd) {
	e := newEntry()
	e.now = m.now
	e.dxccLookup = m.dxcc
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
	e.defaultDate()
	cmd := e.focusField(focus)
	m.entry = e
	return m, tea.Batch(cmd, loadTagsCmd(m.logbook), m.entryTickCmd())
}

// closeEntry closes the New QSO screen, discarding the form and canceling
// its callbook lookup. The tags are kept to complete the tags field when it
// opens again, and the last data mode for a rig in a packet mode.
func (m Model) closeEntry() Model {
	old := m.entry
	old.callbook.stop()
	m.entry = newEntry()
	m.entry.gen, m.entry.tags = old.gen, old.tags
	m.entry.dataMode, m.entry.dataSubmode = old.dataMode, old.dataSubmode
	return m
}

// editCurrent returns a command that loads the QSO under the index cursor,
// which is also the one the pager shows, to open it in the form. It does
// nothing while the row is loading.
func (m Model) editCurrent() tea.Cmd {
	q, ok := m.idx.current()
	if !ok || m.logbook == nil {
		return nil
	}
	return loadEditCmd(m.logbook, q.ID)
}

// loadEditCmd returns a command that loads the stored QSO with the given ID
// for editing.
func loadEditCmd(s logbook.Store, id int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		q, err := s.Get(ctx, id)
		if err != nil {
			return editLoadedMsg{err: fmt.Errorf("load QSO %d to edit: %w", id, err)}
		}
		return editLoadedMsg{qso: q}
	}
}

// applyEditLoaded opens the loaded QSO in the form, or shows the error
// loading it on the index status line. A QSO landing once the screen is
// already open is dropped.
func (m Model) applyEditLoaded(msg editLoadedMsg) (Model, tea.Cmd) {
	if m.entry.open {
		return m, nil
	}
	if msg.err != nil {
		m.idx.msg = msg.err.Error()
		return m, nil
	}
	return m.openEdit(msg.qso)
}

// openEdit opens the screen on the stored QSO q, in post mode, with every
// tab filled in from q, and loads the logbook's tags. The Station tab shows
// q's own values rather than a profile's, though picking a profile still
// fills them in. The rig is never applied, and the clock does not run.
func (m Model) openEdit(q logbook.QSO) (Model, tea.Cmd) {
	e := newEntry()
	e.now = m.now
	e.dxccLookup = m.dxcc
	e.setStations(m.stations)
	e.editing, e.orig = true, q
	e.setForm(qsoentry.Load(q))
	// The stored values are the form's own, not unsaved changes.
	e.base = e.form()
	e.open = true
	e.gen = m.entry.gen + 1
	e.tags = m.entry.tags
	e.dataMode, e.dataSubmode = m.entry.dataMode, m.entry.dataSubmode
	cmd := e.focusField(qsoentry.Date)
	m.entry = e
	return m, tea.Batch(cmd, loadTagsCmd(m.logbook))
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
		e.defaultDate()
		return m, nil
	}
	e.onLocked = strings.TrimSpace(e.input(qsoentry.Call).Value()) != ""
	if !e.onLocked || e.input(qsoentry.Date).Value() == "" || e.input(qsoentry.TimeOn).Value() == "" {
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
	in := e.input(qsoentry.Tags)
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
	// A key press scrolls the form back to the focused field.
	e.wheeled = false
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case e.saving:
		// Other keys wait for the save, so it lands on the form it saved.
	case e.list.open && !key.Matches(msg, pickListKeys.Leave):
		return m.updateList(msg)
	case key.Matches(msg, k.Save):
		return m.saveEntry(false)
	case key.Matches(msg, k.SaveClose):
		return m.saveEntry(true)
	case key.Matches(msg, k.Close):
		if !e.unsaved() {
			return m.closeEntry(), nil
		}
		question := msgDiscardEntry
		if e.editing {
			question = msgDiscardEdit
		}
		return m.askConfirm(question, func(m Model) (Model, tea.Cmd) {
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
		// An edited QSO stays in post mode, so the rig never changes it.
		if !e.editing {
			return m.toggleLive()
		}
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
	case key.Matches(msg, k.Editor) && slices.Contains(entryEditorFields, e.focus):
		return m.openEditor()
	case isPicked(e.focus) && !e.readOnly(e.focus) && key.Matches(msg, k.List) &&
		(msg.String() != "space" || e.picker(e.focus).typed == ""):
		// A space after typing is part of a code, such as VARA HF.
		e.openList()
	case isMultiLine(e.focus) && key.Matches(msg, e.area(e.focus).KeyMap.InsertNewline):
		// Enter starts a new line of a multi-line field rather than moving
		// on.
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

// openList opens the list of the focused picker's options, below the most
// used modes for the mode.
func (e *entry) openList() {
	var common []adif.Enum
	if e.focus == qsoentry.Mode {
		common = adif.CommonModes()
	}
	p := e.picker(e.focus)
	e.list = newPickList(common, p.options, p.value())
}

// updateList sends a key press to the open list, then fills in what
// follows from the item picked, if any.
func (m Model) updateList(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	e := &m.entry
	it, ok := e.list.update(msg)
	if !ok {
		return m, nil
	}
	before := e.form()
	e.pick(it)
	return m, m.followEdit(e.focus, before)
}

// pick chooses the list item it in the focused picker: the option it was,
// so of two stations with one name the one picked, or else, as when the
// rig changed the options while the list was open, the option with its
// code. A common mode none of the modes is a submode, such as FT4, which
// picks its mode and then it.
func (e *entry) pick(it pickItem) {
	p := e.picker(e.focus)
	old := p.i
	switch {
	case it.i >= 0 && it.i < len(p.options) && p.options[it.i].Code == it.Code:
		p.i, p.typed = it.i, ""
	case e.focus == qsoentry.Mode && !strings.EqualFold(adif.ModeOf(it.Code), it.Code):
		p.set(adif.ModeOf(it.Code))
		e.picker(qsoentry.Submode).setOptions(e.submodes())
		e.picker(qsoentry.Submode).set(it.Code)
	default:
		p.set(it.Code)
	}
	if p.i != old && e.focus == qsoentry.StationName {
		e.fillStation()
	}
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
		if ok && e.picker(e.focus).update(k) && e.focus == qsoentry.StationName {
			e.fillStation()
		}
	case isMultiLine(e.focus):
		*e.area(e.focus), cmd = e.area(e.focus).Update(msg)
	default:
		in := e.input(e.focus)
		if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, in.KeyMap.AcceptSuggestion) && in.CurrentSuggestion() != "" {
			// The suggestion replaces what was typed, so a tag keeps the
			// logbook's spelling.
			in.SetValue(in.CurrentSuggestion())
			in.CursorEnd()
			break
		}
		*in, cmd = in.Update(msg)
	}
	return m, tea.Batch(cmd, m.followEdit(e.focus, before))
}

// followEdit fills in what follows from a change by hand to field f, from
// the form before, if the form changed, and returns the command that keeps
// the tab shown.
func (m *Model) followEdit(f qsoentry.Field, before qsoentry.Form) tea.Cmd {
	e := &m.entry
	after := e.form()
	if after == before {
		return nil
	}
	e.markHand(f)
	e.fillIn(before, after)
	e.followCall(before, after, m.now())
	cmd := e.keepTab()
	m.revalidateEntry()
	return cmd
}

// openEditor opens the field with focus in the user's editor, suspending
// the screen until it exits. A failure to start it is shown on the status
// line.
func (m Model) openEditor() (Model, tea.Cmd) {
	e := &m.entry
	argv, err := editor.Command(m.getenv)
	if err != nil {
		e.err = fmt.Errorf("open editor: %w", err)
		return m, nil
	}
	f := e.focus
	cmd, path, err := editor.Prepare(argv, e.value(f))
	if err != nil {
		e.err = fmt.Errorf("open editor: %w", err)
		return m, nil
	}
	gen := e.gen
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return entryEditedMsg{gen: gen, field: f, path: path, err: err}
	})
}

// applyEdited puts the text left by the editor in its field, flattened to
// one line for the comment, then fills in what follows as for a change by
// hand. The file is always removed. An editor that failed keeps the old
// value and shows its error; an edit for a closed screen, or an earlier
// opening, is dropped, though a failure to read or remove its file still
// shows on whichever screen is open.
func (m Model) applyEdited(msg entryEditedMsg) (Model, tea.Cmd) {
	e := &m.entry
	text, readErr := editor.ReadBack(msg.path)
	if !e.open || msg.gen != e.gen {
		if readErr != nil {
			err := fmt.Errorf("edit %s: %w", entryLabels[msg.field], readErr)
			if e.open {
				e.err = err
			} else {
				m.idx.msg = err.Error()
			}
		}
		return m, nil
	}
	if err := errors.Join(msg.err, readErr); err != nil {
		e.err = fmt.Errorf("edit %s: %w", entryLabels[msg.field], err)
		return m, nil
	}
	if isMultiLine(msg.field) {
		text = editor.MultiLine(text)
	} else {
		text = editor.SingleLine(text)
	}
	before := e.form()
	e.setValue(msg.field, text)
	if !isMultiLine(msg.field) {
		e.input(msg.field).CursorEnd()
	}
	return m, m.followEdit(msg.field, before)
}

// fillIn fills in what follows from a change to the form from before to
// after: the submodes and default reports from the mode, the bands from the
// freqs, the DXCC entity, continent and zones from the call and the tag
// completions from the tags. A data mode is kept for a rig in a packet mode.
func (e *entry) fillIn(before, after qsoentry.Form) {
	changed := func(f qsoentry.Field) bool { return after[f] != before[f] }
	if changed(qsoentry.Mode) {
		e.picker(qsoentry.Submode).setOptions(e.submodes())
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
	if changed(qsoentry.Call) {
		e.fillDXCC()
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
		if v := e.input(f).Value(); v == "" || v == e.rst {
			e.input(f).SetValue(rst)
		}
	}
	e.rst = rst
}

// fillBand fills in the band of the freq in freq, in the band field if it
// is empty or holds last, the band last filled in. It returns the band
// filled in.
func (e *entry) fillBand(freq, band qsoentry.Field, last string) string {
	derived := freqBand(e.input(freq).Value())
	if v := e.input(band).Value(); v == "" || v == last {
		e.input(band).SetValue(derived)
	}
	return derived
}

// validateEntry checks the form, keeping its errors for the view.
func (m *Model) validateEntry() {
	_, err := qsoentry.Build(m.entry.withOff(m.entry.form()), m.now())
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
// when close is true, closes the screen. While editing, it saves the form
// over the stored QSO instead, keeping what the form doesn't show. A form
// that fails validation shows its errors, with focus on the first field in
// error, on its tab, and is not saved.
func (m Model) saveEntry(close bool) (Model, tea.Cmd) {
	e := &m.entry
	if e.saving {
		return m, nil
	}
	now := m.now()
	e.tickClock(now)
	e.canonicalize()
	f := e.withOff(e.form())
	if e.live {
		// The QSO ends as it is saved. The form keeps no time off, so a
		// save that fails leaves none behind.
		f[qsoentry.DateOff] = now.UTC().Format(entryDateLayout)
		f[qsoentry.TimeOff] = now.UTC().Format(entryTimeLayout)
	}
	var q logbook.QSO
	var err error
	if e.editing {
		q, err = qsoentry.Edit(f, e.orig, now)
	} else {
		q, err = qsoentry.Build(f, now)
	}
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
	if e.editing {
		return m, updateQSOCmd(m.logbook, q, close)
	}
	return m, addQSOCmd(m.logbook, q, close)
}

// updateQSOCmd returns a command that saves q over the stored QSO with its
// ID, then reads it back, so the screen has the stored copy, with its
// derived fields worked out again.
func updateQSOCmd(s logbook.Store, q logbook.QSO, close bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		if err := s.Update(ctx, q); err != nil {
			return entrySavedMsg{close: close, err: fmt.Errorf("save QSO %d with %s: %w", q.ID, q.Call, err)}
		}
		saved, err := s.Get(ctx, q.ID)
		if err != nil {
			return entrySavedMsg{close: close, err: fmt.Errorf("reload saved QSO %d: %w", q.ID, err)}
		}
		return entrySavedMsg{qso: saved, close: close}
	}
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
// next QSO, keeping the values that carry over, or closes the screen. An
// edited QSO stays in the form as saved instead, and the pager shows it. A
// failed save keeps the form as it is.
func (m Model) applySaved(msg entrySavedMsg) (Model, tea.Cmd) {
	e := &m.entry
	e.saving = false
	if msg.err != nil {
		e.err = msg.err
		return m, nil
	}
	saved := "Saved QSO with " + msg.qso.Call + "."
	if e.editing {
		saved = "Updated QSO with " + msg.qso.Call + "."
	}
	for _, t := range msg.qso.Tags {
		if !slices.ContainsFunc(e.tags, func(s string) bool { return strings.EqualFold(s, t) }) {
			e.tags = append(e.tags, t)
		}
	}
	// The saved QSO is now an earlier QSO with its call.
	e.worked = workedBefore{}
	m.idx.restart()
	reload := loadIndexCmd(m.logbook, m.idx.gen, m.idx.limit.expr, m.idx.sort, msg.qso.ID, m.idx.cursor)
	if e.editing && m.pager.open {
		// The pager shows the saved QSO, as stored, rather than the copy it
		// fetched before.
		m.pager.fetched, m.pager.pending, m.pager.qso, m.pager.err = msg.qso.ID, 0, msg.qso, nil
	}
	if msg.close {
		m = m.closeEntry()
		m.idx.msg = saved
		return m, reload
	}
	if e.editing {
		// The form stays on the QSO, now as saved.
		e.orig = msg.qso
		e.setForm(qsoentry.Load(msg.qso))
		e.base = e.form()
		e.msg = saved
		return m, reload
	}
	next := e.form().Carry()
	e.setForm(next)
	e.base = next
	e.defaultDate()
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
// bar, the open tab, below the QSO tab the worked-before panel, and a
// status line.
func (m Model) entryView(width, height int) string {
	st, e := m.styles, &m.entry
	lines := []string{tabBar(st, e.tabNames(), slices.Index(e.tabs(), e.tab), width)}
	form, end := m.entryBody(width)
	lines = append(lines, form[e.skip(len(form), end, height):]...)
	lines = lines[:min(len(lines), max(height-1, 0))]
	for len(lines) < height-1 {
		lines = append(lines, "")
	}
	status := "New QSO · post"
	switch {
	case e.editing:
		status = "Edit QSO"
	case e.live:
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

// tabNames returns the names in the tab bar of the tabs shown.
func (e *entry) tabNames() []string {
	tabs := e.tabs()
	names := make([]string, len(tabs))
	for i, t := range tabs {
		names[i] = entryTabNames[t]
	}
	return names
}

// entryBody returns the lines of the New QSO screen below the tab bar,
// each fitted to width cells: the open tab's rows and, below the QSO tab's,
// the worked-before panel. It also returns the line after the focused row
// and its errors.
func (m Model) entryBody(width int) (lines []string, end int) {
	lines, row, rows := m.entryFields(width)
	if m.entry.tab == entryTabQSO {
		lines = append(lines, m.workedView(width)...)
	}
	return lines, row + rows
}

// skip returns the number of the n body lines scrolled off the top of a
// New QSO screen height lines high, below the tab bar and above the status
// line: as far as the wheel scrolled them, or else just far enough to show
// the focused row and its errors, which end at line end. The worked-before
// panel below the QSO tab's rows is cut first.
func (e *entry) skip(n, end, height int) int {
	room := max(height-2, 0)
	if e.wheeled {
		return min(max(e.scroll, 0), max(n-room, 0))
	}
	return max(end-room, 0)
}

// wheelEntry scrolls the New QSO screen's body step lines down, or up when
// step is negative, between its first line and its last.
func (m *Model) wheelEntry(step int) {
	e := &m.entry
	form, end := m.entryBody(m.bodyWidth())
	e.scroll = e.skip(len(form), end, m.bodyHeight()) + step
	e.wheeled = true
	e.scroll = e.skip(len(form), end, m.bodyHeight())
}

// entryFields renders the open tab's rows, each fitted to width cells and
// followed by a line with the errors of its fields, if any. It also returns
// the line of the focused field's row and the number of lines that row
// takes.
func (m Model) entryFields(width int) (lines []string, focus, rows int) {
	e := &m.entry
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
		if e.list.open && slices.Contains(row, e.focus) {
			lines = append(lines, m.entryList(slices.Index(row, e.focus), width, colWidth, lw)...)
		}
		if slices.Contains(row, e.focus) {
			focus, rows = start, len(lines)-start
		}
	}
	return lines, focus, rows
}

// entryList renders the open list below the value of the field in column
// col of a row of fields colWidth cells wide, labeled lw cells wide, fitted
// to width cells. In a narrow window it takes the whole width.
func (m Model) entryList(col, width, colWidth, lw int) []string {
	indent := col*(colWidth+2) + lw + 2
	if width-indent < entryListMinWidth {
		indent = 0
	}
	lines := m.entry.list.view(m.styles, width-indent)
	for i, l := range lines {
		lines[i] = strings.Repeat(" ", indent) + l
	}
	return lines
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
// cells. The call is followed by its DXCC entity and LoTW status and the
// grid by the bearing and distance to it. A multi-line field takes a line
// for each line of its text area.
func (m Model) entryCell(f qsoentry.Field, width, lw int) string {
	st, e := m.styles, &m.entry
	focused := f == e.focus
	label := st.label.Render(pad(entryLabels[f], lw))
	if focused {
		label = st.title.Render(pad(entryLabels[f], lw))
	}
	room := max(width-lw-2, 1)
	var value string
	switch {
	case isMultiLine(f):
		// The lines after the first line up under it.
		value = strings.ReplaceAll(e.area(f).View(), "\n", "\n"+strings.Repeat(" ", lw+2))
	case isPicked(f):
		// The hand mark keeps its room, cutting the description first.
		mark := m.entryHandMark(f)
		if mark != "" {
			room -= lipgloss.Width(mark) + 2
		}
		value = e.picker(f).view(st, focused, room)
		if mark != "" {
			value += "  " + mark
		}
	default:
		suffix, suffixAt := m.entrySuffix(f)
		// A copy, since the view must not resize the widget the copies of
		// the Model share.
		in := *e.input(f)
		if c := e.offCopy(f); c != "" {
			in.Placeholder = c
		}
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
// of the input before it: the DXCC entity, LoTW status and callbook status
// after the call, the path after the grid and the hand mark after the freq
// and band. It returns "" for other fields.
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
		parts := slices.DeleteFunc([]string{m.entryDXCC(), m.entryLoTW(), m.entryCallbook()}, func(s string) bool { return s == "" })
		return strings.Join(parts, "  "), 14
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
	e := &m.entry
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

// sizeEntryAreas fits the multi-line fields to the body width beside their
// labels, and the notes field to the Notes tab of a body height lines tall,
// below the tab bar and above the status line.
func (m *Model) sizeEntryAreas(height int) {
	for _, f := range entryMultiLine {
		lw := entryLayouts[entryTabOf(f)].labelWidth
		m.entry.area(f).SetWidth(max(m.bodyWidth()-lw-2, 1))
	}
	m.entry.area(qsoentry.Notes).SetHeight(max(height-2, 1))
}
