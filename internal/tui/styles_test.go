package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

var (
	black = color.RGBA{A: 0xff}
	white = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

func TestDefaultStylesForTheBackground(t *testing.T) {
	tests := []struct {
		name  string
		style func(styles) lipgloss.Style
		dark  string
		light string
	}{
		{"muted", func(st styles) lipgloss.Style { return st.muted }, "8", "244"},
		{"stale", func(st styles) lipgloss.Style { return st.stale }, "8", "244"},
		{"hamQTH", func(st styles) lipgloss.Style { return st.hamQTH }, "3", "136"},
		{"lotwActive", func(st styles) lipgloss.Style { return st.lotwActive }, "214", "166"},
		{"lotwInactive", func(st styles) lipgloss.Style { return st.lotwInactive }, "8", "244"},
		{"connected", func(st styles) lipgloss.Style { return st.connected }, "2", "2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, lipgloss.Color(tt.dark), tt.style(defaultStyles(true)).GetForeground(), "dark")
			assert.Equal(t, lipgloss.Color(tt.light), tt.style(defaultStyles(false)).GetForeground(), "light")
		})
	}
}

func TestCursorBar(t *testing.T) {
	for _, tt := range []struct {
		dark bool
		bg   string
	}{{true, "8"}, {false, "244"}} {
		t.Run(fmt.Sprintf("dark=%t", tt.dark), func(t *testing.T) {
			c := defaultStyles(tt.dark).cursor
			assert.Equal(t, lipgloss.Color(tt.bg), c.GetBackground(), "the bar is the muted gray")
			assert.Equal(t, barText, c.GetForeground())
			assert.True(t, c.GetBold())
			assert.False(t, c.GetReverse())

			st := defaultStyles(tt.dark).withoutColor()
			assert.True(t, st.cursor.GetReverse(), "without colors the cursor is reverse video")
			assert.Equal(t, lipgloss.Color(tt.bg), st.lotwInactive.GetForeground(), "only the cursor changes")
		})
	}
}

func TestCursorWithoutColors(t *testing.T) {
	for _, p := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.NoTTY} {
		t.Run(p.String(), func(t *testing.T) {
			m := New()
			m, _ = update(t, m, tea.ColorProfileMsg{Profile: p})
			assert.True(t, m.styles.cursor.GetReverse(), "the cursor stays visible without colors")

			m, _ = update(t, m, tea.BackgroundColorMsg{Color: white})
			assert.False(t, m.styles.dark)
			assert.True(t, m.styles.cursor.GetReverse(), "a background reply keeps it")

			m, _ = update(t, m, tea.ColorProfileMsg{Profile: colorprofile.ANSI})
			assert.Equal(t, defaultStyles(false).cursor, m.styles.cursor, "with colors it is the bar")
		})
	}

	m := New(WithBackground(false))
	m, _ = update(t, m, tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	assert.True(t, m.styles.cursor.GetReverse())
	assert.False(t, m.styles.dark, "a fixed background stays fixed")
}

// assertBackground checks that m's styles, and those of its inputs and
// help, are the ones for a dark or light background.
func assertBackground(t *testing.T, m Model, dark bool) {
	t.Helper()
	assert.Equal(t, dark, m.styles.dark)
	assert.Equal(t, defaultStyles(dark).muted.GetForeground(), m.styles.muted.GetForeground())
	assertFooterHelpStyles(t, m.help.Styles, defaultStyles(dark))
	assert.Equal(t, inputStyles(dark), m.input.Styles())
	assert.Equal(t, inputStyles(dark), m.entry.input(qsoentry.Call).Styles())
	assert.Equal(t, areaStyles(dark), m.entry.area(qsoentry.Notes).Styles())
}

// assertFooterHelpStyles checks that the footer's key help hs renders its
// keys like the help modal's and the rest in st's muted gray.
func assertFooterHelpStyles(t *testing.T, hs help.Styles, st styles) {
	t.Helper()
	assert.Equal(t, st.helpKey, hs.ShortKey, "short key")
	assert.Equal(t, st.helpKey, hs.FullKey, "full key")
	for name, s := range map[string]lipgloss.Style{
		"short desc":      hs.ShortDesc,
		"full desc":       hs.FullDesc,
		"short separator": hs.ShortSeparator,
		"full separator":  hs.FullSeparator,
		"ellipsis":        hs.Ellipsis,
	} {
		assert.Equal(t, st.muted, s, name)
	}
}

func TestFooterHelpStyles(t *testing.T) {
	m := New()
	assertFooterHelpStyles(t, m.help.Styles, m.styles)

	m, _ = update(t, m, tea.BackgroundColorMsg{Color: white})
	assertFooterHelpStyles(t, m.help.Styles, defaultStyles(false))

	m, _ = update(t, m, tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	assertFooterHelpStyles(t, m.help.Styles, m.styles)
}

func TestFooterHelpHasNoFixedColors(t *testing.T) {
	for _, dark := range []bool{true, false} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			m, _ := newIndexModel(t, 3, 80, 24, WithBackground(dark))
			out := m.help.View(m.contextKeys())
			require.NotEmpty(t, out)
			assert.NotContains(t, out, "38;2;", "no 24-bit colors in the footer")
		})
	}
}

