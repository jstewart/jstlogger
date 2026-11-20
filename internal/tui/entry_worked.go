package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/callsign"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

const (
	// workedDelay is how long the call, band and mode must stay unchanged
	// before the worked-before panel looks them up, so typing a call does
	// not query the logbook on every key press.
	workedDelay = 300 * time.Millisecond
	// workedRecent is the number of earlier QSOs the panel lists.
	workedRecent = 5
)

// workedKey is what a worked-before lookup is for: the base callsign, the
// band and the mode, which is the submode when there is one.
type workedKey struct {
	call, band, mode string
}

// workedBefore is the New QSO screen's worked-before panel: the earlier
// QSOs with the call being entered.
type workedBefore struct {
	// wanted is the lookup last asked for, waiting out workedDelay or
	// under way. A tick or result for another is dropped.
	wanted workedKey
	// key is the lookup that res and err are for.
	key workedKey
	res workedResult
	err error
}

// workedResult is the outcome of a worked-before lookup.
type workedResult struct {
	// total is the number of earlier QSOs with the call, and onBand and
	// onMode the number of them on the band and in the mode.
	total, onBand, onMode int
	// recent are the newest of them, newest first.
	recent []logbook.QSO
}

// entryWorkedTickMsg ends the wait before the worked-before lookup for key,
// on the New QSO screen's opening gen.
type entryWorkedTickMsg struct {
	gen int
	key workedKey
}

// entryWorkedMsg carries the result of the worked-before lookup for key, on
// the New QSO screen's opening gen, or the error looking it up.
type entryWorkedMsg struct {
	gen int
	key workedKey
	res workedResult
	err error
}

// workedKeyOf returns the worked-before lookup for the form f. Its call is
// "" when f's call has no base callsign.
func workedKeyOf(f qsoentry.Form) workedKey {
	mode := f[qsoentry.Submode]
	if strings.TrimSpace(mode) == "" {
		mode = f[qsoentry.Mode]
	}
	return workedKey{
		call: callsign.Base(f[qsoentry.Call]),
		band: strings.ToLower(strings.TrimSpace(f[qsoentry.Band])),
		mode: strings.ToUpper(strings.TrimSpace(mode)),
	}
}

// fetchWorked returns a command that waits workedDelay and then asks for
// the worked-before lookup of the call, band and mode in the New QSO
// screen, when they have changed since it last asked. A form without a
// call asks for nothing.
func (m *Model) fetchWorked() tea.Cmd {
	e := &m.entry
	if !e.open || m.logbook == nil {
		return nil
	}
	k := workedKeyOf(e.form())
	if k == e.worked.wanted {
		return nil
	}
	e.worked.wanted = k
	if k.call == "" {
		return nil
	}
	gen := e.gen
	return m.tick(workedDelay, func(time.Time) tea.Msg {
		return entryWorkedTickMsg{gen: gen, key: k}
	})
}

// tickWorked starts the worked-before lookup the tick is for, unless the
// screen has closed or the form has changed since. An edited QSO is left
// out of its own lookup.
func (m Model) tickWorked(msg entryWorkedTickMsg) tea.Cmd {
	e := &m.entry
	if !e.open || msg.gen != e.gen || msg.key != e.worked.wanted {
		return nil
	}
	var exclude int64
	if e.editing {
		exclude = e.orig.ID
	}
	return workedCmd(m.logbook, msg.gen, msg.key, exclude)
}

// workedCmd returns a command that looks up k in s for the New QSO
// screen's opening gen, leaving out the QSO with ID exclude, if not 0.
func workedCmd(s logbook.Store, gen int, k workedKey, exclude int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), logbookTimeout)
		defer cancel()
		res, err := lookupWorked(ctx, s, k, exclude)
		if err != nil {
			err = fmt.Errorf("look up QSOs with %s: %w", k.call, err)
		}
		return entryWorkedMsg{gen: gen, key: k, res: res, err: err}
	}
}

