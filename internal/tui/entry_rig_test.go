package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"jstlogger/internal/qsoentry"
	"jstlogger/internal/rig"
)

var syncRigKey = keyPress('r', tea.ModCtrl)

// rigAt returns the rig state on hz in the Hamlib mode mode.
func rigAt(hz int64, mode string) rig.StateChanged {
	return rig.StateChanged{State: rig.State{Freq: hz, Mode: mode}}
}

// assertRigFields checks the freq, band, mode and submode in the form.
func assertRigFields(t *testing.T, m Model, freq, band, mode, submode string) {
	t.Helper()
	assert.Equal(t, freq, entryValue(m, qsoentry.Freq), "freq")
	assert.Equal(t, band, entryValue(m, qsoentry.Band), "band")
	assert.Equal(t, mode, entryValue(m, qsoentry.Mode), "mode")
	assert.Equal(t, submode, entryValue(m, qsoentry.Submode), "submode")
}

// entryLine returns the plain line of the New QSO screen holding label,
// from the label on.
func entryLine(m Model, label string) string {
	for l := range strings.SplitSeq(bodyText(m), "\n") {
		if _, rest, ok := strings.Cut(l, label+" "); ok {
			return label + " " + rest
		}
	}
	return ""
}

func TestEntryLiveFollowsRig(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))

	assertRigFields(t, m, "14.074", "20m", "SSB", "USB")
	assert.Equal(t, "59", entryValue(m, qsoentry.RSTSent))

	m, _ = update(t, m, rigAt(7_030_000, "CW"))

	assertRigFields(t, m, "7.03", "40m", "CW", "")
	assert.Equal(t, "599", entryValue(m, qsoentry.RSTSent), "the default report follows the rig's mode")
	assert.Equal(t, "599", entryValue(m, qsoentry.RSTRcvd))
}

func TestEntryLiveKeepsTypedReport(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))
	m = focusOn(t, m, qsoentry.RSTRcvd)
	m = press(t, m, special(tea.KeyBackspace))
	m = typeText(t, m, "7")

	m, _ = update(t, m, rigAt(7_030_000, "CW"))

	assert.Equal(t, "599", entryValue(m, qsoentry.RSTSent))
	assert.Equal(t, "57", entryValue(m, qsoentry.RSTRcvd), "a typed report is kept")
}

func TestEntryLiveHandEditedFieldStopsFollowingRig(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))
	assert.NotContains(t, entryLine(m, "Freq"), "manual")
	m = focusOn(t, m, qsoentry.Freq)
	m = press(t, m, special(tea.KeyBackspace))
	m = typeText(t, m, "6")

	m, _ = update(t, m, rigAt(14_230_000, "LSB"))

	assertRigFields(t, m, "14.076", "20m", "SSB", "LSB")
	assert.Contains(t, entryLine(m, "Freq"), "manual", "the freq is marked as typed")
	assert.NotContains(t, entryLine(m, "Mode"), "manual")

	m = press(t, m, syncRigKey)

	assertRigFields(t, m, "14.23", "20m", "SSB", "LSB")
	assert.NotContains(t, entryLine(m, "Freq"), "manual")
	m, _ = update(t, m, rigAt(7_074_000, "LSB"))
	assert.Equal(t, "7.074", entryValue(m, qsoentry.Freq), "the freq follows the rig again")
}

func TestEntryLiveHandEditedBandAndMode(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))
	m = focusOn(t, m, qsoentry.Mode)
	m = typeText(t, m, "ft8")
	m = focusOn(t, m, qsoentry.Band)
	m = press(t, m, special(tea.KeyBackspace), special(tea.KeyBackspace), special(tea.KeyBackspace))
	m = typeText(t, m, "17m")

	m, _ = update(t, m, rigAt(7_030_000, "CW"))

	assertRigFields(t, m, "7.03", "17m", "FT8", "")
	assert.Contains(t, entryLine(m, "Mode"), "manual")
	assert.Contains(t, entryLine(m, "Band"), "manual")

	m = press(t, m, syncRigKey)

	assertRigFields(t, m, "7.03", "40m", "CW", "")
	assert.NotContains(t, entryLine(m, "Mode"), "manual")
	assert.NotContains(t, entryLine(m, "Band"), "manual")
}

func TestEntryLivePacketModeKeepsLastDataMode(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "PKTUSB"))
	assertRigFields(t, m, "14.074", "20m", "", "")

	m = focusOn(t, m, qsoentry.Mode)
	m = typeText(t, m, "mfsk")
	m = press(t, m, tab())
	m = typeText(t, m, "ft4")
	m = press(t, m, syncRigKey)
	assertRigFields(t, m, "14.074", "20m", "MFSK", "FT4")

	m, _ = update(t, m, rigAt(14_230_000, "USB"))
	assertRigFields(t, m, "14.23", "20m", "SSB", "USB")
	m, _ = update(t, m, rigAt(14_080_000, "PKTUSB"))
	assertRigFields(t, m, "14.08", "20m", "MFSK", "FT4")
	assert.Equal(t, "-10", entryValue(m, qsoentry.RSTSent))

	m = press(t, m, esc())
	m = press(t, m, newQSOKey)
	assertRigFields(t, m, "14.08", "20m", "MFSK", "FT4")
}

func TestEntryLiveKeepsValuesWhenRigDisconnects(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))

	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Disconnected})

	assertRigFields(t, m, "14.074", "20m", "SSB", "USB")
	assert.True(t, m.entry.live)
	header := plain(m.headerView())
	assert.Contains(t, header, "14.074.000 USB")
	assert.Contains(t, header, "ic7300 not connected", "the rig indicator shows the values are stale")
}

func TestEntryPostIgnoresRig(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))
	m = press(t, m, liveKey)

	m, _ = update(t, m, rigAt(7_030_000, "CW"))
	assertRigFields(t, m, "14.074", "20m", "SSB", "USB")

	m = press(t, m, liveKey)
	assertRigFields(t, m, "7.03", "40m", "CW", "")
}

func TestEntryRigIsNotAnUnsavedChange(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(14_074_000, "USB"))
	m, _ = update(t, m, rigAt(7_030_000, "CW"))

	m = press(t, m, esc())

	assert.False(t, m.entry.open)
	assert.Equal(t, promptNone, m.prompt)
}

func TestEntrySwitchToPostKeepsFilledInValuesSaved(t *testing.T) {
	m := newLiveEntryModel(t, newEntryStore(t, nil), rigAt(7_030_000, "CW"))
	m, _ = at(t, m, 5*time.Second)

	m = press(t, m, liveKey, esc())

	assert.False(t, m.entry.open, "what the clock and rig filled in isn't an unsaved change")
	assert.Equal(t, promptNone, m.prompt)
}
