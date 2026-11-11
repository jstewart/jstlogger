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

	"jstlogger/internal/callsign"
	"jstlogger/internal/logbook"
)

// fakeEQSL serves IsAG from members, keyed by callsign, records the calls
// looked up and fails every lookup with err.
type fakeEQSL struct {
	mu      sync.Mutex
	members map[string]bool
	calls   []string
	err     error
}

func (f *fakeEQSL) IsAG(ctx context.Context, call string) (bool, error) {
	if _, ok := ctx.Deadline(); !ok {
		return false, errors.New("fakeEQSL: ctx has no deadline")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if f.err != nil {
		return false, f.err
	}
	return f.members[call], nil
}

// lookedUp returns the calls looked up so far.
func (f *fakeEQSL) lookedUp() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// newEQSLModel returns a Model in an 80 by 24 window with the pager open on
// the first of qsos, the eQSL lookup f and opts.
func newEQSLModel(t *testing.T, f *fakeEQSL, qsos []logbook.QSO, opts ...Option) Model {
	t.Helper()
	s := &fakeIndexStore{qsos: qsos}
	m := New(append([]Option{WithLogbook(s), WithEQSL(f), WithClock(func() time.Time { return lotwNow })}, opts...)...)
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)
	m = pressPager(t, m, enter())
	require.True(t, m.pager.open)
	return m
}

// eqslLine returns the plain QSO tab row labeled eQSL AG, and whether there
// is one.
func eqslLine(m Model) (string, bool) {
	for l := range strings.SplitSeq(plain(m.View().Content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "eQSL AG ") {
			return strings.TrimSpace(l), true
		}
	}
	return "", false
}

// eqslValue returns the value of the eQSL AG row, failing when there is
// none.
func eqslValue(t *testing.T, m Model) string {
	t.Helper()
	line, ok := eqslLine(m)
	require.True(t, ok, "there is an eQSL AG row")
	return strings.TrimSpace(strings.TrimPrefix(line, "eQSL AG"))
}

func TestEQSLBadgeStylesDiffer(t *testing.T) {
	st := defaultStyles()
	ag, notAG, plainE := st.eQSLAG.Render("[E]"), st.eQSLNotAG.Render("[E]"), st.eQSL.Render("[E]")
	assert.NotEqual(t, ag, notAG)
	assert.NotEqual(t, ag, plainE, "a member's badge differs from today's")
	assert.NotEqual(t, notAG, plainE, "a non-member's badge differs from today's")
}

func TestEQSLBadgeByMembership(t *testing.T) {
	st, url := defaultStyles(), callsign.EQSLURL("W1AW")
	ag := st.eQSLAG.Hyperlink(url).Render("[E]")
	notAG := st.eQSLNotAG.Hyperlink(url).Render("[E]")
	today := st.eQSL.Hyperlink(url).Render("[E]")
	tests := []struct {
		name  string
		f     *fakeEQSL
		badge string
		value string
	}{
		{"member", &fakeEQSL{members: map[string]bool{"W1AW": true}}, ag, "yes"},
		{"not a member", &fakeEQSL{members: map[string]bool{}}, notAG, pagerEmpty},
		{"lookup failed", &fakeEQSL{err: errors.New("disk on fire")}, today, "lookup failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newEQSLModel(t, tt.f, []logbook.QSO{lotwQSO(1, "W1AW")})

			raw := m.View().Content
			assert.Contains(t, raw, tt.badge, "the badge is styled by membership")
			assert.Equal(t, 3, strings.Count(raw, "\x1b]8;;http"), "the [E] badge is still a link")
			assert.Contains(t, callLine(t, m), "W1AW [Q] [H] [E]")
			assert.Equal(t, tt.value, eqslValue(t, m))
		})
	}
}

func TestEQSLLookupFailedShown(t *testing.T) {
	m := newEQSLModel(t, &fakeEQSL{err: errors.New("disk on fire")}, []logbook.QSO{lotwQSO(1, "W1AW")})

	assert.Contains(t, plain(m.footerView()), "disk on fire", "the error is shown on the status line")
	m = pressPager(t, m, tab())
	assert.NotContains(t, plain(m.footerView()), "disk on fire", "a key press clears the error")
}

func TestEQSLWithoutLookup(t *testing.T) {
	m := newLookupModel(t, "W1AW", 80)

	url := callsign.EQSLURL("W1AW")
	assert.Contains(t, m.View().Content, defaultStyles().eQSL.Hyperlink(url).Render("[E]"), "[E] is as before")
	_, ok := eqslLine(m)
	assert.False(t, ok, "no eQSL AG row without a lookup")
}

func TestEQSLPendingShowsTodaysBadge(t *testing.T) {
	f := &fakeEQSL{members: map[string]bool{"W1AW": true}}
	s := &fakeIndexStore{qsos: []logbook.QSO{lotwQSO(1, "W1AW")}}
	m := New(WithLogbook(s), WithEQSL(f))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)
	m, _ = update(t, m, enter())

	url := callsign.EQSLURL("W1AW")
	assert.Contains(t, m.View().Content, defaultStyles().eQSL.Hyperlink(url).Render("[E]"))
	assert.Equal(t, pagerEmpty, eqslValue(t, m), "pending shows the empty-field dash")
}

