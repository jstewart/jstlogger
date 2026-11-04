package indexfmt

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// sampleQSO is a QSO with every field the expandos read set.
func sampleQSO() logbook.QSO {
	return logbook.QSO{
		StartUTC:        time.Date(2024, time.March, 5, 7, 8, 9, 0, time.FixedZone("EST", -5*3600)),
		Call:            "W1AW",
		StationCallsign: "K1ABC",
		Operator:        "K1XYZ",
		FreqHz:          sql.Null[int64]{V: 14_074_000, Valid: true},
		Band:            "20m",
		Mode:            "MFSK",
		Submode:         "FT4",
		RSTSent:         "-10",
		RSTRcvd:         "+03",
		Name:            "Hiram",
		Gridsquare:      "FN31",
		DXCC:            sql.Null[int]{V: 291, Valid: true},
		Country:         "United States",
		Cont:            "NA",
		DistanceKm:      sql.Null[float64]{V: 160.9344, Valid: true},
	}
}

func TestParseDefault(t *testing.T) {
	f, err := Parse(Default)
	require.NoError(t, err)

	type col struct {
		field byte
		width int
		flex  bool
		title string
	}
	want := []col{
		{'Z', 1, false, ""},
		{'{', 10, false, "Date"},
		{'{', 5, false, "Time"},
		{'c', 10, false, "Call"},
		{'m', 6, false, "Mode"},
		{'s', 5, false, "RST S"},
		{'r', 5, false, "RST R"},
		{'b', 5, false, "Band"},
		{'C', 0, true, "Country"},
		{'S', 10, false, "Station"},
	}
	require.Len(t, f.Items, 2*len(want)-1)
	titles := f.Titles()
	require.Len(t, titles, len(f.Items))
	for i, it := range f.Items {
		if i%2 == 1 {
			assert.True(t, it.IsLiteral(), "item %d", i)
			assert.Equal(t, " ", it.Literal, "item %d", i)
			assert.Equal(t, " ", titles[i], "item %d", i)
			continue
		}
		w := want[i/2]
		assert.False(t, it.IsLiteral(), "item %d", i)
		assert.Equal(t, w.field, it.Field, "item %d", i)
		assert.Equal(t, w.width, it.Width, "item %d", i)
		assert.Equal(t, w.flex, it.Flex, "item %d", i)
		assert.Equal(t, w.title, titles[i], "item %d", i)
		if w.field != 'Z' {
			assert.True(t, it.Left, "item %d", i)
		}
	}
	assert.Equal(t, "%Y-%m-%d", f.Items[2].Time)
	assert.Equal(t, "%H:%M", f.Items[4].Time)

	vals := f.Values(sampleQSO(), true, Km)
	assert.Equal(t, []string{
		"*", " ", "2024-03-05", " ", "12:08", " ", "W1AW", " ", "FT4", " ",
		"-10", " ", "+03", " ", "20m", " ", "United States", " ", "K1ABC",
	}, vals)
	assert.Equal(t, " ", f.Values(sampleQSO(), false, Km)[0])
}

func TestParseItems(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Item
	}{
		{"literal", "ab%c", []Item{{Literal: "ab"}, {Field: 'c'}}},
		{"percent escape merges", "a%%b%c", []Item{{Literal: "a%b"}, {Field: 'c'}}},
		{"widest", "%255c", []Item{{Field: 'c', Width: MaxWidth}}},
		{"right aligned", "%8c", []Item{{Field: 'c', Width: 8}}},
		{"left aligned", "%-8c", []Item{{Field: 'c', Width: 8, Left: true}}},
		{"natural", "%c", []Item{{Field: 'c'}}},
		{"flex", "%*C", []Item{{Field: 'C', Flex: true}}},
		{"left flex", "%-*C", []Item{{Field: 'C', Flex: true, Left: true}}},
		{"mark intrinsic", "%Z", []Item{{Field: 'Z', Width: 1}}},
		{"mark explicit", "%3Z", []Item{{Field: 'Z', Width: 3}}},
		{"time", "%{%H}|", []Item{{Field: '{', Time: "%H"}, {Literal: "|"}}},
		{"multi digit", "%12n", []Item{{Field: 'n', Width: 12}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, f.Items)
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantMsg string
	}{
		{"unknown expando", "%c %q", `unknown expando "%q" at column 4`},
		{"unknown strftime", "%{%Y %Q}", `unknown strftime conversion "%Q" at column 6`},
		{"trailing percent", "%c %", "lone % at column 4"},
		{"trailing percent after width", "%-10", "lone % at column 1"},
		{"trailing percent in braces", "%{%H%}", "lone % at column 5"},
		{"unterminated brace", "%c %{%H", "unterminated { at column 5"},
		{"two flex", "%*c %*C", "more than one flexible column at column 5"},
		{"zero width", "%0c", "zero width at column 1"},
		{"zero width left", "%-00c", "zero width at column 1"},
		{"too wide", "%c %256c", "width 256 is more than 255 at column 4"},
		{"huge width", "%99999999999999999999c", "width 99999999999999999999 is more than 255 at column 1"},
		{"empty", "", "no columns"},
		{"only literal text", "a%%b", "no columns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.in)
			require.ErrorIs(t, err, ErrSyntax)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.Contains(t, err.Error(), "indexfmt: ")
		})
	}
}

