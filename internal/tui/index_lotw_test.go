package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
)

// lotwIndexFormat is an index format with a %L column.
const lotwIndexFormat = "%L %-10c"

// newIndexLoTWModel returns a Model in an 80 by 24 window showing qsos in
// the index laid out by format, with a fixed clock and, unless f is nil,
// the LoTW lookup f, and runs the lookups the initial load starts.
func newIndexLoTWModel(t *testing.T, f *fakeLoTW, format string, qsos ...logbook.QSO) Model {
	t.Helper()
	parsed, err := indexfmt.Parse(format)
	require.NoError(t, err)
	opts := []Option{
		WithLogbook(&fakeIndexStore{qsos: qsos}),
		WithClock(func() time.Time { return lotwNow }),
		WithIndexFormat(parsed, indexfmt.Km),
	}
	if f != nil {
		opts = append(opts, WithLoTW(f))
	}
	m := New(opts...)
	m = runPager(t, m, m.Init())
	m, cmd := update(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return runPager(t, m, cmd)
}

// rowLine returns the raw body line whose plain text contains call.
func rowLine(t *testing.T, m Model, call string) string {
	t.Helper()
	for l := range strings.SplitSeq(m.View().Content, "\n") {
		if strings.Contains(plain(l), call) {
			return l
		}
	}
	require.Failf(t, "no row", "no line shows %s", call)
	return ""
}

// lotwUsers is a fakeLoTW user list: W1AW uploaded recently, K1ABC within
// the year and N0OLD over a year ago.
func lotwUsers() map[string]lotwUser {
	return map[string]lotwUser{
		"W1AW":  {last: daysBefore(3)},
		"K1ABC": {last: daysBefore(90)},
		"N0OLD": {last: daysBefore(400)},
	}
}

func TestIndexLoTWColumn(t *testing.T) {
	st := defaultStyles(true)
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, lotwIndexFormat,
		lotwQSO(5, "K9XYZ"), lotwQSO(4, "W1AW"), lotwQSO(3, "K1ABC"), lotwQSO(2, "N0OLD"), lotwQSO(1, "P/QRP/7"))

	assert.Equal(t, [][]string{{"K1ABC", "K9XYZ", "N0OLD", "W1AW"}}, f.lookedUpBatches(),
		"one batch of the distinct base calls, without the call that has none")
	assert.Contains(t, plain(rowLine(t, m, "Call")), "L Call", "the column is titled L")
	assert.Contains(t, plain(rowLine(t, m, "K9XYZ")), "  K9XYZ", "a non-user is blank")
	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "L W1AW")
	assert.Contains(t, rowLine(t, m, "W1AW"), st.lotwRecent.Render("L"))
	assert.Contains(t, rowLine(t, m, "K1ABC"), st.lotwActive.Render("L"))
	assert.Contains(t, rowLine(t, m, "N0OLD"), st.lotwInactive.Render("L"))
	assert.Contains(t, plain(rowLine(t, m, "P/QRP/7")), "  P/QRP/7")
}

// cursorRow returns the index row that shows call, without the body's
// padding around it.
func cursorRow(t *testing.T, m Model, call string) string {
	t.Helper()
	line := rowLine(t, m, call)
	require.True(t, strings.HasPrefix(line, " ") && strings.HasSuffix(line, " "), "the body pads %q", line)
	return line[1 : len(line)-1]
}

func TestIndexLoTWCursorRowKeepsColor(t *testing.T) {
	st := defaultStyles(true)
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC"))

	line := cursorRow(t, m, "W1AW")
	assert.Contains(t, line, onBar(st.cursor, st.lotwRecent).Render("L"), "the cursor row's L keeps its color")
	assert.Empty(t, offStyle(line, barBg), "the bar is unbroken")
	assert.Equal(t, m.bodyWidth(), lipgloss.Width(line))

	m = pressPager(t, m, keyPress('j', 0))
	assert.Contains(t, rowLine(t, m, "W1AW"), st.lotwRecent.Render("L"), "off the cursor it is colored")
	assert.Contains(t, rowLine(t, m, "K1ABC"), onBar(st.cursor, st.lotwActive).Render("L"))
}

