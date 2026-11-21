package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/rig"
)

var (
	tabKey      = special(tea.KeyTab)
	shiftTabKey = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
)

// newTunedRigModel returns a sized Model wired to a fresh fakeRig, connected
// and tuned to 14.074 MHz USB with a 2400 Hz passband.
func newTunedRigModel(t *testing.T) (Model, *fakeRig) {
	t.Helper()
	m, f := newRigModel(t)
	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Connected})
	m, _ = update(t, m, rig.StateChanged{State: rig.State{Freq: 14074000, Mode: "USB", Passband: 2400}})
	return m, f
}

// rigModalText returns the open rig modal, unstyled, as drawn over the
// body, or "" when it is closed.
func rigModalText(m Model) string {
	if !m.rigModal.open {
		return ""
	}
	return plain(m.rigModalView(m.bodyWidth(), m.bodyHeight()))
}

// clearField erases the focused text field of the rig modal.
func clearField(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = update(t, m, keyPress('u', tea.ModCtrl))
	return m
}

// applyRigModal presses enter in the rig modal and runs the command that
// follows, if any.
func applyRigModal(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := update(t, m, enterKey)
	if cmd == nil {
		return m
	}
	return run(t, m, cmd)
}

func TestRigModalOpensPrefilled(t *testing.T) {
	tests := []struct {
		name  string
		key   tea.KeyPressMsg
		focus rigField
	}{
		{"f focuses the freq", keyPress('f', 0), rigFieldFreq},
		{"m focuses the mode", keyPress('m', 0), rigFieldMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, f := newTunedRigModel(t)
			m, _ = update(t, m, tt.key)

			require.True(t, m.rigModal.open)
			assert.Equal(t, tt.focus, m.rigModal.focus)
			assert.Equal(t, "14.074.000", m.rigModal.freq.Value())
			assert.Equal(t, "USB", m.rigModal.modeValue())
			assert.Equal(t, "2400", m.rigModal.passband.Value())
			assert.Empty(t, f.freqs)
			assert.Empty(t, f.modes)

			v := rigModalText(m)
			assert.Contains(t, v, "Freq")
			assert.Contains(t, v, "Mode")
			assert.Contains(t, v, "Passband")
			assert.Contains(t, v, "14.074.000")
		})
	}
}

func TestRigModalOpensBlankWithoutState(t *testing.T) {
	m, _ := newRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))

	require.True(t, m.rigModal.open)
	assert.Empty(t, m.rigModal.freq.Value())
	assert.Empty(t, m.rigModal.modeValue(), "no mode is chosen until one is picked")
	assert.Empty(t, m.rigModal.passband.Value())
}

func TestRigModalZeroPassbandIsBlank(t *testing.T) {
	m, _ := newRigModel(t)
	m, _ = update(t, m, rig.StateChanged{State: rig.State{Freq: 7074000, Mode: "PKTUSB"}})
	m, _ = update(t, m, keyPress('f', 0))
	assert.Empty(t, m.rigModal.passband.Value())
}

func TestRigModalEscCancels(t *testing.T) {
	m, f := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7074")

	m, cmd := update(t, m, escKey)
	assert.False(t, m.rigModal.open)
	assert.Nil(t, cmd)
	assert.Empty(t, f.freqs)
	assert.Empty(t, f.modes)

	// Reopening starts from the rig's state again.
	m, _ = update(t, m, keyPress('f', 0))
	assert.Equal(t, "14.074.000", m.rigModal.freq.Value())
}

func TestRigModalCtrlCQuits(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyPress('f', 0), keyPress('m', 0)} {
		m, _ := newTunedRigModel(t)
		m, _ = update(t, m, k)
		_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
		require.NotNil(t, cmd)
		assert.IsType(t, tea.QuitMsg{}, cmd())
	}
}

func TestRigModalCapturesKeys(t *testing.T) {
	m, _ := newRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))

	// q, ?, f and m are typed into the field.
	m = typeText(t, m, "q?fm")
	assert.True(t, m.rigModal.open)
	assert.False(t, m.helpModal.open)
	assert.Equal(t, "q?fm", m.rigModal.freq.Value())
}

