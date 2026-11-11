package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

// entryDaysAgo returns the time n days before entryNow.
func entryDaysAgo(n int) time.Time {
	return entryNow.Add(-time.Duration(n) * 24 * time.Hour)
}

// newEntryLoTWModel returns a model with the New QSO screen open, focus on
// the call and the LoTW lookup f.
func newEntryLoTWModel(t *testing.T, f *fakeLoTW) Model {
	t.Helper()
	m := newEntryModel(t, newEntryStore(t, nil), WithLoTW(f))
	return focusOn(t, m, qsoentry.Call)
}

// typeCall types s into the field with focus, running the commands each
// key press returns.
func typeCall(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = press(t, m, keyPress(r, 0))
	}
	return m
}

// entryCallLine returns the plain line of the New QSO screen holding the
// call.
func entryCallLine(t *testing.T, m Model) string {
	t.Helper()
	for l := range strings.SplitSeq(bodyText(m), "\n") {
		if l = strings.TrimSpace(l); l == "Call" || strings.HasPrefix(l, "Call ") {
			return l
		}
	}
	require.Fail(t, "no Call line", bodyText(m))
	return ""
}

func TestEntryLoTWShowsStatus(t *testing.T) {
	tests := []struct {
		name  string
		days  int
		style func(styles) string
		text  string
	}{
		{"recent", 3, func(st styles) string { return st.lotwRecent.Render("[L]") }, "[L] last upload 3 days ago"},
		{"active", 100, func(st styles) string { return st.lotwActive.Render("[L]") }, "[L] last upload 100 days ago"},
		{"inactive", 400, func(st styles) string { return st.lotwInactive.Render("[L]") }, "[L] last upload 400 days ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: entryDaysAgo(tt.days)}}}
			m := newEntryLoTWModel(t, f)

			m = typeCall(t, m, "w1aw")

			assert.Contains(t, entryCallLine(t, m), "w1aw")
			assert.Contains(t, entryCallLine(t, m), tt.text)
			assert.Contains(t, m.View().Content, tt.style(m.styles), "the badge is styled by recency")
		})
	}
}

func TestEntryLoTWLooksUpTheBaseCall(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: entryDaysAgo(3)}}}
	m := newEntryLoTWModel(t, f)

	m = typeCall(t, m, "VE3/W1AW/P")

	assert.Contains(t, entryCallLine(t, m), "VE3/W1AW/P")
	assert.Contains(t, entryCallLine(t, m), "[L]")
	n := 0
	for _, call := range f.lookedUp() {
		if call == "W1AW" {
			n++
		}
	}
	assert.Equal(t, 1, n, "the /P suffix keeps the base call, so it is not looked up again")
}

func TestEntryLoTWNotAUser(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{}}
	m := newEntryLoTWModel(t, f)

	m = typeCall(t, m, "K1ABC")

	assert.Contains(t, f.lookedUp(), "K1ABC")
	assert.NotContains(t, entryCallLine(t, m), "[L]")
	assert.NotContains(t, entryCallLine(t, m), "upload")
}

func TestEntryLoTWLookupFailed(t *testing.T) {
	f := &fakeLoTW{err: errors.New("disk on fire")}
	m := newEntryLoTWModel(t, f)

	m = typeCall(t, m, "W1AW")

	assert.Contains(t, entryCallLine(t, m), "LoTW lookup failed")
	assert.Contains(t, plain(m.footerView()), "disk on fire", "the error is shown in the footer")
}

func TestEntryLoTWWithoutLookup(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)

	m = typeCall(t, m, "W1AW")

	assert.NotContains(t, entryCallLine(t, m), "[L]")
	assert.NotContains(t, entryCallLine(t, m), "LoTW")
}

func TestEntryLoTWStaleReplyDropped(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: entryDaysAgo(3)}, "W1A": {last: entryDaysAgo(400)}}}
	m := newEntryLoTWModel(t, f)

	// Type the call, holding back each LoTW reply.
	held := map[string]entryLoTWMsg{}
	for _, r := range "W1AW" {
		var cmd tea.Cmd
		m, cmd = update(t, m, keyPress(r, 0))
		for _, msg := range collectMsgs(cmd) {
			if reply, ok := msg.(entryLoTWMsg); ok {
				held[reply.call] = reply
			}
		}
	}
	require.Contains(t, held, "W1A")
	require.Contains(t, held, "W1AW")

	m, _ = update(t, m, held["W1A"])
	assert.NotContains(t, entryCallLine(t, m), "[L]", "the reply for W1A doesn't apply to W1AW")

	m, _ = update(t, m, held["W1AW"])
	assert.Contains(t, entryCallLine(t, m), "[L] last upload 3 days ago")

	// A reply landing after the screen closed is dropped.
	m = m.closeEntry()
	m, _ = update(t, m, held["W1AW"])
	assert.Empty(t, m.entry.lotw.call)
}

func TestEntryLoTWClearedAfterSave(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{"W1AW": {last: entryDaysAgo(3)}}}
	s := newEntryStore(t, nil)
	s.expectAdd()
	m := newEntryModel(t, s, WithLoTW(f))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Date: "2026-11-09", qsoentry.TimeOn: "14:05"})
	m = focusOn(t, m, qsoentry.Call)
	m = typeCall(t, m, "W1AW")
	require.Contains(t, entryCallLine(t, m), "[L]")

	m = press(t, m, saveKey)

	require.Len(t, s.added, 1)
	assert.Empty(t, entryValue(m, qsoentry.Call), "the call doesn't carry over")
	assert.NotContains(t, entryCallLine(t, m), "[L]")
}

func TestEntryLoTWSyncedLooksUpAgain(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{}}
	m := newEntryLoTWModel(t, f)
	m = typeCall(t, m, "W1AW")
	require.NotContains(t, entryCallLine(t, m), "[L]")

	f.mu.Lock()
	f.users["W1AW"] = lotwUser{last: entryDaysAgo(3)}
	f.mu.Unlock()
	m, cmd := update(t, m, LoTWSynced{})
	m = runIndex(t, m, cmd)

	assert.Equal(t, 2, len(slices.DeleteFunc(f.lookedUp(), func(c string) bool { return c != "W1AW" })))
	assert.Contains(t, entryCallLine(t, m), "[L] last upload 3 days ago")
}