func TestEQSLRowPlacement(t *testing.T) {
	labels := func(m Model) []string {
		var out []string
		for l := range strings.SplitSeq(plain(m.View().Content), "\n") {
			for _, label := range []string{"Call ", "LoTW ", "eQSL AG ", "Band "} {
				if strings.HasPrefix(strings.TrimSpace(l), label) {
					out = append(out, strings.TrimSpace(label))
				}
			}
		}
		return out
	}

	m := newEQSLModel(t, &fakeEQSL{}, []logbook.QSO{lotwQSO(1, "W1AW")})
	assert.Equal(t, []string{"Call", "eQSL AG", "Band"}, labels(m), "without LoTW the row follows Call")

	m = newEQSLModel(t, &fakeEQSL{}, []logbook.QSO{lotwQSO(1, "W1AW")}, WithLoTW(&fakeLoTW{}))
	assert.Equal(t, []string{"Call", "LoTW", "eQSL AG", "Band"}, labels(m), "with LoTW the row follows LoTW")
}

func TestEQSLLookupNormalizedAndCached(t *testing.T) {
	f := &fakeEQSL{members: map[string]bool{"W1AW": true}}
	m := newEQSLModel(t, f, []logbook.QSO{lotwQSO(3, "VE3/W1AW/P"), lotwQSO(2, "w1aw"), lotwQSO(1, "K1ABC")})

	assert.Equal(t, "yes", eqslValue(t, m))
	m = pressPager(t, m, keyPress('j', 0))
	assert.Equal(t, "yes", eqslValue(t, m))
	assert.Equal(t, []string{"W1AW"}, f.lookedUp(), "one lookup serves the same base call")

	m = pressPager(t, m, keyPress('j', 0))
	assert.Equal(t, pagerEmpty, eqslValue(t, m))
	assert.Equal(t, []string{"W1AW", "K1ABC"}, f.lookedUp())

	// Closing the pager forgets the lookup.
	pressPager(t, m, keyPress('q', 0), enter())
	assert.Equal(t, []string{"W1AW", "K1ABC", "K1ABC"}, f.lookedUp())
}

func TestEQSLNoLookupWithoutBaseCall(t *testing.T) {
	f := &fakeEQSL{}
	newEQSLModel(t, f, []logbook.QSO{lotwQSO(1, "P/QRP/7")})
	assert.Empty(t, f.lookedUp())
}

func TestEQSLStaleReplyDropped(t *testing.T) {
	f := &fakeEQSL{members: map[string]bool{"W1AW": true}}
	s := &fakeIndexStore{qsos: []logbook.QSO{lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC")}}
	m := New(WithLogbook(s), WithEQSL(f))
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = runIndex(t, m, cmd)

	// Open the pager, holding back the eQSL reply for W1AW.
	m, cmd = update(t, m, enter())
	var held []eqslLookupMsg
	for _, msg := range collectMsgs(cmd) {
		if r, ok := msg.(eqslLookupMsg); ok {
			held = append(held, r)
		}
	}
	require.Len(t, held, 1)
	require.Equal(t, "W1AW", held[0].call)

	// The cursor moves on before the reply lands.
	m = pressPager(t, m, keyPress('j', 0))
	m, _ = update(t, m, held[0])
	assert.Equal(t, "K1ABC", m.pager.eqsl.call, "the W1AW reply does not replace K1ABC's")
	assert.Equal(t, pagerEmpty, eqslValue(t, m))

	// A reply landing after the pager closed is dropped too.
	m = m.closePager()
	m, _ = update(t, m, held[0])
	assert.Empty(t, m.pager.eqsl.call)
}

func TestEQSLSyncFailedShown(t *testing.T) {
	f := &fakeEQSL{members: map[string]bool{"W1AW": true}}
	m := newEQSLModel(t, f, []logbook.QSO{lotwQSO(1, "W1AW")})

	m, _ = update(t, m, EQSLSynced{Err: errors.New("list down")})
	assert.Contains(t, plain(m.footerView()), "eQSL sync failed: list down")
	assert.Equal(t, "yes", eqslValue(t, m), "the stored data keeps working")
}

func TestEQSLSyncedLooksUpAgain(t *testing.T) {
	f := &fakeEQSL{members: map[string]bool{}}
	m := newEQSLModel(t, f, []logbook.QSO{lotwQSO(1, "W1AW")})
	assert.Equal(t, pagerEmpty, eqslValue(t, m))

	m, _ = update(t, m, EQSLSynced{Err: errors.New("list down")})
	f.mu.Lock()
	f.members["W1AW"] = true
	f.mu.Unlock()

	m, cmd := update(t, m, EQSLSynced{})
	m = runPager(t, m, cmd)
	assert.Equal(t, []string{"W1AW", "W1AW"}, f.lookedUp())
	assert.Equal(t, "yes", eqslValue(t, m))
	assert.NotContains(t, plain(m.footerView()), "eQSL sync failed", "a successful sync clears the error")
}
