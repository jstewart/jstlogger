package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

// workedQSOs are the fixture QSOs of the worked-before tests, oldest first:
// three with W1AW, under its own call and two others, and one with K1ABC.
var workedQSOs = []logbook.QSO{
	{StartUTC: entryDaysAgo(30), Call: "W1AW", Band: "20m", Mode: "SSB", RSTSent: "59", RSTRcvd: "57"},
	{StartUTC: entryDaysAgo(20), Call: "VE3/W1AW", Band: "40m", Mode: "CW", RSTSent: "599", RSTRcvd: "579"},
	{StartUTC: entryDaysAgo(10), Call: "W1AW/P", Band: "20m", Mode: "MFSK", Submode: "FT4", RSTSent: "-10", RSTRcvd: "-12"},
	{StartUTC: entryDaysAgo(5), Call: "K1ABC", Band: "15m", Mode: "FT8"},
}

// newWorkedStore returns a SQLite logbook of qsos.
func newWorkedStore(t *testing.T, qsos ...logbook.QSO) *logbook.SQLiteStore {
	t.Helper()
	s, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	for _, q := range qsos {
		_, err := s.Add(t.Context(), q)
		require.NoError(t, err)
	}
	return s
}

func TestLookupWorked(t *testing.T) {
	s := newWorkedStore(t, workedQSOs...)

	tests := []struct {
		name       string
		key        workedKey
		total      int
		badges     []string
		wantRecent []string
	}{
		{"new call", workedKey{"K9XYZ", "20m", "SSB"}, 0, []string{"new call"}, nil},
		{"worked on the band and in the mode", workedKey{"W1AW", "20m", "SSB"}, 3, nil,
			[]string{"W1AW/P", "VE3/W1AW", "W1AW"}},
		{"new band", workedKey{"W1AW", "15m", "SSB"}, 3, []string{"new band"}, nil},
		{"new mode", workedKey{"W1AW", "20m", "FT8"}, 3, []string{"new mode"}, nil},
		{"new band and mode", workedKey{"W1AW", "10m", "RTTY"}, 3, []string{"new band", "new mode"}, nil},
		{"submode", workedKey{"W1AW", "40m", "FT4"}, 3, nil, nil},
		{"no band or mode", workedKey{"W1AW", "", ""}, 3, nil, nil},
		{"one QSO", workedKey{"K1ABC", "15m", "FT8"}, 1, nil, []string{"K1ABC"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := lookupWorked(t.Context(), s, tt.key, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.total, res.total)
			assert.Equal(t, tt.badges, res.badges(tt.key))
			if tt.wantRecent != nil {
				var calls []string
				for _, q := range res.recent {
					calls = append(calls, q.Call)
				}
				assert.Equal(t, tt.wantRecent, calls, "newest first")
			}
		})
	}
}

func TestLookupWorkedListsTheNewest(t *testing.T) {
	var qsos []logbook.QSO
	for i := range workedRecent + 2 {
		qsos = append(qsos, logbook.QSO{StartUTC: entryDaysAgo(10 - i), Call: "W1AW", Band: "20m", Mode: "CW"})
	}
	s := newWorkedStore(t, qsos...)

	res, err := lookupWorked(t.Context(), s, workedKey{"W1AW", "20m", "CW"}, 0)
	require.NoError(t, err)
	assert.Equal(t, workedRecent+2, res.total)
	require.Len(t, res.recent, workedRecent)
	assert.Equal(t, entryDaysAgo(10-workedRecent-1), res.recent[0].StartUTC)
}

func TestWorkedKeyOf(t *testing.T) {
	var f qsoentry.Form
	f[qsoentry.Call], f[qsoentry.Band], f[qsoentry.Mode] = " ve3/w1aw/p ", " 20M ", "ssb"
	assert.Equal(t, workedKey{"W1AW", "20m", "SSB"}, workedKeyOf(f))

	f[qsoentry.Mode], f[qsoentry.Submode] = "MFSK", "ft4"
	assert.Equal(t, workedKey{"W1AW", "20m", "FT4"}, workedKeyOf(f), "the submode is the mode")

	f[qsoentry.Call] = "/P"
	assert.Empty(t, workedKeyOf(f).call)
}

