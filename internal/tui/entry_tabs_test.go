package tui

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

// homeStation and parkStation are the station profiles of the New QSO tab
// tests.
var (
	homeStation = qsoentry.Station{
		Name: "home", Callsign: "W1AW", Operator: "W1AW", Grid: "FN31",
		State: "CT", City: "Newington", POTA: []string{"US-0001", "US-0002"},
	}
	parkStation = qsoentry.Station{
		Name: "park", Callsign: "W1AW/P", Operator: "W1AW", Grid: "FN32", SOTA: "W1/HA-001",
	}
)

// tabNames returns the names in the New QSO screen's tab bar.
func tabNames(m Model) []string {
	bar, _, _ := strings.Cut(plain(m.entryView(m.bodyWidth(), m.bodyHeight())), "\n")
	return strings.Fields(strings.Trim(bar, "─"))
}

func TestEntryTabFieldNavigation(t *testing.T) {
	tests := []struct {
		key  rune
		tab  entryTab
		want []qsoentry.Field
	}{
		{'2', entryTabStation, []qsoentry.Field{
			qsoentry.StationCallsign, qsoentry.Operator, qsoentry.MyRig, qsoentry.TxPwr,
			qsoentry.MyGridsquare, qsoentry.MyPOTARef, qsoentry.MySOTARef, qsoentry.MyIOTA,
			qsoentry.MyWWFFRef, qsoentry.MyState, qsoentry.MyCity,
		}},
		{'3', entryTabGeneral, []qsoentry.Field{
			qsoentry.DXCC, qsoentry.Cont, qsoentry.CQZ, qsoentry.ITUZ, qsoentry.State,
			qsoentry.Cnty, qsoentry.Region, qsoentry.IOTA, qsoentry.SOTARef, qsoentry.WWFFRef,
			qsoentry.POTARef, qsoentry.Sig, qsoentry.SigInfo, qsoentry.DarcDok, qsoentry.Email,
			qsoentry.PropMode, qsoentry.AntPath,
		}},
		{'4', entryTabNotes, []qsoentry.Field{qsoentry.Notes}},
		{'5', entryTabQSL, []qsoentry.Field{
			qsoentry.QSLSent, qsoentry.QSLSentVia, qsoentry.QSLVia, qsoentry.QSLMsg,
			qsoentry.LoTWSent, qsoentry.EQSLSent, qsoentry.ClubLogSent,
		}},
	}
	for _, tt := range tests {
		t.Run(entryTabNames[tt.tab], func(t *testing.T) {
			m := newEntryModel(t, newEntryStore(t, nil), WithStations([]qsoentry.Station{homeStation}))
			m = press(t, m, altKey(tt.key))
			require.Equal(t, tt.tab, m.entry.tab)

			var got []qsoentry.Field
			for range tt.want {
				got = append(got, m.entry.focus)
				m = press(t, m, tab())
			}
			assert.Equal(t, tt.want, got, "tab moves through the tab's fields in order")
			assert.Equal(t, tt.want[0], m.entry.focus, "tab wraps to the tab's first field")
			m = press(t, m, shiftTab())
			assert.Equal(t, tt.want[len(tt.want)-1], m.entry.focus, "shift+tab wraps to the tab's last field")
			assert.Equal(t, tt.tab, m.entry.tab, "the fields stay on their tab")
		})
	}
}

func TestEntryTabLabels(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil), WithStations([]qsoentry.Station{homeStation}))
	for key, labels := range map[rune][]string{
		'2': {"Station call", "Operator", "Radio", "TX power", "My grid", "My POTA", "My SOTA", "My IOTA", "My WWFF", "My state", "My city"},
		'3': {"DXCC", "Continent", "CQ zone", "ITU zone", "State", "County", "Region", "IOTA", "SOTA", "WWFF", "POTA", "SIG", "SIG info", "DOK", "Email", "Prop mode", "Ant path"},
		'4': {"Notes"},
		'5': {"Card sent", "Sent via", "QSL via", "QSL msg", "LoTW sent", "eQSL sent", "Club Log sent"},
	} {
		m = press(t, m, altKey(key))
		body := bodyText(m)
		for _, l := range labels {
			assert.Contains(t, body, l, "tab %c", key)
		}
	}
}

