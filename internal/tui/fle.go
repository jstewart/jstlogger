package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/callinfo"
	"jstlogger/internal/editor"
	"jstlogger/internal/fle"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

const (
	// fleBadge ends the FLE dialog's title line.
	fleBadge = " esc cancel "
	// fleChrome is the number of lines of the dialog, inside its frame,
	// around the rows: the title and a blank line above, and a blank line
	// and the hint below.
	fleChrome = 4
	// fleColumnGap separates the columns of the QSO rows.
	fleColumnGap = 2
	// fleMinWidth is the narrowest the dialog is drawn, in a body at least
	// that wide.
	fleMinWidth = 40

	// msgFLESaving is the status message for A while QSOs entered as FLE
	// are saved.
	msgFLESaving = "FLE QSOs are being saved."
	// msgFLEEmpty is the status message for an FLE text with no lines.
	msgFLEEmpty = "No QSOs entered."
	// msgFLEDiscarded is the status message for an FLE text canceled in
	// the dialog.
	msgFLEDiscarded = "FLE discarded."
)

// fleKeyMap holds the keybindings of the open FLE dialog. No other key
// reaches the screen behind it.
type fleKeyMap struct {
	// Save is enabled only when the text has a QSO to save.
	Save     key.Binding
	Edit     key.Binding
	Cancel   key.Binding
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Help     key.Binding
	Quit     key.Binding
}

var fleKeys = fleKeyMap{
	Save:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save QSOs")),
	Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit text")),
	Cancel:   key.NewBinding(key.WithKeys("esc", "q"), key.WithHelp("esc/q", "cancel")),
	Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("j/k/↑/↓", "scroll")),
	Down:     key.NewBinding(key.WithKeys("j", "down")),
	PageUp:   key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup/pgdn", "page")),
	PageDown: key.NewBinding(key.WithKeys("pgdown")),
	Help: key.NewBinding(
		key.WithKeys("?", "f1"),
		key.WithHelp("?/f1", "help"),
	),
	Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}

func (k fleKeyMap) ShortHelp() []key.Binding {
	// Save, edit and cancel come first so a narrow window still shows
	// them.
	return []key.Binding{k.Save, k.Edit, k.Cancel, k.Up, k.Help}
}

func (k fleKeyMap) FullHelp() [][]key.Binding {
	return sectionKeys(k.sections())
}

func (k fleKeyMap) sections() []helpSection {
	return []helpSection{
		{"FLE", []key.Binding{k.Save, k.Edit, k.Cancel, k.Help, k.Quit}},
		{"scroll", []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown}},
	}
}

// fleDialog shows the QSOs parsed from text entered as Fast Log Entry, and
// the lines that could not be read, over the body while open, to save or
// edit them.
type fleDialog struct {
	open bool
	// text is the text as the editor left it, which e opens again.
	text string
	// result is what fle.Parse read from text.
	result fle.Result
	// scroll is the index in rows of the first row shown.
	scroll int
}

// fleEditedMsg reports that the editor opened on FLE text exited, with
// err when it failed, leaving the text in the file at path.
type fleEditedMsg struct {
	path string
	err  error
}

// fleSavedMsg reports the save of QSOs entered as FLE: how many were added
// and skipped as duplicates, and how many were tagged callinfo.PendingTag,
// or err when the save failed.
type fleSavedMsg struct {
	res     logbook.ImportResult
	pending int
	err     error
}

// keys returns the dialog's keybindings, save enabled only when there is a
// QSO to save.
func (d fleDialog) keys() fleKeyMap {
	k := fleKeys
	k.Save.SetEnabled(len(d.result.QSOs) > 0)
	return k
}

// startFLE opens the user's editor on an empty text to enter QSOs as FLE,
// unless QSOs entered earlier are still being saved.
func (m Model) startFLE() (Model, tea.Cmd) {
	if m.fleSaving {
		m.idx.msg = msgFLESaving
		return m, nil
	}
	// No earlier text is kept, so an editor that fails brings no dialog
	// back.
	m.fle = fleDialog{}
	return m.openFLEEditor("")
}

// openFLEEditor opens text in the user's editor, suspending the screen
// until it exits, and closes the FLE dialog, keeping its text and result
// for an editor that fails. A failure to start it is shown on the status
// line, leaving the dialog as it was.
func (m Model) openFLEEditor(text string) (Model, tea.Cmd) {
	argv, err := editor.Command(m.getenv)
	if err != nil {
		m.idx.msg = fmt.Sprintf("open editor: %v", err)
		return m, nil
	}
	cmd, path, err := editor.Prepare(argv, text)
	if err != nil {
		m.idx.msg = fmt.Sprintf("open editor: %v", err)
		return m, nil
	}
	m.fle.open, m.fle.scroll = false, 0
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return fleEditedMsg{path: path, err: err}
	})
}