// requestsBackground reports whether cmd asks the terminal for its
// background color.
func requestsBackground(cmd tea.Cmd) bool {
	want := fmt.Sprintf("%T", tea.RequestBackgroundColor())
	for _, msg := range collectMsgs(cmd) {
		if fmt.Sprintf("%T", msg) == want {
			return true
		}
	}
	return false
}

func TestBackgroundDetected(t *testing.T) {
	m := New()
	assert.True(t, requestsBackground(m.Init()), "the terminal is asked for its background")
	assertBackground(t, m, true)

	m, _ = update(t, m, tea.BackgroundColorMsg{Color: white})
	assertBackground(t, m, false)

	m, _ = update(t, m, tea.BackgroundColorMsg{Color: black})
	assertBackground(t, m, true)
}

func TestBackgroundDetectedWithALogbook(t *testing.T) {
	m := New(WithLogbook(&fakeIndexStore{}))
	assert.True(t, requestsBackground(m.Init()))
}

func TestBackgroundFixed(t *testing.T) {
	for _, dark := range []bool{true, false} {
		t.Run(fmt.Sprintf("dark=%t", dark), func(t *testing.T) {
			m := New(WithBackground(dark))
			assert.False(t, requestsBackground(m.Init()), "a fixed background is not asked for")
			assertBackground(t, m, dark)

			m, _ = update(t, m, tea.BackgroundColorMsg{Color: white})
			m, _ = update(t, m, tea.BackgroundColorMsg{Color: black})
			assertBackground(t, m, dark)
		})
	}
}

func TestBackgroundStylesLaterWidgets(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24, WithRig("ic7300", &fakeRig{}))
	m, _ = update(t, m, tea.BackgroundColorMsg{Color: white})

	m, _ = update(t, m, keyPress('?', 0))
	require.True(t, m.helpModal.open)
	assert.Equal(t, inputStyles(false), m.helpModal.input.Styles(), "the help modal opened after the reply")
	m, _ = update(t, m, keyPress('?', 0))

	m, _ = update(t, m, rigKey)
	require.True(t, m.rigModal.open)
	assert.Equal(t, inputStyles(false), m.rigModal.freq.Styles(), "the rig modal opened after the reply")
	assert.Equal(t, inputStyles(false), m.rigModal.passband.Styles())
}

func TestBackgroundRestylesOpenModals(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24, WithRig("ic7300", &fakeRig{}))
	m, _ = update(t, m, rigKey)
	require.True(t, m.rigModal.open)

	m, _ = update(t, m, tea.BackgroundColorMsg{Color: white})

	assert.Equal(t, inputStyles(false), m.rigModal.freq.Styles())
	assert.Equal(t, inputStyles(false), m.rigModal.passband.Styles())
}

func TestTitleBadge(t *testing.T) {
	for _, dark := range []bool{true, false} {
		b := defaultStyles(dark).titleBadge
		assert.True(t, b.GetBold(), "dark=%t: the badge is bold", dark)
		assert.True(t, b.GetReverse(), "dark=%t: the badge is reverse video", dark)
		assert.Equal(t, badge, b.GetForeground(), "dark=%t: the badge is the badge color", dark)
	}

	m, _ := newIndexModel(t, 1, 80, 24)
	header := m.headerView()
	assert.Contains(t, header, m.styles.titleBadge.Render(" jstlogger "), "the title is a padded badge")
	assert.True(t, strings.HasPrefix(plain(header), "  jstlogger  "), "the badge follows the header's padding: %q", plain(header))
}

// TestWithoutColorLeavesStylesAlone guards the style set shared by copies
// of the styles: restyling for no colors makes new styles rather than
// changing the ones other Model copies hold.
func TestWithoutColorLeavesStylesAlone(t *testing.T) {
	st := defaultStyles(true)
	cursor := st.cursor
	colorless := st.withoutColor()
	assert.True(t, colorless.cursor.GetReverse(), "the colorless cursor is reverse video")
	assert.Equal(t, cursor, st.cursor, "the original styles keep their cursor")
	assert.Equal(t, st.muted, colorless.muted, "the other styles carry over")
}

// TestBadgesShareAColor checks that every reverse pill, the header's title,
// the status line's position and the modals' close badge, is one color.
func TestBadgesShareAColor(t *testing.T) {
	for _, dark := range []bool{true, false} {
		st := defaultStyles(dark)
		assert.Equal(t, blue, badge, "the badges are blue")
		assert.Equal(t, badge, st.titleBadge.GetForeground(), "dark=%t: the title badge", dark)
		assert.Equal(t, badge, st.statusChip.GetForeground(), "dark=%t: the position chip", dark)
		assert.Equal(t, badge, st.helpBadge.GetForeground(), "dark=%t: the close badge", dark)
	}
}
