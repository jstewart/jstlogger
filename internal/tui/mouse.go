package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// wheelLines is the number of lines one step of the mouse wheel scrolls.
const wheelLines = 3

// mouseMode returns the mouse reporting the screen needs: clicks and the
// wheel while the pager or the New QSO screen is open, for their tab bars
// and scrolling, and none on the index, which keeps the terminal's own
// text selection and wheel.
func (m Model) mouseMode() tea.MouseMode {
	if m.logbook != nil && (m.pager.open || m.entry.open) {
		return tea.MouseModeCellMotion
	}
	return tea.MouseModeNone
}

// bodyCell returns the cell of the body's content under the mouse, and
// whether the mouse is on it rather than the header, footer or padding.
func (m Model) bodyCell(mouse tea.Mouse) (x, y int, ok bool) {
	x = mouse.X - m.styles.body.GetPaddingLeft()
	y = mouse.Y - lipgloss.Height(m.headerView()) - m.styles.body.GetPaddingTop()
	return x, y, x >= 0 && x < m.bodyWidth() && y >= 0 && y < m.bodyHeight()
}

// click handles a left click on a tab in the tab bar of the pager or the
// New QSO screen, which opens the tab as the tab keys do. Other clicks do
// nothing, as do clicks while a prompt is open.
func (m Model) click(mouse tea.Mouse) (Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft || m.logbook == nil || m.promptOpen() {
		return m, nil
	}
	x, y, ok := m.bodyCell(mouse)
	// The body's last line is the status line, which a short window
	// shows in place of the tab bar.
	if !ok || y >= m.bodyHeight()-1 {
		return m, nil
	}
	switch {
	case m.entry.open:
		e := &m.entry
		if y != 0 || e.saving {
			return m, nil
		}
		if i := tabAt(e.tabNames(), m.bodyWidth(), x); i >= 0 {
			e.msg, e.wheeled = "", false
			return m, e.goTab(e.tabs()[i])
		}
	case m.pager.open:
		if y != m.idx.height {
			return m, nil
		}
		if i := tabAt(pagerTabNames[:], m.bodyWidth(), x); i >= 0 {
			m.pager.tab, m.pager.scroll = pagerTab(i), 0
		}
	}
	return m, nil
}

// wheel scrolls the open tab of the pager, or the New QSO screen's form,
// wheelLines lines up or down. It does nothing while a prompt is open.
func (m Model) wheel(mouse tea.Mouse) Model {
	if m.logbook == nil || m.promptOpen() {
		return m
	}
	var step int
	switch mouse.Button {
	case tea.MouseWheelDown:
		step = wheelLines
	case tea.MouseWheelUp:
		step = -wheelLines
	default:
		return m
	}
	switch {
	case m.entry.open:
		m.wheelEntry(step)
	case m.pager.open:
		bottom := m.pagerBottom()
		m.pager.scroll = min(max(min(m.pager.scroll, bottom)+step, 0), bottom)
	}
	return m
}
