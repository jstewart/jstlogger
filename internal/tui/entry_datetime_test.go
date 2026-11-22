package tui

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

// retype types v in place of the value of field f.
func retype(t *testing.T, m Model, f qsoentry.Field, v string) Model {
	t.Helper()
	return fill(t, m, map[qsoentry.Field]string{f: v})
}

func TestEntryPostModeOpensOnToday(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	assert.Equal(t, "2026-11-10", entryValue(m, qsoentry.Date))
	assert.Empty(t, entryValue(m, qsoentry.TimeOn), "nobody entered a time on")
	assert.False(t, m.entry.unsaved(), "today's date is no unsaved change")

	m = press(t, m, esc())
	assert.False(t, m.entry.open, "esc closes without asking")
}

func TestEntryDateCarriesAfterSave(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newEntryModel(t, s)
	m = retype(t, m, qsoentry.Date, "y")
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "1430", qsoentry.Call: "W1AW"})
	m = press(t, m, saveKey)
	require.Len(t, s.added, 1)
	assert.Equal(t, "2026-11-09", entryValue(m, qsoentry.Date), "a paper log's date carries, as typed")
	assert.Equal(t, "14:30", entryValue(m, qsoentry.TimeOn), "and so does its time on")
	assert.False(t, m.entry.unsaved())
}

func TestEntryPostSaveKeepsTimeOnButNotTimeOff(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	s.expectAdd()
	m := newEntryModel(t, s)
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "1430", qsoentry.TimeOff: "1445", qsoentry.Call: "W1AW"})

	m = press(t, m, saveKey)

	require.Len(t, s.added, 1)
	assert.Equal(t, "14:30", entryValue(m, qsoentry.TimeOn))
	assert.Empty(t, entryValue(m, qsoentry.TimeOff), "a typed time off does not carry")
	assert.Contains(t, entryLine(m, "Time off"), "14:30", "time off copies the kept time on")
	assert.Equal(t, qsoentry.Call, m.entry.focus, "the next QSO starts at the call")

	m = typeText(t, m, "k1abc")
	m = press(t, m, saveKey)

	require.Len(t, s.added, 2)
	assert.Equal(t, time.Date(2026, 11, 10, 14, 30, 0, 0, time.UTC), s.added[1].StartUTC, "the next QSO saves with the kept time on")
}

func TestEntryEmptyDateIsTodayAfterLiveToggle(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))
	require.True(t, m.entry.live)
	m.entry.input(qsoentry.Date).SetValue("")
	m = press(t, m, liveKey)
	require.False(t, m.entry.live)
	assert.Equal(t, "2026-11-10", entryValue(m, qsoentry.Date), "post mode fills in an empty date")
}

func TestEntryOffFieldsCopyOnFields(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	assert.Contains(t, entryLine(m, "Date off"), "2026-11-10", "date off shows the date it copies")
	assert.Empty(t, entryValue(m, qsoentry.DateOff), "a copy is not typed")

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "1430"})
	assert.Contains(t, entryLine(m, "Time off"), "14:30", "time off copies the time on")
	m = retype(t, m, qsoentry.TimeOn, "1435")
	assert.Contains(t, entryLine(m, "Time off"), "14:35", "the copy follows the time on")
	assert.Empty(t, entryValue(m, qsoentry.TimeOff), "a copy is not typed")

	m = retype(t, m, qsoentry.Date, "y")
	assert.Contains(t, entryLine(m, "Date off"), "2026-11-09", "the copy follows the date")
}

func TestEntryTypingOffFieldStopsCopying(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "1430", qsoentry.TimeOff: "1445"})
	assert.Equal(t, "1445", entryValue(m, qsoentry.TimeOff), "typing replaces the copy")

	m = retype(t, m, qsoentry.TimeOn, "1500")
	assert.Equal(t, "14:45", entryValue(m, qsoentry.TimeOff), "a typed time off stays put")

	m = retype(t, m, qsoentry.TimeOff, "")
	assert.Empty(t, entryValue(m, qsoentry.TimeOff))
	assert.Contains(t, entryLine(m, "Time off"), "15:00", "clearing starts the copying again")
}

