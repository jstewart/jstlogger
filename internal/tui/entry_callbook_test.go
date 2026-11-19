package tui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	cbmock "jstlogger/internal/callbook/mock"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

// testCallbookFields are the fields the callbook fills in, in the order
// callbookValues returns them.
var testCallbookFields = []qsoentry.Field{
	qsoentry.Name, qsoentry.QTH, qsoentry.Gridsquare, qsoentry.State, qsoentry.Cnty, qsoentry.Email,
}

// callbookValues returns the name, QTH, grid, state, county and email in
// the form.
func callbookValues(m Model) [6]string {
	var v [6]string
	for i, f := range testCallbookFields {
		v[i] = entryValue(m, f)
	}
	return v
}

// w1awResult is what QRZ.com returns for W1AW.
var w1awResult = callbook.Result{
	Call: "W1AW", Name: "Hiram Maxim", QTH: "Newington", Grid: "FN31pr",
	State: "CT", County: "Hartford", Email: "w1aw@arrl.org", Service: callbook.ServiceQRZ,
}

// ve3abcResult is what HamQTH returns for VE3ABC, a county without a state.
var ve3abcResult = callbook.Result{
	Call: "VE3ABC", Name: "Alice", QTH: "Ottawa", Grid: "FN25",
	County: "Carleton", Email: "ve3abc@example.com", Service: callbook.ServiceHamQTH,
}

// newCallbookModel returns a model on s, looking calls up in l unless it is
// nil, with the New QSO screen open and focus on the call. The ticks that
// end the callbook's waits are kept in the returned slice rather than sent;
// other ticks are dropped.
func newCallbookModel(t *testing.T, s logbook.Store, l callbook.Looker) (Model, *[]entryCallbookTickMsg) {
	t.Helper()
	opts := []Option{WithLogbook(s)}
	if l != nil {
		opts = append(opts, WithCallbook(l))
	}
	m := New(opts...)
	ticks := new([]entryCallbookTickMsg)
	m.now = func() time.Time { return entryNow }
	m.tick = callbookTicks(t, ticks)
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = runIndex(t, m, cmd)
	m = press(t, m, newQSOKey)
	require.True(t, m.entry.open)
	return focusOn(t, m, qsoentry.Call), ticks
}

// callbookTicks returns a tick function that keeps the ticks ending the
// callbook's waits in ticks and drops the others.
func callbookTicks(t *testing.T, ticks *[]entryCallbookTickMsg) func(time.Duration, func(time.Time) tea.Msg) tea.Cmd {
	return func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		if msg, ok := fn(time.Time{}).(entryCallbookTickMsg); ok {
			assert.Equal(t, callbookDelay, d)
			*ticks = append(*ticks, msg)
		}
		return nil
	}
}

// startLookup sends the last of ticks and returns the commands it starts,
// without running them.
func startLookup(t *testing.T, m Model, ticks []entryCallbookTickMsg) (Model, tea.Cmd) {
	t.Helper()
	require.NotEmpty(t, ticks, "a lookup waits for its tick")
	return update(t, m, ticks[len(ticks)-1])
}

// finishLookup runs cmd and feeds the callbook replies it returns into m.
func finishLookup(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range collectMsgs(cmd) {
		if msg, ok := msg.(entryCallbookMsg); ok {
			m, _ = update(t, m, msg)
		}
	}
	return m
}

// lookUp sends the last of ticks and feeds the lookup it starts back into m.
func lookUp(t *testing.T, m Model, ticks []entryCallbookTickMsg) Model {
	t.Helper()
	m, cmd := startLookup(t, m, ticks)
	return finishLookup(t, m, cmd)
}

// newLooker returns a mock callbook that answers each call in results, and
// reports every other call not found.
func newLooker(t *testing.T, results map[string]callbook.Result) *cbmock.MockLooker {
	t.Helper()
	l := cbmock.NewMockLooker(gomock.NewController(t))
	l.EXPECT().Lookup(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, call string) (callbook.Result, error) {
		if r, ok := results[call]; ok {
			return r, nil
		}
		return callbook.Result{}, callbook.ErrNotFound
	}).AnyTimes()
	return l
}

