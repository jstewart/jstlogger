package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	cbmock "jstlogger/internal/callbook/mock"
	"jstlogger/internal/logbook"
)

var bulkSyncKey = keyPress('c', 0)

// k4aResult and k5aResult are what QRZ.com returns for the calls of QSOs 4
// and 5 of newPatternStore.
var (
	k4aResult = callbook.Result{
		Call: "K4A", Name: "Dana", QTH: "Atlanta", Grid: "EM73", State: "GA", Service: callbook.ServiceQRZ,
	}
	k5aResult = callbook.Result{
		Call: "K5A", Name: "Eve", QTH: "Austin", Grid: "EM10", State: "TX", Service: callbook.ServiceQRZ,
	}
)

// newSyncModel returns a model on a logbook of n QSOs, looking calls up in
// l unless it is nil, with QSOs n and n-1 marked and the cursor on the row
// below them.
func newSyncModel(t *testing.T, n int, l callbook.Looker) (Model, *logbook.SQLiteStore) {
	t.Helper()
	s := newPatternStore(t, n)
	opts := []Option{WithLogbook(s)}
	if l != nil {
		opts = append(opts, WithCallbook(l))
	}
	m := New(opts...)
	m = runIndex(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m = runIndex(t, m, cmd)
	m = press(t, m, toggleKey, toggleKey)
	require.Len(t, m.idx.marks, 2)
	return m, s
}

// startSync asks to sync the marked QSOs, answers yes, and returns the
// command of the first step, without running it.
func startSync(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m = press(t, m, prefixKey, bulkSyncKey)
	require.Equal(t, promptConfirm, m.prompt)
	m, cmd := update(t, m, keyPress('y', 0))
	require.NotNil(t, m.sync, "the sync runs")
	return m, cmd
}

// nextStep runs cmd and returns the sync step it reports.
func nextStep(t *testing.T, cmd tea.Cmd) callbookSyncStepMsg {
	t.Helper()
	var steps []callbookSyncStepMsg
	for _, msg := range collectMsgs(cmd) {
		if s, ok := msg.(callbookSyncStepMsg); ok {
			steps = append(steps, s)
		}
	}
	require.Len(t, steps, 1, "one lookup at a time")
	return steps[0]
}

// runSync runs cmd and the steps of the sync that follow it, and the index
// loads they lead to.
func runSync(t *testing.T, m Model, cmd tea.Cmd) Model {
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
		case callbookSyncStepMsg, indexLoadedMsg, indexBlockMsg, indexBulkMsg:
			var next tea.Cmd
			m, next = update(t, m, msg)
			queue = append(queue, next)
		}
	}
	return m
}

// getQSO returns the stored QSO with ID id.
func getQSO(t *testing.T, s logbook.Store, id int64) logbook.QSO {
	t.Helper()
	q, err := s.Get(t.Context(), id)
	require.NoError(t, err)
	return q
}

func TestSyncConfirmation(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	m, _ := newSyncModel(t, 5, l)

	m = press(t, m, prefixKey, bulkSyncKey)

	assert.Equal(t, promptConfirm, m.prompt)
	assert.Contains(t, plain(m.footerView()), "Look up 2 QSOs in the callbook? [y/N]")
}

func TestSyncCancelAsks(t *testing.T) {
	// The mock expects no lookups.
	l := cbmock.NewMockLooker(gomock.NewController(t))
	m, _ := newSyncModel(t, 5, l)
	m = press(t, m, prefixKey, bulkSyncKey)

	m, cmd := update(t, m, keyPress('n', 0))

	assert.Equal(t, promptNone, m.prompt)
	assert.Empty(t, collectMsgs(cmd))
	assert.Nil(t, m.sync)
	assert.Len(t, m.idx.marks, 2)
}