// failingCountStore is a logbook whose Count fails.
type failingCountStore struct {
	logbook.Store
}

func (failingCountStore) Count(context.Context, logbook.Expr) (int, error) {
	return 0, errors.New("disk on fire")
}

// newWorkedModel returns a model on s with the New QSO screen open. The
// ticks that end the worked-before panel's waits are kept in the returned
// slice rather than sent; other ticks are dropped.
func newWorkedModel(t *testing.T, s logbook.Store) (Model, *[]entryWorkedTickMsg) {
	t.Helper()
	ticks := new([]entryWorkedTickMsg)
	m := New(WithLogbook(s))
	m.now = func() time.Time { return entryNow }
	m.tick = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		if msg, ok := fn(time.Time{}).(entryWorkedTickMsg); ok {
			assert.Equal(t, workedDelay, d)
			*ticks = append(*ticks, msg)
		}
		return nil
	}
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = runIndex(t, m, cmd)
	m = press(t, m, newQSOKey)
	require.True(t, m.entry.open)
	return m, ticks
}

// sendWorkedTick sends tick and feeds the lookup it starts, if any, back
// into m.
func sendWorkedTick(t *testing.T, m Model, tick entryWorkedTickMsg) Model {
	t.Helper()
	m, cmd := update(t, m, tick)
	for _, msg := range collectMsgs(cmd) {
		if msg, ok := msg.(entryWorkedMsg); ok {
			m, _ = update(t, m, msg)
		}
	}
	return m
}

// lastTick returns the last of ticks.
func lastTick(t *testing.T, ticks []entryWorkedTickMsg) entryWorkedTickMsg {
	t.Helper()
	require.NotEmpty(t, ticks)
	return ticks[len(ticks)-1]
}

func TestEntryWorkedPanel(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	assert.NotContains(t, bodyText(m), "Worked before", "no panel without a call")

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "w1aw", qsoentry.Mode: "rtty", qsoentry.Band: "20m"})
	assert.Contains(t, bodyText(m), "Worked before  W1AW  Looking up…")

	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	body := bodyText(m)
	assert.Contains(t, body, "Worked before  W1AW  3 earlier QSOs  NEW MODE")
	assert.NotContains(t, body, "NEW BAND")
	assert.Contains(t, body, "2026-10-31 18:30  20m    FT4      -10 / -12")
	assert.Contains(t, body, "2026-10-21 18:30  40m    CW       599 / 579")
	assert.Contains(t, body, "2026-10-11 18:30  20m    SSB      59 / 57")
	assert.Less(t, strings.Index(body, "-10 / -12"), strings.Index(body, "599 / 579"), "newest first")
}

func TestEntryWorkedPanelOnlyOnQSOTab(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "W1AW"})
	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	require.Contains(t, bodyText(m), "Worked before")

	m = press(t, m, nextTabKey)
	assert.NotContains(t, bodyText(m), "Worked before")
}

func TestEntryWorkedNewCall(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "K9XYZ", qsoentry.Band: "20m"})
	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	assert.Contains(t, bodyText(m), "Worked before  K9XYZ  0 earlier QSOs  NEW CALL")
}

func TestEntryWorkedWaitsForTheFormToSettle(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	m = focusOn(t, m, qsoentry.Call)
	m = typeText(t, m, "W1A")
	require.Len(t, *ticks, 3, "one wait per change")
	first := (*ticks)[0]

	m, cmd := update(t, m, first)
	assert.Nil(t, cmd, "a wait for an earlier call starts no lookup")

	m = typeText(t, m, "W")
	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	assert.Contains(t, bodyText(m), "W1AW  3 earlier QSOs")

	n := len(*ticks)
	m = press(t, m, keyPress('/', 0), keyPress('P', 0))
	assert.Len(t, *ticks, n, "a portable call with the same base waits for nothing")
	assert.Contains(t, bodyText(m), "W1AW  3 earlier QSOs")
}