func TestValues(t *testing.T) {
	q := sampleQSO()
	tests := []struct {
		name   string
		format string
		q      func(*logbook.QSO)
		unit   Unit
		want   string
	}{
		{"call", "%c", nil, Km, "W1AW"},
		{"submode", "%m", nil, Km, "FT4"},
		{"mode without submode", "%m", func(q *logbook.QSO) { q.Submode = "" }, Km, "MFSK"},
		{"rst sent", "%s", nil, Km, "-10"},
		{"rst rcvd", "%r", nil, Km, "+03"},
		{"band", "%b", nil, Km, "20m"},
		{"freq", "%f", nil, Km, "14.074"},
		{"freq whole", "%f", func(q *logbook.QSO) { q.FreqHz.V = 7_000_000 }, Km, "7"},
		{"freq fine", "%f", func(q *logbook.QSO) { q.FreqHz.V = 14_075_219 }, Km, "14.075219"},
		{"freq missing", "%f", func(q *logbook.QSO) { q.FreqHz.Valid = false }, Km, ""},
		{"country", "%C", nil, Km, "United States"},
		{"station", "%S", nil, Km, "K1ABC"},
		{"operator", "%o", nil, Km, "K1XYZ"},
		{"name", "%n", nil, Km, "Hiram"},
		{"grid", "%g", nil, Km, "FN31"},
		{"distance km", "%D", nil, Km, "161"},
		{"distance mi", "%D", nil, Mi, "100"},
		{"distance missing", "%D", func(q *logbook.QSO) { q.DistanceKm.Valid = false }, Km, ""},
		{"continent", "%k", nil, Km, "NA"},
		{"dxcc", "%x", nil, Km, "291"},
		{"dxcc missing", "%x", func(q *logbook.QSO) { q.DXCC.Valid = false }, Km, ""},
		{"time in utc", "%{%Y-%m-%d %H:%M:%S}", nil, Km, "2024-03-05 12:08:09"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qq := q
			if tt.q != nil {
				tt.q(&qq)
			}
			f, err := Parse(tt.format)
			require.NoError(t, err)
			assert.Equal(t, []string{tt.want}, f.Values(qq, false, tt.unit))
		})
	}
}

func TestFormatDistance(t *testing.T) {
	tests := []struct {
		name string
		km   float64
		unit Unit
		want string
	}{
		{"km rounds", 1234.5, Km, "1235"},
		{"mi converts", 160.9344, Mi, "100"},
		{"unknown unit is km", 12.4, Unit(""), "12"},
		{"zero", 0, Mi, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatDistance(tt.km, tt.unit))
		})
	}
}

func TestTitles(t *testing.T) {
	f, err := Parse("%c|%m%s%r%b%f%C%S%o%n%g%D%k%x%Z%{%H:%M}%{%d}%{%b}")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"Call", "|", "Mode", "RST S", "RST R", "Band", "Freq", "Country", "Station",
		"Op", "Name", "Grid", "Dist", "Cont", "DXCC", "", "Time", "Date", "Date",
	}, f.Titles())
}
