package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/qsoentry"
)

// entryTab is one of the New QSO screen's tabs.
type entryTab int

// The New QSO screen's tabs, in tab bar order. The Satellite tab is only
// shown while the propagation mode is SAT.
const (
	entryTabQSO entryTab = iota
	entryTabStation
	entryTabGeneral
	entryTabSatellite
	entryTabNotes
	entryTabQSL
	// entryTabCount is the number of tabs.
	entryTabCount
)

// entryTabNames are the tab bar names of the New QSO screen's tabs.
var entryTabNames = [entryTabCount]string{"QSO", "Station", "General", "Satellite", "Notes", "QSL"}

// entryLayout lays out one tab of the New QSO screen.
type entryLayout struct {
	// rows each hold one field or two side by side. Tab moves through
	// them in order.
	rows [][]qsoentry.Field
	// labelWidth is the width of the tab's field labels.
	labelWidth int
}

// entryLayouts lays out each tab of the New QSO screen.
var entryLayouts = [entryTabCount]entryLayout{
	entryTabQSO: {labelWidth: 8, rows: [][]qsoentry.Field{
		{qsoentry.Date, qsoentry.TimeOn},
		{qsoentry.DateOff, qsoentry.TimeOff},
		{qsoentry.Call},
		{qsoentry.Mode, qsoentry.Submode},
		{qsoentry.Freq, qsoentry.Band},
		{qsoentry.FreqRX, qsoentry.BandRX},
		{qsoentry.RSTSent, qsoentry.RSTRcvd},
		{qsoentry.Name},
		{qsoentry.QTH},
		{qsoentry.Gridsquare},
		{qsoentry.Comment},
		{qsoentry.Tags},
	}},
	entryTabStation: {labelWidth: 12, rows: [][]qsoentry.Field{
		// The station picker only shows with more than one profile.
		{qsoentry.StationName},
		{qsoentry.StationCallsign, qsoentry.Operator},
		{qsoentry.MyRig, qsoentry.TxPwr},
		{qsoentry.MyGridsquare},
		{qsoentry.MyPOTARef, qsoentry.MySOTARef},
		{qsoentry.MyIOTA, qsoentry.MyWWFFRef},
		{qsoentry.MyState, qsoentry.MyCity},
	}},
	entryTabGeneral: {labelWidth: 9, rows: [][]qsoentry.Field{
		{qsoentry.DXCC, qsoentry.Cont},
		{qsoentry.CQZ, qsoentry.ITUZ},
		{qsoentry.State, qsoentry.Cnty},
		{qsoentry.Region},
		{qsoentry.IOTA, qsoentry.SOTARef},
		{qsoentry.WWFFRef, qsoentry.POTARef},
		{qsoentry.Sig, qsoentry.SigInfo},
		{qsoentry.DarcDok},
		{qsoentry.Email},
		{qsoentry.PropMode, qsoentry.AntPath},
	}},
	entryTabSatellite: {labelWidth: 9, rows: [][]qsoentry.Field{
		{qsoentry.SatName},
		{qsoentry.SatMode},
	}},
	entryTabNotes: {labelWidth: 5, rows: [][]qsoentry.Field{
		{qsoentry.Notes},
	}},
	entryTabQSL: {labelWidth: 13, rows: [][]qsoentry.Field{
		{qsoentry.QSLSent, qsoentry.QSLSentVia},
		{qsoentry.QSLVia},
		{qsoentry.QSLMsg},
		{qsoentry.LoTWSent},
		{qsoentry.EQSLSent},
		{qsoentry.ClubLogSent},
	}},
}

// entryLabels are the labels of the form's fields.
var entryLabels = [qsoentry.FieldCount]string{
	// QSO.
	"Date", "Time on", "Date off", "Time off", "Call", "Mode", "Submode",
	"Freq", "Band", "Freq RX", "Band RX", "RST sent", "RST rcvd", "Name",
	"QTH", "Grid", "Comment", "Tags",
	// Station.
	"Station", "Station call", "Operator", "Radio", "TX power", "My grid",
	"My POTA", "My SOTA", "My IOTA", "My WWFF", "My state", "My city",
	// General.
	"DXCC", "Continent", "Region", "CQ zone", "ITU zone", "State", "County",
	"IOTA", "SOTA", "WWFF", "POTA", "SIG", "SIG info", "DOK", "Email",
	"Prop mode", "Ant path",
	// Satellite.
	"Satellite", "Sat mode",
	// Notes.
	"Notes",
	// QSL.
	"Card sent", "Sent via", "QSL via", "QSL msg", "LoTW sent", "eQSL sent",
	"Club Log sent",
}

