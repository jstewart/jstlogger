package tui

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/logbook"
	"jstlogger/internal/logbook/mock"
	"jstlogger/internal/qsoentry"
)

// entryNow is the current time in the New QSO tests.
var entryNow = time.Date(2026, 11, 10, 18, 30, 0, 0, time.UTC)

var (
	newQSOKey    = keyPress('a', 0)
	saveKey      = keyPress('s', tea.ModCtrl)
	saveCloseKey = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}
	nextTabKey   = tea.KeyPressMsg{Code: tea.KeyPgDown, Mod: tea.ModCtrl}
	prevTabKey   = tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModCtrl}
)

func altKey(r rune) tea.KeyPressMsg { return keyPress(r, tea.ModAlt) }

// entryStore is a mock logbook that keeps the QSOs added to it, newest
// first, so the index can show them.
type entryStore struct {
	*mock.MockStore
	qsos []logbook.QSO
	// added holds the QSOs passed to Add.
	added []logbook.QSO
}

// newEntryStore returns a mock logbook of the given QSOs, newest first,
// offering tags for completion. Its Add stores the QSO and gives it the
// next ID.
func newEntryStore(t *testing.T, tags []string, qsos ...logbook.QSO) *entryStore {
	t.Helper()
	s := &entryStore{MockStore: mock.NewMockStore(gomock.NewController(t)), qsos: qsos}
	e := s.EXPECT()
	e.Count(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, logbook.Expr) (int, error) {
		return len(s.qsos), nil
	}).AnyTimes()
	e.Index(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ logbook.Expr, offset, limit int) ([]logbook.QSO, error) {
			return slices.Clone(s.qsos[min(offset, len(s.qsos)):min(offset+limit, len(s.qsos))]), nil
		}).AnyTimes()
	e.IndexOf(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ logbook.Expr, id int64) (int, error) {
			if i := slices.IndexFunc(s.qsos, func(q logbook.QSO) bool { return q.ID == id }); i >= 0 {
				return i, nil
			}
			return 0, logbook.ErrNotFound
		}).AnyTimes()
	var ts []logbook.Tag
	for i, name := range tags {
		ts = append(ts, logbook.Tag{ID: int64(i + 1), Name: name})
	}
	e.Tags(gomock.Any()).Return(ts, nil).AnyTimes()
	return s
}

// expectAdd makes Add store each QSO it is given as the newest.
func (s *entryStore) expectAdd() {
	s.EXPECT().Add(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, q logbook.QSO) (logbook.QSO, error) {
		s.added = append(s.added, q)
		q.ID = int64(len(s.qsos) + 1)
		s.qsos = slices.Insert(s.qsos, 0, q)
		return q, nil
	}).AnyTimes()
}

// newEntryModel returns a model on s in a 100 by 30 window with the New QSO
// screen open.
func newEntryModel(t *testing.T, s logbook.Store, opts ...Option) Model {
	t.Helper()
	m := New(append([]Option{WithLogbook(s)}, opts...)...)
	m.now = func() time.Time { return entryNow }
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runIndex(t, m, cmd)
	m = press(t, m, newQSOKey)
	require.True(t, m.entry.open)
	return m
}

// focusOn presses tab until field f has focus.
func focusOn(t *testing.T, m Model, f qsoentry.Field) Model {
	t.Helper()
	for range qsoentry.FieldCount {
		if m.entry.focus == f {
			return m
		}
		m = press(t, m, tab())
	}
	require.Equal(t, f, m.entry.focus)
	return m
}

// fill types each value into its field, in field order.
func fill(t *testing.T, m Model, values map[qsoentry.Field]string) Model {
	t.Helper()
	for f := range qsoentry.FieldCount {
		v, ok := values[f]
		if !ok {
			continue
		}
		m = focusOn(t, m, f)
		m = typeText(t, m, v)
	}
	return m
}

// entryValue returns the text of field f, or the picked mode or submode.
func entryValue(m Model, f qsoentry.Field) string {
	return m.entry.form()[f]
}

// bodyText returns the plain text of the body.
func bodyText(m Model) string {
	return plain(m.View().Content)
}

func TestEntryOpensFromIndex(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))

	body := bodyText(m)
	for _, name := range []string{"QSO", "Station", "General", "Notes", "QSL"} {
		assert.Contains(t, body, " "+name+" ", "the tab bar lists %s", name)
	}
	assert.NotContains(t, body, "Satellite", "the satellite tab needs PROP_MODE=SAT")
	for _, label := range []string{"Date", "Time on", "Call", "Mode", "Submode", "Freq", "Band", "RST sent", "RST rcvd", "Name", "QTH", "Grid", "Comment", "Tags"} {
		assert.Contains(t, body, label)
	}
	assert.Equal(t, entryTabQSO, m.entry.tab)
	assert.Equal(t, qsoentry.Date, m.entry.focus, "post mode starts at the date")
	assert.Contains(t, plain(m.footerView()), "ctrl+s save")
}

