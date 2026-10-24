package tui

import "charm.land/lipgloss/v2"

// ANSI palette indexes keep colors consistent with the user's terminal theme.
var (
	accent = lipgloss.Color("6")
	muted  = lipgloss.Color("8")
)

type styles struct {
	header lipgloss.Style
	body   lipgloss.Style
	footer lipgloss.Style
	muted  lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header: lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(0, 1),
		body:   lipgloss.NewStyle().Padding(0, 1),
		footer: lipgloss.NewStyle().Padding(0, 1),
		muted:  lipgloss.NewStyle().Foreground(muted),
	}
}