func TestEntryStationPrefilledFromProfile(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil), WithStations([]qsoentry.Station{homeStation, parkStation}))

	for f, want := range map[qsoentry.Field]string{
		qsoentry.StationName: "home", qsoentry.StationCallsign: "W1AW", qsoentry.Operator: "W1AW",
		qsoentry.MyGridsquare: "FN31", qsoentry.MyPOTARef: "US-0001,US-0002", qsoentry.MyState: "CT",
		qsoentry.MyCity: "Newington", qsoentry.MySOTARef: "", qsoentry.MyRig: "",
	} {
		assert.Equal(t, want, entryValue(m, f), "%s comes from the first profile", f)
	}
	m = press(t, m, altKey('2'))
	body := bodyText(m)
	assert.Contains(t, body, "W1AW")
	assert.Contains(t, body, "Newington")

	m = press(t, m, esc())
	assert.False(t, m.entry.open, "the profile's values are not unsaved changes")
	assert.Equal(t, promptNone, m.prompt)
}

func TestEntryStationPickerHiddenWithOneProfile(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil), WithStations([]qsoentry.Station{homeStation}))

	m = press(t, m, altKey('2'))

	assert.Equal(t, qsoentry.StationCallsign, m.entry.focus, "no station picker to start at")
	assert.NotContains(t, m.entry.fields(entryTabStation), qsoentry.StationName)
	assert.Equal(t, "home", entryValue(m, qsoentry.StationName), "the station is still named")
}

func TestEntryStationPickerSwitchesProfiles(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newEntryModel(t, s, WithStations([]qsoentry.Station{homeStation, parkStation}))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.MyRig: "IC-705", qsoentry.TxPwr: "5"})

	m = press(t, m, altKey('2'))
	require.Equal(t, qsoentry.StationName, m.entry.focus, "the station picker comes first")
	assert.Contains(t, bodyText(m), "home")
	m = press(t, m, special(tea.KeyRight))

	for f, want := range map[qsoentry.Field]string{
		qsoentry.StationName: "park", qsoentry.StationCallsign: "W1AW/P", qsoentry.MyGridsquare: "FN32",
		qsoentry.MySOTARef: "W1/HA-001", qsoentry.MyPOTARef: "", qsoentry.MyState: "", qsoentry.MyCity: "",
		qsoentry.MyRig: "IC-705", qsoentry.TxPwr: "5",
	} {
		assert.Equal(t, want, entryValue(m, f), "%s after picking the park profile", f)
	}
	assert.Contains(t, bodyText(m), "park")

	m = typeText(t, m, "h")
	assert.Equal(t, "home", entryValue(m, qsoentry.StationName), "typing picks a profile by name")
	assert.Equal(t, "FN31", entryValue(m, qsoentry.MyGridsquare))
	assert.Equal(t, "IC-705", entryValue(m, qsoentry.MyRig), "the rig is not part of a profile")
}

// everyTabQSO is a QSO with a value on every tab.
var everyTabQSO = map[qsoentry.Field]string{
	qsoentry.Date: "2026-11-09", qsoentry.TimeOn: "1405", qsoentry.Call: "k1abc", qsoentry.Mode: "fm",
	qsoentry.Freq: "145.900",
	// Station.
	qsoentry.MyRig: "IC-9700", qsoentry.TxPwr: "50",
	// General.
	qsoentry.DXCC: "291", qsoentry.Cont: "na", qsoentry.CQZ: "5", qsoentry.ITUZ: "8", qsoentry.State: "ma",
	qsoentry.Cnty: "MA,Middlesex", qsoentry.Email: "k1abc@example.com", qsoentry.PropMode: "sat",
	// Satellite.
	qsoentry.SatName: "iss", qsoentry.SatMode: "v/u",
	// QSL.
	qsoentry.QSLSent: "r", qsoentry.QSLSentVia: "b", qsoentry.QSLVia: "W1BUR", qsoentry.QSLMsg: "Tnx sat QSO",
	qsoentry.LoTWSent: "q", qsoentry.ClubLogSent: "y",
}