func TestRigModalTabCyclesFields(t *testing.T) {
	m, _ := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))

	for _, want := range []rigField{rigFieldMode, rigFieldPassband, rigFieldFreq} {
		m, _ = update(t, m, tabKey)
		assert.Equal(t, want, m.rigModal.focus)
	}
	for _, want := range []rigField{rigFieldPassband, rigFieldMode, rigFieldFreq} {
		m, _ = update(t, m, shiftTabKey)
		assert.Equal(t, want, m.rigModal.focus)
	}
	assert.True(t, m.rigModal.freq.Focused())
	assert.False(t, m.rigModal.passband.Focused())
}

func TestRigModalShowsFocus(t *testing.T) {
	m, _ := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	assert.Regexp(t, `▸ Freq`, rigModalText(m))
	assert.NotRegexp(t, `▸ Mode`, rigModalText(m))

	m, _ = update(t, m, tabKey)
	assert.Regexp(t, `▸ Mode`, rigModalText(m))
	assert.NotRegexp(t, `▸ Freq`, rigModalText(m))
}

func TestRigModalModeListShowsOnlyWhenFocused(t *testing.T) {
	m, _ := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	assert.NotContains(t, rigModalText(m), "PKTUSB", "the mode shows compactly")

	m, _ = update(t, m, tabKey)
	assert.Contains(t, rigModalText(m), "LSB", "the focused mode shows the list")
}

func TestRigModalApplyFreq(t *testing.T) {
	m, f := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7074 khz")

	m, cmd := update(t, m, enterKey)
	require.NotNil(t, cmd)
	assert.True(t, m.rigModal.open, "the modal waits for the rig")
	assert.True(t, m.rigModal.pending)
	assert.Contains(t, rigModalText(m), "applying")
	assert.Empty(t, f.freqs, "controller should only be called when the command runs")

	m = run(t, m, cmd)
	assert.False(t, m.rigModal.open)
	assert.Equal(t, []int64{7074000}, f.freqs)
	assert.Empty(t, f.modes, "the mode did not change")
	assert.Nil(t, m.rigErr)
}

func TestRigModalApplyFromAnyField(t *testing.T) {
	for _, field := range []rigField{rigFieldMode, rigFieldPassband} {
		m, f := newTunedRigModel(t)
		m, _ = update(t, m, keyPress('f', 0))
		m = clearField(t, m)
		m = typeText(t, m, "7.074")
		for m.rigModal.focus != field {
			m, _ = update(t, m, tabKey)
		}
		m = applyRigModal(t, m)
		assert.False(t, m.rigModal.open)
		assert.Equal(t, []int64{7074000}, f.freqs)
	}
}

func TestRigModalApplyMode(t *testing.T) {
	tests := []struct {
		name     string
		keys     []tea.KeyPressMsg
		want     []modeCall
		wantFreq []int64
	}{
		{
			name: "a new mode takes the rig's default passband",
			keys: typeKeys("pktu"),
			want: []modeCall{{"PKTUSB", 0}},
		},
		{
			name: "a new mode and passband",
			keys: append(typeKeys("cw"), tabKey, keyPress('u', tea.ModCtrl), keyPress('5', 0), keyPress('0', 0), keyPress('0', 0)),
			want: []modeCall{{"CW", 500}},
		},
		{
			name: "a new passband resends the mode",
			keys: []tea.KeyPressMsg{tabKey, keyPress('u', tea.ModCtrl), keyPress('1', 0), keyPress('8', 0), keyPress('0', 0), keyPress('0', 0)},
			want: []modeCall{{"USB", 1800}},
		},
		{
			name: "a blank passband is the rig's default",
			keys: []tea.KeyPressMsg{tabKey, keyPress('u', tea.ModCtrl)},
			want: []modeCall{{"USB", 0}},
		},
		{
			name: "down moves to the next mode",
			keys: []tea.KeyPressMsg{special(tea.KeyDown)},
			want: []modeCall{{"LSB", 0}},
		},
		{
			name: "backspace erases the filter",
			keys: append(typeKeys("cwr"), special(tea.KeyBackspace)),
			want: []modeCall{{"CW", 0}},
		},
		{
			name: "the same mode is no change",
			keys: append(typeKeys("lsb"), special(tea.KeyBackspace), special(tea.KeyBackspace), special(tea.KeyBackspace), keyPress('u', 0), keyPress('s', 0)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, f := newTunedRigModel(t)
			m, _ = update(t, m, keyPress('m', 0))
			for _, k := range tt.keys {
				m, _ = update(t, m, k)
			}
			m = applyRigModal(t, m)
			assert.False(t, m.rigModal.open)
			assert.Equal(t, tt.want, f.modes)
			assert.Empty(t, f.freqs)
		})
	}
}

