package tui

import (
	"context"
	"database/sql"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
	"jstlogger/internal/rig"
)

// editKey opens the QSO under the cursor in the form.
var editKey = keyPress('e', 0)

// editedID is the ID of the QSO the edit tests edit.
const editedID = 7

// editForm is the form of the QSO the edit tests edit, with every field
// filled in. Its station, club, is not among the profiles the tests offer.
func editForm() qsoentry.Form {
	var f qsoentry.Form
	for k, v := range map[qsoentry.Field]string{
		qsoentry.Date: "2026-10-01", qsoentry.TimeOn: "14:05:00",
		qsoentry.DateOff: "2026-10-01", qsoentry.TimeOff: "14:10:00",
		qsoentry.Call: "K1ABC", qsoentry.Mode: "MFSK", qsoentry.Submode: "FT4",
		qsoentry.Freq: "14.08", qsoentry.Band: "20m", qsoentry.FreqRX: "14.081", qsoentry.BandRX: "20m",
		qsoentry.RSTSent: "-10", qsoentry.RSTRcvd: "-12",
		qsoentry.Name: "Ann", qsoentry.QTH: "Boston", qsoentry.Gridsquare: "FN42",
		qsoentry.Comment: "hi", qsoentry.Tags: "contest, pota",

		qsoentry.StationName: "club", qsoentry.StationCallsign: "K1CLB", qsoentry.Operator: "K1XX",
		qsoentry.MyRig: "IC-7300", qsoentry.TxPwr: "100", qsoentry.MyGridsquare: "FN31PR",
		qsoentry.MyPOTARef: "US-0001", qsoentry.MySOTARef: "W1/HA-001", qsoentry.MyIOTA: "NA-001",
		qsoentry.MyWWFFRef: "KFF-1234", qsoentry.MyState: "CT", qsoentry.MyCity: "Newington",

		qsoentry.DXCC: "291", qsoentry.Cont: "NA", qsoentry.Region: "NONE", qsoentry.CQZ: "5",
		qsoentry.ITUZ: "8", qsoentry.State: "MA", qsoentry.Cnty: "MA,Suffolk", qsoentry.IOTA: "NA-046",
		qsoentry.SOTARef: "W1/MA-001", qsoentry.WWFFRef: "KFF-0001", qsoentry.POTARef: "US-1234",
		qsoentry.Sig: "WAB", qsoentry.SigInfo: "TQ38", qsoentry.DarcDok: "B36",
		qsoentry.Email: "ann@example.com", qsoentry.PropMode: "SAT", qsoentry.AntPath: "S",

		qsoentry.SatName: "AO-91", qsoentry.SatMode: "U/V",

		qsoentry.Notes: "first line\nsecond line",

		qsoentry.QSLSent: "Q", qsoentry.QSLSentVia: "B", qsoentry.QSLVia: "via K1MGR",
		qsoentry.QSLMsg: "TNX QSO", qsoentry.LoTWSent: "Y", qsoentry.EQSLSent: "R", qsoentry.ClubLogSent: "N",
	} {
		f[k] = v
	}
	return f
}

// editedQSO returns the QSO editForm builds as the logbook stores it, with
// what the form does not show added: an ID, UUID, country and distance, a
// contest ID and received LoTW and card QSLs.
func editedQSO(t *testing.T) logbook.QSO {
	t.Helper()
	q, err := qsoentry.Build(editForm(), entryNow)
	require.NoError(t, err)
	q.ID, q.UUID = editedID, "6f1c2d3e-0000-4000-8000-000000000007"
	q.Country = "United States"
	q.DistanceKm = sql.Null[float64]{V: 123.4, Valid: true}
	q.Fields["CONTEST_ID"] = "ARRL-DX-CW"
	rcvd := sql.Null[time.Time]{V: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), Valid: true}
	for i, c := range q.Confirmations {
		if c.Service == logbook.ServiceLoTW || c.Service == logbook.ServiceCard {
			q.Confirmations[i].Rcvd, q.Confirmations[i].RcvdDate = "Y", rcvd
		}
	}
	return q
}

// cloneEdited returns a deep copy of q, so the store's copy and the test's
// never share maps or slices.
func cloneEdited(q logbook.QSO) logbook.QSO {
	q.Fields = maps.Clone(q.Fields)
	q.Confirmations = slices.Clone(q.Confirmations)
	q.Tags = slices.Clone(q.Tags)
	return q
}

// editStore is a mock logbook holding the edited QSO, oldest, after two
// others. Its Get returns a stored QSO and its Update stores the QSO it is
// given, keeping the QSOs newest first.
type editStore struct {
	*entryStore
	// updated holds the QSOs passed to Update.
	updated []logbook.QSO
}

