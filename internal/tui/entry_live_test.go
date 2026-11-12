package tui

import (
	"database/sql"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
	"jstlogger/internal/rig"
)

var (
	liveKey      = keyPress('l', tea.ModCtrl)
	resetTimeKey = keyPress('t', tea.ModCtrl)
)

// noTick stands in for tea.Tick in the New QSO tests, which send the clock
// ticks themselves.
func noTick(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }

// newLiveEntryModel returns a model on s in a 100 by 30 window with the rig
// connected and the New QSO screen open. msgs, such as the rig state, are
// sent before the screen opens.
func newLiveEntryModel(t *testing.T, s logbook.Store, msgs ...tea.Msg) Model {
	t.Helper()
	m := New(WithLogbook(s), WithRig("ic7300", &fakeRig{}))
	m.now = func() time.Time { return entryNow }
	m.tick = noTick
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runIndex(t, m, cmd)
	for _, msg := range append([]tea.Msg{rig.ConnectionChanged{Status: rig.Connected}}, msgs...) {
		m, _ = update(t, m, msg)
	}
	m = press(t, m, newQSOKey)
	require.True(t, m.entry.open)
	return m
}

// at sets the clock to d after entryNow and ticks the New QSO screen's
// clock, returning the next tick.
func at(t *testing.T, m Model, d time.Duration) (Model, tea.Cmd) {
	t.Helper()
	m.now = func() time.Time { return entryNow.Add(d) }
	return update(t, m, entryTickMsg{gen: m.entry.gen})
}

func TestEntryIsLiveWhenRigConnected(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))

	assert.True(t, m.entry.live)
	assert.Contains(t, bodyText(m), "New QSO · live")
	assert.Equal(t, qsoentry.Call, m.entry.focus, "live mode starts at the call")
	assert.Equal(t, "2026-11-10", entryValue(m, qsoentry.Date))
	assert.Equal(t, "18:30:00", entryValue(m, qsoentry.TimeOn))
	assert.Contains(t, plain(m.footerView()), "ctrl+l live/post")
}

func TestEntryIsPostWithoutRig(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil), WithRig("ic7300", &fakeRig{}))
	assert.False(t, m.entry.live, "a rig that isn't connected doesn't make the screen live")
	assert.Empty(t, entryValue(m, qsoentry.TimeOn))
}

func TestEntryOpenStartsTheClock(t *testing.T) {
	m := New(WithLogbook(newEntryStore(t, nil)))
	m.now = func() time.Time { return entryNow.Add(250 * time.Millisecond) }
	var ticks []time.Duration
	m.tick = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		ticks = append(ticks, d)
		return func() tea.Msg { return fn(time.Time{}) }
	}

	m, cmd := m.openEntry()

	require.Equal(t, []time.Duration{750 * time.Millisecond}, ticks, "the clock ticks on the second")
	var tick entryTickMsg
	for _, msg := range cmd().(tea.BatchMsg) {
		if msg == nil {
			continue
		}
		if got, ok := msg().(entryTickMsg); ok {
			tick = got
		}
	}
	assert.Equal(t, m.entry.gen, tick.gen)
}

func TestEntryLiveClockTicks(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))

	m, next := at(t, m, 5*time.Second)

	assert.Equal(t, "18:30:05", entryValue(m, qsoentry.TimeOn))
	assert.Nil(t, next, "noTick schedules nothing")

	m.now = func() time.Time { return entryNow.Add(time.Minute) }
	m, _ = update(t, m, entryTickMsg{gen: m.entry.gen - 1})
	assert.Equal(t, "18:30:05", entryValue(m, qsoentry.TimeOn), "a tick from an earlier opening is dropped")
}

func TestEntryLiveLocksTimeOnAtTheCall(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))
	m, _ = at(t, m, 5*time.Second)

	m = typeText(t, m, "k")
	m, _ = at(t, m, time.Minute)
	m = typeText(t, m, "1abc")

	assert.Equal(t, "18:30:05", entryValue(m, qsoentry.TimeOn), "time on locks at the first key in the call")

	m = press(t, m, special(tea.KeyBackspace), special(tea.KeyBackspace), special(tea.KeyBackspace),
		special(tea.KeyBackspace), special(tea.KeyBackspace))
	require.Empty(t, entryValue(m, qsoentry.Call))
	assert.Equal(t, "18:31:00", entryValue(m, qsoentry.TimeOn), "clearing the call starts the timing over")
	m, _ = at(t, m, 2*time.Minute)
	assert.Equal(t, "18:32:00", entryValue(m, qsoentry.TimeOn))
}