func TestRigModalApplyFreqAndMode(t *testing.T) {
	m, f := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "10.136")
	m, _ = update(t, m, tabKey)
	m = typeText(t, m, "pktu")

	m, cmd := update(t, m, enterKey)
	m = run(t, m, cmd)
	assert.False(t, m.rigModal.open)
	assert.Equal(t, []int64{10136000}, f.freqs)
	assert.Equal(t, []modeCall{{"PKTUSB", 0}}, f.modes)
}

func TestRigModalNoChangeCloses(t *testing.T) {
	m, f := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	// The same frequency written another way is no change.
	m = clearField(t, m)
	m = typeText(t, m, "14074")

	m, cmd := update(t, m, enterKey)
	assert.False(t, m.rigModal.open)
	assert.Nil(t, cmd)
	assert.Empty(t, f.freqs)
	assert.Empty(t, f.modes)
}

func TestRigModalWithoutState(t *testing.T) {
	t.Run("nothing entered closes", func(t *testing.T) {
		m, f := newRigModel(t)
		m, _ = update(t, m, keyPress('f', 0))
		m, cmd := update(t, m, enterKey)
		assert.False(t, m.rigModal.open)
		assert.Nil(t, cmd)
		assert.Empty(t, f.freqs)
		assert.Empty(t, f.modes)
	})
	t.Run("freq alone", func(t *testing.T) {
		m, f := newRigModel(t)
		m, _ = update(t, m, keyPress('f', 0))
		m = typeText(t, m, "14.074")
		m = applyRigModal(t, m)
		assert.False(t, m.rigModal.open)
		assert.Equal(t, []int64{14074000}, f.freqs)
		assert.Empty(t, f.modes)
	})
	t.Run("a picked mode", func(t *testing.T) {
		m, f := newRigModel(t)
		m, _ = update(t, m, keyPress('m', 0))
		m = typeText(t, m, "fm")
		m = applyRigModal(t, m)
		assert.False(t, m.rigModal.open)
		assert.Equal(t, []modeCall{{"FM", 0}}, f.modes)
	})
	t.Run("a passband needs a mode", func(t *testing.T) {
		m, f := newRigModel(t)
		m, _ = update(t, m, keyPress('m', 0))
		m, _ = update(t, m, tabKey)
		m = typeText(t, m, "2400")
		m, cmd := update(t, m, enterKey)
		assert.Nil(t, cmd)
		assert.True(t, m.rigModal.open)
		require.Error(t, m.rigModal.err)
		assert.Contains(t, rigModalText(m), "choose a mode")
		assert.Empty(t, f.modes)
	})
}

func TestRigModalParseErrors(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{
			name: "bad freq",
			keys: append([]tea.KeyPressMsg{keyPress('u', tea.ModCtrl)}, typeKeys("abc")...),
			want: `invalid frequency "abc"`,
		},
		{
			name: "blank freq",
			keys: []tea.KeyPressMsg{keyPress('u', tea.ModCtrl)},
			want: "enter a frequency",
		},
		{
			name: "bad passband",
			keys: append([]tea.KeyPressMsg{tabKey, tabKey, keyPress('u', tea.ModCtrl)}, typeKeys("wide")...),
			want: `invalid passband "wide"`,
		},
		{
			name: "negative passband",
			keys: append([]tea.KeyPressMsg{tabKey, tabKey, keyPress('u', tea.ModCtrl)}, typeKeys("-5")...),
			want: `invalid passband "-5": must not be`,
		},
		{
			name: "a filter matching no mode",
			keys: append([]tea.KeyPressMsg{tabKey}, typeKeys("zzz")...),
			want: "choose a mode",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, f := newTunedRigModel(t)
			m, _ = update(t, m, keyPress('f', 0))
			for _, k := range tt.keys {
				m, _ = update(t, m, k)
			}
			m, cmd := update(t, m, enterKey)
			assert.Nil(t, cmd)
			assert.True(t, m.rigModal.open, "the modal stays open")
			require.Error(t, m.rigModal.err)
			assert.Contains(t, rigModalText(m), tt.want)
			assert.Empty(t, f.freqs)
			assert.Empty(t, f.modes)
		})
	}
}

