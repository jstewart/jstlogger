package tui

import "charm.land/lipgloss/v2"

// ANSI palette indexes keep colors consistent with the user's terminal theme.
var (
	accent = lipgloss.Color("6")
	muted  = lipgloss.Color("8")
	good   = lipgloss.Color("2")
	bad    = lipgloss.Color("1")
	yellow = lipgloss.Color("3")
	blue   = lipgloss.Color("4")
	purple = lipgloss.Color("5")
	// amber is distinct from yellow, which HamQTH's badge uses.
	amber = lipgloss.Color("214")
)

type styles struct {
	header lipgloss.Style
	title  lipgloss.Style
	body   lipgloss.Style
	footer lipgloss.Style
	muted  lipgloss.Style
	err    lipgloss.Style

	// indexHeader renders the index column header and cursor the bar
	// across the row under the cursor.
	indexHeader lipgloss.Style
	cursor      lipgloss.Style

	// tab renders the open pager tab and label a field's label.
	tab   lipgloss.Style
	label lipgloss.Style

	// rigFreq renders the rig frequency and mode while connected; stale
	// renders them while they may be out of date.
	rigFreq   lipgloss.Style
	stale     lipgloss.Style
	connected lipgloss.Style

	// qrz, hamQTH and eQSL render the pager's lookup badges for those
	// sites, each in its own color.
	qrz    lipgloss.Style
	hamQTH lipgloss.Style
	eQSL   lipgloss.Style

	// lotwRecent, lotwActive and lotwInactive render the pager's LoTW
	// badge for a callsign that uploaded recently, within the year, or
	// longer ago.
	lotwRecent   lipgloss.Style
	lotwActive   lipgloss.Style
	lotwInactive lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:       lipgloss.NewStyle().Padding(0, 1),
		title:        lipgloss.NewStyle().Bold(true).Foreground(accent),
		body:         lipgloss.NewStyle().Padding(0, 1),
		footer:       lipgloss.NewStyle().Padding(0, 1),
		muted:        lipgloss.NewStyle().Foreground(muted),
		err:          lipgloss.NewStyle().Foreground(bad),
		indexHeader:  lipgloss.NewStyle().Bold(true),
		cursor:       lipgloss.NewStyle().Reverse(true),
		tab:          lipgloss.NewStyle().Bold(true).Reverse(true),
		label:        lipgloss.NewStyle().Bold(true),
		rigFreq:      lipgloss.NewStyle().Bold(true),
		stale:        lipgloss.NewStyle().Foreground(muted).Faint(true),
		connected:    lipgloss.NewStyle().Foreground(good),
		qrz:          lipgloss.NewStyle().Bold(true).Foreground(blue),
		hamQTH:       lipgloss.NewStyle().Bold(true).Foreground(yellow),
		eQSL:         lipgloss.NewStyle().Bold(true).Foreground(purple),
		lotwRecent:   lipgloss.NewStyle().Bold(true).Foreground(good),
		lotwActive:   lipgloss.NewStyle().Bold(true).Foreground(amber),
		lotwInactive: lipgloss.NewStyle().Foreground(muted).Faint(true),
	}
}