func TestEntryLiveResetTimeOn(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))
	m = typeText(t, m, "k1abc")
	m.now = func() time.Time { return entryNow.Add(5 * time.Minute) }

	m = press(t, m, resetTimeKey)

	assert.Equal(t, "18:35:00", entryValue(m, qsoentry.TimeOn))
	m, _ = at(t, m, 6*time.Minute)
	assert.Equal(t, "18:35:00", entryValue(m, qsoentry.TimeOn), "time on stays locked")
}

func TestEntryLiveSaveSetsTimeOff(t *testing.T) {
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newLiveEntryModel(t, s)
	m = typeText(t, m, "k1abc")
	m.now = func() time.Time { return entryNow.Add(12*time.Minute + 10*time.Second) }

	m = press(t, m, saveKey)

	require.Len(t, s.added, 1)
	q := s.added[0]
	assert.Equal(t, entryNow, q.StartUTC)
	assert.Equal(t, sql.Null[time.Time]{V: entryNow.Add(12*time.Minute + 10*time.Second), Valid: true}, q.EndUTC)

	assert.Empty(t, entryValue(m, qsoentry.Call))
	assert.Equal(t, "18:42:10", entryValue(m, qsoentry.TimeOn), "the next QSO's clock runs")
	assert.Empty(t, entryValue(m, qsoentry.TimeOff))
	assert.Equal(t, qsoentry.Call, m.entry.focus, "the next QSO starts at the call")
	m, _ = at(t, m, 13*time.Minute)
	assert.Equal(t, "18:43:00", entryValue(m, qsoentry.TimeOn))
}

func TestEntryLiveTimesAreReadOnly(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))

	m = press(t, m, shiftTab())
	assert.Equal(t, qsoentry.Tags, m.entry.focus, "shift+tab skips the times")
	m = press(t, m, tab())
	assert.Equal(t, qsoentry.Call, m.entry.focus, "tab skips the times")
}

func TestEntryToggleLiveAndPost(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))
	m, _ = at(t, m, 5*time.Second)

	m = press(t, m, liveKey)

	assert.False(t, m.entry.live)
	assert.Contains(t, bodyText(m), "New QSO · post")
	assert.Equal(t, "18:30:05", entryValue(m, qsoentry.TimeOn), "post mode keeps the times")
	m, _ = at(t, m, time.Minute)
	assert.Equal(t, "18:30:05", entryValue(m, qsoentry.TimeOn), "the clock stops")
	m = focusOn(t, m, qsoentry.TimeOn)
	m = press(t, m, special(tea.KeyBackspace))
	assert.Equal(t, "18:30:0", entryValue(m, qsoentry.TimeOn), "the times are editable")

	m = press(t, m, liveKey)
	assert.True(t, m.entry.live)
	assert.Equal(t, qsoentry.Call, m.entry.focus, "focus leaves the read-only time")
	assert.Equal(t, "18:31:00", entryValue(m, qsoentry.TimeOn), "the clock runs again")
}

func TestEntryToggleLiveWithACall(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "k1abc"})

	m = press(t, m, liveKey)

	assert.True(t, m.entry.live, "live mode works without a rig")
	assert.Equal(t, "18:30:00", entryValue(m, qsoentry.TimeOn), "time on is filled in")
	m, _ = at(t, m, time.Minute)
	assert.Equal(t, "18:30:00", entryValue(m, qsoentry.TimeOn), "and locked, since there is a call")
}

func TestEntryLiveClockIsNotAnUnsavedChange(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))
	m, _ = at(t, m, 5*time.Second)

	m = press(t, m, esc())

	assert.False(t, m.entry.open)
	assert.Equal(t, promptNone, m.prompt)
}

func TestEntryLiveFailedSaveLeavesNoTimeOff(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil))

	m = press(t, m, saveKey, liveKey)

	assert.Contains(t, bodyText(m), "call is required")
	assert.Empty(t, entryValue(m, qsoentry.DateOff), "post mode doesn't take over a time off nobody set")
	assert.Empty(t, entryValue(m, qsoentry.TimeOff))
}
