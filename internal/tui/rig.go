package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"jstlogger/internal/rig"
)

// RigController changes the rig's settings.
type RigController interface {
	SetFreq(ctx context.Context, hz int64) error
	SetMode(ctx context.Context, mode string, passband int) error
}

// Option configures a Model built by New.
type Option func(*Model)

// WithRig connects the UI to a rig: it shows the rig status in the header and
// enables the frequency and mode prompts, which send commands to c.
func WithRig(c RigController) Option {
	return func(m *Model) {
		m.rig = c
	}
}

// rigTimeout bounds each command sent to the rig controller.
const rigTimeout = 5 * time.Second

// rigResultMsg reports the outcome of a command sent to the rig controller.
type rigResultMsg struct {
	err error
}

// promptKind identifies the open input prompt, if any.
type promptKind int

const (
	promptNone promptKind = iota
	promptFreq
	promptMode
)

// rigState is the rig status as last reported by the rig events.
type rigState struct {
	state     rig.State
	haveState bool
	conn      rig.ConnectionChanged
}

// promptOpen reports whether an input prompt is capturing key presses.
func (m Model) promptOpen() bool {
	return m.prompt != promptNone
}

// newInput returns the text input used by the rig prompts.
func newInput() textinput.Model {
	ti := textinput.New()
	s := textinput.DefaultDarkStyles()
	// A static cursor keeps the prompt from scheduling blink ticks.
	s.Cursor.Blink = false
	ti.SetStyles(s)
	return ti
}

// openPrompt resets the input and opens the prompt of the given kind.
func (m Model) openPrompt(kind promptKind) (Model, tea.Cmd) {
	m.prompt = kind
	m.promptErr = nil
	m.input.Reset()
	switch kind {
	case promptFreq:
		m.input.Prompt = "freq> "
		m.input.Placeholder = "14.074, 7074 kHz…"
	case promptMode:
		m.input.Prompt = "mode> "
		m.input.Placeholder = "USB 2400…"
	}
	m.sizeInput()
	return m, m.input.Focus()
}

// closePrompt closes the open prompt.
func (m Model) closePrompt() Model {
	m.prompt = promptNone
	m.promptErr = nil
	m.input.Blur()
	return m
}

// sizeInput fits the input to the footer width.
func (m *Model) sizeInput() {
	m.input.SetWidth(max(m.width-m.styles.footer.GetHorizontalFrameSize()-lipgloss.Width(m.input.Prompt)-1, 1))
}

// updatePrompt handles a key press while a prompt is open. Every key goes to
// the prompt except submit, cancel and force quit.
func (m Model) updatePrompt(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.promptKeys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.promptKeys.Cancel):
		return m.closePrompt(), nil
	case key.Matches(msg, m.promptKeys.Submit):
		return m.submitPrompt()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// submitPrompt parses the prompt input. On success it closes the prompt and
// returns a command that applies the setting to the rig; on failure it keeps
// the prompt open and records the parse error.
func (m Model) submitPrompt() (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.prompt {
	case promptFreq:
		hz, err := parseFreq(m.input.Value())
		if err != nil {
			m.promptErr = err
			return m, nil
		}
		cmd = rigCmd(fmt.Sprintf("set frequency %s", formatFreq(hz)), func(ctx context.Context) error {
			return m.rig.SetFreq(ctx, hz)
		})
	case promptMode:
		mode, passband, err := parseMode(m.input.Value())
		if err != nil {
			m.promptErr = err
			return m, nil
		}
		cmd = rigCmd(fmt.Sprintf("set mode %s", mode), func(ctx context.Context) error {
			return m.rig.SetMode(ctx, mode, passband)
		})
	}
	return m.closePrompt(), cmd
}

// rigCmd returns a command that runs fn with a timeout and reports the result
// as a rigResultMsg. desc describes the command in the error.
func rigCmd(desc string, fn func(ctx context.Context) error) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), rigTimeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			return rigResultMsg{err: fmt.Errorf("%s: %w", desc, err)}
		}
		return rigResultMsg{}
	}
}