func TestEntryOpensOnAnEmptyLogbook(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	assert.Equal(t, 0, m.idx.total)
	assert.True(t, m.entry.open)
}

func TestEntryFieldNavigation(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))

	m = press(t, m, tab())
	assert.Equal(t, qsoentry.TimeOn, m.entry.focus)
	m = press(t, m, enter())
	assert.Equal(t, qsoentry.DateOff, m.entry.focus, "enter moves on too")
	m = press(t, m, shiftTab(), shiftTab())
	assert.Equal(t, qsoentry.Date, m.entry.focus)
	m = press(t, m, shiftTab())
	assert.Equal(t, qsoentry.Tags, m.entry.focus, "shift+tab wraps to the last field")
	m = press(t, m, tab())
	assert.Equal(t, qsoentry.Date, m.entry.focus, "tab wraps to the first field")
}

func TestEntryTabs(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))

	m = press(t, m, altKey('3'))
	assert.Equal(t, entryTabGeneral, m.entry.tab)
	assert.Contains(t, bodyText(m), "not available yet")
	m = press(t, m, nextTabKey)
	assert.Equal(t, entryTabNotes, m.entry.tab)
	m = press(t, m, nextTabKey, nextTabKey)
	assert.Equal(t, entryTabQSO, m.entry.tab, "ctrl+pgdn wraps")
	m = press(t, m, prevTabKey)
	assert.Equal(t, entryTabQSL, m.entry.tab, "ctrl+pgup wraps")
	m = press(t, m, altKey('9'))
	assert.Equal(t, entryTabQSL, m.entry.tab, "there is no ninth tab")

	m = press(t, m, tab())
	assert.Equal(t, qsoentry.Date, m.entry.focus, "fields move only on the QSO tab")
	m = press(t, m, altKey('1'))
	assert.Equal(t, entryTabQSO, m.entry.tab)
}

func TestEntryTypingIsNotAGlobalKey(t *testing.T) {
	r := &fakeRig{}
	m := newEntryModel(t, newEntryStore(t, nil), WithRig("test", r))
	m = focusOn(t, m, qsoentry.Call)

	m = typeText(t, m, "qf?m")

	assert.Equal(t, "qf?m", entryValue(m, qsoentry.Call))
	assert.Equal(t, promptNone, m.prompt, "f and m don't open the rig prompts")
	assert.False(t, m.help.ShowAll, "? doesn't open help")
}

func TestEntryCtrlCQuits(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestEntryModeFiltersSubmodes(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))

	m = focusOn(t, m, qsoentry.Mode)
	m = typeText(t, m, "mfsk")
	assert.Equal(t, "MFSK", entryValue(m, qsoentry.Mode))
	m = press(t, m, tab())
	m = typeText(t, m, "ft4")
	assert.Equal(t, "FT4", entryValue(m, qsoentry.Submode))

	m = typeText(t, m, "usb")
	assert.Equal(t, "FT4", entryValue(m, qsoentry.Submode), "USB is not an MFSK submode")

	m = press(t, m, shiftTab())
	m = typeText(t, m, "ssb")
	assert.Equal(t, "SSB", entryValue(m, qsoentry.Mode))
	assert.Empty(t, entryValue(m, qsoentry.Submode), "FT4 is not an SSB submode")
	m = press(t, m, tab())
	m = typeText(t, m, "u")
	assert.Equal(t, "USB", entryValue(m, qsoentry.Submode))
}

func TestEntryDefaultRST(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))

	m = focusOn(t, m, qsoentry.Mode)
	m = typeText(t, m, "cw")
	assert.Equal(t, "599", entryValue(m, qsoentry.RSTSent))
	assert.Equal(t, "599", entryValue(m, qsoentry.RSTRcvd))

	m = focusOn(t, m, qsoentry.RSTSent)
	m = press(t, m, special(tea.KeyBackspace))
	m = typeText(t, m, "8")
	m = focusOn(t, m, qsoentry.Mode)
	m = typeText(t, m, "ft8")

	assert.Equal(t, "598", entryValue(m, qsoentry.RSTSent), "a typed report is kept")
	assert.Equal(t, "-10", entryValue(m, qsoentry.RSTRcvd))

	m = typeText(t, m, "ssb")
	assert.Equal(t, "59", entryValue(m, qsoentry.RSTRcvd))
	m = press(t, m, special(tea.KeyHome))
	assert.Empty(t, entryValue(m, qsoentry.RSTRcvd), "no mode, no report")
}

