package dxcc

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture parses the hand-made testdata/cty.csv.
func fixture(t *testing.T) *Table {
	t.Helper()
	f, err := os.Open("testdata/cty.csv")
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	table, err := Parse(f)
	require.NoError(t, err)
	return table
}

var (
	unitedStates = Entity{DXCC: 291, Name: "United States", Continent: "NA", CQZone: 5, ITUZone: 8, Lat: 37.60, Lon: -91.87}
	hawaii       = Entity{DXCC: 110, Name: "Hawaii", Continent: "OC", CQZone: 31, ITUZone: 61, Lat: 21.12, Lon: -157.48}
	guantanamo   = Entity{DXCC: 105, Name: "Guantanamo Bay", Continent: "NA", CQZone: 8, ITUZone: 11, Lat: 20.00, Lon: -75.00}
	canada       = Entity{DXCC: 1, Name: "Canada", Continent: "NA", CQZone: 5, ITUZone: 9, Lat: 44.35, Lon: -78.75}
	spain        = Entity{DXCC: 281, Name: "Spain", Continent: "EU", CQZone: 14, ITUZone: 37, Lat: 40.37, Lon: -4.88}
	scotland     = Entity{DXCC: 279, Name: "Scotland", Continent: "EU", CQZone: 14, ITUZone: 27, Lat: 56.82, Lon: -4.18}
	euRussia     = Entity{DXCC: 54, Name: "European Russia", Continent: "EU", CQZone: 16, ITUZone: 29, Lat: 53.65, Lon: 41.37}
	asRussia     = Entity{DXCC: 15, Name: "Asiatic Russia", Continent: "AS", CQZone: 17, ITUZone: 30, Lat: 55.88, Lon: 84.08}
)