// applyFLEEdited reads back the text the editor left and opens the FLE
// dialog on what it parses to. The file is always removed. An editor that
// failed, or a file that could not be read, shows its error on the status
// line and, when it was opened with e, brings the dialog back on the text
// it was opened with, so nothing entered is lost. A text of blank lines
// only enters nothing.
func (m Model) applyFLEEdited(msg fleEditedMsg) (Model, tea.Cmd) {
	text, readErr := editor.ReadBack(msg.path)
	if err := errors.Join(msg.err, readErr); err != nil {
		m.idx.msg = fmt.Sprintf("edit FLE: %v", err)
		m.fle.open = m.fle.text != ""
		return m, nil
	}
	if strings.TrimSpace(text) == "" {
		m.fle = fleDialog{}
		m.idx.msg = msgFLEEmpty
		return m, nil
	}
	var station qsoentry.Station
	if len(m.stations) > 0 {
		station = m.stations[0]
	}
	m.fle = fleDialog{
		open:   true,
		text:   text,
		result: fle.Parse(text, fle.Options{Now: m.now(), Station: station}),
	}
	return m, nil
}

// updateFLE handles a key press while the FLE dialog is open. No key
// reaches the screen behind it, but ctrl+c still quits.
func (m Model) updateFLE(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	d := &m.fle
	k := d.keys()
	listHeight := m.fleListHeight()
	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		return m.openHelp(), nil
	case key.Matches(msg, k.Cancel):
		m.fle = fleDialog{}
		m.idx.msg = msgFLEDiscarded
	case key.Matches(msg, k.Edit):
		return m.openFLEEditor(d.text)
	case key.Matches(msg, k.Save):
		return m.saveFLE()
	case key.Matches(msg, k.Up):
		d.scrollBy(-1, m.fleRowCount(), listHeight)
	case key.Matches(msg, k.Down):
		d.scrollBy(1, m.fleRowCount(), listHeight)
	case key.Matches(msg, k.PageUp):
		d.scrollBy(-listHeight, m.fleRowCount(), listHeight)
	case key.Matches(msg, k.PageDown):
		d.scrollBy(listHeight, m.fleRowCount(), listHeight)
	}
	return m, nil
}

// scrollBy scrolls the rows n rows down, or up for a negative n, stopping
// at the first and last of total rows, for a window of listHeight rows.
func (d *fleDialog) scrollBy(n, total, listHeight int) {
	bottom := max(total-listHeight, 0)
	d.scroll = min(max(min(d.scroll, bottom)+n, 0), bottom)
}

// wheelFLE scrolls the open FLE dialog step rows.
func (m Model) wheelFLE(step int) Model {
	m.fle.scrollBy(step, m.fleRowCount(), m.fleListHeight())
	return m
}

// saveFLE closes the dialog and saves its QSOs, filled in from the DXCC
// table, the callbook and the logbook, showing the progress on the status
// line.
func (m Model) saveFLE() (Model, tea.Cmd) {
	qsos := make([]logbook.QSO, len(m.fle.result.QSOs))
	for i, q := range m.fle.result.QSOs {
		qsos[i] = q.QSO
	}
	m.fle = fleDialog{}
	m.fleSaving = true
	m.idx.progress = "Saving " + countQSOs(len(qsos)) + "…"
	looker := callinfo.Looker{
		DXCC:            m.dxcc,
		Callbook:        m.callbook,
		Logbook:         m.logbook,
		CallbookTimeout: callbookTimeout,
		LogbookTimeout:  logbookTimeout,
	}
	return m, saveFLECmd(context.Background(), looker, m.logbook, qsos)
}

// saveFLECmd returns a command that fills in each of qsos with l, in turn,
// and imports them into s, skipping duplicates. A QSO whose lookup finds
// the callbook unreachable is tagged callinfo.PendingTag, and so is each
// after it, which is not looked up in the callbook, so a batch doesn't
// wait out a timeout for each QSO. The import is bounded by
// logbookTimeout.
func saveFLECmd(ctx context.Context, l callinfo.Looker, s logbook.Store, qsos []logbook.QSO) tea.Cmd {
	return func() tea.Msg {
		filled, pending := fillFLE(ctx, l, qsos)
		ictx, cancel := context.WithTimeout(ctx, logbookTimeout)
		defer cancel()
		res, err := s.Import(ictx, filled, nil)
		if err != nil {
			return fleSavedMsg{err: fmt.Errorf("save FLE QSOs: %w", err)}
		}
		return fleSavedMsg{res: res, pending: pending}
	}
}

