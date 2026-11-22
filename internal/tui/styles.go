package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// ANSI palette indexes keep colors consistent with the user's terminal theme.
var (
	accent = lipgloss.Color("6")
	muted  = lipgloss.Color("8")
	good   = lipgloss.Color("2")
	bad    = lipgloss.Color("1")
	yellow = lipgloss.Color("3")
	blue   = lipgloss.Color("4")
	purple = lipgloss.Color("5")
	// brightPurple is the bright variant of purple, which the eQSL badge
	// uses.
	brightPurple = lipgloss.Color("13")
	// amber is distinct from yellow, which HamQTH's badge uses.
	amber = lipgloss.Color("214")
	// barText is the bright white of the text on the cursor bar.
	barText = lipgloss.Color("15")
)

// On a light background, fixed 256-color stand-ins replace the palette
// colors that light themes get wrong: themes map ANSI 8 to anything from a
// light gray to their darkest color, and yellow and amber are pale on
// white.
var (
	lightMuted  = lipgloss.Color("244")
	lightYellow = lipgloss.Color("136")
	lightAmber  = lipgloss.Color("166")
)

type styles struct {
	// dark reports whether the styles are for a dark background.
	dark bool

	header lipgloss.Style
	title  lipgloss.Style
	body   lipgloss.Style
	footer lipgloss.Style
	muted  lipgloss.Style
	err    lipgloss.Style

	// indexHeader renders the index column header and cursor the bar
	// across the row under the cursor, which keeps the row's cell colors.
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

	// eQSLAG and eQSLNotAG render the eQSL badge for a callsign known to
	// be, or known not to be, an eQSL Authenticity Guaranteed member.
	eQSLAG    lipgloss.Style
	eQSLNotAG lipgloss.Style

	// lotwRecent, lotwActive and lotwInactive render the pager's LoTW
	// badge for a callsign that uploaded recently, within the year, or
	// longer ago.
	lotwRecent   lipgloss.Style
	lotwActive   lipgloss.Style
	lotwInactive lipgloss.Style

	// newBadge renders the New QSO screen's worked-before badges, such as
	// NEW BAND.
	newBadge lipgloss.Style

	// helpBox frames the help modal, helpBadge renders the close badge in
	// its corner, and helpHeading and helpKey render its section headings
	// and keys.
	helpBox     lipgloss.Style
	helpBadge   lipgloss.Style
	helpHeading lipgloss.Style
	helpKey     lipgloss.Style
}

// defaultStyles returns the styles for a dark or a light background.
func defaultStyles(dark bool) styles {
	ld := lipgloss.LightDark(dark)
	muted, yellow, amber := ld(lightMuted, muted), ld(lightYellow, yellow), ld(lightAmber, amber)
	return styles{
		dark:         dark,
		header:       lipgloss.NewStyle().Padding(0, 1),
		title:        lipgloss.NewStyle().Bold(true).Foreground(accent),
		body:         lipgloss.NewStyle().Padding(0, 1),
		footer:       lipgloss.NewStyle().Padding(0, 1),
		muted:        lipgloss.NewStyle().Foreground(muted),
		err:          lipgloss.NewStyle().Foreground(bad),
		indexHeader:  lipgloss.NewStyle().Bold(true),
		cursor:       lipgloss.NewStyle().Background(muted).Foreground(barText).Bold(true),
		tab:          lipgloss.NewStyle().Bold(true).Reverse(true),
		label:        lipgloss.NewStyle().Bold(true),
		rigFreq:      lipgloss.NewStyle().Bold(true),
		stale:        lipgloss.NewStyle().Foreground(muted).Faint(true),
		connected:    lipgloss.NewStyle().Foreground(good),
		qrz:          lipgloss.NewStyle().Bold(true).Foreground(blue),
		hamQTH:       lipgloss.NewStyle().Bold(true).Foreground(yellow),
		eQSL:         lipgloss.NewStyle().Bold(true).Foreground(purple),
		eQSLAG:       lipgloss.NewStyle().Bold(true).Foreground(brightPurple),
		eQSLNotAG:    lipgloss.NewStyle().Foreground(purple).Faint(true),
		lotwRecent:   lipgloss.NewStyle().Bold(true).Foreground(good),
		lotwActive:   lipgloss.NewStyle().Bold(true).Foreground(amber),
		lotwInactive: lipgloss.NewStyle().Foreground(muted).Faint(true),
		newBadge:     lipgloss.NewStyle().Bold(true).Foreground(good),
		helpBox:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(blue).Padding(0, 1),
		helpBadge:    lipgloss.NewStyle().Bold(true).Reverse(true).Foreground(blue),
		helpHeading:  lipgloss.NewStyle().Bold(true).Foreground(blue),
		helpKey:      lipgloss.NewStyle().Bold(true).Foreground(purple),
	}
}

// withoutColor returns st for a terminal that shows no colors, such as
// under NO_COLOR. The cursor bar would lose its background there, so the
// cursor is reverse video instead.
func (st styles) withoutColor() styles {
	st.cursor = lipgloss.NewStyle().Reverse(true)
	return st
}

// WithBackground fixes the styles for a dark or a light background, rather
// than asking the terminal for its background color.
func WithBackground(dark bool) Option {
	return func(m *Model) {
		*m = m.setBackground(dark)
		m.fixedBackground = true
	}
}

// setBackground restyles m, with its help and text inputs, for a dark or a
// light background, and for a terminal without colors when it is one.
func (m Model) setBackground(dark bool) Model {
	m.styles = defaultStyles(dark)
	if m.colorless {
		m.styles = m.styles.withoutColor()
	}
	m.help.Styles = help.DefaultStyles(dark)
	m.input.SetStyles(inputStyles(dark))
	m.entry.setDark(dark)
	if m.helpModal.open {
		m.helpModal.input.SetStyles(inputStyles(dark))
	}
	if m.rigModal.open {
		m.rigModal.freq.SetStyles(inputStyles(dark))
		m.rigModal.passband.SetStyles(inputStyles(dark))
	}
	return m
}

// setColorProfile restyles m for the terminal's color profile p: without
// colors, the cursor is reverse video.
func (m Model) setColorProfile(p colorprofile.Profile) Model {
	m.colorless = p == colorprofile.ASCII || p == colorprofile.NoTTY
	return m.setBackground(m.styles.dark)
}