func TestOnBar(t *testing.T) {
	for _, dark := range []bool{true, false} {
		st := defaultStyles(dark)
		s := onBar(st.cursor, st.lotwRecent)
		assert.Equal(t, st.lotwRecent.GetForeground(), s.GetForeground(), "a cell keeps its color")
		assert.Equal(t, st.cursor.GetBackground(), s.GetBackground(), "on the bar")

		s = onBar(st.cursor, st.lotwInactive)
		assert.Equal(t, st.cursor.GetForeground(), s.GetForeground(), "a color the bar would hide is the bar's text color")
		assert.True(t, s.GetFaint(), "it is still dim")
		assert.False(t, s.GetBold(), "and not bold")

		s = onBar(st.withoutColor().cursor, st.lotwRecent)
		assert.True(t, s.GetReverse(), "without colors the cell is reversed with the rest of the row")
	}
}

func TestIndexLoTWCursorRowWithoutColors(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC"))
	m, _ = update(t, m, tea.ColorProfileMsg{Profile: colorprofile.ASCII})

	line := cursorRow(t, m, "W1AW")
	assert.Contains(t, plain(line), "L W1AW")
	assert.Empty(t, offStyle(line, "7"), "the reverse video is unbroken")
}

func TestIndexLoTWPagerStrip(t *testing.T) {
	st := defaultStyles(true)
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC"))

	m = pressPager(t, m, enter())
	require.True(t, m.pager.open)
	lines := strings.Split(m.View().Content, "\n")
	strip := strings.Join(lines[1:1+m.idx.height], "\n")
	assert.Contains(t, plain(strip), "L W1AW")
	assert.Contains(t, strip, onBar(st.cursor, st.lotwRecent).Render("L"), "the strip's cursor row keeps its color")
	assert.Empty(t, offStyle(cursorRow(t, m, "W1AW"), barBg), "the strip uses the bar")
	assert.Contains(t, strip, st.lotwActive.Render("L"))
}

func TestIndexLoTWBaseCalls(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(3, "K9XYZ"), lotwQSO(2, "VE3/W1AW/P"), lotwQSO(1, "w1aw"))

	assert.Equal(t, [][]string{{"K9XYZ", "W1AW"}}, f.lookedUpBatches())
	assert.Contains(t, plain(rowLine(t, m, "VE3/W1AW/P")), "L VE3/W1AW/P")
	assert.Contains(t, plain(rowLine(t, m, "w1aw")), "L w1aw")
}

func TestIndexLoTWNotInFormat(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, indexfmt.Default, lotwQSO(1, "W1AW"))

	assert.Empty(t, f.lookedUpBatches(), "no lookups without %L")
	assert.NotContains(t, plain(rowLine(t, m, "W1AW")), "L ")
}

func TestIndexLoTWWithoutLookup(t *testing.T) {
	m := newIndexLoTWModel(t, nil, lotwIndexFormat, lotwQSO(1, "W1AW"))

	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "  W1AW", "blank without a LoTW lookup")
}

func TestIndexLoTWLaterBlocks(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers()}
	qsos := make([]logbook.QSO, indexBlockSize+20)
	for i := range qsos {
		call := "W1AW"
		if i >= indexBlockSize {
			call = "K1ABC"
		}
		if i == len(qsos)-1 {
			call = "N0OLD"
		}
		qsos[i] = lotwQSO(int64(len(qsos)-i), call)
	}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, qsos...)
	require.Equal(t, [][]string{{"W1AW"}}, f.lookedUpBatches())

	m = pressPager(t, m, keyPress('*', 0))
	m = runPager(t, m, m.layout())

	assert.Equal(t, [][]string{{"W1AW"}, {"K1ABC", "N0OLD"}}, f.lookedUpBatches(),
		"a later block looks up only calls not already looked up")
	assert.Contains(t, plain(rowLine(t, m, "K1ABC")), "L K1ABC")
}

func TestIndexLoTWLookupError(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers(), err: errors.New("disk on fire")}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC"))

	assert.Contains(t, plain(m.footerView()), "disk on fire", "the error is shown on the status line")
	assert.Contains(t, plain(rowLine(t, m, "K1ABC")), "  K1ABC", "the rows stay blank")

	m = pressPager(t, m, keyPress('j', 0))
	m = runPager(t, m, m.layout())
	assert.Len(t, f.lookedUpBatches(), 1, "moving the cursor does not retry a failed lookup")
}

