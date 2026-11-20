package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// wheelLines is the number of lines one step of the mouse wheel scrolls.
const wheelLines = 3

// mouseMode returns the mouse reporting the screen needs: clicks and the
// wheel whenever a logbook is open, for the index's rows, the pager's strip
// and the tab bars and scrolling of the pager and the New QSO screen, and
// none without a logbook, which leaves the terminal its own text selection
// and wheel.
func (m Model) mouseMode() tea.MouseMode {
	if m.logbook != nil {
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
// New QSO screen, which opens the tab as the tab keys do, on a QSO in the
// index, which opens it in the pager as enter does, or on a QSO in the
// strip above the pager, which moves the cursor to it as j and k do. Other
// clicks do nothing, as do clicks while a prompt is open or a prefix is
// armed.
func (m Model) click(mouse tea.Mouse) (Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft || m.logbook == nil || m.promptOpen() ||
		m.idx.prefix || m.pager.lookupPrefix {
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
		if y < m.idx.height {
			return m.clickStrip(y), nil
		}
		if y != m.idx.height {
			return m, nil
		}
		if i := tabAt(pagerTabNames[:], m.bodyWidth(), x); i >= 0 {
			m.pager.tab, m.pager.scroll = pagerTab(i), 0
		}
	default:
		return m.clickIndex(y)
	}
	return m, nil
}

// clickIndex opens the QSO on line y of the index's body in the pager, as
// moving to it and pressing enter does. Line 0 is the column header, which
// does nothing, as do the empty lines below the last QSO.
func (m Model) clickIndex(y int) (Model, tea.Cmd) {
	row := y - 1
	if !m.idx.loaded || m.idx.total == 0 || row < 0 || row >= m.idx.visible() {
		return m, nil
	}
	m.idx.moveTo(m.idx.top + row)
	return m.openPager(), nil
}

// clickStrip moves the cursor to the QSO on line y of the index strip above
// the pager, as j and k do. Another QSO starts at the top of the open tab;
// clicking the cursor's own row leaves the scroll alone, as do the empty
// lines below the last QSO.
func (m Model) clickStrip(y int) Model {
	if y >= m.idx.visible() {
		return m
	}
	cursor := m.idx.cursor
	m.idx.moveTo(m.idx.top + y)
	if m.idx.cursor != cursor {
		m.pager.scroll = 0
	}
	return m
}

// wheel scrolls the open tab of the pager, or the New QSO screen's form,
// wheelLines lines up or down, or moves the index's cursor wheelLines rows,
// stopping silently at the first and last QSOs, with the view following.
// It does nothing while a prompt is open.
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
	default:
		m.idx.moveTo(m.idx.cursor + step)
	}
	return m
}
