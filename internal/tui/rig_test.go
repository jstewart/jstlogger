package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/rig"
)

// fakeRig is a RigController that records calls and returns err.
type fakeRig struct {
	freqs []int64
	modes []modeCall
	err   error
}

type modeCall struct {
	mode     string
	passband int
}

func (f *fakeRig) SetFreq(ctx context.Context, hz int64) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("fakeRig: ctx has no deadline")
	}
	f.freqs = append(f.freqs, hz)
	return f.err
}

func (f *fakeRig) SetMode(ctx context.Context, mode string, passband int) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("fakeRig: ctx has no deadline")
	}
	f.modes = append(f.modes, modeCall{mode, passband})
	return f.err
}

var (
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

// newRigModel returns a sized Model wired to a fresh fakeRig.
func newRigModel(t *testing.T) (Model, *fakeRig) {
	t.Helper()
	f := &fakeRig{}
	m, _ := update(t, New(WithRig("ic7300", f)), tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, f
}

// typeText sends each rune of s to m as a key press.
func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		var cmd tea.Cmd
		m, cmd = update(t, m, keyPress(r, 0))
		assertNotQuit(t, cmd)
	}
	return m
}

// assertNotQuit fails if cmd would quit the program.
func assertNotQuit(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd != nil {
		assert.NotEqual(t, tea.QuitMsg{}, cmd(), "command should not quit")
	}
}

// run executes cmd and feeds its message back into m.
func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	require.NotNil(t, cmd)
	m, _ = update(t, m, cmd())
	return m
}

func TestFormatFreq(t *testing.T) {
	tests := []struct {
		hz   int64
		want string
	}{
		{14074000, "14.074.000"},
		{7074000, "7.074.000"},
		{144174000, "144.174.000"},
		{1840500, "1.840.500"},
		{475000, "0.475.000"},
		{10368100000, "10368.100.000"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, formatFreq(tt.hz))
		})
	}
}