func TestRigModalParseErrorFixed(t *testing.T) {
	m, f := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "abc")
	m, _ = update(t, m, enterKey)
	require.Error(t, m.rigModal.err)

	m = clearField(t, m)
	m = typeText(t, m, "7.074")
	m = applyRigModal(t, m)
	assert.False(t, m.rigModal.open)
	assert.Equal(t, []int64{7074000}, f.freqs)
}

func TestRigModalRigError(t *testing.T) {
	m, f := newTunedRigModel(t)
	f.err = errors.New("rig busy")
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7.074")
	m = applyRigModal(t, m)

	assert.True(t, m.rigModal.open, "the modal stays open to try again")
	assert.False(t, m.rigModal.pending)
	require.Error(t, m.rigModal.err)
	assert.ErrorIs(t, m.rigModal.err, f.err)
	assert.Contains(t, rigModalText(m), "rig busy")
	assert.Nil(t, m.rigErr, "the error shows in the modal, not the footer")
	assert.Empty(t, m.freqs.entries, "a failed frequency is not recorded")

	// Trying again once the rig is free succeeds.
	f.err = nil
	m = applyRigModal(t, m)
	assert.False(t, m.rigModal.open)
	assert.Equal(t, []string{"7.074.000"}, m.freqs.entries)
}

func TestRigModalPendingIgnoresKeys(t *testing.T) {
	m, _ := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7.074")
	m, cmd := update(t, m, enterKey)
	require.NotNil(t, cmd)

	for _, k := range []tea.KeyPressMsg{keyPress('1', 0), tabKey, enterKey, special(tea.KeyF1)} {
		var next tea.Cmd
		m, next = update(t, m, k)
		assert.Nil(t, next)
	}
	assert.Equal(t, "7.074", m.rigModal.freq.Value())
	assert.Equal(t, rigFieldFreq, m.rigModal.focus)
	assert.False(t, m.helpModal.open)
	assert.True(t, m.rigModal.pending)
}

func TestRigModalEscWhilePending(t *testing.T) {
	m, f := newTunedRigModel(t)
	f.err = errors.New("rig busy")
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7.074")
	m, cmd := update(t, m, enterKey)
	require.NotNil(t, cmd)

	m, _ = update(t, m, escKey)
	assert.False(t, m.rigModal.open)

	// The result arrives with the modal closed, so shows in the footer.
	m = run(t, m, cmd)
	require.Error(t, m.rigErr)
	assert.Contains(t, m.View().Content, "rig busy")
	assert.Empty(t, m.freqs.entries)
}

func TestRigModalStaleResultIgnored(t *testing.T) {
	m, f := newTunedRigModel(t)
	f.err = errors.New("rig busy")
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7.074")
	m, cmd := update(t, m, enterKey)
	require.NotNil(t, cmd)

	m, _ = update(t, m, escKey)
	m, _ = update(t, m, keyPress('f', 0))
	m = run(t, m, cmd)
	assert.True(t, m.rigModal.open)
	assert.NoError(t, m.rigModal.err, "the reopened modal ignores the earlier result")
	assert.Nil(t, m.rigErr)
}

