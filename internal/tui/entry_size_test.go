package tui

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/qsoentry"
)

// TestModelCopyIsSmall guards against widgets held by value in the Model,
// which Bubble Tea copies on every message.
func TestModelCopyIsSmall(t *testing.T) {
	const limit = 64 << 10
	size := unsafe.Sizeof(Model{})
	t.Logf("a Model is %d B", size)
	assert.Less(t, size, uintptr(limit), "a Model copy is under 64 KiB")
}

func TestEntryWidgetOfTheWrongKind(t *testing.T) {
	e := newEntry()
	assert.PanicsWithValue(t, "tui: Notes has no text input", func() { e.input(qsoentry.Notes) })
	assert.PanicsWithValue(t, "tui: Call has no text area", func() { e.area(qsoentry.Call) })
	assert.PanicsWithValue(t, "tui: Call has no picker", func() { e.picker(qsoentry.Call) })
}

func TestEntryReopensOnAFreshForm(t *testing.T) {
	m := newEntryModel(t, newEntryStore(t, nil))
	m = focusOn(t, m, qsoentry.Call)
	m = typeText(t, m, "K1ABC")
	held := m

	m = press(t, m, esc(), keyPress('y', 0))
	require.False(t, m.entry.open)
	assert.Equal(t, "K1ABC", entryValue(held, qsoentry.Call), "closing leaves a model held from before as it was")

	m = press(t, m, newQSOKey)
	require.True(t, m.entry.open)
	assert.Empty(t, entryValue(m, qsoentry.Call), "the form opens empty")
	m = focusOn(t, m, qsoentry.Call)
	m = typeText(t, m, "W1AW")

	assert.Equal(t, "W1AW", entryValue(m, qsoentry.Call))
	assert.Equal(t, "K1ABC", entryValue(held, qsoentry.Call), "the new form doesn't touch a model held from before")
}