func TestParseFreq(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "14.074", want: 14074000},
		{in: "7", want: 7000000},
		{in: "144.174", want: 144174000},
		{in: "999.9999995", want: 1000000000},
		{in: "14074", want: 14074000},
		{in: "7074.5", want: 7074500},
		{in: "1000", want: 1000000},
		{in: "14074000", want: 14074000},
		{in: "1000000", want: 1000000},
		{in: "  14.074  ", want: 14074000},
		{in: "14.074MHz", want: 14074000},
		{in: "14.074 mhz", want: 14074000},
		{in: "7074 kHz", want: 7074000},
		{in: "7074KHZ", want: 7074000},
		{in: "500 hz", want: 500},
		{in: "14074000Hz", want: 14074000},
		{in: "0.0005 khz", want: 1},
		{in: "14.074.000", want: 14074000},
		{in: "7.074.500", want: 7074500},
		{in: " 144.174.000 ", want: 144174000},
		{in: "14.074.000 mhz", wantErr: true},
		{in: "14.074.00", wantErr: true},
		{in: "14.74.000", wantErr: true},
		{in: "0.000.000", wantErr: true},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "mhz", wantErr: true},
		{in: "14.074 ghz", wantErr: true},
		{in: "0", wantErr: true},
		{in: "-7", wantErr: true},
		{in: "0.1 hz", wantErr: true},
		{in: "inf", wantErr: true},
		{in: "nan", wantErr: true},
		{in: "1e30 mhz", wantErr: true},
		{in: "14 074", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseFreq(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParsePassband(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr string
	}{
		{in: "", want: 0},
		{in: "   ", want: 0},
		{in: "2400", want: 2400},
		{in: " 3000 ", want: 3000},
		{in: "0", want: 0},
		{in: "wide", wantErr: `invalid passband "wide": invalid syntax`},
		{in: "2400.5", wantErr: `invalid passband "2400.5": invalid syntax`},
		{in: "-1", wantErr: `invalid passband "-1": must not be negative`},
		{in: "24 00", wantErr: `invalid passband "24 00": invalid syntax`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parsePassband(tt.in)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRigApplyCmd(t *testing.T) {
	boom := errors.New("boom")
	tests := []struct {
		name      string
		change    rigChange
		err       error
		wantFreqs []int64
		wantModes []modeCall
		wantErr   string
	}{
		{
			name:      "freq",
			change:    rigChange{setFreq: true, hz: 7074000},
			wantFreqs: []int64{7074000},
		},
		{
			name:      "mode",
			change:    rigChange{setMode: true, mode: "CW", passband: 500},
			wantModes: []modeCall{{"CW", 500}},
		},
		{
			name:      "freq then mode",
			change:    rigChange{setFreq: true, hz: 14074000, setMode: true, mode: "PKTUSB"},
			wantFreqs: []int64{14074000},
			wantModes: []modeCall{{"PKTUSB", 0}},
		},
		{
			name:      "a freq error stops the mode",
			change:    rigChange{setFreq: true, hz: 14074000, setMode: true, mode: "USB"},
			err:       boom,
			wantFreqs: []int64{14074000},
			wantErr:   "set frequency 14.074.000: boom",
		},
		{
			name:      "mode error",
			change:    rigChange{setMode: true, mode: "USB", passband: 2400},
			err:       boom,
			wantModes: []modeCall{{"USB", 2400}},
			wantErr:   "set mode USB 2400: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRig{err: tt.err}
			msg := rigApplyCmd(f, 7, tt.change)()
			res, ok := msg.(rigResultMsg)
			require.True(t, ok, "got %T", msg)
			assert.Equal(t, 7, res.gen)
			assert.Equal(t, tt.wantFreqs, f.freqs)
			assert.Equal(t, tt.wantModes, f.modes)
			if tt.wantErr == "" {
				assert.NoError(t, res.err)
				return
			}
			require.ErrorIs(t, res.err, boom)
			assert.Equal(t, tt.wantErr, res.err.Error())
		})
	}
}

func TestRigStatus(t *testing.T) {
	state := rig.StateChanged{State: rig.State{Freq: 14074000, Mode: "USB", Passband: 2400}}
	tests := []struct {
		name     string
		msgs     []tea.Msg
		want     []string
		wantNot  []string
		wantMute bool
	}{
		{
			name: "before any state",
			want: []string{"—", "ic7300 connecting"},
		},
		{
			name: "connecting with state",
			msgs: []tea.Msg{state},
			want: []string{"14.074.000 USB", "ic7300 connecting"},
		},
		{
			name:    "connected",
			msgs:    []tea.Msg{rig.ConnectionChanged{Status: rig.Connected}, state},
			want:    []string{"14.074.000 USB", "ic7300 connected"},
			wantNot: []string{"not connected"},
		},
		{
			name: "passband is not shown",
			msgs: []tea.Msg{
				rig.ConnectionChanged{Status: rig.Connected},
				rig.StateChanged{State: rig.State{Freq: 7074000, Mode: "PKTUSB", Passband: 3000}},
			},
			want:    []string{"7.074.000 PKTUSB", "connected"},
			wantNot: []string{"3000"},
		},
		{
			name: "disconnected while retrying",
			msgs: []tea.Msg{
				state,
				rig.ConnectionChanged{Status: rig.Disconnected, Err: errors.New("refused"), RetryIn: 4 * time.Second},
			},
			want:    []string{"14.074.000 USB", "ic7300 not connected"},
			wantNot: []string{"retry", "refused"},
		},
		{
			name: "disconnected",
			msgs: []tea.Msg{rig.ConnectionChanged{Status: rig.Disconnected}},
			want: []string{"—", "ic7300 not connected"},
		},
		{
			name: "command results are ignored",
			msgs: []tea.Msg{
				rig.ConnectionChanged{Status: rig.Connected},
				state,
				rig.CommandResult{Command: "set_freq 1", Err: errors.New("boom")},
			},
			want:    []string{"14.074.000 USB", "connected"},
			wantNot: []string{"boom"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newRigModel(t)
			for _, msg := range tt.msgs {
				m, _ = update(t, m, msg)
			}
			header := m.headerView()
			for _, w := range tt.want {
				assert.Contains(t, header, w)
			}
			for _, w := range tt.wantNot {
				assert.NotContains(t, header, w)
			}
		})
	}
}

func TestRigStatusStaleStyle(t *testing.T) {
	state := rig.StateChanged{State: rig.State{Freq: 14074000, Mode: "USB"}}
	m, _ := newRigModel(t)
	m, _ = update(t, m, state)

	stale := m.styles.stale.Render("14.074.000 USB")
	for _, status := range []rig.ConnStatus{rig.Connecting, rig.Disconnected} {
		m, _ = update(t, m, rig.ConnectionChanged{Status: status})
		assert.Contains(t, m.rigStatusView(), stale, "%s should render stale", status)
	}
	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Connected})
	assert.Contains(t, m.rigStatusView(), m.styles.rigFreq.Render("14.074.000 USB"))
}

func TestNoRigHeader(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = update(t, m, rig.StateChanged{State: rig.State{Freq: 14074000, Mode: "USB"}})
	assert.NotContains(t, m.View().Content, "14.074.000")
	assert.NotContains(t, m.View().Content, "—")
}

func TestControllerErrorShown(t *testing.T) {
	m, _ := newRigModel(t)

	// A rig result with the modal closed shows in the footer.
	m, _ = update(t, m, rigResultMsg{err: errors.New("rig busy")})
	assert.Contains(t, m.View().Content, "rig busy")

	// A key press clears the error.
	m, _ = update(t, m, keyPress('x', 0))
	assert.NotContains(t, m.View().Content, "rig busy")

	// So does the next successful command.
	m, _ = update(t, m, rigResultMsg{err: errors.New("rig busy")})
	assert.Contains(t, m.View().Content, "rig busy")
	m, _ = update(t, m, rigResultMsg{})
	assert.NotContains(t, m.View().Content, "rig busy")
}

// markPatternKey opens the mark-pattern prompt, a text prompt with no
// input to start with.
var markPatternKey = keyPress('T', 0)

func TestPromptCapturesKeys(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24, WithRig("ic7300", &fakeRig{}))
	m = press(t, m, markPatternKey)

	// q, ?, f and m are ordinary input while the prompt is open.
	m = typeText(t, m, "q?fm")
	assert.True(t, m.promptOpen())
	assert.False(t, m.helpModal.open, "? should not open help in the prompt")
	assert.False(t, m.rigModal.open, "f and m should not open the rig modal in the prompt")
	assert.Equal(t, "q?fm", m.input.Value())
}