func TestEntryWorkedDropsStaleResults(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "K1ABC"})
	stale := lastTick(t, *ticks)
	for range len("K1ABC") {
		m = press(t, m, special(tea.KeyBackspace))
	}
	m = typeText(t, m, "W1AW")

	m, _ = update(t, m, entryWorkedMsg{gen: stale.gen, key: stale.key, res: workedResult{total: 1}})
	assert.Contains(t, bodyText(m), "Worked before  W1AW  Looking up…")

	m, _ = update(t, m, entryWorkedMsg{gen: m.entry.gen - 1, key: workedKeyOf(m.entry.form()), res: workedResult{total: 9}})
	assert.Contains(t, bodyText(m), "Looking up…", "a lookup for an earlier opening is dropped")
}

func TestEntryWorkedLookupFails(t *testing.T) {
	m, ticks := newWorkedModel(t, failingCountStore{newWorkedStore(t)})
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "W1AW"})
	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	assert.Contains(t, bodyText(m), "Worked before  W1AW  Lookup failed")
}

func TestEntryWorkedDropsTicksAfterClose(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "W1AW"})
	m = m.closeEntry()

	_, cmd := update(t, m, lastTick(t, *ticks))
	assert.Nil(t, cmd)
}

func TestEntryWorkedCountsTheSavedQSO(t *testing.T) {
	m, ticks := newWorkedModel(t, newWorkedStore(t, workedQSOs...))
	m = fill(t, m, map[qsoentry.Field]string{
		qsoentry.Date: "2026-11-09", qsoentry.TimeOn: "1405", qsoentry.Call: "W1AW",
		qsoentry.Mode: "CW", qsoentry.Band: "40m",
	})
	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	require.Contains(t, bodyText(m), "3 earlier QSOs")

	m = press(t, m, saveKey)
	require.Contains(t, m.entry.msg, "Saved")
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "W1AW"})
	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	assert.Contains(t, bodyText(m), "4 earlier QSOs")
}

func TestLookupWorkedLeavesOutTheEditedQSO(t *testing.T) {
	s := newWorkedStore(t, workedQSOs...)
	ids, err := s.IDs(t.Context(), logbook.CallBaseIs{Call: "W1AW"})
	require.NoError(t, err)
	require.Len(t, ids, 3)
	oldest := ids[len(ids)-1]

	k := workedKey{"W1AW", "20m", "SSB"}
	res, err := lookupWorked(t.Context(), s, k, oldest)
	require.NoError(t, err)
	assert.Equal(t, 2, res.total)
	assert.Equal(t, 1, res.onBand)
	assert.Zero(t, res.onMode, "the only SSB QSO is the one left out")
	for _, q := range res.recent {
		assert.NotEqual(t, oldest, q.ID)
	}
}

func TestEntryWorkedLeavesOutTheEditedQSO(t *testing.T) {
	s := newWorkedStore(t, workedQSOs...)
	ids, err := s.IDs(t.Context(), logbook.CallBaseIs{Call: "W1AW"})
	require.NoError(t, err)
	q, err := s.Get(t.Context(), ids[len(ids)-1])
	require.NoError(t, err)
	require.Equal(t, "W1AW", q.Call)

	m, ticks := newWorkedModel(t, s)
	m = m.closeEntry()
	m, _ = update(t, m, editLoadedMsg{qso: q})
	require.True(t, m.entry.editing)

	m = sendWorkedTick(t, m, lastTick(t, *ticks))
	body := bodyText(m)
	assert.Contains(t, body, "Worked before  W1AW  2 earlier QSOs  NEW MODE")
	assert.NotContains(t, body, "59 / 57", "the edited QSO is not listed")
}