// newEditStore returns an editStore holding q and two newer QSOs.
func newEditStore(t *testing.T, q logbook.QSO) *editStore {
	t.Helper()
	s := &editStore{entryStore: newEntryStore(t, nil, append(testQSOs(3)[:2], cloneEdited(q))...)}
	s.expectGet()
	s.expectUpdate()
	return s
}

// expectGet makes Get return the stored QSO with the ID given.
func (s *editStore) expectGet() {
	s.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id int64) (logbook.QSO, error) {
		i := slices.IndexFunc(s.qsos, func(q logbook.QSO) bool { return q.ID == id })
		if i < 0 {
			return logbook.QSO{}, logbook.ErrNotFound
		}
		return cloneEdited(s.qsos[i]), nil
	}).AnyTimes()
}

// expectUpdate makes Update replace the stored QSO with the ID of the one
// it is given, then sort the QSOs newest first.
func (s *editStore) expectUpdate() {
	s.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, q logbook.QSO) error {
		s.updated = append(s.updated, cloneEdited(q))
		i := slices.IndexFunc(s.qsos, func(o logbook.QSO) bool { return o.ID == q.ID })
		if i < 0 {
			return logbook.ErrNotFound
		}
		s.qsos[i] = cloneEdited(q)
		slices.SortStableFunc(s.qsos, func(a, b logbook.QSO) int { return b.StartUTC.Compare(a.StartUTC) })
		return nil
	}).AnyTimes()
}

// runEdit runs cmd as runIndex does, also feeding back the edit loads and
// the pager's QSO fetches.
func runEdit(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case indexLoadedMsg, indexBlockMsg, indexLimitMsg, indexMarksMsg, indexSearchMsg, indexBulkMsg,
			entryTagsMsg, entrySavedMsg, entryLoTWMsg, editLoadedMsg, pagerQSOMsg:
			var next tea.Cmd
			m, next = update(t, m, msg)
			queue = append(queue, next)
		}
	}
	return m
}

// pressEdit sends each key to m and runs the commands that follow with
// runEdit.
func pressEdit(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = update(t, m, k)
		m = runEdit(t, m, cmd)
	}
	return m
}

// newEditIndexModel returns a model on s in a 100 by 30 window, showing the
// index with the cursor on the edited QSO, the oldest.
func newEditIndexModel(t *testing.T, s logbook.Store, opts ...Option) Model {
	t.Helper()
	m := New(append([]Option{WithLogbook(s)}, opts...)...)
	m.now = func() time.Time { return entryNow }
	m.tick = noTick
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runIndex(t, m, cmd)
	m = press(t, m, special(tea.KeyEnd))
	q, ok := m.idx.current()
	require.True(t, ok)
	require.EqualValues(t, editedID, q.ID)
	return m
}

// openEditModel returns a model on s with the edited QSO open in the form,
// opened from the index.
func openEditModel(t *testing.T, s logbook.Store, opts ...Option) Model {
	t.Helper()
	m := pressEdit(t, newEditIndexModel(t, s, opts...), editKey)
	require.True(t, m.entry.open)
	return m
}

// homeStations are two station profiles, neither of them the edited QSO's.
var homeStations = []qsoentry.Station{
	{Name: "home", Callsign: "W1AW", Operator: "W1AW", Grid: "FN31"},
	{Name: "portable", Callsign: "W1AW/P", Operator: "W1AW", Grid: "FN32"},
}

// editAppend moves to field f and types text after its value.
func editAppend(t *testing.T, m Model, f qsoentry.Field, text string) Model {
	t.Helper()
	m = focusOn(t, m, f)
	return typeText(t, m, text)
}

func TestEditOpensFromIndex(t *testing.T) {
	stored := editedQSO(t)
	m := openEditModel(t, newEditStore(t, stored), WithStations(homeStations))

	assert.False(t, m.entry.live, "an edit opens in post mode")
	assert.Equal(t, qsoentry.Load(stored), m.entry.form(), "every tab is filled from the stored QSO")
	for f, want := range map[qsoentry.Field]string{
		qsoentry.Call: "K1ABC", qsoentry.Mode: "MFSK", qsoentry.Freq: "14.08",
		qsoentry.StationName: "club", qsoentry.StationCallsign: "K1CLB",
		qsoentry.SigInfo: "TQ38", qsoentry.SatName: "AO-91",
		qsoentry.Notes:   "first line\nsecond line",
		qsoentry.QSLSent: "Q", qsoentry.LoTWSent: "Y", qsoentry.ClubLogSent: "N",
	} {
		assert.Equal(t, want, entryValue(m, f), "%s", f)
	}
	assert.False(t, m.entry.unsaved(), "nothing has changed yet")
	assert.Contains(t, bodyText(m), "Edit QSO")
	assert.NotContains(t, bodyText(m), "New QSO")
}