// fillFLE returns qsos filled in with l, as saveFLECmd describes, and the
// number tagged callinfo.PendingTag.
func fillFLE(ctx context.Context, l callinfo.Looker, qsos []logbook.QSO) ([]logbook.QSO, int) {
	out := make([]logbook.QSO, len(qsos))
	unreachable, pending := false, 0
	for i, q := range qsos {
		filled, err := l.Fill(ctx, q)
		// Any other failed lookup is not reported: the QSO is saved with
		// what was found, as on the New QSO screen.
		if l.Callbook != nil && callinfo.Unreachable(err) {
			unreachable = true
			l.Callbook = nil
		}
		if unreachable && !slices.Contains(filled.Tags, callinfo.PendingTag) {
			filled.Tags = append(slices.Clone(filled.Tags), callinfo.PendingTag)
			pending++
		}
		out[i] = filled
	}
	return out, pending
}

// applyFLESaved reports the save of QSOs entered as FLE on the status line
// and, when it succeeded, reloads the index.
func (m Model) applyFLESaved(msg fleSavedMsg) (Model, tea.Cmd) {
	m.fleSaving = false
	m.idx.progress = ""
	if msg.err != nil {
		m.idx.msg = msg.err.Error()
		return m, nil
	}
	m.idx.msg = fleSummary(msg)
	return m.refreshIndex()
}

// fleSummary returns the outcome of a save, e.g. "Added 4 QSOs, skipped 1
// duplicate." When QSOs were tagged callinfo.PendingTag, it says so without
// a count, since some of them may have been skipped as duplicates.
func fleSummary(msg fleSavedMsg) string {
	parts := []string{"Added " + countQSOs(msg.res.Added)}
	if n := msg.res.Skipped; n > 0 {
		parts = append(parts, "skipped "+plural(n, "duplicate"))
	}
	s := strings.Join(parts, ", ") + "."
	if msg.pending > 0 && msg.res.Added > 0 {
		s += " Callbook unreachable: tagged " + callinfo.PendingTag + "."
	}
	return s
}

// title returns the dialog's title, e.g. "FLE: 5 QSOs, 2 lines
// ignored".
func (d fleDialog) title() string {
	title := "FLE: " + countQSOs(len(d.result.QSOs))
	if n := len(d.result.Ignored); n > 0 {
		title += ", " + fleIgnoredCount(n)
	}
	return title
}

// fleIgnoredCount returns the number of lines ignored, e.g. "2 lines
// ignored".
func fleIgnoredCount(n int) string {
	return plural(n, "line") + " ignored"
}

// hint returns the line that ends the dialog, without save when there is
// nothing to save.
func (d fleDialog) hint() string {
	if len(d.result.QSOs) == 0 {
		return "e edit · esc cancel"
	}
	return "s save · e edit · esc cancel"
}

// fleMode returns the mode shown for q: its submode, or else its mode.
func fleMode(q logbook.QSO) string {
	if q.Submode != "" {
		return q.Submode
	}
	return q.Mode
}

// qsoCells returns the cells of each QSO's row: date, time on, call, band,
// mode, report sent and report received.
func (d fleDialog) qsoCells() [][]string {
	cells := make([][]string, 0, len(d.result.QSOs))
	for _, p := range d.result.QSOs {
		q := p.QSO
		cells = append(cells, []string{
			q.StartUTC.UTC().Format(qsoentry.DateLayout), q.StartUTC.UTC().Format("15:04"),
			q.Call, q.Band, fleMode(q), q.RSTSent, q.RSTRcvd,
		})
	}
	return cells
}

// fleHeader holds the column headings of the QSO rows.
var fleHeader = []string{"Date", "Time", "Call", "Band", "Mode", "Sent", "Rcvd"}

// columns returns the width of each column of the QSO rows, and the width
// of a whole row.
func (d fleDialog) columns() ([]int, int) {
	widths := make([]int, len(fleHeader))
	for _, row := range append([][]string{fleHeader}, d.qsoCells()...) {
		for i, c := range row {
			widths[i] = max(widths[i], lipgloss.Width(c))
		}
	}
	total := (len(widths) - 1) * fleColumnGap
	for _, w := range widths {
		total += w
	}
	return widths, total
}