// entryPlaceholders are shown in empty fields that need a hint.
var entryPlaceholders = map[qsoentry.Field]string{
	qsoentry.Date:      "YYYY-MM-DD",
	qsoentry.TimeOn:    "HH:MM",
	qsoentry.DateOff:   "YYYY-MM-DD",
	qsoentry.TimeOff:   "HH:MM",
	qsoentry.Freq:      "MHz or kHz",
	qsoentry.FreqRX:    "MHz or kHz",
	qsoentry.Tags:      "comma-separated",
	qsoentry.TxPwr:     "watts",
	qsoentry.MyPOTARef: "comma-separated",
	qsoentry.POTARef:   "comma-separated",
	qsoentry.DXCC:      "entity code",
	qsoentry.Cnty:      "e.g. MA,Middlesex",
}

// entryPicked are the fields picked from a list of options rather than
// typed.
var entryPicked = []qsoentry.Field{
	qsoentry.StationName, qsoentry.Mode, qsoentry.Submode, qsoentry.PropMode, qsoentry.AntPath,
	qsoentry.QSLSent, qsoentry.QSLSentVia, qsoentry.LoTWSent, qsoentry.EQSLSent, qsoentry.ClubLogSent,
}

// isPicked reports whether field f is picked rather than typed.
func isPicked(f qsoentry.Field) bool {
	return slices.Contains(entryPicked, f)
}

// entryTabOf returns the tab field f is on.
func entryTabOf(f qsoentry.Field) entryTab {
	for t, l := range entryLayouts {
		for _, row := range l.rows {
			if slices.Contains(row, f) {
				return entryTab(t)
			}
		}
	}
	return entryTabQSO
}

// tabs returns the tabs shown, in tab bar order: every tab but Satellite,
// which only shows while the propagation mode is SAT.
func (e entry) tabs() []entryTab {
	sat := strings.EqualFold(e.pickers[qsoentry.PropMode].value(), "SAT")
	var ts []entryTab
	for t := range entryTabCount {
		if t != entryTabSatellite || sat {
			ts = append(ts, t)
		}
	}
	return ts
}

// shown reports whether field f is shown on its tab: every field but the
// station picker, which only shows with more than one profile to pick.
func (e entry) shown(f qsoentry.Field) bool {
	return f != qsoentry.StationName || len(e.stations) > 1
}

// rows returns the rows of tab t, holding the fields shown.
func (e entry) rows(t entryTab) [][]qsoentry.Field {
	var rows [][]qsoentry.Field
	for _, row := range entryLayouts[t].rows {
		var fs []qsoentry.Field
		for _, f := range row {
			if e.shown(f) {
				fs = append(fs, f)
			}
		}
		if len(fs) > 0 {
			rows = append(rows, fs)
		}
	}
	return rows
}

// fields returns the fields shown on tab t, in tab order.
func (e entry) fields(t entryTab) []qsoentry.Field {
	return slices.Concat(e.rows(t)...)
}

// focusable reports whether field f can take focus: it is shown, on a tab
// shown, and can be edited.
func (e entry) focusable(f qsoentry.Field) bool {
	return e.shown(f) && slices.Contains(e.tabs(), entryTabOf(f)) && !e.readOnly(f)
}

// goTab opens tab t, giving its first field that can be edited focus.
func (e *entry) goTab(t entryTab) tea.Cmd {
	e.tab = t
	fs := e.fields(t)
	if i := slices.IndexFunc(fs, func(f qsoentry.Field) bool { return !e.readOnly(f) }); i >= 0 {
		return e.focusField(fs[i])
	}
	return nil
}

// stepTab opens the tab step tabs on from the open one among those shown,
// wrapping around.
func (e *entry) stepTab(step int) tea.Cmd {
	ts := e.tabs()
	n := len(ts)
	i := max(slices.Index(ts, e.tab), 0)
	return e.goTab(ts[((i+step)%n+n)%n])
}

// keepTab moves from the open tab to General once it is no longer shown, as
// the Satellite tab is once the propagation mode is no longer SAT.
func (e *entry) keepTab() tea.Cmd {
	if slices.Contains(e.tabs(), e.tab) {
		return nil
	}
	return e.goTab(entryTabGeneral)
}

// stepFocus returns the field step fields on from the one with focus,
// among the open tab's fields, wrapping around and skipping read-only
// fields.
func (e entry) stepFocus(step int) qsoentry.Field {
	fs := e.fields(e.tab)
	n := len(fs)
	i := slices.Index(fs, e.focus)
	if i < 0 {
		// Focus off the tab steps onto its first field, or its last.
		i = 0
		if step > 0 {
			i = n - 1
		}
	}
	for range n {
		i = ((i+step)%n + n) % n
		if !e.readOnly(fs[i]) {
			return fs[i]
		}
	}
	return e.focus
}