func TestEditOpensFromPager(t *testing.T) {
	stored := editedQSO(t)
	m := newEditIndexModel(t, newEditStore(t, stored))
	m = pressEdit(t, m, enter())
	require.True(t, m.pager.open)

	m = pressEdit(t, m, editKey)

	require.True(t, m.entry.open)
	assert.False(t, m.entry.live)
	assert.Equal(t, qsoentry.Load(stored), m.entry.form())
}

func TestEditKeyIsInHelp(t *testing.T) {
	m := newEditIndexModel(t, newEditStore(t, editedQSO(t)))
	assert.Regexp(t, `\be +edit\b`, helpText(m))

	m = pressEdit(t, m, enter())
	assert.Regexp(t, `\be +edit\b`, helpText(m))
}

func TestEditPagerLookupStillWorks(t *testing.T) {
	var opened []string
	open := func(_ context.Context, url string) error {
		opened = append(opened, url)
		return nil
	}
	m := newEditIndexModel(t, newEditStore(t, editedQSO(t)), WithURLOpener(open))
	m = pressEdit(t, m, enter())

	m, _ = update(t, m, keyPress('g', 0))
	m, cmd := update(t, m, editKey)
	require.NotNil(t, cmd)
	cmd()

	assert.False(t, m.entry.open, "ge is a lookup, not an edit")
	assert.Len(t, opened, 1)
}

func TestEditNoChangeSaveKeepsTheQSO(t *testing.T) {
	stored := editedQSO(t)
	s := newEditStore(t, stored)
	m := openEditModel(t, s, WithStations(homeStations))

	pressEdit(t, m, saveKey)

	require.Len(t, s.updated, 1)
	assert.Equal(t, stored, s.updated[0])
	assert.Equal(t, "ARRL-DX-CW", s.updated[0].Fields["CONTEST_ID"], "an unshown field is kept")
	i := slices.IndexFunc(s.updated[0].Confirmations, func(c logbook.Confirmation) bool { return c.Service == logbook.ServiceLoTW })
	require.GreaterOrEqual(t, i, 0)
	assert.Equal(t, "Y", s.updated[0].Confirmations[i].Rcvd, "a received QSL is kept")
	assert.Empty(t, s.added, "an edit never adds a QSO")
}

func TestEditEachTab(t *testing.T) {
	tests := []struct {
		name  string
		field qsoentry.Field
		text  string
		check func(t *testing.T, q logbook.QSO)
	}{
		{"QSO", qsoentry.Name, "e", func(t *testing.T, q logbook.QSO) {
			assert.Equal(t, "Anne", q.Name)
		}},
		{"Station", qsoentry.MyCity, "x", func(t *testing.T, q logbook.QSO) {
			assert.Equal(t, "Newingtonx", q.Fields[qsoentry.MyCity.String()])
		}},
		{"General", qsoentry.SigInfo, "9", func(t *testing.T, q logbook.QSO) {
			assert.Equal(t, "TQ389", q.Fields[qsoentry.SigInfo.String()])
		}},
		{"Satellite", qsoentry.SatMode, "U", func(t *testing.T, q logbook.QSO) {
			assert.Contains(t, strings.ToUpper(q.Fields[qsoentry.SatMode.String()]), "U/VU")
		}},
		{"Notes", qsoentry.Notes, "!", func(t *testing.T, q logbook.QSO) {
			assert.Contains(t, q.Fields[qsoentry.Notes.String()], "!")
		}},
		{"QSL", qsoentry.LoTWSent, "n", func(t *testing.T, q logbook.QSO) {
			i := slices.IndexFunc(q.Confirmations, func(c logbook.Confirmation) bool { return c.Service == logbook.ServiceLoTW })
			require.GreaterOrEqual(t, i, 0)
			assert.Equal(t, "N", q.Confirmations[i].Sent)
			assert.Equal(t, "Y", q.Confirmations[i].Rcvd, "the received status is kept")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newEditStore(t, editedQSO(t))
			m := openEditModel(t, s, WithStations(homeStations))
			m = editAppend(t, m, tt.field, tt.text)
			require.True(t, m.entry.unsaved())

			m = pressEdit(t, m, saveKey)

			require.Len(t, s.updated, 1)
			q := s.updated[0]
			assert.EqualValues(t, editedID, q.ID)
			assert.Equal(t, "ARRL-DX-CW", q.Fields["CONTEST_ID"])
			tt.check(t, q)
			assert.True(t, m.entry.open, "ctrl+s leaves the form open")
			assert.False(t, m.entry.unsaved(), "the saved QSO is the form's own")
			assert.Equal(t, qsoentry.Load(s.qsos[slices.IndexFunc(s.qsos, func(o logbook.QSO) bool { return o.ID == editedID })]), m.entry.form(),
				"the form shows the stored QSO, not the next QSO's carried values")
		})
	}
}