func TestSyncFillsInMarkedQSOs(t *testing.T) {
	ctrl := gomock.NewController(t)
	l := cbmock.NewMockLooker(ctrl)
	gomock.InOrder(
		l.EXPECT().Lookup(gomock.Any(), "K4A").Return(k4aResult, nil),
		l.EXPECT().Lookup(gomock.Any(), "K5A").Return(k5aResult, nil),
	)
	m, s := newSyncModel(t, 5, l)
	// QSO 5's name was entered by hand.
	q5 := getQSO(t, s, 5)
	q5.Name = "Evelyn"
	require.NoError(t, s.Update(t.Context(), q5))

	m, cmd := startSync(t, m)
	step := nextStep(t, cmd)
	assert.Equal(t, int64(4), step.id, "the lowest ID comes first")
	m, cmd = update(t, m, step)
	assert.Contains(t, statusLine(t, m), "Looking up 2/2… esc to stop")
	m = runSync(t, m, cmd)

	assert.Nil(t, m.sync, "the sync is done")
	assert.Empty(t, m.idx.marks, "the synced QSOs are unmarked")
	assert.Equal(t, "QSO 3/5 · 2 updated", statusLine(t, m))
	q4 := getQSO(t, s, 4)
	assert.Equal(t, "Dana", q4.Name)
	assert.Equal(t, "EM73", q4.Gridsquare)
	q5 = getQSO(t, s, 5)
	assert.Equal(t, "Evelyn", q5.Name, "the name entered by hand is kept")
	assert.Equal(t, "Austin", q5.QTH)
}

func TestSyncUnchanged(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, cmd := startSync(t, m)
	m = runSync(t, m, cmd)
	require.Empty(t, m.idx.marks)

	// Mark the synced QSOs again and sync them once more.
	m = press(t, m, keyPress('k', 0), keyPress('k', 0), toggleKey, toggleKey)
	require.Len(t, m.idx.marks, 2)
	m, cmd = startSync(t, m)
	m = runSync(t, m, cmd)

	assert.Contains(t, statusLine(t, m), "2 unchanged")
	assert.Empty(t, m.idx.marks)
}

func TestSyncNotFoundStaysMarked(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)

	m, cmd := startSync(t, m)
	m = runSync(t, m, cmd)

	assert.Equal(t, map[int64]bool{4: true}, m.idx.marks, "the QSO not found stays marked")
	assert.Equal(t, "QSO 3/5 · 1 marked · 1 updated, 1 not found", statusLine(t, m))
}

func TestSyncStopsWhenUnreachable(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	netErr := &callbook.NetworkError{Service: callbook.ServiceQRZ, Err: errors.New("connection refused")}
	l.EXPECT().Lookup(gomock.Any(), gomock.Any()).Return(callbook.Result{}, netErr).Times(1)
	m, _ := newSyncModel(t, 5, l)

	m, cmd := startSync(t, m)
	m = runSync(t, m, cmd)

	assert.Nil(t, m.sync)
	assert.Len(t, m.idx.marks, 2, "both stay marked")
	assert.Equal(t, "QSO 3/5 · 2 marked · stopped: callbook unreachable", statusLine(t, m))
}

func TestSyncOtherFailureContinues(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	gomock.InOrder(
		l.EXPECT().Lookup(gomock.Any(), "K4A").Return(callbook.Result{}, errors.New("bad reply")),
		l.EXPECT().Lookup(gomock.Any(), "K5A").Return(k5aResult, nil),
	)
	m, _ := newSyncModel(t, 5, l)

	m, cmd := startSync(t, m)
	m = runSync(t, m, cmd)

	assert.Equal(t, map[int64]bool{4: true}, m.idx.marks)
	assert.Equal(t, "QSO 3/5 · 1 marked · 1 updated, 1 failed", statusLine(t, m))
}

func TestSyncProgressSurvivesKeys(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)

	m, cmd := startSync(t, m)
	assert.Contains(t, statusLine(t, m), "Looking up 1/2… esc to stop")
	m = press(t, m, keyPress('j', 0))
	assert.Contains(t, statusLine(t, m), "Looking up 1/2… esc to stop", "a key press keeps the progress")

	m = runSync(t, m, cmd)
	assert.NotContains(t, statusLine(t, m), "Looking up")
}

func TestSyncProgressInPager(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, _ = startSync(t, m)

	m = press(t, m, enterKey)

	require.True(t, m.pager.open)
	assert.Contains(t, plain(m.View().Content), "Looking up 1/2… esc to stop")
}