func TestRigModalFreqHistory(t *testing.T) {
	m, _ := newTunedRigModel(t)
	for _, freq := range []string{"7074", "14.074", "14074", "10.136"} {
		m, _ = update(t, m, keyPress('f', 0))
		m = clearField(t, m)
		m = typeText(t, m, freq)
		m = applyRigModal(t, m)
		require.False(t, m.rigModal.open)
		// The rig reports the new frequency.
		hz, err := parseFreq(freq)
		require.NoError(t, err)
		m, _ = update(t, m, rig.StateChanged{State: rig.State{Freq: hz, Mode: "USB", Passband: 2400}})
	}
	assert.Equal(t, []string{"7.074.000", "14.074.000", "10.136.000"}, m.freqs.entries,
		"each is recorded once, the same written either way, and the unchanged frequency not again")

	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "21")
	for _, want := range []string{"10.136.000", "14.074.000", "7.074.000", "7.074.000"} {
		m, _ = update(t, m, historyPrevKeys[0])
		assert.Equal(t, want, m.rigModal.freq.Value())
	}
	for _, want := range []string{"14.074.000", "10.136.000", "21"} {
		m, _ = update(t, m, historyNextKeys[1])
		assert.Equal(t, want, m.rigModal.freq.Value())
	}

	// A recalled frequency applies.
	m, _ = update(t, m, historyPrevKeys[1])
	m, _ = update(t, m, historyPrevKeys[1])
	m, cmd := update(t, m, enterKey)
	require.NotNil(t, cmd)
	m = run(t, m, cmd)
	assert.False(t, m.rigModal.open)
}

func TestRigModalHistoryKeysOnlyOnFreq(t *testing.T) {
	m, _ := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m = typeText(t, m, "7.074")
	m = applyRigModal(t, m)

	m, _ = update(t, m, keyPress('f', 0))
	m, _ = update(t, m, tabKey)
	m, _ = update(t, m, tabKey)
	m, _ = update(t, m, special(tea.KeyUp))
	assert.Equal(t, "2400", m.rigModal.passband.Value(), "up does not recall a frequency into the passband")
	assert.False(t, m.rigModal.keys().HistoryPrev.Enabled())
}

func TestFreqHistoryKeptApartFromPatterns(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24, WithRig("ic7300", &fakeRig{}))
	m.freqs.add("7.074.000")
	m.patterns.add("~b 20m")

	m = press(t, m, markPatternKey, special(tea.KeyUp))
	assert.Equal(t, "~b 20m", m.input.Value())
	m = press(t, m, special(tea.KeyUp))
	assert.Equal(t, "~b 20m", m.input.Value(), "no frequency in the pattern history")
	m = press(t, m, escKey)

	m = press(t, m, keyPress('f', 0), special(tea.KeyUp))
	assert.Equal(t, "7.074.000", m.rigModal.freq.Value())
	m = press(t, m, special(tea.KeyUp))
	assert.Equal(t, "7.074.000", m.rigModal.freq.Value(), "no pattern in the frequency history")
}

func TestRigModalRecentModesFirst(t *testing.T) {
	m, _ := newTunedRigModel(t)
	for _, mode := range []string{"cw", "fm", "pktu"} {
		m, _ = update(t, m, keyPress('m', 0))
		m = typeText(t, m, mode)
		m = applyRigModal(t, m)
		require.False(t, m.rigModal.open)
	}
	assert.Equal(t, []string{"PKTUSB", "FM", "CW"}, m.recentModes)

	// A mode used again moves to the front, once.
	m, _ = update(t, m, keyPress('m', 0))
	m = typeText(t, m, "cw")
	m = applyRigModal(t, m)
	assert.Equal(t, []string{"CW", "PKTUSB", "FM"}, m.recentModes)

	m, _ = update(t, m, keyPress('m', 0))
	items := m.rigModal.mode.items()
	require.GreaterOrEqual(t, len(items), 3)
	assert.Equal(t, []string{"CW", "PKTUSB", "FM"}, []string{items[0].Code, items[1].Code, items[2].Code})
}

func TestAddRecentModeCaps(t *testing.T) {
	var recent []string
	for _, mode := range []string{"A", "B", "C", "D", "E", "F", "B"} {
		recent = addRecentMode(recent, mode)
	}
	assert.Equal(t, []string{"B", "F", "E", "D", "C"}, recent)
}