func TestEntryFreqFillsBand(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Freq: "14074"})
	assert.Equal(t, "20m", entryValue(m, qsoentry.Band))
	m = typeText(t, m, "0")
	assert.Empty(t, entryValue(m, qsoentry.Band), "140740 kHz is in no band")
	m = press(t, m, special(tea.KeyBackspace))
	assert.Equal(t, "20m", entryValue(m, qsoentry.Band))

	m = focusOn(t, m, qsoentry.Band)
	m = press(t, m, special(tea.KeyBackspace), special(tea.KeyBackspace), special(tea.KeyBackspace))
	m = typeText(t, m, "17m")
	m = focusOn(t, m, qsoentry.Freq)
	m = typeText(t, m, "1")
	assert.Equal(t, "17m", entryValue(m, qsoentry.Band), "a typed band is kept")

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.FreqRX: "7.074"})
	assert.Equal(t, "40m", entryValue(m, qsoentry.BandRX))
}

func TestEntryShowsBearingAndDistance(t *testing.T) {
	st := qsoentry.Station{Callsign: "W1AW", Grid: "FN42"}
	m := newEntryModel(t, newEntryStore(t, nil), WithStation(st))

	assert.NotContains(t, bodyText(m), "°")
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Gridsquare: "IO91"})
	assert.Contains(t, bodyText(m), "53° · 3227 mi")

	m = newEntryModel(t, newEntryStore(t, nil), WithStation(st), WithIndexFormat(defaultIndexFormat, "km"))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Gridsquare: "IO91"})
	assert.Contains(t, bodyText(m), "53° · 5194 km")
}

func TestEntrySaveShowsErrorsInline(t *testing.T) {
	s := newEntryStore(t, nil)
	m := newEntryModel(t, s)
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Gridsquare: "ZZ99"})

	m = press(t, m, saveKey)

	body := bodyText(m)
	assert.Contains(t, body, "date is required")
	assert.Contains(t, body, "call is required")
	assert.Contains(t, body, `"ZZ99" is not a Maidenhead locator`)
	assert.Equal(t, qsoentry.Date, m.entry.focus, "the first field in error has focus")
	assert.Empty(t, s.added, "nothing is saved")

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "K1ABC"})
	assert.NotContains(t, bodyText(m), "call is required", "an error clears once fixed")
	assert.Contains(t, bodyText(m), "date is required")
}

// paperQSO is a QSO typed in from a paper log.
var paperQSO = map[qsoentry.Field]string{
	qsoentry.Date:       "2026-11-09",
	qsoentry.TimeOn:     "1405",
	qsoentry.Call:       "k1abc",
	qsoentry.Mode:       "cw",
	qsoentry.Freq:       "7030",
	qsoentry.Name:       "Ann",
	qsoentry.Gridsquare: "fn42",
	qsoentry.Comment:    "first QSO",
	qsoentry.Tags:       "pota",
}

func TestEntrySave(t *testing.T) {
	s := newEntryStore(t, nil, testQSOs(2)...)
	s.expectAdd()
	st := qsoentry.Station{Callsign: "W1AW", Operator: "K1XX", Grid: "FN31"}
	m := newEntryModel(t, s, WithStation(st))
	m = fill(t, m, paperQSO)

	m = press(t, m, saveKey)

	require.Len(t, s.added, 1)
	q := s.added[0]
	assert.Equal(t, time.Date(2026, 11, 9, 14, 5, 0, 0, time.UTC), q.StartUTC)
	assert.Equal(t, "K1ABC", q.Call)
	assert.Equal(t, "CW", q.Mode)
	assert.Equal(t, sql.Null[int64]{V: 7_030_000, Valid: true}, q.FreqHz)
	assert.Equal(t, "40m", q.Band)
	assert.Equal(t, "599", q.RSTSent)
	assert.Equal(t, "599", q.RSTRcvd)
	assert.Equal(t, "Ann", q.Name)
	assert.Equal(t, "FN42", q.Gridsquare)
	assert.Equal(t, "first QSO", q.Comment)
	assert.Equal(t, []string{"pota"}, q.Tags)
	assert.Equal(t, "W1AW", q.StationCallsign)
	assert.Equal(t, "K1XX", q.Operator)
	assert.Equal(t, "FN31", q.MyGridsquare)

	assert.True(t, m.entry.open, "ctrl+s leaves the screen open for the next QSO")
	for f, want := range map[qsoentry.Field]string{
		qsoentry.Date: "2026-11-09", qsoentry.Mode: "CW", qsoentry.Freq: "7030", qsoentry.Band: "40m",
		qsoentry.RSTSent: "599", qsoentry.RSTRcvd: "599",
		qsoentry.TimeOn: "", qsoentry.Call: "", qsoentry.Name: "", qsoentry.Gridsquare: "",
		qsoentry.Comment: "", qsoentry.Tags: "",
	} {
		assert.Equal(t, want, entryValue(m, f), "%s after saving", f)
	}
	assert.Equal(t, qsoentry.TimeOn, m.entry.focus, "the next QSO starts at its time")
	assert.Contains(t, bodyText(m), "Saved QSO with K1ABC.")
	assert.Equal(t, 3, m.idx.total, "the index reloads")

	// The carried-over values are not unsaved changes.
	m = press(t, m, esc())
	assert.False(t, m.entry.open)
}

