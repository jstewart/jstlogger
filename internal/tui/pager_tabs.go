package tui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
)

// pagerTab is one of the pager's tabs.
type pagerTab int

// The pager's tabs, in tab bar order.
const (
	tabQSO pagerTab = iota
	tabStation
	tabQSL
	tabADIF
	// pagerTabCount is the number of tabs.
	pagerTabCount
)

// pagerTabNames are the tab bar names of the pager's tabs.
var pagerTabNames = [pagerTabCount]string{"QSO", "Station", "QSL", "ADIF"}

// String returns the tab's name in the tab bar.
func (t pagerTab) String() string {
	return pagerTabNames[t]
}

// next returns the tab after t, wrapping to the first.
func (t pagerTab) next() pagerTab {
	return (t + 1) % pagerTabCount
}

// prev returns the tab before t, wrapping to the last.
func (t pagerTab) prev() pagerTab {
	return (t + pagerTabCount - 1) % pagerTabCount
}

// qslDateFormat is the layout of the dates on the QSL tab.
const qslDateFormat = "2006-01-02"

// tabBar renders a tab bar of the named tabs, with the tab at index active
// highlighted, ruled to width cells. Tabs that don't fit are cut off.
func tabBar(st styles, names []string, active, width int) string {
	var b strings.Builder
	room := width
	for i, n := range names {
		if room <= 0 {
			break
		}
		name := truncate(" "+n+" ", room, "")
		room -= lipgloss.Width(name)
		if i == active {
			name = st.tab.Render(name)
		}
		b.WriteString(name)
	}
	return b.String() + st.muted.Render(strings.Repeat("─", max(room, 0)))
}

// pagerRow is one labeled row of a pager tab. suffix, already styled,
// follows a value that leaves room for it, and is dropped otherwise.
type pagerRow struct{ label, value, suffix string }

// pagerRows renders rows with their labels padded to the widest, each fitted
// to width cells. An empty value shows pagerEmpty, so the row keeps its
// place.
func pagerRows(st styles, rows []pagerRow, width int) string {
	lw := 0
	for _, r := range rows {
		lw = max(lw, lipgloss.Width(r.label))
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		label := st.label.Render(pad(truncate(r.label, width, ""), min(lw, width)))
		room := max(width-lw-2, 0)
		if room == 0 {
			lines[i] = label
			continue
		}
		// A value broken over lines, such as an imported comment, would
		// push the rows below it down.
		value := strings.Join(strings.Fields(r.value), " ")
		if value == "" {
			value = st.muted.Render(truncate(pagerEmpty, room, ""))
		} else {
			value = truncate(value, room, "…")
		}
		// The suffix is styled, so it is shown whole or not at all rather
		// than truncated.
		if r.suffix != "" && lipgloss.Width(value)+1+lipgloss.Width(r.suffix) <= room {
			value += " " + r.suffix
		}
		lines[i] = label + "  " + value
	}
	return strings.Join(lines, "\n")
}

// stationTab renders the Station tab: our side of the QSO, with
// stationName naming the station, then the QSO's MY_ ADIF fields sorted by
// name.
func stationTab(st styles, q logbook.QSO, stationName string, width int) string {
	var power string
	if q.TxPwr.Valid {
		power = strconv.FormatFloat(q.TxPwr.V, 'f', -1, 64) + " W"
	}
	rows := []pagerRow{
		{label: "Station callsign", value: q.StationCallsign},
		{label: "Operator", value: q.Operator},
		{label: "Station grid", value: q.MyGridsquare},
		{label: "TX power", value: power},
		{label: "Station name", value: stationName},
	}
	var names []string
	for name := range q.Fields {
		if strings.HasPrefix(name, "MY_") {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		rows = append(rows, pagerRow{label: name, value: q.Fields[name]})
	}
	return pagerRows(st, rows, width)
}

// qslServiceOrder ranks the well-known confirmation services; any other
// service comes after them.
var qslServiceOrder = []string{logbook.ServiceCard, logbook.ServiceLoTW, logbook.ServiceEQSL}

// qslServiceNames are the display names of the well-known confirmation
// services. Any other service is shown as it is stored.
var qslServiceNames = map[string]string{
	logbook.ServiceCard: "Card",
	logbook.ServiceLoTW: "LoTW",
	logbook.ServiceEQSL: "eQSL",
}

// qslTab renders the QSL tab: a sent and a received row for each
// confirmation service, card, LoTW and eQSL first, then the others by name.
func qslTab(st styles, q logbook.QSO, width int) string {
	if len(q.Confirmations) == 0 {
		return st.muted.Render(truncate("No confirmations", width, "…"))
	}
	rank := func(service string) int {
		if i := slices.Index(qslServiceOrder, service); i >= 0 {
			return i
		}
		return len(qslServiceOrder)
	}
	cs := slices.Clone(q.Confirmations)
	slices.SortStableFunc(cs, func(a, b logbook.Confirmation) int {
		return cmp.Or(cmp.Compare(rank(a.Service), rank(b.Service)), cmp.Compare(a.Service, b.Service))
	})
	rows := make([]pagerRow, 0, 2*len(cs))
	for _, c := range cs {
		name, ok := qslServiceNames[c.Service]
		if !ok {
			name = c.Service
		}
		sent := logbook.QSLSentText(c.Sent)
		if c.SentDate.Valid {
			sent += " " + c.SentDate.V.UTC().Format(qslDateFormat)
		}
		rcvd := logbook.QSLRcvdText(c.Rcvd)
		if c.RcvdDate.Valid {
			rcvd += " " + c.RcvdDate.V.UTC().Format(qslDateFormat)
		}
		rows = append(rows,
			pagerRow{label: name + " sent", value: sent},
			pagerRow{label: name + " received", value: rcvd},
		)
	}
	return pagerRows(st, rows, width)
}

// adifTab renders the ADIF tab: the QSO's ADIF fields, as exported, sorted
// by name.
func adifTab(st styles, q logbook.QSO, width int) string {
	r := logbook.RecordFromQSO(q)
	slices.SortStableFunc(r, func(a, b adif.Field) int { return cmp.Compare(a.Name, b.Name) })
	rows := make([]pagerRow, len(r))
	for i, f := range r {
		rows[i] = pagerRow{label: f.Name, value: f.Value}
	}
	return pagerRows(st, rows, width)
}
