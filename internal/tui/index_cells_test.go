package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
	"jstlogger/internal/lotw"
)

// cellQSO is a QSO with every cell the index colors filled in.
func cellQSO(id int64, call, band string) logbook.QSO {
	return logbook.QSO{
		ID: id, StartUTC: time.Date(2026, 10, 31, 13, 34, 56, 0, time.UTC),
		Call: call, Band: band, Mode: "CW", RSTSent: "599", RSTRcvd: "579",
	}
}

func TestCellStyle(t *testing.T) {
	st := defaultStyles(true)
	f, err := indexfmt.Parse("%Z %{%Y-%m-%d} %-10c %m %s %r %b %L %T")
	require.NoError(t, err)
	q := cellQSO(1, "W1AW", "20m")
	q.Tags = []string{"sota", "pota"}

	tests := []struct {
		name   string
		field  byte
		row    indexfmt.Row
		recent lotw.Recency
		want   lipgloss.Style
		styled bool
	}{
		{name: "call", field: 'c', want: st.indexCall, styled: true},
		{name: "date", field: indexfmt.FieldTime, want: st.indexDim, styled: true},
		{name: "rst sent", field: 's', want: st.indexDim, styled: true},
		{name: "rst received", field: 'r', want: st.indexDim, styled: true},
		{name: "marked", field: 'Z', row: indexfmt.Row{Marked: true}, want: st.indexMark, styled: true},
		{name: "unmarked", field: 'Z'},
		{name: "band", field: 'b', want: st.bands["20m"], styled: true},
		{name: "mode", field: 'm'},
		{name: "lotw", field: indexfmt.FieldLoTW, row: indexfmt.Row{LoTW: true}, recent: lotw.Recent, want: st.lotwRecent, styled: true},
		{name: "lotw unknown", field: indexfmt.FieldLoTW},
		{name: "tags", field: indexfmt.FieldTags, want: st.indexDim, styled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cells := f.Values(q, tt.row, indexfmt.Km)
			j := slices.IndexFunc(f.Items, func(it indexfmt.Item) bool { return it.Field == tt.field })
			require.GreaterOrEqual(t, j, 0, "the format has the field")
			s, ok := cellStyle(st, f.Items[j], cells[j], tt.row, tt.recent)
			assert.Equal(t, tt.styled, ok)
			if tt.styled {
				assert.Equal(t, tt.want, s)
			}
		})
	}
}

func TestCellStyleBands(t *testing.T) {
	st := defaultStyles(true)
	it := indexfmt.Item{Field: 'b'}
	for band, c := range map[string]string{
		"160m": "9", "80m": "1", "40m": "3", "30m": "11", "20m": "2",
		"17m": "14", "15m": "4", "12m": "13", "10m": "5", "6m": "12",
	} {
		s, ok := cellStyle(st, it, band, indexfmt.Row{}, lotw.Unknown)
		require.True(t, ok, band)
		assert.Equal(t, lipgloss.Color(c), s.GetForeground(), band)
	}
	s, ok := cellStyle(st, it, "20M", indexfmt.Row{}, lotw.Unknown)
	assert.True(t, ok, "the band is matched ignoring case")
	assert.Equal(t, lipgloss.Color("2"), s.GetForeground())

	for _, band := range []string{"60m", "2m", "70cm", ""} {
		_, ok := cellStyle(st, it, band, indexfmt.Row{}, lotw.Unknown)
		assert.False(t, ok, "%q keeps the default foreground", band)
	}
}