func TestEditEsc(t *testing.T) {
	m := openEditModel(t, newEditStore(t, editedQSO(t)))
	m = pressEdit(t, m, esc())
	assert.False(t, m.entry.open, "esc closes an unchanged form at once")
	assert.Equal(t, promptNone, m.prompt)

	m = pressEdit(t, m, editKey)
	m = editAppend(t, m, qsoentry.Name, "e")
	m = pressEdit(t, m, esc())
	require.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), "Discard changes to this QSO?")
	m = pressEdit(t, m, keyPress('n', 0))
	assert.True(t, m.entry.open)
	m = pressEdit(t, m, esc(), keyPress('y', 0))
	assert.False(t, m.entry.open)
}

func TestEditNeverAppliesTheRig(t *testing.T) {
	stored := editedQSO(t)
	s := newEditStore(t, stored)
	m := New(WithLogbook(s), WithRig("ic7300", &fakeRig{}))
	m.now = func() time.Time { return entryNow }
	m.tick = noTick
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runIndex(t, m, cmd)
	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Connected})
	m, _ = update(t, m, rigAt(7_030_000, "CW"))
	m = press(t, m, special(tea.KeyEnd))

	m = pressEdit(t, m, editKey)
	require.True(t, m.entry.open)
	assert.False(t, m.entry.live, "an edit opens in post mode with the rig connected")
	assertRigFields(t, m, "14.08", "20m", "MFSK", "FT4")

	m, _ = update(t, m, rigAt(3_573_000, "USB"))
	assertRigFields(t, m, "14.08", "20m", "MFSK", "FT4")

	m = pressEdit(t, m, keyPress('l', tea.ModCtrl))
	assert.False(t, m.entry.live, "an edit can't go live")
	m = pressEdit(t, m, syncRigKey)
	assertRigFields(t, m, "14.08", "20m", "MFSK", "FT4")
	assert.NotContains(t, helpText(m), "live/post")

	m = pressEdit(t, m, saveKey)
	require.Len(t, s.updated, 1)
	assert.Equal(t, stored, s.updated[0])
}

func TestEditShowsTheStoredStation(t *testing.T) {
	stored := editedQSO(t)
	s := newEditStore(t, stored)
	m := openEditModel(t, s, WithStations(homeStations))

	assert.Equal(t, "club", entryValue(m, qsoentry.StationName))
	assert.Equal(t, "K1CLB", entryValue(m, qsoentry.StationCallsign))
	m = press(t, m, altKey('2'))
	assert.Contains(t, bodyText(m), "club")

	m = focusOn(t, m, qsoentry.StationName)
	m = typeText(t, m, "p")
	assert.Equal(t, "portable", entryValue(m, qsoentry.StationName))
	assert.Equal(t, "W1AW/P", entryValue(m, qsoentry.StationCallsign), "picking a profile fills in its values")
	assert.Equal(t, "FN32", entryValue(m, qsoentry.MyGridsquare))

	pressEdit(t, m, saveKey)
	require.Len(t, s.updated, 1)
	assert.Equal(t, "portable", s.updated[0].Fields[logbook.StationField])
	assert.Equal(t, "W1AW/P", s.updated[0].StationCallsign)
}

func TestEditKeepsAnUnnamedStation(t *testing.T) {
	stored := editedQSO(t)
	delete(stored.Fields, logbook.StationField)
	s := newEditStore(t, stored)
	m := openEditModel(t, s, WithStations(homeStations[:1]))

	assert.Empty(t, entryValue(m, qsoentry.StationName), "no station name, not the profile's")
	assert.Equal(t, "K1CLB", entryValue(m, qsoentry.StationCallsign))
	assert.True(t, m.entry.shown(qsoentry.StationName), "the picker shows the stored station beside the profile")

	m = pressEdit(t, m, saveKey)
	require.Len(t, s.updated, 1)
	assert.Equal(t, stored, s.updated[0])
}

