package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"jstlogger/internal/adif"
)

var listOptions = []adif.Enum{
	{},
	{Code: "B", Description: "bureau"},
	{Code: "D", Description: "direct"},
	{Code: "E", Description: "electronic"},
	{Code: "MS", Description: "meteor scatter"},
}

// listKeys sends each key to l, returning it and the last item picked.
func listKeys(l pickList, keys ...tea.KeyPressMsg) (pickList, pickItem, bool) {
	var it pickItem
	var ok bool
	for _, k := range keys {
		it, ok = l.update(k)
	}
	return l, it, ok
}

func TestPickListOpensOnTheValue(t *testing.T) {
	l := newPickList(nil, listOptions, "e")
	assert.True(t, l.open)
	assert.Equal(t, "E", l.items()[l.cursor].Code)

	l = newPickList(nil, listOptions, "nosuch")
	assert.Equal(t, 0, l.cursor, "a value none of the options starts at the top")

	l = newPickList([]adif.Enum{{Code: "MS"}}, listOptions, "")
	assert.Equal(t, 0, l.cursor, "no value starts at the top, on the common items")
}

func TestPickListCommonFirst(t *testing.T) {
	common := []adif.Enum{{Code: "MS", Description: "meteor"}, {Code: "FT4", Description: "MFSK submode"}}
	l := newPickList(common, listOptions, "MS")

	assert.Equal(t, []string{"MS", "FT4", "", "B", "D", "E", "MS"}, adif.Codes(itemEnums(l.items())))
	assert.Equal(t, 0, l.cursor, "the value is highlighted among the common ones")
	assert.Equal(t, 2, l.sep(), "the full list follows the common ones")
	items := l.items()
	assert.Equal(t, -1, items[1].i, "a common item none of the options picks no option")
	assert.Equal(t, 4, items[0].i, "a common item that is an option picks it")
}

func TestPickListFilters(t *testing.T) {
	common := []adif.Enum{{Code: "MS", Description: "meteor scatter"}}
	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{"by code", "ms", []string{"MS"}},
		{"by description", "meteor", []string{"MS"}},
		{"ignoring case", "BUREAU", []string{"B"}},
		{"within a word", "rect", []string{"D"}},
		{"several", "e", []string{"MS", "B", "D", "E"}},
		{"none", "zzz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, _, _ := listKeys(newPickList(common, listOptions, ""), typeKeys(tt.filter)...)
			assert.Equal(t, tt.filter, l.filter)
			assert.Equal(t, tt.want, nilIfEmpty(adif.Codes(itemEnums(l.items()))))
			assert.Equal(t, -1, l.sep(), "a filtered list has no separator")
		})
	}
}

func TestPickListErase(t *testing.T) {
	l, _, _ := listKeys(newPickList(nil, listOptions, ""), append(typeKeys("bur"), special(tea.KeyBackspace))...)
	assert.Equal(t, "bu", l.filter)
}

func TestPickListMovesAndPicks(t *testing.T) {
	up, down, enter := special(tea.KeyUp), special(tea.KeyDown), special(tea.KeyEnter)

	l, it, ok := listKeys(newPickList(nil, listOptions, "B"), down, down, enter)
	assert.True(t, ok)
	assert.Equal(t, "E", it.Code)
	assert.Equal(t, 3, it.i)
	assert.False(t, l.open, "picking closes the list")

	l, _, _ = listKeys(newPickList(nil, listOptions, ""), up)
	assert.Equal(t, 0, l.cursor, "up stops at the top")
	l, _, _ = listKeys(newPickList(nil, listOptions, "MS"), down)
	assert.Equal(t, 4, l.cursor, "down stops at the bottom")

	_, it, ok = listKeys(newPickList(nil, listOptions, ""), append(typeKeys("bureau"), enter)...)
	assert.True(t, ok)
	assert.Equal(t, "B", it.Code, "enter picks the highlighted match")

	l, _, ok = listKeys(newPickList(nil, listOptions, ""), append(typeKeys("zzz"), enter)...)
	assert.False(t, ok, "nothing to pick")
	assert.True(t, l.open)
}

func TestPickListCancels(t *testing.T) {
	l, _, ok := listKeys(newPickList(nil, listOptions, "B"), special(tea.KeyDown), special(tea.KeyEscape))
	assert.False(t, ok)
	assert.False(t, l.open)
}

func TestPickListView(t *testing.T) {
	st := defaultStyles(true)
	common := []adif.Enum{{Code: "FT4", Description: "MFSK submode"}}
	l := newPickList(common, listOptions, "D")

	lines := l.view(st, 40)
	text := plain(strings.Join(lines, "\n"))
	assert.Contains(t, text, "type to filter")
	assert.Contains(t, text, "FT4  MFSK submode")
	assert.Contains(t, text, "D    direct")
	assert.Contains(t, text, "MS   meteor scatter")
	assert.Contains(t, text, "──", "a line separates the common items")
	assert.Less(t, strings.Index(text, "FT4"), strings.Index(text, "──"))

	l, _, _ = listKeys(l, typeKeys("zzz")...)
	text = plain(strings.Join(l.view(st, 40), "\n"))
	assert.Contains(t, text, "zzz")
	assert.Contains(t, text, "no matches")
}

func TestPickListViewScrolls(t *testing.T) {
	st := defaultStyles(true)
	var options []adif.Enum
	for _, c := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		options = append(options, adif.Enum{Code: string(c)})
	}
	l := newPickList(nil, options, "Z")

	lines := l.view(st, 20)
	assert.Len(t, lines, pickListHeight+1, "the filter line and a window of items")
	assert.Equal(t, "Z", strings.TrimSpace(plain(lines[len(lines)-1])), "the highlighted item shows")
}

func TestPickListViewFitsNarrowWidths(t *testing.T) {
	st := defaultStyles(true)
	for _, width := range []int{0, 1, 5, 12} {
		for _, line := range newPickList(nil, listOptions, "MS").view(st, width) {
			assert.LessOrEqual(t, lipgloss.Width(line), width, "%q", plain(line))
		}
	}
}

func itemEnums(items []pickItem) []adif.Enum {
	out := make([]adif.Enum, len(items))
	for i, it := range items {
		out[i] = it.Enum
	}
	return out
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}
