package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// lotwNow is the fixed time the LoTW tests run at.
var lotwNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// lotwUser is a LoTW user's last upload in a fakeLoTW.
type lotwUser struct {
	last time.Time
}

// fakeLoTW serves LastUpload from users, keyed by callsign, records the
// calls looked up and fails every lookup with err.
type fakeLoTW struct {
	mu    sync.Mutex
	users map[string]lotwUser
	calls []string
	err   error
}

func (f *fakeLoTW) LastUpload(ctx context.Context, call string) (time.Time, bool, error) {
	if _, ok := ctx.Deadline(); !ok {
		return time.Time{}, false, errors.New("fakeLoTW: ctx has no deadline")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if f.err != nil {
		return time.Time{}, false, f.err
	}
	u, ok := f.users[call]
	return u.last, ok, nil
}

// lookedUp returns the calls looked up so far.
func (f *fakeLoTW) lookedUp() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// daysBefore returns the time n days before lotwNow.
func daysBefore(n int) time.Time {
	return lotwNow.Add(-time.Duration(n) * 24 * time.Hour)
}

// newLoTWModel returns a Model in an 80 by 24 window with the pager open on
// the first of qsos, a fixed clock and the LoTW lookup f.
func newLoTWModel(t *testing.T, f *fakeLoTW, qsos ...logbook.QSO) Model {
	t.Helper()
	s := &fakeIndexStore{qsos: qsos}
	m := New(WithLogbook(s), WithLoTW(f), WithClock(func() time.Time { return lotwNow }))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)
	m = pressPager(t, m, enter())
	require.True(t, m.pager.open)
	return m
}

// lotwQSO returns a QSO with the given ID and call.
func lotwQSO(id int64, call string) logbook.QSO {
	return logbook.QSO{ID: id, StartUTC: time.Date(2026, 10, 31, 13, 34, 56, 0, time.UTC), Call: call}
}

// lotwLine returns the plain QSO tab row labeled LoTW, and whether there is
// one.
func lotwLine(m Model) (string, bool) {
	for l := range strings.SplitSeq(plain(m.View().Content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "LoTW ") {
			return strings.TrimSpace(l), true
		}
	}
	return "", false
}

func TestLoTWBadgeByRecency(t *testing.T) {
	st := defaultStyles()
	tests := []struct {
		name  string
		days  int
		style string
		text  string
	}{
		{"recent", 3, st.lotwRecent.Render("[L]"), "last upload 3 days ago"},
		{"today", 0, st.lotwRecent.Render("[L]"), "last upload today"},
		{"one day", 1, st.lotwRecent.Render("[L]"), "last upload 1 day ago"},
		{"active", 90, st.lotwActive.Render("[L]"), "last upload 90 days ago"},
		{"inactive", 400, st.lotwInactive.Render("[L]"), "last upload 400 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: daysBefore(tt.days)}}}
			m := newLoTWModel(t, f, lotwQSO(1, "W1AW"))

			assert.Contains(t, callLine(t, m), "W1AW [Q] [H] [E] [L]")
			assert.Contains(t, m.View().Content, tt.style, "the badge is styled by recency")
			line, ok := lotwLine(m)
			require.True(t, ok, "there is a LoTW row")
			assert.Contains(t, line, tt.text)
		})
	}
}

func TestLoTWBadgeStylesDiffer(t *testing.T) {
	st := defaultStyles()
	recent, active, inactive := st.lotwRecent.Render("[L]"), st.lotwActive.Render("[L]"), st.lotwInactive.Render("[L]")
	assert.NotEqual(t, recent, active)
	assert.NotEqual(t, active, inactive)
	assert.NotEqual(t, recent, inactive)
	assert.NotEqual(t, active, st.hamQTH.Render("[L]"), "amber is not HamQTH's yellow")
}

func TestLoTWBadgeIsNotALink(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: daysBefore(3)}}}
	m := newLoTWModel(t, f, lotwQSO(1, "W1AW"))

	raw := m.View().Content
	assert.Equal(t, 3, strings.Count(raw, "\x1b]8;;http"), "only the Q, H and E badges link")
}

func TestLoTWNotAUser(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{}}
	m := newLoTWModel(t, f, lotwQSO(1, "K1ABC"))

	assert.NotContains(t, callLine(t, m), "[L]")
	line, ok := lotwLine(m)
	require.True(t, ok)
	assert.Contains(t, line, "not a LoTW user")
}

