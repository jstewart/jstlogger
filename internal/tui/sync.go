package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/callinfo"
	"jstlogger/internal/logbook"
)

// msgSyncRunning is the status message for ;c while a sync of the marked
// QSOs with the callbook runs.
const msgSyncRunning = "A callbook sync is already running."

// syncStop stops the sync of the marked QSOs with the callbook.
var syncStop = key.NewBinding(key.WithKeys("esc"))

// callbookSync is a run of ;c: the marked QSOs looked up in the callbook,
// one at a time, and filled in.
type callbookSync struct {
	// gen numbers the runs, so a step of an earlier one is dropped.
	gen int
	// ids are the QSOs to look up, and next indexes the one under way.
	ids  []int64
	next int
	// looker looks the QSOs up in the callbook alone.
	looker callinfo.Looker
	// ctx is canceled, by cancel, to stop the run.
	ctx    context.Context
	cancel context.CancelFunc
	// counts holds the number of QSOs with each outcome.
	counts [callinfo.SyncFailed + 1]int
	// synced are the IDs of the QSOs updated or found unchanged, which
	// are unmarked when the run finishes.
	synced []int64
	// stopErr is the error that stopped the run before its last QSO, and
	// canceled reports that esc stopped it.
	stopErr  error
	canceled bool
}

// callbookSyncStepMsg reports the outcome of looking up the QSO with ID id
// in run gen of the sync.
type callbookSyncStepMsg struct {
	gen     int
	id      int64
	outcome callinfo.SyncOutcome
	err     error
}

// askSync asks whether to look up the marked QSOs in the callbook. A yes
// looks up the QSOs the question counted. Only one sync runs at a time.
func (m Model) askSync(ids []int64) (Model, tea.Cmd) {
	if m.sync != nil {
		m.idx.msg = msgSyncRunning
		return m, nil
	}
	q := fmt.Sprintf("Look up %s in the callbook? [y/N]", countQSOs(len(ids)))
	return m.askConfirm(q, func(m Model) (Model, tea.Cmd) {
		return m.startSync(ids)
	}), nil
}

// startSync starts looking up the QSOs with the given IDs in the callbook,
// the first of them at once.
func (m Model) startSync(ids []int64) (Model, tea.Cmd) {
	if m.sync != nil {
		m.idx.msg = msgSyncRunning
		return m, nil
	}
	m.syncGen++
	ctx, cancel := context.WithCancel(context.Background())
	m.sync = &callbookSync{
		gen: m.syncGen,
		ids: ids,
		looker: callinfo.Looker{
			DXCC:            m.dxcc,
			Callbook:        m.callbook,
			CallbookTimeout: callbookTimeout,
		},
		ctx:    ctx,
		cancel: cancel,
	}
	m.idx.progress = m.sync.progress()
	return m, m.sync.stepCmd(m.logbook)
}

// progress returns the status of the run, e.g. "Looking up 37/120… esc to
// stop".
func (r *callbookSync) progress() string {
	return "Looking up " + formatCount(r.next+1) + "/" + formatCount(len(r.ids)) + "… esc to stop"
}

// stepCmd returns a command that looks up the QSO under way in s.
func (r *callbookSync) stepCmd(s logbook.Store) tea.Cmd {
	return syncStepCmd(r.ctx, r.looker, s, r.gen, r.ids[r.next])
}

// syncStepCmd returns a command that fills in the stored QSO with ID id
// from the callbook, for run gen of the sync. The step is bounded by the
// callbook lookup's timeout and logbookTimeout each for loading and saving
// the QSO, so a stuck logbook doesn't stall the run.
func syncStepCmd(ctx context.Context, l callinfo.Looker, s logbook.Store, gen int, id int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, l.CallbookTimeout+2*logbookTimeout)
		defer cancel()
		outcome, err := l.Sync(ctx, s, id)
		return callbookSyncStepMsg{gen: gen, id: id, outcome: outcome, err: err}
	}
}

// stopSync stops the sync on esc. The lookup under way is canceled, and the
// run finishes once it returns.
func (m Model) stopSync() Model {
	m.sync.cancel()
	m.sync.canceled = true
	m.idx.progress = "Stopping…"
	return m
}

// applySyncStep records the outcome of a step of the running sync, then
// looks up the next QSO or finishes. An error that would fail every QSO
// left, such as an unreachable callbook, stops the run; any other failure
// is counted and the run goes on. A step of an earlier run is dropped.
func (m Model) applySyncStep(msg callbookSyncStepMsg) (Model, tea.Cmd) {
	r := m.sync
	if r == nil || msg.gen != r.gen || r.next >= len(r.ids) || msg.id != r.ids[r.next] {
		return m, nil
	}
	switch {
	case msg.err != nil && callinfo.StopsSync(msg.err):
		if !errors.Is(msg.err, context.Canceled) {
			r.stopErr = msg.err
		}
		return m.finishSync()
	case msg.err != nil:
		r.counts[callinfo.SyncFailed]++
	default:
		r.counts[msg.outcome]++
		if msg.outcome == callinfo.SyncUpdated || msg.outcome == callinfo.SyncUnchanged {
			r.synced = append(r.synced, msg.id)
		}
	}
	r.next++
	if r.canceled || r.next >= len(r.ids) {
		return m.finishSync()
	}
	m.idx.progress = r.progress()
	return m, r.stepCmd(m.logbook)
}

// finishSync ends the running sync and reports it: the synced QSOs are
// unmarked, and the index reloads when any was updated. The QSOs not
// found, failed or not reached stay marked, so a sync again retries them.
func (m Model) finishSync() (Model, tea.Cmd) {
	r := m.sync
	r.cancel()
	m.sync = nil
	m.idx.progress = ""
	return m.applyBulk(indexBulkMsg{
		status: r.summary(),
		unmark: r.synced,
		reload: r.counts[callinfo.SyncUpdated] > 0,
	})
}

// summary returns the outcome of the run, e.g. "84 updated, 12 unchanged,
// 3 not found, stopped: callbook unreachable".
func (r *callbookSync) summary() string {
	var parts []string
	for _, c := range []struct {
		outcome callinfo.SyncOutcome
		label   string
	}{
		{callinfo.SyncUpdated, "updated"},
		{callinfo.SyncUnchanged, "unchanged"},
		{callinfo.SyncNotFound, "not found"},
		{callinfo.SyncFailed, "failed"},
	} {
		if n := r.counts[c.outcome]; n > 0 {
			parts = append(parts, formatCount(n)+" "+c.label)
		}
	}
	switch {
	case r.canceled:
		parts = append(parts, "stopped: canceled")
	case r.stopErr != nil:
		why := callbookFailure(r.stopErr)
		if why == "" {
			why = "lookup failed"
		}
		parts = append(parts, "stopped: "+why)
	}
	return strings.Join(parts, ", ")
}