func TestNewQSOHidesTheStationPickerWithOneProfile(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil), WithStations(homeStations[:1]))
	assert.False(t, m.entry.shown(qsoentry.StationName))
}

func TestEditKeepsUnknownPickedValues(t *testing.T) {
	stored := editedQSO(t)
	stored.Mode, stored.Submode = "FOO", "BAR"
	stored.Fields[qsoentry.PropMode.String()] = "XYZ"
	m := openEditModel(t, newEditStore(t, stored))

	assert.Equal(t, "FOO", entryValue(m, qsoentry.Mode))
	assert.Equal(t, "BAR", entryValue(m, qsoentry.Submode))
	assert.Equal(t, "XYZ", entryValue(m, qsoentry.PropMode))
	assert.Equal(t, qsoentry.Load(stored), m.entry.form())
	assert.False(t, m.entry.unsaved())
}

func TestEditSaveKeepsTheQSOSelected(t *testing.T) {
	s := newEditStore(t, editedQSO(t))
	m := openEditModel(t, s)
	// A newer date moves the QSO from last to first.
	m = focusOn(t, m, qsoentry.Date)
	for range len("2026-10-01") {
		m = press(t, m, special(tea.KeyBackspace))
	}
	m = typeText(t, m, "2026-11-09")
	m = focusOn(t, m, qsoentry.DateOff)
	for range len("2026-10-01") {
		m = press(t, m, special(tea.KeyBackspace))
	}
	m = typeText(t, m, "2026-11-09")

	m = pressEdit(t, m, saveCloseKey)

	require.Len(t, s.updated, 1)
	assert.False(t, m.entry.open)
	q, ok := m.idx.current()
	require.True(t, ok)
	assert.EqualValues(t, editedID, q.ID, "the edited QSO stays selected")
	assert.Equal(t, 0, m.idx.cursor, "where it moved to")
	assert.Contains(t, statusLine(t, m), "K1ABC")
}

func TestEditFromPagerShowsTheUpdatedQSO(t *testing.T) {
	s := newEditStore(t, editedQSO(t))
	m := newEditIndexModel(t, s)
	m = pressEdit(t, m, enter())
	m = pressEdit(t, m, editKey)
	m = editAppend(t, m, qsoentry.Name, "e")

	m = pressEdit(t, m, saveKey)
	require.True(t, m.entry.open)
	assert.EqualValues(t, editedID, m.pager.fetched)
	assert.Equal(t, "Anne", m.pager.qso.Name, "the pager shows the saved QSO")

	m = pressEdit(t, m, esc())
	assert.False(t, m.entry.open)
	assert.True(t, m.pager.open, "closing returns to the pager")
	assert.Contains(t, bodyText(m), "Anne")
}

func TestEditSaveAndCloseReturnsToPager(t *testing.T) {
	s := newEditStore(t, editedQSO(t))
	m := newEditIndexModel(t, s)
	m = pressEdit(t, m, enter())
	m = pressEdit(t, m, editKey)
	m = editAppend(t, m, qsoentry.Name, "e")

	m = pressEdit(t, m, saveCloseKey)

	assert.False(t, m.entry.open)
	assert.True(t, m.pager.open)
	q, ok := m.idx.current()
	require.True(t, ok)
	assert.EqualValues(t, editedID, q.ID)
	assert.Equal(t, "Anne", q.Name)
	assert.Equal(t, "Anne", m.pager.qso.Name)
}

func TestEditLoadErrorIsShown(t *testing.T) {
	s := &editStore{entryStore: newEntryStore(t, nil, append(testQSOs(3)[:2], editedQSO(t))...)}
	s.EXPECT().Get(gomock.Any(), gomock.Any()).Return(logbook.QSO{}, assert.AnError).AnyTimes()
	m := newEditIndexModel(t, s)

	m = pressEdit(t, m, editKey)

	assert.False(t, m.entry.open)
	assert.Contains(t, statusLine(t, m), assert.AnError.Error())
}

func TestEditUpdateErrorIsShown(t *testing.T) {
	s := &editStore{entryStore: newEntryStore(t, nil, append(testQSOs(3)[:2], editedQSO(t))...)}
	s.expectGet()
	s.EXPECT().Update(gomock.Any(), gomock.Any()).Return(assert.AnError)
	m := openEditModel(t, s)
	m = editAppend(t, m, qsoentry.Name, "e")

	m = pressEdit(t, m, saveCloseKey)

	assert.True(t, m.entry.open, "the form stays open")
	assert.Equal(t, "Anne", entryValue(m, qsoentry.Name))
	assert.Contains(t, bodyText(m), assert.AnError.Error())
}