// fleRow lays cells out in columns of widths.
func fleRow(cells []string, widths []int) string {
	var b strings.Builder
	for i, c := range cells {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", fleColumnGap))
		}
		if i == len(cells)-1 {
			b.WriteString(c)
			break
		}
		b.WriteString(pad(c, widths[i]))
	}
	return b.String()
}

// lines renders every row in width cells: a heading and a row for each
// QSO, in columns, then, when any line was ignored, a heading and each
// ignored line with why, wrapped.
func (d fleDialog) lines(st styles, width int) []string {
	var lines []string
	if len(d.result.QSOs) > 0 {
		widths, _ := d.columns()
		lines = append(lines, fit(st.helpHeading.Render(fleRow(fleHeader, widths)), width))
		for _, row := range d.qsoCells() {
			lines = append(lines, fit(fleRow(row, widths), width))
		}
	}
	if n := len(d.result.Ignored); n > 0 {
		if len(lines) > 0 {
			lines = append(lines, fit("", width))
		}
		lines = append(lines, fit(st.helpHeading.Render(fleIgnoredCount(n)), width))
		for _, ig := range d.result.Ignored {
			text := fmt.Sprintf("line %d: %s — %s", ig.Line, ig.Text, ig.Reason)
			lines = append(lines, wrapLines(st.err, text, width)...)
		}
	}
	return lines
}

// size returns the outer width and height of the dialog drawn in a space w
// by ht cells, the width of its rows and the number of rows it shows at
// once. It is as wide as its title, hint and QSO rows need, and as tall as
// its rows need, within the space.
func (d fleDialog) size(st styles, w, ht int) (outerW, outerH, rowWidth, listHeight int) {
	fw, fh := st.helpBox.GetHorizontalFrameSize(), st.helpBox.GetVerticalFrameSize()
	_, tableWidth := d.columns()
	need := max(
		lipgloss.Width(d.title())+1+lipgloss.Width(fleBadge),
		1+lipgloss.Width(d.hint()),
		1+tableWidth+helpBar,
		fleMinWidth,
	)
	outerW = min(need+fw, w)
	inner := max(outerW-fw, 1)
	rowWidth = max(inner-1-helpBar, 1)
	rows := len(d.lines(st, rowWidth))
	outerH = min(rows+fleChrome+fh, ht)
	return outerW, outerH, rowWidth, max(outerH-fh-fleChrome, 0)
}

// view renders the dialog in a box fitting a space width by height cells:
// the title and badge, a window of the rows from scroll with a scroll bar
// beside it, and the hint.
func (d fleDialog) view(st styles, width, height int) string {
	w, _, rowWidth, listHeight := d.size(st, width, height)
	inner := max(w-st.helpBox.GetHorizontalFrameSize(), 1)

	badge := st.helpBadge.Render(fleBadge)
	title := st.label.Render(d.title())
	gap := max(inner-lipgloss.Width(title)-lipgloss.Width(badge), 1)

	rows := d.lines(st, rowWidth)
	start := min(d.scroll, max(len(rows)-listHeight, 0))
	shown := rows[start:min(start+listHeight, len(rows))]
	bar := scrollBar(st, len(rows), listHeight, start)
	lines := []string{fit(title+strings.Repeat(" ", gap)+badge, inner), ""}
	for i := range listHeight {
		row, b := "", " "
		if i < len(shown) {
			row = shown[i]
		}
		if bar != nil {
			b = bar[i]
		}
		lines = append(lines, fit(" "+pad(row, rowWidth)+" "+b, inner))
	}
	lines = append(lines, "", fit(" "+st.muted.Render(d.hint()), inner))
	return st.helpBox.Render(strings.Join(lines, "\n"))
}

// fleView renders the FLE dialog for a body width by height cells.
func (m Model) fleView(width, height int) string {
	return m.fle.view(m.styles, width, height)
}

// fleListHeight is the number of rows the open FLE dialog shows at once.
func (m Model) fleListHeight() int {
	_, _, _, listHeight := m.fle.size(m.styles, m.bodyWidth(), m.bodyHeight())
	return listHeight
}

// fleRowCount is the number of rows of the open FLE dialog, as drawn in
// the body.
func (m Model) fleRowCount() int {
	_, _, rowWidth, _ := m.fle.size(m.styles, m.bodyWidth(), m.bodyHeight())
	return len(m.fle.lines(m.styles, rowWidth))
}