// failingLooker returns a mock callbook that fails every lookup with err.
func failingLooker(t *testing.T, err error) *cbmock.MockLooker {
	t.Helper()
	l := cbmock.NewMockLooker(gomock.NewController(t))
	l.EXPECT().Lookup(gomock.Any(), gomock.Any()).Return(callbook.Result{}, err).AnyTimes()
	return l
}

// setCall replaces the call with call, leaving focus on it.
func setCall(t *testing.T, m Model, call string) Model {
	t.Helper()
	m = focusOn(t, m, qsoentry.Call)
	m = press(t, m, clearKey)
	return typeCall(t, m, call)
}

func TestEntryCallbookFillsIn(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"W1AW": w1awResult, "VE3ABC": ve3abcResult})
	m, ticks := newCallbookModel(t, newWorkedStore(t), l)

	m = typeCall(t, m, "w1aw")
	assert.Equal(t, [6]string{}, callbookValues(m), "nothing is filled in before the wait ends")
	m = lookUp(t, m, *ticks)
	assert.Equal(t, [6]string{"Hiram Maxim", "Newington", "FN31pr", "CT", "CT,Hartford", "w1aw@arrl.org"},
		callbookValues(m), "the call's details are filled in")
	assert.Contains(t, entryCallLine(t, m), "QRZ.com", "the source is shown")

	m = setCall(t, m, "VE3ABC")
	m = lookUp(t, m, *ticks)
	assert.Equal(t, [6]string{"Alice", "Ottawa", "FN25", "", "", "ve3abc@example.com"},
		callbookValues(m), "a changed call replaces the details; a county without a state is no CNTY")
	assert.Contains(t, entryCallLine(t, m), "HamQTH")
}

func TestEntryCallbookWaitsForTyping(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	l.EXPECT().Lookup(gomock.Any(), "W1AW").Return(w1awResult, nil).Times(1)
	m, ticks := newCallbookModel(t, newWorkedStore(t), l)

	m = typeCall(t, m, "W1AW")
	require.Len(t, *ticks, 4, "each key press restarts the wait")
	for _, tick := range (*ticks)[:3] {
		var cmd tea.Cmd
		m, cmd = update(t, m, tick)
		m = finishLookup(t, m, cmd)
	}
	assert.Equal(t, [6]string{}, callbookValues(m), "a wait for an earlier call looks nothing up")

	m = lookUp(t, m, *ticks)
	assert.Equal(t, "Hiram Maxim", entryValue(m, qsoentry.Name))
}

func TestEntryCallbookKeepsTypedValues(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"W1AW": w1awResult, "VE3ABC": ve3abcResult})
	m, ticks := newCallbookModel(t, newWorkedStore(t), l)
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Name: "Bob"})

	m = setCall(t, m, "W1AW")
	m = lookUp(t, m, *ticks)
	assert.Equal(t, "Bob", entryValue(m, qsoentry.Name), "a typed name is kept")
	assert.Equal(t, "Newington", entryValue(m, qsoentry.QTH))

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Gridsquare: "FN42"})
	m = setCall(t, m, "VE3ABC")
	m = lookUp(t, m, *ticks)
	assert.Equal(t, [6]string{"Bob", "Ottawa", "FN42", "", "", "ve3abc@example.com"}, callbookValues(m),
		"typed values are kept and filled ones replaced")
}