func TestRigModalConnectionStatus(t *testing.T) {
	tests := []struct {
		name    string
		conn    rig.ConnectionChanged
		want    []string
		wantNot []string
	}{
		{"connecting", rig.ConnectionChanged{Status: rig.Connecting}, []string{"connecting"}, nil},
		{"connected", rig.ConnectionChanged{Status: rig.Connected}, []string{"connected"}, []string{"not connected"}},
		{
			"disconnected with reason",
			rig.ConnectionChanged{
				Status:  rig.Disconnected,
				Err:     errors.New("connection refused"),
				RetryIn: 30 * time.Second,
			},
			[]string{"ic7300 not connected", "connection refused", "retry in 30s"},
			nil,
		},
		{"disconnected", rig.ConnectionChanged{Status: rig.Disconnected}, []string{"ic7300 not connected"}, []string{"retry"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newRigModel(t)
			m, _ = update(t, m, tt.conn)
			header := m.headerView()
			m, _ = update(t, m, keyPress('f', 0))
			v := rigModalText(m)
			for _, w := range tt.want {
				assert.Contains(t, v, w)
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, v, w)
			}
			// The header never shows the reason.
			assert.NotContains(t, header, "connection refused")
			assert.NotContains(t, m.headerView(), "connection refused")
		})
	}
}

func TestRigModalDisconnectedStillApplies(t *testing.T) {
	m, f := newRigModel(t)
	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Disconnected, Err: errors.New("refused")})
	m, _ = update(t, m, keyPress('f', 0))
	m = typeText(t, m, "7.074")
	m = applyRigModal(t, m)
	assert.False(t, m.rigModal.open)
	assert.Equal(t, []int64{7074000}, f.freqs)
}

func TestRigModalHelp(t *testing.T) {
	m, _ := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))

	v := m.View().Content
	assert.Contains(t, v, "enter")
	assert.Contains(t, v, "esc")
	assert.IsType(t, rigModalKeyMap{}, m.contextKeys())

	help := helpText(m)
	assert.Contains(t, help, "prev freq")
	assert.NotContains(t, help, "filter modes", "the mode keys are for the mode field")

	m, _ = update(t, m, tabKey)
	help = helpText(m)
	assert.Contains(t, help, "filter modes")
	assert.NotContains(t, help, "prev freq")

	m, _ = update(t, m, special(tea.KeyF1))
	require.True(t, m.helpModal.open, "f1 opens the help over the modal")
	assert.True(t, m.rigModal.open)
	assert.Contains(t, modalText(t, m), "next field")

	m, _ = update(t, m, escKey)
	assert.False(t, m.helpModal.open)
	assert.True(t, m.rigModal.open, "closing the help leaves the modal open")
}

func TestRigModalPaste(t *testing.T) {
	m, f := newTunedRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = clearField(t, m)
	m, _ = update(t, m, tea.PasteMsg{Content: "3.573"})
	assert.Equal(t, "3.573", m.rigModal.freq.Value())

	m, _ = update(t, m, tabKey)
	m, _ = update(t, m, tea.PasteMsg{Content: "pktu"})
	assert.Equal(t, "PKTUSB", m.rigModal.modeValue())

	m = applyRigModal(t, m)
	assert.Equal(t, []int64{3573000}, f.freqs)
	assert.Equal(t, []modeCall{{"PKTUSB", 0}}, f.modes)
}

func TestRigModalMouseDoesNothing(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24, WithRig("ic7300", &fakeRig{}))
	m = press(t, m, keyPress('f', 0))
	require.True(t, m.rigModal.open)
	cursor := m.idx.cursor

	m, _ = update(t, m, tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	assert.False(t, m.pager.open, "a click does not open the QSO behind the modal")
	m, _ = update(t, m, wheelDown())
	assert.Equal(t, cursor, m.idx.cursor, "the wheel does not move the cursor behind the modal")
	assert.True(t, m.rigModal.open)
}

func TestRigModalWithLogbook(t *testing.T) {
	m, _ := newIndexModel(t, 5, 80, 24, WithRig("ic7300", &fakeRig{}))
	m = press(t, m, keyPress('m', 0))
	require.True(t, m.rigModal.open)
	// j filters the mode list, not the index behind.
	cursor := m.idx.cursor
	m = press(t, m, keyPress('j', 0))
	assert.Equal(t, cursor, m.idx.cursor)
	assert.Equal(t, "j", m.rigModal.mode.filter)
}