func TestEntrySaveFromEveryTab(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newEntryModel(t, s, WithStations([]qsoentry.Station{homeStation}))
	m = fill(t, m, everyTabQSO)
	m = focusOn(t, m, qsoentry.Notes)
	m = typeText(t, m, "first sat QSO")
	m = press(t, m, enter())
	m = typeText(t, m, "good signals")

	m = press(t, m, saveKey)

	require.Len(t, s.added, 1)
	q := s.added[0]
	assert.Equal(t, "K1ABC", q.Call)
	assert.Equal(t, "W1AW", q.StationCallsign)
	assert.Equal(t, "W1AW", q.Operator)
	assert.Equal(t, "FN31", q.MyGridsquare)
	assert.Equal(t, sql.Null[float64]{V: 50, Valid: true}, q.TxPwr)
	assert.Equal(t, sql.Null[int]{V: 291, Valid: true}, q.DXCC)
	assert.Equal(t, "NA", q.Cont)
	assert.Equal(t, sql.Null[int]{V: 5, Valid: true}, q.CQZ)
	assert.Equal(t, sql.Null[int]{V: 8, Valid: true}, q.ITUZ)
	assert.Equal(t, "MA", q.State)
	assert.Equal(t, "MA,Middlesex", q.Cnty)
	for name, want := range map[string]string{
		logbook.StationField: "home", "MY_RIG": "IC-9700", "MY_POTA_REF": "US-0001,US-0002",
		"MY_STATE": "CT", "MY_CITY": "Newington", "EMAIL": "k1abc@example.com", "PROP_MODE": "SAT",
		"SAT_NAME": "ISS", "SAT_MODE": "V/U", "NOTES": "first sat QSO\ngood signals",
		"QSL_SENT_VIA": "B", "QSL_VIA": "W1BUR", "QSLMSG": "Tnx sat QSO",
	} {
		assert.Equal(t, want, q.Fields[name], "field %s", name)
	}
	assert.Equal(t, []logbook.Confirmation{
		{Service: logbook.ServiceCard, Sent: "R"},
		{Service: logbook.ServiceLoTW, Sent: "Q"},
		{Service: logbook.ServiceClubLog, Sent: "Y"},
	}, q.Confirmations)

	// What carries over to the next QSO.
	for f, want := range map[qsoentry.Field]string{
		qsoentry.MyRig: "IC-9700", qsoentry.TxPwr: "50", qsoentry.StationName: "home",
		qsoentry.PropMode: "SAT", qsoentry.SatName: "iss", qsoentry.SatMode: "v/u",
		qsoentry.QSLSent: "R", qsoentry.QSLSentVia: "B", qsoentry.QSLVia: "W1BUR", qsoentry.LoTWSent: "Q",
		qsoentry.ClubLogSent: "Y",
		qsoentry.DXCC:        "", qsoentry.Cont: "", qsoentry.Email: "", qsoentry.Cnty: "", qsoentry.Notes: "",
	} {
		assert.Equal(t, want, entryValue(m, f), "%s after saving", f)
	}
	assert.Contains(t, tabNames(m), "Satellite", "the satellite tab stays with the carried PROP_MODE")
	m = press(t, m, esc())
	assert.False(t, m.entry.open, "the carried values are not unsaved changes")
}

func TestEntrySatelliteTabFollowsPropMode(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	assert.Equal(t, []string{"QSO", "Station", "General", "Notes", "QSL"}, tabNames(m))

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.PropMode: "sat"})
	require.Equal(t, "SAT", entryValue(m, qsoentry.PropMode))
	assert.Equal(t, []string{"QSO", "Station", "General", "Satellite", "Notes", "QSL"}, tabNames(m))

	m = press(t, m, nextTabKey)
	assert.Equal(t, entryTabSatellite, m.entry.tab, "the satellite tab follows general")
	assert.Equal(t, qsoentry.SatName, m.entry.focus)
	m = press(t, m, nextTabKey)
	assert.Equal(t, entryTabNotes, m.entry.tab)
	m = press(t, m, altKey('4'))
	assert.Equal(t, entryTabSatellite, m.entry.tab, "alt+4 is the satellite tab while shown")
	m = press(t, m, altKey('6'))
	assert.Equal(t, entryTabQSL, m.entry.tab)

	m = focusOn(t, m, qsoentry.PropMode)
	m = press(t, m, special(tea.KeyHome))
	assert.Empty(t, entryValue(m, qsoentry.PropMode))
	assert.Equal(t, []string{"QSO", "Station", "General", "Notes", "QSL"}, tabNames(m))
	m = press(t, m, altKey('4'))
	assert.Equal(t, entryTabNotes, m.entry.tab, "alt+4 is notes again")
}

