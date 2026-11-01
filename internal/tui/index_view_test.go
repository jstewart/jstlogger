package tui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// sgr matches the ANSI styling sequences lipgloss emits.
var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plain strips styling from s.
func plain(s string) string {
	return sgr.ReplaceAllString(s, "")
}

func TestIndexCells(t *testing.T) {
	q := logbook.QSO{
		ID:              7,
		StartUTC:        time.Date(2026, 10, 31, 13, 34, 56, 0, time.FixedZone("EST", -5*3600)),
		Call:            "JA1ABC",
		Mode:            "MFSK",
		Submode:         "FT4",
		RSTSent:         "-10",
		RSTRcvd:         "+02",
		Band:            "20m",
		Country:         "Japan",
		StationCallsign: "W1AW",
	}

	assert.Equal(t,
		[]string{" ", "2026-10-31", "18:34", "JA1ABC", "FT4", "-10", "+02", "20m", "Japan", "W1AW"},
		indexCells(q, false), "UTC date and time without seconds, submode over mode")

	q.Submode = ""
	cells := indexCells(q, true)
	assert.Equal(t, "*", cells[0])
	assert.Equal(t, "MFSK", cells[4], "mode when there is no submode")
}

func TestFormatCells(t *testing.T) {
	cells := []string{"*", "2026-10-31", "13:34", "JA1ABC", "FT4", "-10", "+02", "20m", "Japan", "W1AW"}

	line := formatCells(cells, 78)
	assert.Equal(t, 78, lipgloss.Width(line), "the line fills the width")
	assert.Equal(t, "* 2026-10-31 13:34 JA1ABC     FT4    -10   +02   20m   Japan        W1AW      ", line)

	wide := formatCells(cells, 100)
	assert.Equal(t, 100, lipgloss.Width(wide))
	assert.Contains(t, wide, "Japan"+strings.Repeat(" ", 30)+"W1AW", "the country takes the extra width")

	narrow := formatCells(cells, 30)
	assert.Equal(t, "* 2026-10-31 13:34 JA1ABC     ", narrow, "a narrow window truncates the line")
}

func TestFormatCellsTruncatesValues(t *testing.T) {
	cells := []string{" ", "2026-10-31", "13:34", "VP2E/W1AW/P", "FT8", "", "", "20m", "Bosnia-Herzegovina", "W1AW/VE3XYZ"}

	line := formatCells(cells, 78)

	assert.Equal(t, 78, lipgloss.Width(line))
	assert.Contains(t, line, "VP2E/W1AW…")
	assert.Contains(t, line, "Bosnia-Herz…")
	assert.True(t, strings.HasSuffix(line, "W1AW/VE3X…"))
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s     string
		width int
		tail  string
		want  string
	}{
		{"short", 10, "…", "short"},
		{"exactly", 7, "…", "exactly"},
		{"truncated", 6, "…", "trunc…"},
		{"truncated", 6, "", "trunca"},
		{"Curaçao", 5, "…", "Cura…"},
		{"日本国", 5, "…", "日本…"},
		{"abc", 0, "…", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, truncate(tt.s, tt.width, tt.tail), "truncate(%q, %d, %q)", tt.s, tt.width, tt.tail)
	}
}

func TestFormatCount(t *testing.T) {
	tests := map[int]string{
		0:         "0",
		12:        "12",
		999:       "999",
		1000:      "1,000",
		4567:      "4,567",
		123456:    "123,456",
		1_234_567: "1,234,567",
		-4567:     "-4,567",
	}
	for n, want := range tests {
		assert.Equal(t, want, formatCount(n))
	}
}

func TestIndexView(t *testing.T) {
	x := loadedIndex(4_567, 5)
	x.cursor, x.top = 11, 8
	x.marks[x.blocks[0][9].ID] = true
	x.marks[x.blocks[0][11].ID] = true
	x.marks[42] = true

	lines := strings.Split(x.view(defaultStyles(), 78, 7), "\n")

	require.Len(t, lines, 7, "header, five rows and the status line")
	assert.True(t, strings.HasPrefix(plain(lines[0]), "  Date       Time  Call       Mode   RST S RST R Band  Country      Station"))
	assert.Contains(t, lines[1], "K4559A", "row 9 is the first visible")
	assert.True(t, strings.HasPrefix(lines[2], "* "), "row 10 is marked")
	assert.True(t, strings.HasPrefix(plain(lines[4]), "* "), "the cursor row can be marked too")
	assert.Contains(t, lines[4], "K4556A")
	assert.NotEqual(t, lines[4], plain(lines[4]), "the cursor row is highlighted")
	assert.Equal(t, 78, lipgloss.Width(lines[4]), "the cursor bar spans the row")
	assert.Equal(t, "QSO 12/4,567 · 3 marked", lines[6])
}

func TestIndexViewStatusMessage(t *testing.T) {
	x := loadedIndex(3, 5)
	x.msg = msgFirstQSO

	lines := strings.Split(x.view(defaultStyles(), 78, 7), "\n")

	require.Len(t, lines, 7, "the status line stays at the bottom")
	assert.Equal(t, "QSO 1/3 · You are on the first QSO.", lines[6])
}

func TestIndexViewPlaceholder(t *testing.T) {
	x := loadedIndex(1000, 5)
	delete(x.blocks, 1)
	x.last()

	lines := strings.Split(x.view(defaultStyles(), 40, 7), "\n")

	require.Len(t, lines, 7)
	for _, l := range lines[1:6] {
		assert.Equal(t, "  …", strings.TrimRight(plain(l), " "))
	}
}

func TestIndexViewStates(t *testing.T) {
	st := defaultStyles()

	assert.Equal(t, "Loading…", plain(newIndex().view(st, 78, 10)))
	assert.Equal(t, "No QSOs yet.", plain(loadedIndex(0, 8).view(st, 78, 10)))

	x := newIndex()
	x.applyLoad(indexLoadedMsg{err: assert.AnError})
	assert.Equal(t, assert.AnError.Error(), plain(x.view(st, 78, 10)))
}