func TestEntryCallbookClearsWhenNotFound(t *testing.T) {
	s := newWorkedStore(t, logbook.QSO{StartUTC: entryDaysAgo(3), Call: "ZZ1ZZ", Band: "20m", Mode: "CW", Name: "Zed"})
	m, ticks := newCallbookModel(t, s, newLooker(t, map[string]callbook.Result{"W1AW": w1awResult}))

	m = typeCall(t, m, "W1AW")
	m = lookUp(t, m, *ticks)
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.QTH: "Hartford"})
	m = setCall(t, m, "ZZ1ZZ")
	m = lookUp(t, m, *ticks)

	assert.Equal(t, [6]string{"", "Hartford", "", "", "", ""}, callbookValues(m),
		"filled values are cleared, a typed QTH kept and the logbook not used")
	line := entryCallLine(t, m)
	for _, s := range []string{"QRZ.com", "logbook", "failed"} {
		assert.NotContains(t, line, s, "no source or failure is shown")
	}
}

func TestEntryCallbookClearsWithoutCall(t *testing.T) {
	m, ticks := newCallbookModel(t, newWorkedStore(t), newLooker(t, map[string]callbook.Result{"W1AW": w1awResult}))
	m = typeCall(t, m, "W1AW")
	m = lookUp(t, m, *ticks)
	n := len(*ticks)

	m = press(t, m, clearKey)

	assert.Equal(t, [6]string{}, callbookValues(m), "filled values are cleared")
	assert.Len(t, *ticks, n, "no call is looked up")
	assert.NotContains(t, entryCallLine(t, m), "QRZ.com")
}

func TestEntryCallbookDropsStaleReply(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	// The callbook answers even once the lookup is canceled.
	l.EXPECT().Lookup(gomock.Any(), "W1AW").Return(w1awResult, nil).AnyTimes()
	m, ticks := newCallbookModel(t, newWorkedStore(t), l)
	m = typeCall(t, m, "W1AW")
	m, cmd := startLookup(t, m, *ticks)

	m = typeCall(t, m, "X")
	m = finishLookup(t, m, cmd)

	assert.Equal(t, [6]string{}, callbookValues(m), "a reply for an earlier call is dropped")
	assert.NotContains(t, entryCallLine(t, m), "QRZ.com")
}

func TestEntryCallbookCancels(t *testing.T) {
	tests := []struct {
		name string
		do   func(t *testing.T, m Model) Model
	}{
		{"call changes", func(t *testing.T, m Model) Model {
			return editAppend(t, m, qsoentry.Call, "X")
		}},
		{"screen closes", func(t *testing.T, m Model) Model {
			m = press(t, m, special(tea.KeyEscape))
			m = press(t, m, keyPress('y', 0))
			require.False(t, m.entry.open)
			return m
		}},
		{"QSO saved", func(t *testing.T, m Model) Model {
			m = press(t, m, saveKey)
			require.Contains(t, m.entry.msg, "Saved QSO")
			return m
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctxErr error
			l := cbmock.NewMockLooker(gomock.NewController(t))
			l.EXPECT().Lookup(gomock.Any(), "W1AW").DoAndReturn(func(ctx context.Context, call string) (callbook.Result, error) {
				ctxErr = ctx.Err()
				if ctxErr != nil {
					return callbook.Result{}, fmt.Errorf("callbook: lookup %s: %w", call, ctxErr)
				}
				return w1awResult, nil
			}).Times(1)
			m, ticks := newCallbookModel(t, newWorkedStore(t), l)
			m = fill(t, m, map[qsoentry.Field]string{
				qsoentry.Date: "2026-11-09", qsoentry.TimeOn: "1405", qsoentry.Mode: "cw", qsoentry.Freq: "7030",
			})
			m = editAppend(t, m, qsoentry.Call, "W1AW")
			m, cmd := startLookup(t, m, *ticks)

			m = tt.do(t, m)
			m = finishLookup(t, m, cmd)

			assert.ErrorIs(t, ctxErr, context.Canceled, "the lookup is canceled")
			if m.entry.open {
				assert.NotContains(t, entryCallLine(t, m), "failed", "a canceled lookup shows no failure")
				assert.Empty(t, entryValue(m, qsoentry.Name))
			}
		})
	}
}