func TestEntryHelpCountsTabsShown(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m.help.ShowAll = true
	assert.Regexp(t, `alt\+1-5\s+go to tab`, plain(m.footerView()), "no satellite tab, so five tabs")

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.PropMode: "sat"})
	m.help.ShowAll = true
	assert.Regexp(t, `alt\+1-6\s+go to tab`, plain(m.footerView()), "the satellite tab makes six")
}

func TestEntryLeavesHiddenSatelliteTab(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.PropMode: "sat"})
	m = press(t, m, altKey('4'))
	require.Equal(t, entryTabSatellite, m.entry.tab)

	// Nothing on the satellite tab changes the propagation mode, but a
	// form put in place, as after a save, can.
	f := m.entry.form()
	f[qsoentry.PropMode] = ""
	m.entry.setForm(f)
	_ = m.entry.keepTab()

	assert.Equal(t, entryTabGeneral, m.entry.tab)
	assert.Equal(t, qsoentry.DXCC, m.entry.focus)
}

func TestEntryNotesAreMultiLine(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = press(t, m, altKey('4'))
	require.Equal(t, qsoentry.Notes, m.entry.focus)

	m = typeText(t, m, "line one")
	m = press(t, m, enter())
	m = typeText(t, m, "line two")
	assert.Equal(t, "line one\nline two", entryValue(m, qsoentry.Notes), "enter inserts a newline")
	body := bodyText(m)
	assert.Contains(t, body, "line one")
	assert.Contains(t, body, "line two")

	m = press(t, m, tab())
	assert.Equal(t, "line one\nline two", entryValue(m, qsoentry.Notes), "tab doesn't go into the notes")
	m = press(t, m, altKey('1'))
	assert.Equal(t, qsoentry.Date, m.entry.focus, "the notes give up focus")
	m = typeText(t, m, "2")
	assert.Equal(t, "line one\nline two", entryValue(m, qsoentry.Notes))

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 12})
	m = press(t, m, altKey('4'))
	assert.LessOrEqual(t, lipgloss.Height(m.View().Content), 12, "the notes fit the window")
	assert.Contains(t, bodyText(m), "New QSO · post", "the status line stays")
}

func TestEntrySaveJumpsToTheTabInError(t *testing.T) {
	s := newEntryStore(t, nil)
	m := newEntryModel(t, s)
	m = fill(t, m, paperQSO)
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.ITUZ: "99", qsoentry.Email: "nope"})
	m = press(t, m, altKey('1'))

	m = press(t, m, saveKey)

	assert.Empty(t, s.added)
	assert.Equal(t, entryTabGeneral, m.entry.tab)
	assert.Equal(t, qsoentry.ITUZ, m.entry.focus, "the first field in error has focus")
	body := bodyText(m)
	assert.Contains(t, body, `"99" is not an ITU zone`)
	assert.Contains(t, body, `"nope" is not an email address`)
}

func TestEntryPickersShowQSLStatusWords(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.QSLSent: "q", qsoentry.EQSLSent: "i"})

	assert.Equal(t, "Q", entryValue(m, qsoentry.QSLSent))
	body := bodyText(m)
	assert.Contains(t, body, "Queued")
	assert.Contains(t, body, "Ignored")
}

func TestEntryTabsFitNarrowWindows(t *testing.T) {
	for _, width := range []int{12, 20, 40, 60} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m := newEntryModel(t, newEntryStore(t, nil), WithStations([]qsoentry.Station{homeStation, parkStation}))
			m = fill(t, m, map[qsoentry.Field]string{
				qsoentry.ITUZ: "99", qsoentry.PropMode: "sat", qsoentry.QSLMsg: strings.Repeat("x", 80),
			})
			m = fill(t, m, map[qsoentry.Field]string{qsoentry.Notes: strings.Repeat("word ", 40)})
			m = press(t, m, saveKey)
			m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 30})

			for _, k := range []rune{'2', '3', '4', '5', '6'} {
				m = press(t, m, altKey(k))
				v := m.View().Content
				assert.Equal(t, 30, lipgloss.Height(v), "tab %c fills the window without wrapping", k)
				for i, line := range strings.Split(v, "\n") {
					assert.LessOrEqual(t, lipgloss.Width(line), width, "tab %c line %d: %q", k, i, plain(line))
				}
			}
		})
	}
}