func TestEntrySavesCopiedOffFields(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newEntryModel(t, s)
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "1430", qsoentry.Call: "W1AW"})
	press(t, m, saveKey)
	require.Len(t, s.added, 1)
	q := s.added[0]
	assert.Equal(t, time.Date(2026, 11, 10, 14, 30, 0, 0, time.UTC), q.StartUTC)
	require.True(t, q.EndUTC.Valid, "the time off copies the time on")
	assert.Equal(t, q.StartUTC, q.EndUTC.V)
}

func TestEntryOffFieldsCrossMidnight(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newEntryModel(t, s)
	m = retype(t, m, qsoentry.Date, "-1")
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "2350", qsoentry.TimeOff: "0010", qsoentry.Call: "W1AW"})
	assert.Contains(t, entryLine(m, "Date off"), "2026-11-10", "the date off moves to the next day")

	press(t, m, saveKey)
	require.Len(t, s.added, 1)
	q := s.added[0]
	assert.Equal(t, time.Date(2026, 11, 9, 23, 50, 0, 0, time.UTC), q.StartUTC)
	assert.Equal(t, time.Date(2026, 11, 10, 0, 10, 0, 0, time.UTC), q.EndUTC.V)
}

func TestEntryCopiesDontAskBeforeDiscarding(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "1430"})
	m = retype(t, m, qsoentry.TimeOn, "")
	assert.Contains(t, entryLine(m, "Date off"), "2026-11-10")
	m = press(t, m, esc())
	assert.False(t, m.entry.open, "copied values are no unsaved change")
}

func TestEntryRewritesDatesAndTimesOnLeavingThem(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = retype(t, m, qsoentry.Date, "t")
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.TimeOn: "14", qsoentry.TimeOff: "1445", qsoentry.DateOff: "1110"})
	m = press(t, m, tab())
	assert.Equal(t, "2026-11-10", entryValue(m, qsoentry.Date))
	assert.Equal(t, "14:00", entryValue(m, qsoentry.TimeOn))
	assert.Equal(t, "2026-11-10", entryValue(m, qsoentry.DateOff))
	assert.Equal(t, "14:45", entryValue(m, qsoentry.TimeOff))

	m = retype(t, m, qsoentry.TimeOn, "99")
	m = press(t, m, tab())
	assert.Equal(t, "99", entryValue(m, qsoentry.TimeOn), "a value that doesn't parse is left for its error")

	m = newEntryModel(t, newEntryStore(t, nil))
	m = retype(t, m, qsoentry.Date, "t")
	m = press(t, m, esc())
	assert.False(t, m.entry.open, "t is today, no change from the date it opened on")
}

func TestEntryPlaceholdersShowShortForms(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = retype(t, m, qsoentry.Date, "")
	assert.Contains(t, entryLine(m, "Date"), "t y -N")
	assert.Contains(t, entryLine(m, "Time on"), "n HH")
}

func TestEntryEditDoesntCopyOrDefault(t *testing.T) {
	q := editedQSO(t)
	q.EndUTC = sql.Null[time.Time]{}
	m := openEditModel(t, newEditStore(t, q))
	assert.Empty(t, entryValue(m, qsoentry.DateOff))
	assert.Empty(t, entryValue(m, qsoentry.TimeOff))
	assert.NotContains(t, entryLine(m, "Time off"), q.StartUTC.Format("15:04"), "an edit shows no copy")
}

func TestEntryLiveModeKeepsItsClock(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))
	require.True(t, m.entry.live)
	assert.Equal(t, "18:30:00", entryValue(m, qsoentry.TimeOn))
	assert.Empty(t, entryValue(m, qsoentry.TimeOff))
	assert.NotContains(t, entryLine(m, "Time off"), "18:30", "live mode shows no copy")
}
