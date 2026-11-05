package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// fakeOpener records the URLs it is asked to open and returns err.
type fakeOpener struct {
	urls []string
	err  error
}

func (f *fakeOpener) open(ctx context.Context, url string) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("fakeOpener: ctx has no deadline")
	}
	f.urls = append(f.urls, url)
	return f.err
}

// newLookupModel returns a Model in a width by 24 window with the pager open
// on a single QSO with the given call, and the options opts.
func newLookupModel(t *testing.T, call string, width int, opts ...Option) Model {
	t.Helper()
	q := logbook.QSO{ID: 1, StartUTC: time.Date(2026, 10, 31, 13, 34, 56, 0, time.UTC), Call: call}
	s := &fakeIndexStore{qsos: []logbook.QSO{q}}
	m := New(append([]Option{WithLogbook(s)}, opts...)...)
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: width, Height: 24})
	m = runIndex(t, m, cmd)
	m = pressPager(t, m, enter())
	require.True(t, m.pager.open)
	return m
}

// callLine returns the plain QSO tab row labeled Call.
func callLine(t *testing.T, m Model) string {
	t.Helper()
	for l := range strings.SplitSeq(plain(m.View().Content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "Call ") {
			return l
		}
	}
	require.Fail(t, "no Call row in the pager")
	return ""
}

// lookup presses g then r and returns the model and the command the second
// key returned.
func lookup(t *testing.T, m Model, r rune) (Model, tea.Cmd) {
	t.Helper()
	m = pressPager(t, m, keyPress('g', 0))
	return update(t, m, keyPress(r, 0))
}

// lookupResult runs cmd and returns the single lookupResultMsg it produces.
func lookupResult(t *testing.T, cmd tea.Cmd) lookupResultMsg {
	t.Helper()
	var got []lookupResultMsg
	for _, msg := range collectMsgs(cmd) {
		if r, ok := msg.(lookupResultMsg); ok {
			got = append(got, r)
		}
	}
	require.Len(t, got, 1)
	return got[0]
}

func TestLookupBadgesShown(t *testing.T) {
	m := newLookupModel(t, "VE3/W1AW/P", 80)

	assert.Contains(t, callLine(t, m), "VE3/W1AW/P [Q] [H] [E]")
	raw := m.View().Content
	for _, url := range []string{
		"https://www.qrz.com/db/W1AW",
		"https://www.hamqth.com/W1AW",
		"https://www.eqsl.cc/Member.cfm?W1AW",
	} {
		assert.Contains(t, raw, "\x1b]8;;"+url+"\x07", "the badge links to %s", url)
	}
}

func TestLookupBadgesDroppedWhenNarrow(t *testing.T) {
	m := newLookupModel(t, "VE3/W1AW/P", 30)

	line := callLine(t, m)
	assert.Contains(t, line, "VE3/W1AW/P", "the call is kept")
	assert.NotContains(t, line, "[Q]")
	assert.NotContains(t, m.View().Content, "\x1b]8;", "no links are left behind")
}

func TestLookupBadgesNeedABaseCall(t *testing.T) {
	for _, call := range []string{"", "P/QRP/7"} {
		t.Run(call, func(t *testing.T) {
			m := newLookupModel(t, call, 80)
			assert.NotContains(t, callLine(t, m), "[Q]")
			assert.NotContains(t, m.View().Content, "\x1b]8;")
		})
	}
}

func TestLookupKeysOpenURLs(t *testing.T) {
	tests := []struct {
		key  rune
		want string
	}{
		{'q', "https://www.qrz.com/db/W1AW"},
		{'h', "https://www.hamqth.com/W1AW"},
		{'e', "https://www.eqsl.cc/Member.cfm?W1AW"},
	}
	for _, tt := range tests {
		t.Run(string(tt.key), func(t *testing.T) {
			f := &fakeOpener{}
			m := newLookupModel(t, "VE3/W1AW/P", 80, WithURLOpener(f.open))

			m, cmd := lookup(t, m, tt.key)
			assert.True(t, m.pager.open, "g%c leaves the pager open", tt.key)
			assert.False(t, m.pager.lookupPrefix, "the prefix is spent")
			r := lookupResult(t, cmd)
			require.NoError(t, r.err)
			assert.Equal(t, []string{tt.want}, f.urls)
		})
	}
}

func TestLookupPrefixStatus(t *testing.T) {
	f := &fakeOpener{}
	m := newLookupModel(t, "W1AW", 80, WithURLOpener(f.open))

	m = pressPager(t, m, keyPress('g', 0))
	assert.Contains(t, statusLine(t, m), "lookup-prefix")

	m = pressPager(t, m, esc())
	assert.True(t, m.pager.open, "esc cancels the prefix, not the pager")
	assert.NotContains(t, statusLine(t, m), "lookup-prefix")
	assert.Equal(t, "QSO 1/1", statusLine(t, m), "esc cancels silently")

	m = pressPager(t, m, keyPress('g', 0), keyPress('x', 0))
	assert.True(t, m.pager.open)
	assert.NotContains(t, statusLine(t, m), "lookup-prefix")
	assert.Contains(t, statusLine(t, m), notLookupMsg("x"))
	assert.Empty(t, f.urls)
}

func TestLookupPrefixResetOnClose(t *testing.T) {
	f := &fakeOpener{}
	m := newLookupModel(t, "W1AW", 80, WithURLOpener(f.open))

	m = pressPager(t, m, keyPress('g', 0))
	m = m.closePager()
	assert.False(t, m.pager.lookupPrefix)
}

func TestLookupNoBaseCall(t *testing.T) {
	f := &fakeOpener{}
	m := newLookupModel(t, "P/QRP/7", 80, WithURLOpener(f.open))

	m = pressPager(t, m, keyPress('g', 0), keyPress('q', 0))
	assert.True(t, m.pager.open)
	assert.Empty(t, f.urls)
	assert.Contains(t, statusLine(t, m), msgNoLookupCall)
}

func TestLookupErrorShown(t *testing.T) {
	f := &fakeOpener{err: errors.New("no browser")}
	m := newLookupModel(t, "W1AW", 80, WithURLOpener(f.open))

	m, cmd := lookup(t, m, 'q')
	m, _ = update(t, m, lookupResult(t, cmd))

	require.Error(t, m.lookupErr)
	assert.ErrorIs(t, m.lookupErr, f.err)
	assert.Contains(t, plain(m.footerView()), "no browser")

	// A key press clears the error.
	m = pressPager(t, m, keyPress('j', 0))
	assert.NotContains(t, plain(m.footerView()), "no browser")
}

func TestLookupKeysDisabledWithoutOpener(t *testing.T) {
	m := newLookupModel(t, "W1AW", 80)

	m, cmd := lookup(t, m, 'q')
	assert.False(t, m.pager.lookupPrefix)
	assert.NotContains(t, statusLine(t, m), "lookup-prefix")
	assert.False(t, m.pager.open, "without an opener, q closes the pager as usual")
	assertNotQuit(t, cmd)

	m = newLookupModel(t, "W1AW", 80)
	m.help.ShowAll = true
	assert.NotContains(t, plain(m.footerView()), "QRZ")
}

func TestLookupHelp(t *testing.T) {
	f := &fakeOpener{}
	m := newLookupModel(t, "W1AW", 120, WithURLOpener(f.open))
	m.help.ShowAll = true

	footer := plain(m.footerView())
	for _, want := range []string{"gq", "QRZ", "gh", "HamQTH", "ge", "eQSL"} {
		assert.Contains(t, footer, want)
	}
}