func TestEntrySaveAndClose(t *testing.T) {
	s := newEntryStore(t, nil, testQSOs(5)...)
	s.expectAdd()
	m := newEntryModel(t, s)
	// Close the screen to move to the oldest QSO, then open it again.
	m = press(t, m, esc(), special(tea.KeyEnd), newQSOKey)
	require.Equal(t, 4, m.idx.cursor)
	m = fill(t, m, paperQSO)

	m = press(t, m, saveCloseKey)

	require.Len(t, s.added, 1)
	assert.False(t, m.entry.open)
	assert.Equal(t, 6, m.idx.total)
	q, ok := m.idx.current()
	require.True(t, ok)
	assert.Equal(t, "K1ABC", q.Call, "the new QSO is selected")
	assert.Contains(t, statusLine(t, m), "Saved QSO with K1ABC.")
}

func TestEntrySaveError(t *testing.T) {
	s := newEntryStore(t, nil)
	s.EXPECT().Add(gomock.Any(), gomock.Any()).Return(logbook.QSO{}, assert.AnError)
	m := newEntryModel(t, s)
	m = fill(t, m, paperQSO)

	m = press(t, m, saveCloseKey)

	assert.True(t, m.entry.open, "the screen stays open")
	assert.Equal(t, "K1ABC", strings.ToUpper(entryValue(m, qsoentry.Call)), "the form is kept")
	assert.Contains(t, bodyText(m), assert.AnError.Error())
}

func TestEntryEscAsksAboutUnsavedChanges(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "K1ABC"})

	m = press(t, m, esc())
	require.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), "Discard the unsaved QSO?")
	m = press(t, m, keyPress('n', 0))
	assert.True(t, m.entry.open, "no keeps the screen open")
	assert.Equal(t, "K1ABC", entryValue(m, qsoentry.Call))

	m = press(t, m, esc(), keyPress('y', 0))
	assert.False(t, m.entry.open)

	m = press(t, m, newQSOKey)
	assert.Empty(t, entryValue(m, qsoentry.Call), "the screen opens on an empty form")
}

func TestEntryEscClosesUnchangedForm(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = press(t, m, esc())
	assert.False(t, m.entry.open)
	assert.Equal(t, promptNone, m.prompt)
}

func TestEntryCompletesTags(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, []string{"POTA", "contest", "portable"}))
	m = focusOn(t, m, qsoentry.Tags)
	right := special(tea.KeyRight)

	m = typeText(t, m, "co")
	m = press(t, m, right)
	assert.Equal(t, "contest", entryValue(m, qsoentry.Tags))

	m = typeText(t, m, ", po")
	m = press(t, m, right)
	assert.Equal(t, "contest, POTA", entryValue(m, qsoentry.Tags))

	m = typeText(t, m, ",c")
	m = press(t, m, right)
	assert.Equal(t, "contest, POTA,c", entryValue(m, qsoentry.Tags), "a tag already given is not offered")
}

func TestEntryPasteGoesToField(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)

	m, _ = update(t, m, tea.PasteMsg{Content: "K1ABC"})

	assert.Equal(t, "K1ABC", entryValue(m, qsoentry.Call))
}

func TestIndexHelpListsNewQSO(t *testing.T) {
	m, _ := newIndexModel(t, 3, 120, 24)
	assert.Contains(t, plain(m.footerView()), "a new QSO")
}

func TestEntryTagLoadErrorIsShown(t *testing.T) {
	s := &entryStore{MockStore: mock.NewMockStore(gomock.NewController(t))}
	e := s.EXPECT()
	e.Count(gomock.Any(), gomock.Any()).Return(0, nil).AnyTimes()
	e.Index(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	e.Tags(gomock.Any()).Return(nil, assert.AnError)

	m := newEntryModel(t, s)

	assert.Contains(t, bodyText(m), "load tags: "+assert.AnError.Error())
	assert.True(t, m.entry.open, "the form still works without completion")
}