func TestEntryCallbookShowsStatus(t *testing.T) {
	netErr := &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("connection refused")}
	limited := w1awResult
	limited.Limited = true
	tests := []struct {
		name   string
		res    callbook.Result
		err    error
		want   string
		failed bool
	}{
		{"QRZ.com", w1awResult, nil, "QRZ.com", false},
		{"limited", limited, nil, "QRZ.com (limited)", false},
		{"HamQTH", ve3abcResult, nil, "HamQTH", false},
		{"login failed", callbook.Result{}, fmt.Errorf("qrz: %w", callbook.ErrAuth), "callbook login failed", true},
		{"rate limit", callbook.Result{}, callbook.ErrRateLimit, "lookup limit reached", true},
		{"unreachable", callbook.Result{}, netErr, "callbook unreachable", true},
		{"unknown", callbook.Result{}, errors.New("callbook: strange reply"), "lookup failed", true},
		{"login before the rest", callbook.Result{}, errors.Join(netErr, callbook.ErrRateLimit, callbook.ErrAuth), "callbook login failed", true},
		{"rate limit before network", callbook.Result{}, errors.Join(netErr, callbook.ErrRateLimit), "lookup limit reached", true},
		{"network before not found", callbook.Result{}, errors.Join(callbook.ErrNotFound, netErr), "callbook unreachable", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := cbmock.NewMockLooker(gomock.NewController(t))
			l.EXPECT().Lookup(gomock.Any(), "W1AW").Return(tt.res, tt.err).AnyTimes()
			m, ticks := newCallbookModel(t, newWorkedStore(t), l)
			m = typeCall(t, m, "W1AW")
			m = lookUp(t, m, *ticks)

			assert.Contains(t, entryCallLine(t, m), tt.want)
			if tt.failed {
				assert.Contains(t, m.entryCallbook(), m.styles.err.Render(tt.want), "a failure is styled as an error")
			}
		})
	}
}

// fallbackQSOs are earlier QSOs with W1AW, oldest first: under its own
// call, then portable.
var fallbackQSOs = []logbook.QSO{
	{
		StartUTC: entryDaysAgo(20), Call: "W1AW", Band: "20m", Mode: "CW",
		Name: "Hiram", QTH: "Newington", Gridsquare: "FN31", State: "CT", Cnty: "CT,Hartford",
		Fields: map[string]string{"EMAIL": "old@arrl.org"},
	},
	{
		StartUTC: entryDaysAgo(10), Call: "W1AW/P", Band: "40m", Mode: "CW",
		Name: "Hiram P", QTH: "Field", Gridsquare: "FN42", State: "MA", Cnty: "MA,Middlesex",
		Fields: map[string]string{"EMAIL": "w1aw@arrl.org"},
	},
}

func TestEntryCallbookFallsBackToLogbook(t *testing.T) {
	tests := []struct {
		name   string
		looker func(t *testing.T) callbook.Looker
		status string
	}{
		{"no callbook", func(*testing.T) callbook.Looker { return nil }, "logbook"},
		{"every service fails", func(t *testing.T) callbook.Looker {
			return failingLooker(t, errors.Join(callbook.ErrNotFound, fmt.Errorf("hamqth: %w", callbook.ErrAuth)))
		}, "callbook login failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ticks := newCallbookModel(t, newWorkedStore(t, fallbackQSOs...), tt.looker(t))

			m = typeCall(t, m, "W1AW")
			m = lookUp(t, m, *ticks)
			assert.Equal(t, [6]string{"Hiram P", "", "", "", "", "w1aw@arrl.org"}, callbookValues(m),
				"the newest QSO with the base call gives the name and email, but not where a portable call was")
			line := entryCallLine(t, m)
			assert.Contains(t, line, "logbook")
			assert.Contains(t, line, tt.status)

			m = setCall(t, m, "W1AW/P")
			m = lookUp(t, m, *ticks)
			assert.Equal(t, [6]string{"Hiram P", "Field", "FN42", "MA", "MA,Middlesex", "w1aw@arrl.org"},
				callbookValues(m), "a QSO with the same call gives its location too")

			m = setCall(t, m, "K9XYZ")
			m = lookUp(t, m, *ticks)
			assert.Equal(t, [6]string{}, callbookValues(m), "a call never worked fills in nothing")
			assert.NotContains(t, entryCallLine(t, m), "logbook")
		})
	}
}