func TestSyncEscStops(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, s := newSyncModel(t, 5, l)
	m, cmd := startSync(t, m)
	ctx := m.sync.ctx

	m, escCmd := update(t, m, esc())

	assert.Error(t, ctx.Err(), "esc cancels the lookup under way")
	assert.Empty(t, collectMsgs(escCmd), "esc does nothing else")
	assert.NotNil(t, m.sync, "the sync waits for the lookup under way")
	assert.Contains(t, statusLine(t, m), "Stopping…")

	m = runSync(t, m, cmd)

	assert.Nil(t, m.sync)
	assert.Len(t, m.idx.marks, 2)
	assert.Equal(t, "QSO 3/5 · 2 marked · stopped: canceled", statusLine(t, m))
	assert.Empty(t, getQSO(t, s, 5).Name, "the rest are not looked up")
}

func TestSyncEscInPagerDoesNotClose(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, _ = startSync(t, m)
	m = press(t, m, enterKey)
	require.True(t, m.pager.open)

	m = press(t, m, esc())

	assert.True(t, m.pager.open, "esc stops the sync, not the pager")
	assert.True(t, m.sync.canceled)
}

func TestSyncCancelRaceCountsResult(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, cmd := startSync(t, m)
	// The lookup finishes before esc is pressed.
	step := nextStep(t, cmd)

	m = press(t, m, esc())
	m = runSync(t, m, func() tea.Msg { return step })

	assert.Equal(t, map[int64]bool{5: true}, m.idx.marks)
	assert.Equal(t, "QSO 3/5 · 1 marked · 1 updated, stopped: canceled", statusLine(t, m))
}

func TestSyncDropsStaleStep(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, cmd := startSync(t, m)
	step := nextStep(t, cmd)

	stale := step
	stale.gen--
	m, staleCmd := update(t, m, stale)

	assert.Empty(t, collectMsgs(staleCmd))
	assert.Equal(t, 0, m.sync.next, "the stale step is dropped")
	assert.Contains(t, statusLine(t, m), "Looking up 1/2")
}

func TestSyncAlreadyRunning(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, _ = startSync(t, m)

	m = press(t, m, prefixKey, bulkSyncKey)

	assert.Equal(t, promptNone, m.prompt, "nothing is asked")
	assert.Contains(t, statusLine(t, m), "A callbook sync is already running.")
}

func TestSyncWithoutCallbook(t *testing.T) {
	m, _ := newSyncModel(t, 5, nil)

	assert.NotContains(t, helpText(m), "sync marked with callbook")
	m = press(t, m, enterKey)
	require.True(t, m.pager.open)
	assert.NotContains(t, helpText(m), "sync marked with callbook", "nor in the pager")
	m = press(t, m, esc())

	m = press(t, m, prefixKey, bulkSyncKey)

	assert.Equal(t, promptNone, m.prompt)
	assert.Nil(t, m.sync)
}

func TestSyncInPagerHelp(t *testing.T) {
	l := cbmock.NewMockLooker(gomock.NewController(t))
	m, _ := newSyncModel(t, 5, l)

	m = press(t, m, enterKey)

	require.True(t, m.pager.open)
	assert.Regexp(t, `;c +sync marked with callbook`, helpText(m))
}

func TestSyncEscOnNewQSOScreen(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, _ = startSync(t, m)
	m = press(t, m, newQSOKey)
	require.True(t, m.entry.open)

	m = press(t, m, esc())

	assert.False(t, m.entry.open, "esc closes the New QSO screen")
	require.NotNil(t, m.sync)
	assert.False(t, m.sync.canceled, "the sync keeps running")
}

func TestSyncEscCancelsPrefixFirst(t *testing.T) {
	l := newLooker(t, map[string]callbook.Result{"K4A": k4aResult, "K5A": k5aResult})
	m, _ := newSyncModel(t, 5, l)
	m, _ = startSync(t, m)
	m = press(t, m, prefixKey)
	require.True(t, m.idx.prefix)

	m = press(t, m, esc())

	assert.False(t, m.idx.prefix)
	assert.False(t, m.sync.canceled, "esc cancels the prefix, not the sync")
}