func TestLoTWLookupErrorShowsNoBadge(t *testing.T) {
	f := &fakeLoTW{err: errors.New("disk on fire")}
	m := newLoTWModel(t, f, lotwQSO(1, "W1AW"))

	assert.NotContains(t, callLine(t, m), "[L]")
	line, ok := lotwLine(m)
	require.True(t, ok, "the row keeps its place")
	assert.Contains(t, line, "lookup failed")
	view := plain(m.View().Content)
	assert.NotContains(t, view, "not a LoTW user")
	assert.Contains(t, view, "disk on fire", "the error is shown on the status line")
}

func TestLoTWWithoutLookup(t *testing.T) {
	m := newLookupModel(t, "W1AW", 80)

	assert.NotContains(t, callLine(t, m), "[L]")
	_, ok := lotwLine(m)
	assert.False(t, ok, "no LoTW row without a lookup")
}

func TestLoTWLookupNormalizedAndCached(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: daysBefore(3)}}}
	m := newLoTWModel(t, f, lotwQSO(3, "VE3/W1AW/P"), lotwQSO(2, "w1aw"), lotwQSO(1, "K1ABC"))

	assert.Contains(t, callLine(t, m), "VE3/W1AW/P [Q] [H] [E] [L]")
	m = pressPager(t, m, keyPress('j', 0))
	assert.Contains(t, callLine(t, m), "w1aw [Q] [H] [E] [L]")
	assert.Equal(t, []string{"W1AW"}, f.lookedUp(), "one lookup serves the same base call")

	m = pressPager(t, m, keyPress('j', 0))
	assert.NotContains(t, callLine(t, m), "[L]")
	assert.Equal(t, []string{"W1AW", "K1ABC"}, f.lookedUp())
}

func TestLoTWNoLookupWithoutBaseCall(t *testing.T) {
	f := &fakeLoTW{}
	newLoTWModel(t, f, lotwQSO(1, "P/QRP/7"))
	assert.Empty(t, f.lookedUp())
}

func TestLoTWStaleReplyDropped(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: daysBefore(3)}, "K1ABC": {last: daysBefore(400)}}}
	s := &fakeIndexStore{qsos: []logbook.QSO{lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC")}}
	m := New(WithLogbook(s), WithLoTW(f), WithClock(func() time.Time { return lotwNow }))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)

	// Open the pager, holding back the LoTW reply for W1AW.
	m, cmd = update(t, m, enter())
	var held []lotwLookupMsg
	for _, msg := range collectMsgs(cmd) {
		if r, ok := msg.(lotwLookupMsg); ok {
			held = append(held, r)
		}
	}
	require.Len(t, held, 1)
	require.Equal(t, "W1AW", held[0].call)

	// The cursor moves on before the reply lands.
	m = pressPager(t, m, keyPress('j', 0))
	m, _ = update(t, m, held[0])
	assert.Equal(t, "K1ABC", m.pager.lotw.call, "the W1AW reply does not replace K1ABC's")
	assert.NotContains(t, callLine(t, m), "W1AW")

	// A reply landing after the pager closed is dropped too.
	m = m.closePager()
	m, _ = update(t, m, held[0])
	assert.Empty(t, m.pager.lotw.call)
}

func TestLoTWSyncFailedShown(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: daysBefore(3)}}}
	m := newLoTWModel(t, f, lotwQSO(1, "W1AW"))

	m, _ = update(t, m, LoTWSynced{Err: errors.New("feed down")})
	assert.Contains(t, plain(m.footerView()), "LoTW sync failed: feed down")
	assert.Contains(t, callLine(t, m), "[L]", "the stored data keeps working")
}

func TestLoTWSyncedLooksUpAgain(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{}}
	m := newLoTWModel(t, f, lotwQSO(1, "W1AW"))
	assert.NotContains(t, callLine(t, m), "[L]")

	m, _ = update(t, m, LoTWSynced{Err: errors.New("feed down")})
	f.mu.Lock()
	f.users["W1AW"] = lotwUser{last: daysBefore(3)}
	f.mu.Unlock()

	m, cmd := update(t, m, LoTWSynced{})
	m = runPager(t, m, cmd)
	assert.Equal(t, []string{"W1AW", "W1AW"}, f.lookedUp())
	assert.Contains(t, callLine(t, m), "W1AW [Q] [H] [E] [L]")
	assert.NotContains(t, plain(m.footerView()), "LoTW sync failed", "a successful sync clears the error")
}
