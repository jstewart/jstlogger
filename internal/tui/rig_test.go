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
	m, _ := update(t, New(WithRig(f)), tea.WindowSizeMsg{Width: 80, Height: 24})
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

func TestParseMode(t *testing.T) {
	tests := []struct {
		in           string
		wantMode     string
		wantPassband int
		wantErr      bool
	}{
		{in: "usb", wantMode: "USB"},
		{in: "USB 2400", wantMode: "USB", wantPassband: 2400},
		{in: "  pktusb   3000 ", wantMode: "PKTUSB", wantPassband: 3000},
		{in: "cw 0", wantMode: "CW"},
		{in: "", wantErr: true},
		{in: "   ", wantErr: true},
		{in: "usb wide", wantErr: true},
		{in: "usb 2400.5", wantErr: true},
		{in: "usb -1", wantErr: true},
		{in: "usb 2400 extra", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			mode, passband, err := parseMode(tt.in)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMode, mode)
			assert.Equal(t, tt.wantPassband, passband)
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
			want: []string{"—", "connecting"},
		},
		{
			name: "connecting with state",
			msgs: []tea.Msg{state},
			want: []string{"14.074.000 USB 2400", "connecting"},
		},
		{
			name: "connected",
			msgs: []tea.Msg{rig.ConnectionChanged{Status: rig.Connected}, state},
			want: []string{"14.074.000 USB 2400", "connected"},
		},
		{
			name: "connected default passband",
			msgs: []tea.Msg{
				rig.ConnectionChanged{Status: rig.Connected},
				rig.StateChanged{State: rig.State{Freq: 7074000, Mode: "PKTUSB"}},
			},
			want:    []string{"7.074.000 PKTUSB", "connected"},
			wantNot: []string{"PKTUSB 0"},
		},
		{
			name: "disconnected with retry",
			msgs: []tea.Msg{
				state,
				rig.ConnectionChanged{Status: rig.Disconnected, Err: errors.New("refused"), RetryIn: 4 * time.Second},
			},
			want: []string{"14.074.000 USB 2400", "disconnected · retry 4s"},
		},
		{
			name:    "disconnected without retry",
			msgs:    []tea.Msg{rig.ConnectionChanged{Status: rig.Disconnected}},
			want:    []string{"—", "disconnected"},
			wantNot: []string{"retry"},
		},
		{
			name: "command results are ignored",
			msgs: []tea.Msg{
				rig.ConnectionChanged{Status: rig.Connected},
				state,
				rig.CommandResult{Command: "set_freq 1", Err: errors.New("boom")},
			},
			want:    []string{"14.074.000 USB 2400", "connected"},
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

func TestFreqPromptSubmit(t *testing.T) {
	m, f := newRigModel(t)

	m, _ = update(t, m, keyPress('f', 0))
	require.True(t, m.promptOpen(), "f should open the prompt")

	m = typeText(t, m, "7074 khz")
	m, cmd := update(t, m, enterKey)
	assert.False(t, m.promptOpen(), "enter should close the prompt")
	require.NotNil(t, cmd)
	assert.Empty(t, f.freqs, "controller should only be called when the command runs")

	m = run(t, m, cmd)
	assert.Equal(t, []int64{7074000}, f.freqs)
	assert.Nil(t, m.rigErr)
}

func TestModePromptSubmit(t *testing.T) {
	m, f := newRigModel(t)

	m, _ = update(t, m, keyPress('m', 0))
	require.True(t, m.promptOpen(), "m should open the prompt")

	m = typeText(t, m, "usb 2400")
	m, cmd := update(t, m, enterKey)
	assert.False(t, m.promptOpen())
	run(t, m, cmd)
	assert.Equal(t, []modeCall{{"USB", 2400}}, f.modes)
}

func TestPromptCapturesKeys(t *testing.T) {
	m, _ := newRigModel(t)
	m, _ = update(t, m, keyPress('m', 0))

	// q, ? and f are ordinary input while the prompt is open.
	m = typeText(t, m, "q?f")
	assert.True(t, m.promptOpen())
	assert.False(t, m.help.ShowAll, "? should not toggle help in the prompt")
	assert.Equal(t, "q?f", m.input.Value())
}

func TestPromptCtrlCQuits(t *testing.T) {
	m, _ := newRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	_, cmd := update(t, m, keyPress('c', tea.ModCtrl))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPromptEscCancels(t *testing.T) {
	m, f := newRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = typeText(t, m, "14.074")

	m, cmd := update(t, m, escKey)
	assert.False(t, m.promptOpen())
	assert.Nil(t, cmd)
	assert.Empty(t, f.freqs)

	// Reopening starts with an empty input.
	m, _ = update(t, m, keyPress('f', 0))
	assert.Empty(t, m.input.Value())
}

func TestPromptParseErrorKeepsPromptOpen(t *testing.T) {
	m, f := newRigModel(t)
	m, _ = update(t, m, keyPress('f', 0))
	m = typeText(t, m, "abc")

	m, cmd := update(t, m, enterKey)
	assert.Nil(t, cmd)
	assert.True(t, m.promptOpen())
	require.Error(t, m.promptErr)
	assert.Contains(t, m.View().Content, m.promptErr.Error())
	assert.Empty(t, f.freqs)

	// Fixing the input and resubmitting succeeds.
	for range 3 {
		m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m = typeText(t, m, "14.074")
	m, cmd = update(t, m, enterKey)
	assert.False(t, m.promptOpen())
	run(t, m, cmd)
	assert.Equal(t, []int64{14074000}, f.freqs)
}

func TestPromptHelp(t *testing.T) {
	m, _ := newRigModel(t)
	assert.Contains(t, m.View().Content, "freq")
	assert.Contains(t, m.View().Content, "mode")

	m, _ = update(t, m, keyPress('f', 0))
	v := m.View().Content
	assert.Contains(t, v, "enter")
	assert.Contains(t, v, "esc")
	assert.NotContains(t, v, "quit", "global help should be replaced by prompt help")
}

func TestControllerErrorShown(t *testing.T) {
	m, f := newRigModel(t)
	f.err = errors.New("rig busy")

	m, _ = update(t, m, keyPress('f', 0))
	m = typeText(t, m, "14.074")
	m, cmd := update(t, m, enterKey)
	m = run(t, m, cmd)

	require.Error(t, m.rigErr)
	assert.ErrorIs(t, m.rigErr, f.err)
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

func TestConnectionErrorShown(t *testing.T) {
	m, _ := newRigModel(t)
	reason := errors.New("start rigctld: executable file not found")

	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Disconnected, Err: reason})
	assert.Contains(t, m.View().Content, "rig: start rigctld: executable file not found")

	// A command error takes the footer while it is shown.
	m, _ = update(t, m, rigResultMsg{err: errors.New("rig busy")})
	assert.Contains(t, m.View().Content, "rig busy")
	assert.NotContains(t, m.View().Content, "executable file not found")

	// Reconnecting clears the reason.
	m, _ = update(t, m, rigResultMsg{})
	m, _ = update(t, m, rig.ConnectionChanged{Status: rig.Connected})
	assert.NotContains(t, m.View().Content, "rig:")
}

func TestNoControllerRigKeysDisabled(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, r := range []rune{'f', 'm'} {
		next, cmd := update(t, m, keyPress(r, 0))
		assert.False(t, next.promptOpen(), "%c should not open a prompt", r)
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
		{"full help", 80, 24, true, nil},
		{"small terminal", 40, 10, false, nil},
		{"tiny terminal", 20, 10, false, nil},
		{"prompt open", 40, 10, false, []tea.Msg{keyPress('f', 0)}},
		{"prompt error", 40, 10, false, []tea.Msg{keyPress('f', 0), keyPress('x', 0), enterKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(WithRig(&fakeRig{}))
			m.help.ShowAll = tt.showAll
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