func TestPromptCtrlCQuits(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, markPatternKey)
	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPromptEscCancels(t *testing.T) {
	m, s := newIndexModel(t, 3, 80, 24)
	m = press(t, m, markPatternKey)
	m = typeText(t, m, "~b 20m")

	m = press(t, m, escKey)
	assert.False(t, m.promptOpen())
	assert.Zero(t, s.idsCalls)

	// Reopening starts with an empty input.
	m = press(t, m, markPatternKey)
	assert.Empty(t, m.input.Value())
}

func TestPromptHelp(t *testing.T) {
	m, _ := newIndexModel(t, 3, 80, 24)
	m = press(t, m, markPatternKey)
	v := m.View().Content
	assert.Contains(t, v, "enter")
	assert.Contains(t, v, "esc")
	assert.NotContains(t, v, "quit", "global help should be replaced by prompt help")
}

// Running without the rig attached is common, so the reason is left to the
// debug log.
func TestConnectionErrorHidden(t *testing.T) {
	m, _ := newRigModel(t)
	reason := errors.New("connect to rigctld at localhost:4532: connection refused")

	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Disconnected, Err: reason})
	assert.Contains(t, m.View().Content, "ic7300 not connected")
	assert.NotContains(t, m.View().Content, "connection refused")
}

func TestNoControllerRigKeysDisabled(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, r := range []rune{'f', 'm'} {
		next, cmd := update(t, m, keyPress(r, 0))
		assert.False(t, next.promptOpen(), "%c should not open a prompt", r)
		assert.False(t, next.rigModal.open, "%c should not open the rig modal", r)
		assert.Nil(t, cmd)
	}
	assert.NotContains(t, m.View().Content, "freq")
	assert.NotContains(t, m.View().Content, "mode")
}

func TestWindowSizeWithRig(t *testing.T) {
	state := rig.StateChanged{State: rig.State{Freq: 144174000, Mode: "PKTUSB", Passband: 3000}}
	disc := rig.ConnectionChanged{Status: rig.Disconnected, RetryIn: 30 * time.Second}
	tests := []struct {
		name          string
		width, height int
		showAll       bool
		keys          []tea.Msg
	}{
		{"short help", 80, 24, false, nil},
		{"help open", 80, 24, true, nil},
		{"small terminal", 40, 10, false, nil},
		{"tiny terminal", 20, 10, false, nil},
		{"modal", 40, 10, false, []tea.Msg{rigKey}},
		{"modal mode", 40, 10, false, []tea.Msg{rigKey, tabKey}},
		{"modal mode tall", 80, 24, false, []tea.Msg{rigKey, tabKey}},
		{"modal mode tiny", 20, 10, false, []tea.Msg{rigKey, tabKey}},
		{"modal error", 40, 10, false, []tea.Msg{rigKey, keyPress('x', 0), enterKey}},
		{"modal error tall", 80, 24, false, []tea.Msg{rigKey, keyPress('x', 0), enterKey}},
		{"modal help", 40, 10, false, []tea.Msg{rigKey, tabKey, special(tea.KeyF1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(WithRig("ic7300", &fakeRig{}))
			if tt.showAll {
				m = m.openHelp()
			}
			msgs := append([]tea.Msg{
				tea.WindowSizeMsg{Width: tt.width, Height: tt.height}, state, disc,
			}, tt.keys...)
			for _, msg := range msgs {
				m, _ = update(t, m, msg)
			}

			v := m.View()
			assert.Equal(t, tt.height, lipgloss.Height(v.Content), "view should fill the terminal height")
			assert.LessOrEqual(t, lipgloss.Width(v.Content), tt.width, "view should not exceed the terminal width")
		})
	}
}
