package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func keyPress(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	k := tea.KeyPressMsg{Code: code, Mod: mod}
	if mod == 0 {
		k.Text = string(code)
	}
	return k
}

// update runs msg through m and returns the resulting Model.
func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	require.True(t, ok, "Update returned %T, want tui.Model", next)
	return got, cmd
}

func TestInit(t *testing.T) {
	assert.Nil(t, New().Init())
}

func TestQuitKeys(t *testing.T) {
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"q", keyPress('q', 0)},
		{"ctrl+c", keyPress('c', tea.ModCtrl)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cmd := update(t, New(), tt.msg)
			require.NotNil(t, cmd)
			assert.IsType(t, tea.QuitMsg{}, cmd())
		})
	}
}

func TestOtherKeysDoNotQuit(t *testing.T) {
	_, cmd := update(t, New(), keyPress('x', 0))
	assert.Nil(t, cmd)
}

func TestHelpToggle(t *testing.T) {
	m := New()
	require.False(t, m.help.ShowAll)

	m, _ = update(t, m, keyPress('?', 0))
	assert.True(t, m.help.ShowAll, "first ? should show full help")

	m, _ = update(t, m, keyPress('?', 0))
	assert.False(t, m.help.ShowAll, "second ? should hide full help")
}

func TestWindowSize(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		showAll       bool
	}{
		{"short help", 80, 24, false},
		{"full help", 80, 24, true},
		{"small terminal", 40, 10, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.help.ShowAll = tt.showAll
			m, _ = update(t, m, tea.WindowSizeMsg{Width: tt.width, Height: tt.height})

			assert.Equal(t, tt.width, m.width)
			assert.Equal(t, tt.height, m.height)

			v := m.View()
			assert.True(t, v.AltScreen)
			assert.Equal(t, tt.height, lipgloss.Height(v.Content), "view should fill the terminal height")
			assert.LessOrEqual(t, lipgloss.Width(v.Content), tt.width, "view should not exceed the terminal width")
		})
	}
}

func TestViewShowsEmptyState(t *testing.T) {
	m, _ := update(t, New(), tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Contains(t, m.View().Content, "No QSOs yet.")
}