// promptView renders the open prompt and any parse error.
func (m Model) promptView() string {
	lines := []string{m.input.View()}
	if m.promptErr != nil {
		lines = append(lines, m.styles.err.Render(m.promptErr.Error()))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// rigStatusView renders the rig frequency, mode and connection status. The
// frequency and mode are shown stale unless the rig is connected. The
// passband is not shown: many Hamlib backends report a nominal width for
// the selected filter rather than the rig's real setting.
func (m Model) rigStatusView() string {
	s := m.rigState
	setting := "—"
	if s.haveState {
		setting = formatFreq(s.state.Freq) + " " + s.state.Mode
	}

	var status string
	switch s.conn.Status {
	case rig.Connected:
		setting = m.styles.rigFreq.Render(setting)
		status = m.styles.connected.Render(s.conn.Status.String())
	case rig.Disconnected:
		setting = m.styles.stale.Render(setting)
		text := s.conn.Status.String()
		if s.conn.RetryIn > 0 {
			text += " · retry " + s.conn.RetryIn.Round(time.Second).String()
		}
		status = m.styles.err.Render(text)
	default:
		setting = m.styles.stale.Render(setting)
		status = m.styles.muted.Render(s.conn.Status.String())
	}
	return setting + m.styles.muted.Render(" · ") + status
}

// formatFreq formats hz as megahertz with dotted grouping, e.g. 14.074.000.
func formatFreq(hz int64) string {
	return fmt.Sprintf("%d.%03d.%03d", hz/1_000_000, hz/1_000%1_000, hz%1_000)
}

// freqUnits maps unit suffixes to multipliers. Longer suffixes come first so
// "khz" is not mistaken for "hz".
var freqUnits = []struct {
	suffix string
	mult   float64
}{
	{"mhz", 1e6},
	{"khz", 1e3},
	{"hz", 1},
}

// parseFreq parses a frequency typed by the user and returns it in hertz. An
// optional hz, khz or mhz suffix sets the unit; without one, values below
// 1000 are megahertz, values below 1,000,000 are kilohertz and larger values
// are hertz.
func parseFreq(s string) (int64, error) {
	num := strings.ToLower(strings.TrimSpace(s))
	if num == "" {
		return 0, errors.New("enter a frequency")
	}

	mult := 0.0
	for _, u := range freqUnits {
		if rest, ok := strings.CutSuffix(num, u.suffix); ok {
			num, mult = strings.TrimSpace(rest), u.mult
			break
		}
	}

	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid frequency %q: %w", s, numErr(err))
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid frequency %q", s)
	}

	if mult == 0 {
		switch {
		case v < 1e3:
			mult = 1e6
		case v < 1e6:
			mult = 1e3
		default:
			mult = 1
		}
	}

	hz := math.Round(v * mult)
	if hz <= 0 {
		return 0, fmt.Errorf("frequency %q must be positive", s)
	}
	if hz >= math.MaxInt64 {
		return 0, fmt.Errorf("frequency %q is too large", s)
	}
	return int64(hz), nil
}

// parseMode parses "MODE [passband]" typed by the user. The mode is
// upper-cased; the passband is in hertz and defaults to 0, the rig's default.
func parseMode(s string) (mode string, passband int, err error) {
	fields := strings.Fields(s)
	switch len(fields) {
	case 0:
		return "", 0, errors.New("enter a mode")
	case 1, 2:
	default:
		return "", 0, fmt.Errorf("invalid mode %q: want MODE [passband]", s)
	}

	if len(fields) == 2 {
		passband, err = strconv.Atoi(fields[1])
		if err != nil {
			return "", 0, fmt.Errorf("invalid passband %q: %w", fields[1], numErr(err))
		}
		if passband < 0 {
			return "", 0, fmt.Errorf("invalid passband %q: must not be negative", fields[1])
		}
	}
	return strings.ToUpper(fields[0]), passband, nil
}

// numErr strips the strconv function and input from a parse error, which the
// caller already reports, leaving just the reason such as "invalid syntax".
func numErr(err error) error {
	if ne, ok := errors.AsType[*strconv.NumError](err); ok {
		return ne.Err
	}
	return err
}