// lookupWorked counts the QSOs in s with the base callsign k.call, in all
// and on k's band and in its mode, and loads the newest of them. A band or
// mode that is "" is not counted. The QSO with ID exclude, the one being
// edited, is left out when exclude is not 0, as it is not an earlier QSO.
func lookupWorked(ctx context.Context, s logbook.Store, k workedKey, exclude int64) (workedResult, error) {
	var res workedResult
	var call logbook.Expr = logbook.CallBaseIs{Call: k.call}
	if exclude != 0 {
		call = logbook.And{call, logbook.Not{X: logbook.Filter{IDs: []int64{exclude}}}}
	}
	var err error
	if res.total, err = s.Count(ctx, call); err != nil || res.total == 0 {
		return res, err
	}
	if res.recent, err = s.Index(ctx, call, logbook.DefaultSort, 0, workedRecent); err != nil {
		return workedResult{}, err
	}
	if k.band != "" {
		if res.onBand, err = s.Count(ctx, logbook.And{call, logbook.Filter{Bands: []string{k.band}}}); err != nil {
			return workedResult{}, err
		}
	}
	if k.mode != "" {
		if res.onMode, err = s.Count(ctx, logbook.And{call, logbook.ModeIs{Mode: k.mode}}); err != nil {
			return workedResult{}, err
		}
	}
	return res, nil
}

// applyWorked keeps the worked-before lookup the panel last asked for. A
// lookup for an earlier form, or one landing after the screen closed, is
// dropped.
func (e *entry) applyWorked(msg entryWorkedMsg) {
	if !e.open || msg.gen != e.gen || msg.key != e.worked.wanted {
		return
	}
	e.worked.key, e.worked.res, e.worked.err = msg.key, msg.res, msg.err
}

// badges returns the worked-before badges of r, the lookup for k: a new
// call with no earlier QSOs, which is new on every band and mode too, or
// else a new band or mode with none on k's band or in its mode. An edited
// QSO is never a new call, as it is already in the logbook, so with no
// earlier QSOs it has no badges.
func (r workedResult) badges(k workedKey, editing bool) []string {
	if r.total == 0 {
		if editing {
			return nil
		}
		return []string{"new call"}
	}
	var out []string
	if k.band != "" && r.onBand == 0 {
		out = append(out, "new band")
	}
	if k.mode != "" && r.onMode == 0 {
		out = append(out, "new mode")
	}
	return out
}

// workedView renders the worked-before panel in width cells: a heading
// with the number of earlier QSOs with the call and the badges, then the
// newest of them. It is nil when the form has no call. Until the lookup
// for the call lands, the heading says it is being looked up, and the
// badges wait for the lookup of the band and mode in the form.
func (m Model) workedView(width int) []string {
	st, w := m.styles, m.entry.worked
	k := workedKeyOf(m.entry.form())
	if k.call == "" || m.logbook == nil {
		return nil
	}
	heading := st.label.Render("Worked before") + "  " + k.call
	if w.key.call != k.call {
		return []string{"", truncate(heading+"  "+st.muted.Render("Looking up…"), width, "…")}
	}
	if w.err != nil {
		return []string{"", truncate(heading+"  "+st.err.Render("Lookup failed"), width, "…")}
	}
	heading += "  " + st.muted.Render(plural(w.res.total, "earlier QSO"))
	if w.key == k {
		for _, b := range w.res.badges(k, m.entry.editing) {
			heading += "  " + st.newBadge.Render(strings.ToUpper(b))
		}
	}
	lines := []string{"", truncate(heading, width, "…")}
	for _, q := range w.res.recent {
		mode := q.Submode
		if mode == "" {
			mode = q.Mode
		}
		row := fmt.Sprintf("  %s  %-6s %-8s %s / %s",
			q.StartUTC.UTC().Format("2006-01-02 15:04"), q.Band, mode, q.RSTSent, q.RSTRcvd)
		lines = append(lines, truncate(strings.TrimRight(row, " /"), width, "…"))
	}
	return lines
}

// plural returns n and noun, with an s unless n is 1.
func plural(n int, noun string) string {
	if n != 1 {
		noun += "s"
	}
	return strconv.Itoa(n) + " " + noun
}
