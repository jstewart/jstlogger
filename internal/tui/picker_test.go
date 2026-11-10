package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

// pick sends each key to p and returns it.
func pick(p picker, keys ...tea.KeyPressMsg) picker {
	for _, k := range keys {
		p.update(k)
	}
	return p
}

// typeKeys returns a key press for each rune of s.
func typeKeys(s string) []tea.KeyPressMsg {
	var out []tea.KeyPressMsg
	for _, r := range s {
		out = append(out, keyPress(r, 0))
	}
	return out
}

var pickerOptions = []string{"", "AM", "CW", "FM", "FT8", "MFSK", "SSB"}

func TestPickerSteps(t *testing.T) {
	right, left := special(tea.KeyRight), special(tea.KeyLeft)
	down, up := special(tea.KeyDown), special(tea.KeyUp)
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{"starts on the first option", nil, ""},
		{"right", []tea.KeyPressMsg{right, right}, "CW"},
		{"down", []tea.KeyPressMsg{down}, "AM"},
		{"left", []tea.KeyPressMsg{right, right, left}, "AM"},
		{"up", []tea.KeyPressMsg{right, up}, ""},
		{"stops at the first", []tea.KeyPressMsg{left}, ""},
		{"stops at the last", []tea.KeyPressMsg{special(tea.KeyEnd), right}, "SSB"},
		{"home", []tea.KeyPressMsg{right, right, special(tea.KeyHome)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pick(newPicker(pickerOptions), tt.keys...)
			assert.Equal(t, tt.want, p.value())
		})
	}
}

func TestPickerTypeAhead(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{"first match", typeKeys("f"), "FM"},
		{"longer prefix", typeKeys("ft"), "FT8"},
		{"case is ignored", typeKeys("ss"), "SSB"},
		{"no match starts again", typeKeys("ftc"), "CW"},
		{"no match at all keeps the value", typeKeys("fz"), "FM"},
		{"backspace shortens", append(typeKeys("ft"), special(tea.KeyBackspace)), "FM"},
		{"backspace to nothing clears", append(typeKeys("f"), special(tea.KeyBackspace)), ""},
		{"stepping forgets what was typed", append(typeKeys("f"), special(tea.KeyRight), keyPress('a', 0)), "AM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := pick(newPicker(pickerOptions), tt.keys...)
			assert.Equal(t, tt.want, p.value())
		})
	}
}

func TestPickerReportsChange(t *testing.T) {
	p := newPicker(pickerOptions)
	assert.True(t, p.update(special(tea.KeyRight)))
	assert.False(t, p.update(keyPress('a', 0)), "already on AM")
	assert.False(t, p.update(special(tea.KeyTab)), "tab is not a picker key")
}

func TestPickerSet(t *testing.T) {
	p := newPicker(pickerOptions)

	assert.True(t, p.set("ssb"))
	assert.Equal(t, "SSB", p.value())
	assert.False(t, p.set("NOSUCH"))
	assert.Equal(t, "SSB", p.value(), "an unknown value is not set")
}

func TestPickerSetOptions(t *testing.T) {
	p := newPicker([]string{"", "LSB", "USB"})
	p.set("USB")

	p.setOptions([]string{"", "FM", "USB"})
	assert.Equal(t, "USB", p.value(), "a value still offered is kept")

	p.setOptions([]string{"", "FT4"})
	assert.Equal(t, "", p.value(), "otherwise the first option is chosen")

	p.setOptions(nil)
	assert.Equal(t, "", p.value())
	assert.False(t, p.update(special(tea.KeyRight)))
}

func TestPickerView(t *testing.T) {
	st := defaultStyles()
	p := newPicker(pickerOptions)

	assert.Equal(t, pagerEmpty, plain(p.view(st, false)))
	assert.Equal(t, "‹ "+pagerEmpty+" ›", plain(p.view(st, true)))
	p.set("CW")
	assert.Equal(t, "CW", plain(p.view(st, false)))
	assert.Equal(t, "‹ CW ›", plain(p.view(st, true)))
}

func TestPickerEraseMultibyte(t *testing.T) {
	p := pick(newPicker([]string{"", "Ä", "ÄÖ"}), keyPress('ä', 0), keyPress('ö', 0))
	assert.Equal(t, "ÄÖ", p.value())
	p = pick(p, special(tea.KeyBackspace))
	assert.Equal(t, "Ä", p.value(), "backspace drops the whole last character")
}