func TestLookup(t *testing.T) {
	table := fixture(t)

	with := func(e Entity, edit func(*Entity)) Entity {
		edit(&e)
		return e
	}

	tests := []struct {
		name string
		call string
		want Entity
		ok   bool
	}{
		{name: "plain prefix", call: "W1AW", want: unitedStates, ok: true},
		{name: "longest prefix wins", call: "KH6ABC", want: hawaii, ok: true},
		{name: "longer prefix with override", call: "VE2ABC", want: with(canada, func(e *Entity) { e.ITUZone = 4 }), ok: true},
		{name: "exact call with zone overrides", call: "AH0AB", want: with(unitedStates, func(e *Entity) { e.CQZone, e.ITUZone = 3, 6 }), ok: true},
		{name: "exact call wins over prefix", call: "4U1WB", want: with(unitedStates, func(e *Entity) { e.CQZone, e.ITUZone = 5, 8 }), ok: true},
		{
			name: "exact call with slash and every override",
			call: "KC4/W1WCN",
			want: Entity{DXCC: 13, Name: "Antarctica", Continent: "AN", CQZone: 30, ITUZone: 71, Lat: -77.85, Lon: 166.67},
			ok:   true,
		},
		{name: "exact call with modifier", call: "EA2EZ/P", want: with(spain, func(e *Entity) { e.Continent = "AF" }), ok: true},
		{name: "base call of exact call with modifier", call: "EA2EZ", want: spain, ok: true},
		{name: "prefix before call", call: "VE3/W1AW", want: canada, ok: true},
		{name: "prefix after call", call: "W1AW/VE3", want: canada, ok: true},
		{name: "country prefix", call: "KH6/W1AW", want: hawaii, ok: true},
		{name: "portable", call: "W1AW/P", want: unitedStates, ok: true},
		{name: "low power", call: "W1AW/QRP", want: unitedStates, ok: true},
		{name: "call area moves entity", call: "UA9ABC/1", want: euRussia, ok: true},
		{name: "call area digit", call: "VE3ABC/2", want: with(canada, func(e *Entity) { e.ITUZone = 4 }), ok: true},
		{name: "call area without area change", call: "UA9ABC", want: asRussia, ok: true},
		{name: "maritime mobile", call: "W1AW/MM", ok: false},
		{name: "aeronautical mobile", call: "W1AW/AM", ok: false},
		{name: "AM as a prefix", call: "AM100ABC", want: spain, ok: true},
		{name: "lowercase and spaces", call: "  w1aw ", want: unitedStates, ok: true},
		{name: "empty", call: "", ok: false},
		{name: "spaces only", call: "   ", ok: false},
		{name: "modifiers only", call: "/P", ok: false},
		{name: "unknown prefix", call: "QQ1ABC", ok: false},
		{
			name: "WAE prefix takes DXCC entity name",
			call: "IT9ABC",
			want: Entity{DXCC: 248, Name: "Italy", Continent: "EU", CQZone: 15, ITUZone: 28, Lat: 37.50, Lon: 14.00},
			ok:   true,
		},
		{name: "WAE exact call takes DXCC entity name", call: "GM3ZET", want: with(scotland, func(e *Entity) { e.Lat, e.Lon = 60.50, -1.50 }), ok: true},
		{name: "later duplicate exact call wins", call: "GB3LER", want: with(scotland, func(e *Entity) { e.Lat, e.Lon = 60.50, -1.50 }), ok: true},
		{name: "unlisted suffix keeps the call's entity", call: "W1AW/QRO", want: unitedStates, ok: true},
		{name: "unlisted letter suffix keeps the call's entity", call: "W1AW/T", want: unitedStates, ok: true},
		{name: "prefix, call and modifier", call: "VE3/W1AW/P", want: canada, ok: true},
		{name: "tie takes the first part as prefix", call: "VE3/KH6", want: canada, ok: true},
		{name: "prefix and maritime mobile", call: "KH6/W1AW/MM", ok: false},
		{name: "MM before the call is a prefix", call: "MM/W1AW", want: scotland, ok: true},
		{name: "AM before the call is a prefix", call: "AM/W1AW", want: spain, ok: true},
		{name: "unlisted modifier before the call", call: "P/W1AW", want: unitedStates, ok: true},
		{name: "KG4 plus two letters is Guantanamo", call: "KG4AB", want: guantanamo, ok: true},
		{name: "other KG4 calls are United States", call: "KG4ABC", want: unitedStates, ok: true},
		{name: "KG4 prefix alone is United States", call: "KG4", want: unitedStates, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := table.Lookup(tt.call)
			require.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseErrors(t *testing.T) {
	const good = "K,United States,291,NA,5,8,37.60,91.87,5.0,K W;\n"
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "too few fields", line: "K,United States,291,NA,5,8;", want: "cty.csv line 2: want 10 fields, got 6"},
		{name: "non-numeric DXCC", line: "K,United States,x,NA,5,8,37.60,91.87,5.0,K;", want: "cty.csv line 2: DXCC number"},
		{name: "non-numeric CQ zone", line: "K,United States,291,NA,x,8,37.60,91.87,5.0,K;", want: "cty.csv line 2: CQ zone"},
		{name: "non-numeric ITU zone", line: "K,United States,291,NA,5,x,37.60,91.87,5.0,K;", want: "cty.csv line 2: ITU zone"},
		{name: "non-numeric latitude", line: "K,United States,291,NA,5,8,x,91.87,5.0,K;", want: "cty.csv line 2: latitude"},
		{name: "non-numeric longitude", line: "K,United States,291,NA,5,8,37.60,x,5.0,K;", want: "cty.csv line 2: longitude"},
		{name: "non-numeric UTC offset", line: "K,United States,291,NA,5,8,37.60,91.87,x,K;", want: "cty.csv line 2: UTC offset"},
		{name: "unknown continent", line: "K,United States,291,XX,5,8,37.60,91.87,5.0,K;", want: "cty.csv line 2: continent"},
		{name: "unterminated override", line: "K,United States,291,NA,5,8,37.60,91.87,5.0,K(5;", want: "cty.csv line 2: token \"K(5\""},
		{name: "bad CQ zone override", line: "K,United States,291,NA,5,8,37.60,91.87,5.0,K(x);", want: "cty.csv line 2: token \"K(x)\""},
		{name: "bad lat/lon override", line: "K,United States,291,NA,5,8,37.60,91.87,5.0,K<1>;", want: "cty.csv line 2: token \"K<1>\""},
		{name: "bad continent override", line: "K,United States,291,NA,5,8,37.60,91.87,5.0,K{XX};", want: "cty.csv line 2: token \"K{XX}\""},
		{name: "empty token prefix", line: "K,United States,291,NA,5,8,37.60,91.87,5.0,=(5);", want: "cty.csv line 2: token \"=(5)\""},
		{name: "WAE entity without parent", line: "*IT9,Sicily,248,EU,15,28,37.50,-14.00,-1.0,IT9;", want: "cty.csv line 2: no DXCC entity 248"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(good + tt.line + "\n"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestParseCRLFAndBlankLines(t *testing.T) {
	table, err := Parse(strings.NewReader("\r\nK,United States,291,NA,5,8,37.60,91.87,5.0,K W;\r\n\r\n"))
	require.NoError(t, err)
	got, ok := table.Lookup("W1AW")
	require.True(t, ok)
	assert.Equal(t, unitedStates, got)
}

func TestDefault(t *testing.T) {
	table := Default()
	require.Same(t, table, Default(), "Default parses the embedded file once")

	tests := []struct {
		call      string
		dxcc      int
		name      string
		continent string
		cq, itu   int
	}{
		{call: "W1AW", dxcc: 291, name: "United States", continent: "NA", cq: 5, itu: 8},
		{call: "VE3ABC", dxcc: 1, name: "Canada", continent: "NA", cq: 4, itu: 4},
		{call: "W6ABC", dxcc: 291, name: "United States", continent: "NA", cq: 3, itu: 6},
		{call: "UA0ABC", dxcc: 15, name: "Asiatic Russia", continent: "AS", cq: 18, itu: 32},
		// Big CTY's own checks that it, not the contest file, is installed.
		{call: "OR4TN", dxcc: 13, name: "Antarctica", continent: "SA", cq: 38, itu: 67},
		{call: "MR6TMS", dxcc: 279, name: "Scotland", continent: "EU", cq: 14, itu: 27},
		{call: "G4ABC", dxcc: 223, name: "England", continent: "EU", cq: 14, itu: 27},
		{call: "GM3ZET", dxcc: 279, name: "Scotland", continent: "EU", cq: 14, itu: 27},
		{call: "JA1ABC", dxcc: 339, name: "Japan", continent: "AS", cq: 25, itu: 45},
		{call: "4U1WB", dxcc: 291, name: "United States", continent: "NA", cq: 5, itu: 8},
		{call: "KG4AB", dxcc: 105, name: "Guantanamo Bay", continent: "NA", cq: 8, itu: 11},
		{call: "KG4ABC", dxcc: 291, name: "United States", continent: "NA", cq: 5, itu: 8},
	}
	for _, tt := range tests {
		t.Run(tt.call, func(t *testing.T) {
			got, ok := table.Lookup(tt.call)
			require.True(t, ok)
			assert.Equal(t, tt.dxcc, got.DXCC)
			assert.Equal(t, tt.name, got.Name)
			assert.Equal(t, tt.continent, got.Continent)
			assert.Equal(t, tt.cq, got.CQZone)
			assert.Equal(t, tt.itu, got.ITUZone)
		})
	}

	shetland, ok := table.Lookup("GM3ZET")
	require.True(t, ok)
	assert.InDelta(t, 60.50, shetland.Lat, 1e-9, "a WAE entity keeps its own location")
	assert.InDelta(t, -1.50, shetland.Lon, 1e-9)
}
