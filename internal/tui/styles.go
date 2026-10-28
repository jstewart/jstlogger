package tui

import "charm.land/lipgloss/v2"

// ANSI palette indexes keep colors consistent with the user's terminal theme.
var (
	accent = lipgloss.Color("6")
	muted  = lipgloss.Color("8")
	good   = lipgloss.Color("2")
	bad    = lipgloss.Color("1")
)

type styles struct {
	header lipgloss.Style
	title  lipgloss.Style
	body   lipgloss.Style
	footer lipgloss.Style
	muted  lipgloss.Style
	err    lipgloss.Style

	// rigFreq renders the rig frequency and mode while connected; stale
	// renders them while they may be out of date.
	rigFreq   lipgloss.Style
	stale     lipgloss.Style
	connected lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:    lipgloss.NewStyle().Padding(0, 1),
		title:     lipgloss.NewStyle().Bold(true).Foreground(accent),
		body:      lipgloss.NewStyle().Padding(0, 1),
		footer:    lipgloss.NewStyle().Padding(0, 1),
		muted:     lipgloss.NewStyle().Foreground(muted),
		err:       lipgloss.NewStyle().Foreground(bad),
		rigFreq:   lipgloss.NewStyle().Bold(true),
		stale:     lipgloss.NewStyle().Foreground(muted).Faint(true),
		connected: lipgloss.NewStyle().Foreground(good),
	}
}