func TestIndexLoTWLookupRetriedOnReload(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers(), err: errors.New("database is locked")}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(2, "W1AW"), lotwQSO(1, "K1ABC"))
	require.Len(t, f.lookedUpBatches(), 1)

	f.mu.Lock()
	f.err = nil
	f.mu.Unlock()
	m = pressPager(t, m, keyPress('l', tea.ModCtrl))

	assert.Equal(t, [][]string{{"K1ABC", "W1AW"}, {"K1ABC", "W1AW"}}, f.lookedUpBatches(),
		"reloading the index retries the failed lookup")
	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "L W1AW")
}

func TestIndexLoTWKeptUntilSyncLookupLands(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers()}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(1, "W1AW"))
	require.Contains(t, plain(rowLine(t, m, "W1AW")), "L W1AW")

	m, cmd := update(t, m, LoTWSynced{})
	require.NotNil(t, cmd, "a successful sync looks the rows up again")
	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "L W1AW",
		"the old status shows until the new lookup lands")

	f.mu.Lock()
	delete(f.users, "W1AW")
	f.mu.Unlock()
	m = runPager(t, m, cmd)
	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "  W1AW", "the new lookup replaces it")
}

func TestIndexLoTWSynced(t *testing.T) {
	f := &fakeLoTW{users: map[string]lotwUser{}}
	m := newIndexLoTWModel(t, f, lotwIndexFormat, lotwQSO(2, "K9XYZ"), lotwQSO(1, "W1AW"))
	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "  W1AW")

	m, cmd := update(t, m, LoTWSynced{Err: errors.New("feed down")})
	m = runPager(t, m, cmd)
	assert.Len(t, f.lookedUpBatches(), 1, "a failed sync keeps the lookups")

	f.mu.Lock()
	f.users["W1AW"] = lotwUser{last: daysBefore(3)}
	f.mu.Unlock()
	m, cmd = update(t, m, LoTWSynced{})
	m = runPager(t, m, cmd)

	assert.Equal(t, [][]string{{"K9XYZ", "W1AW"}, {"K9XYZ", "W1AW"}}, f.lookedUpBatches(),
		"a successful sync looks the rows up again")
	assert.Contains(t, plain(rowLine(t, m, "W1AW")), "L W1AW")
}

func TestIndexLoTWStaleReplyDropped(t *testing.T) {
	f := &fakeLoTW{users: lotwUsers()}
	parsed, err := indexfmt.Parse(lotwIndexFormat)
	require.NoError(t, err)
	s := &fakeIndexStore{qsos: []logbook.QSO{lotwQSO(1, "W1AW")}}
	m := New(WithLogbook(s), WithLoTW(f), WithClock(func() time.Time { return lotwNow }), WithIndexFormat(parsed, indexfmt.Km))

	// Load the index, holding back the LoTW reply.
	var held []lotwUsersMsg
	for _, msg := range collectMsgs(m.Init()) {
		var cmd tea.Cmd
		m, cmd = update(t, m, msg)
		for _, msg := range collectMsgs(cmd) {
			if r, ok := msg.(lotwUsersMsg); ok {
				held = append(held, r)
			}
		}
	}
	require.Len(t, held, 1)

	// A sync finishes before the reply lands.
	m, _ = update(t, m, LoTWSynced{})
	m, _ = update(t, m, held[0])

	assert.Empty(t, m.idx.lotw.last, "a reply from before the sync is dropped")
}

func TestFormatStyledLine(t *testing.T) {
	st := defaultStyles(true)
	f, err := indexfmt.Parse("%-3L|%-*c")
	require.NoError(t, err)
	cells := []string{"L", "|", "W1AW"}
	style := func(i int, s string) string {
		if f.Items[i].Field != indexfmt.FieldLoTW {
			return s
		}
		return st.lotwRecent.Render(s)
	}

	for _, width := range []int{20, 2} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			line := formatStyledLine(f, cells, width, style)

			assert.Equal(t, formatLine(f, cells, width), plain(line), "styling does not change the text")
			assert.Equal(t, width, lipgloss.Width(line))
			assert.Contains(t, line, st.lotwRecent.Render(truncate("L  ", width, "")), "the cell is styled after it is padded and cut")
		})
	}
}
