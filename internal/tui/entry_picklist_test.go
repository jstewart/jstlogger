package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/adif"
	"jstlogger/internal/qsoentry"
)

var spaceKey = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}

func TestEntryPickListOpens(t *testing.T) {
	for name, k := range map[string]tea.KeyPressMsg{"enter": enterKey, "space": spaceKey} {
		t.Run(name, func(t *testing.T) {
			m := newEntryModel(t, newEntryStore(t, nil))
			m = focusOn(t, m, qsoentry.AntPath)

			m = press(t, m, k)

			require.True(t, m.entry.list.open)
			assert.Equal(t, qsoentry.AntPath, m.entry.focus, "the picker keeps focus")
			body := bodyText(m)
			for _, line := range []string{"G  grayline", "O  other", "S  short path", "L  long path"} {
				assert.Contains(t, body, line)
			}
		})
	}
}

func TestEntryPickListPicksByDescription(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.PropMode)

	m = press(t, m, enterKey)
	m = press(t, m, typeKeys("meteor")...)
	assert.Contains(t, bodyText(m), "MS  meteor scatter")
	assert.NotContains(t, bodyText(m), "aurora", "the filter hides the rest")
	m = press(t, m, enterKey)

	assert.False(t, m.entry.list.open)
	assert.Equal(t, "MS", entryValue(m, qsoentry.PropMode))
	assert.Equal(t, qsoentry.PropMode, m.entry.focus, "picking keeps focus on the picker")
	assert.True(t, m.entry.unsaved(), "a pick is a change")
}

func TestEntryPickListPicksByCode(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.QSLSentVia)

	m = press(t, m, enterKey)
	m = press(t, m, typeKeys("d")...)
	m = press(t, m, enterKey)

	assert.Equal(t, "D", entryValue(m, qsoentry.QSLSentVia))
}

func TestEntryPickListCancels(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.AntPath: "s"})
	m = focusOn(t, m, qsoentry.AntPath)

	m = press(t, m, enterKey, special(tea.KeyDown), escKey)

	assert.True(t, m.entry.open, "esc closes the list, not the screen")
	assert.False(t, m.entry.list.open)
	assert.Equal(t, "S", entryValue(m, qsoentry.AntPath))
	assert.NotContains(t, bodyText(m), "long path")
}

func TestEntryPickListHighlightsTheValue(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.AntPath: "s"})
	m = focusOn(t, m, qsoentry.AntPath)

	m = press(t, m, enterKey, enterKey)

	assert.Equal(t, "S", entryValue(m, qsoentry.AntPath), "enter at once keeps the value")
}

func TestEntryPickListCommonModes(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Mode)

	m = press(t, m, enterKey)
	assert.Contains(t, bodyText(m), "SSB           single sideband", "the list opens on the common modes")
	items := m.entry.list.items()
	sep := m.entry.list.sep()
	assert.Equal(t, []string{"SSB", "CW", "FT8", "FT4", "FM", "AM", "RTTY", "PSK31", "JS8"}, adif.Codes(itemEnums(items[:sep])), "common modes come first")
	assert.Equal(t, adif.Codes(m.entry.picker(qsoentry.Mode).options), adif.Codes(itemEnums(items[sep:])), "then every mode in ADIF order")

	m = press(t, m, typeKeys("ft4")...)
	m = press(t, m, enterKey)

	assert.Equal(t, "MFSK", entryValue(m, qsoentry.Mode), "FT4 is under MFSK")
	assert.Equal(t, "FT4", entryValue(m, qsoentry.Submode))
	assert.Equal(t, "-10", entryValue(m, qsoentry.RSTSent), "the reports follow the mode")
}

func TestEntryPickListKeepsArrowOrder(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Mode)

	m = press(t, m, special(tea.KeyRight))

	assert.Equal(t, "AM", entryValue(m, qsoentry.Mode), "the arrows step in ADIF order")
}

func TestEntryPickListTabMovesOn(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Mode)
	m = press(t, m, enterKey, special(tea.KeyDown))

	m = press(t, m, tab())

	assert.False(t, m.entry.list.open)
	assert.Equal(t, qsoentry.Submode, m.entry.focus, "tab closes the list and moves on")
	assert.Empty(t, entryValue(m, qsoentry.Mode), "without picking")
}

func TestEntryPickListHelp(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)
	assert.NotContains(t, plain(m.footerView()), "enter list", "a text field has no list")

	m = focusOn(t, m, qsoentry.Mode)
	help := plain(m.footerView())
	assert.Contains(t, help, "←/→ change")
	assert.Contains(t, help, "enter list")

	m = press(t, m, enterKey)
	help = plain(m.footerView())
	assert.Contains(t, help, "enter pick")
	assert.Contains(t, help, "esc close")
}

func TestEntryPickListClosesWithTheField(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Mode)
	m = press(t, m, enterKey)

	m.entry.goTab(entryTabQSL)

	assert.False(t, m.entry.list.open, "a list never outlives its picker's focus")
}

func TestEntryPickListFitsNarrowWindows(t *testing.T) {
	for _, width := range []int{12, 20, 40} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			m := newEntryModel(t, newEntryStore(t, nil))
			m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 20})
			m = focusOn(t, m, qsoentry.Mode)
			m = press(t, m, enterKey)
			require.True(t, m.entry.list.open)

			v := m.View().Content
			assert.Equal(t, 20, lipgloss.Height(v), "the screen fills the window without wrapping")
			for i, line := range strings.Split(v, "\n") {
				assert.LessOrEqual(t, lipgloss.Width(line), width, "line %d: %q", i, plain(line))
			}
			assert.Contains(t, plain(v), "SSB", "the list shows")
		})
	}
}

func TestEntryPickListPicksByCodeWhenTheOptionsChange(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Mode: "ssb"})
	m = focusOn(t, m, qsoentry.Submode)
	m = press(t, m, enterKey, special(tea.KeyDown))

	// As when the rig changes the options while the list is open.
	m.entry.picker(qsoentry.Submode).setOptions(codeOptions([]string{"", "USB", "LSB"}))
	m = press(t, m, enterKey)

	assert.Equal(t, "LSB", entryValue(m, qsoentry.Submode), "the code highlighted is picked, not its old place")
}

func TestEntryPickerSpaceContinuesTypeAhead(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = fill(t, m, map[qsoentry.Field]string{qsoentry.Mode: "dynamic"})
	m = focusOn(t, m, qsoentry.Submode)

	m = press(t, m, typeKeys("VARA H")...)

	assert.False(t, m.entry.list.open, "a space after typing is part of the code")
	assert.Equal(t, "VARA HF", entryValue(m, qsoentry.Submode))
}