func TestEntryCallbookFailureStillSaves(t *testing.T) {
	s := newWorkedStore(t)
	m, ticks := newCallbookModel(t, s, failingLooker(t, callbook.ErrAuth))
	m = fill(t, m, map[qsoentry.Field]string{
		qsoentry.Date: "2026-11-09", qsoentry.TimeOn: "1405", qsoentry.Mode: "cw", qsoentry.Freq: "7030",
	})
	m = editAppend(t, m, qsoentry.Call, "W1AW")
	m = lookUp(t, m, *ticks)
	require.Contains(t, entryCallLine(t, m), "callbook login failed")

	m = press(t, m, saveKey)

	assert.Contains(t, m.entry.msg, "Saved QSO with W1AW")
	n, err := s.Count(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.NotContains(t, entryCallLine(t, m), "callbook login failed", "the next QSO starts clean")
}

func TestEditDoesNotLookUpCallbook(t *testing.T) {
	// The mock fails the test if it is asked anything.
	l := cbmock.NewMockLooker(gomock.NewController(t))
	m := openEditModel(t, newEditStore(t, editedQSO(t)), WithCallbook(l))
	ticks := new([]entryCallbookTickMsg)
	m.tick = callbookTicks(t, ticks)
	name := entryValue(m, qsoentry.Name)

	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Call: "VE3ABC"})

	assert.Empty(t, *ticks, "an edited QSO's call is not looked up")
	assert.Equal(t, name, entryValue(m, qsoentry.Name))
}

func TestEntryCallbookStartsWhenFocusLeavesTheCall(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	l.EXPECT().Lookup(gomock.Any(), "W1AW").Return(w1awResult, nil).Times(1)
	m, ticks := newCallbookModel(t, newWorkedStore(t), l)
	m = typeCall(t, m, "W1AW")
	require.NotEmpty(t, *ticks)

	m, cmd := update(t, m, tab())
	require.NotEqual(t, qsoentry.Call, m.entry.focus)
	m = finishLookup(t, m, cmd)
	assert.Equal(t, "Hiram Maxim", entryValue(m, qsoentry.Name), "looked up without waiting")

	m, cmd = update(t, m, (*ticks)[len(*ticks)-1])
	m = finishLookup(t, m, cmd)
	assert.Equal(t, "Hiram Maxim", entryValue(m, qsoentry.Name), "the wait's end looks nothing up again")
}

func TestEntryCallbookStartsAtOnceOffTheCall(t *testing.T) {
	m, ticks := newCallbookModel(t, newWorkedStore(t), newLooker(t, map[string]callbook.Result{"W1AW": w1awResult}))
	m = focusOn(t, m, qsoentry.Name)
	// A call set while another field has focus, as no key press in it does.
	m.entry.input(qsoentry.Call).SetValue("W1AW")

	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = finishLookup(t, m, cmd)

	assert.Empty(t, *ticks, "no wait")
	assert.Equal(t, "Hiram Maxim", entryValue(m, qsoentry.Name))
}

func TestNotFoundOnly(t *testing.T) {
	notFound := fmt.Errorf("qrz: lookup W1AW: %w", callbook.ErrNotFound)
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"not found", notFound, true},
		{"every service not found", errors.Join(notFound, fmt.Errorf("hamqth: %w", callbook.ErrNotFound)), true},
		{"one service failed", errors.Join(notFound, fmt.Errorf("hamqth: %w", callbook.ErrAuth)), false},
		{"nested join", errors.Join(errors.Join(notFound, notFound), notFound), true},
		{"other error", callbook.ErrRateLimit, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, notFoundOnly(tt.err))
		})
	}
}