func TestIndexCellStyles(t *testing.T) {
	st := defaultStyles(true)
	f, err := indexfmt.Parse(indexfmt.Default)
	require.NoError(t, err)
	m := newIndexLoTWModel(t, nil, indexfmt.Default, cellQSO(2, "K1ABC", "40m"), cellQSO(1, "W1AW", "20m"))

	line := rowLine(t, m, "W1AW")
	assert.Contains(t, line, st.indexCall.Render(pad("W1AW", 10)), "the call is bold accent")
	assert.Contains(t, line, st.indexDim.Render(pad("2026-10-31", 10)), "the date is muted")
	assert.Contains(t, line, st.indexDim.Render(pad("13:34", 5)), "the time is muted")
	assert.Contains(t, line, st.indexDim.Render(pad("599", 5)), "the RST sent is muted")
	assert.Contains(t, line, st.indexDim.Render(pad("579", 5)), "the RST received is muted")
	assert.Contains(t, line, st.bands["20m"].Render(pad("20m", 5)), "the band has its color")
	assert.Contains(t, line, "CW", "the mode is plain")

	want := formatLine(f, f.Values(cellQSO(1, "W1AW", "20m"), indexfmt.Row{}, indexfmt.Km), m.bodyWidth())
	assert.Equal(t, " "+want+" ", plain(line), "the styles leave the widths alone")

	cursor := cursorRow(t, m, "K1ABC")
	assert.Contains(t, cursor, onBar(st.cursor, st.indexCall).Render(pad("K1ABC", 10)), "the cursor row keeps the call's color")
	assert.Contains(t, cursor, onBar(st.cursor, st.bands["40m"]).Render(pad("40m", 5)))
	assert.Empty(t, offStyle(cursor, barBg), "the bar is unbroken")
	assert.Equal(t, m.bodyWidth(), lipgloss.Width(cursor))
}

func TestIndexHeaderStyle(t *testing.T) {
	for _, dark := range []bool{true, false} {
		h := defaultStyles(dark).indexHeader
		assert.True(t, h.GetBold(), "dark=%t: the header is bold", dark)
		assert.Equal(t, accent, h.GetForeground(), "dark=%t: the header is the accent color, as the title is", dark)
	}

	st := defaultStyles(true)
	f, err := indexfmt.Parse(indexfmt.Default)
	require.NoError(t, err)
	m := newIndexLoTWModel(t, nil, indexfmt.Default, cellQSO(1, "W1AW", "20m"))
	header := rowLine(t, m, "Call")
	assert.Contains(t, header, st.indexHeader.Render(formatLine(f, f.Titles(), m.bodyWidth())), "the header renders in its style")
}

func TestIndexMarkStyle(t *testing.T) {
	st := defaultStyles(true)
	m := newIndexLoTWModel(t, nil, indexfmt.Default, cellQSO(2, "K1ABC", "40m"), cellQSO(1, "W1AW", "20m"))
	assert.True(t, strings.HasPrefix(rowLine(t, m, "W1AW"), "   \x1b["), "an unmarked row's mark cell is plain")

	m = pressPager(t, m, keyPress('t', 0))
	require.Len(t, m.idx.marks, 1, "t marks the row and moves on")

	assert.Contains(t, rowLine(t, m, "K1ABC"), st.indexMark.Render("*"), "the mark is bold yellow")
	m = pressPager(t, m, keyPress('k', 0))
	assert.Contains(t, rowLine(t, m, "K1ABC"), onBar(st.cursor, st.indexMark).Render("*"), "and on the bar")
}

func TestIndexCellStylesFollowTheField(t *testing.T) {
	st := defaultStyles(true)
	m := newIndexLoTWModel(t, nil, "%-4b|%c", cellQSO(2, "K1ABC", "10m"), cellQSO(1, "W1AW", "160m"))

	line := rowLine(t, m, "W1AW")
	assert.Contains(t, line, st.bands["160m"].Render("160m"))
	assert.Contains(t, line, st.indexCall.Render("W1AW"))
	assert.Contains(t, line, "|", "a literal is plain")
}

func TestIndexCellStylesLightBackground(t *testing.T) {
	st := defaultStyles(false)
	for band, c := range map[string]string{"40m": "136", "30m": "172", "17m": "30", "20m": "2"} {
		assert.Equal(t, lipgloss.Color(c), st.bands[band].GetForeground(), band)
	}
	assert.Equal(t, lipgloss.Color("136"), st.indexMark.GetForeground(), "the mark uses the darker yellow")
	assert.Equal(t, lipgloss.Color("244"), st.indexDim.GetForeground())

	m := New(WithBackground(false))
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, st.bands["40m"], m.styles.bands["40m"])
}
