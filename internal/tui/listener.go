package tui

import tea "charm.land/bubbletea/v2"

// LogbookChanged reports that background work, such as a UDP listener,
// added QSOs to the logbook.
type LogbookChanged struct{}

// refreshIndex reloads the index after a background change to the logbook,
// keeping the cursor on its QSO, without a message or a redraw. While a new
// limit or sort loads, it marks the index stale instead, and the reload
// follows that load (see refreshStale).
func (m Model) refreshIndex() (Model, tea.Cmd) {
	if m.logbook == nil {
		return m, nil
	}
	if m.idx.settling {
		m.idx.stale = true
		return m, nil
	}
	id := m.idx.restart()
	return m, loadIndexCmd(m.logbook, m.idx.gen, m.idx.limit.expr, m.idx.sort, id, m.idx.cursor)
}

// refreshStale returns cmd, along with a reload of the index when stale
// reports that the logbook changed while a new limit or sort loaded.
func (m Model) refreshStale(stale bool, cmd tea.Cmd) (Model, tea.Cmd) {
	if !stale {
		return m, cmd
	}
	m, reload := m.refreshIndex()
	return m, tea.Batch(cmd, reload)
}
