package logbook

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSort(t *testing.T) {
	tests := []struct {
		in   string
		want Sort
	}{
		{"date", Sort{Field: SortDate}},
		{"call", Sort{Field: SortCall}},
		{"band", Sort{Field: SortBand}},
		{"mode", Sort{Field: SortMode}},
		{"country", Sort{Field: SortCountry}},
		{"state", Sort{Field: SortState}},
		{"continent", Sort{Field: SortContinent}},
		{"reverse-date", Sort{Field: SortDate, Reverse: true}},
		{"reverse-band", Sort{Field: SortBand, Reverse: true}},
		{"Band", Sort{Field: SortBand}},
		{"REVERSE-Country", Sort{Field: SortCountry, Reverse: true}},
		{"  call  ", Sort{Field: SortCall}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseSort(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSortInvalid(t *testing.T) {
	for _, in := range []string{"", "reverse-", "freq", "reverse-reverse-call", "reversecall", "call-reverse"} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseSort(in)
			require.ErrorIs(t, err, ErrInvalidSort)
		})
	}
}

func TestSortStringRoundTrips(t *testing.T) {
	for _, f := range SortFields() {
		for _, reverse := range []bool{false, true} {
			s := Sort{Field: f, Reverse: reverse}
			got, err := ParseSort(s.String())
			require.NoError(t, err, s.String())
			assert.Equal(t, s, got)
		}
	}
	assert.Equal(t, "date", Sort{}.String(), "the zero Sort is date, oldest first")
	assert.Equal(t, "reverse-date", DefaultSort.String(), "the default is date, newest first")
	assert.Equal(t, "band", Sort{Field: SortBand}.String())
	assert.Equal(t, "reverse-band", Sort{Field: SortBand, Reverse: true}.String())
}

func TestSortFields(t *testing.T) {
	var names []string
	for _, f := range SortFields() {
		names = append(names, f.String())
	}
	assert.Equal(t, []string{"date", "call", "band", "mode", "country", "state", "continent"}, names)
	assert.Equal(t, SortDate, SortField(0), "date is the zero value")
}

func TestBandOrderCoversEveryBand(t *testing.T) {
	expr := bandOrder()
	for i, b := range bands {
		assert.Contains(t, expr, "WHEN '"+b.name+"' THEN "+strconv.Itoa(i), "band %s", b.name)
	}
	assert.True(t, strings.HasSuffix(expr, "ELSE "+strconv.Itoa(len(bands))+" END"), expr)
}

func TestInvalidSortField(t *testing.T) {
	s := newTestStore(t)
	bad := Sort{Field: SortField(99)}

	_, err := s.Index(t.Context(), nil, bad, 0, 10)
	require.ErrorIs(t, err, ErrInvalidSort)
	_, err = s.IndexOf(t.Context(), nil, bad, 1)
	require.ErrorIs(t, err, ErrInvalidSort)
	_, err = s.Search(t.Context(), nil, nil, bad, 0, false)
	require.ErrorIs(t, err, ErrInvalidSort)
}

// sortRow is one QSO of a sort test: label is stored in Comment so the
// result order can be read back, hours offsets its start time from queryT0.
type sortRow struct {
	label string
	hours int
	set   func(q *QSO)
}

// labels returns the Comment of each QSO.
func labels(qs []QSO) []string {
	out := make([]string, 0, len(qs))
	for _, q := range qs {
		out = append(out, q.Comment)
	}
	return out
}

func TestSQLiteStoreIndexSort(t *testing.T) {
	call := func(v string) func(q *QSO) { return func(q *QSO) { q.Call = v } }
	band := func(v string) func(q *QSO) { return func(q *QSO) { q.Band = v } }
	mode := func(m, sub string) func(q *QSO) { return func(q *QSO) { q.Mode, q.Submode = m, sub } }
	country := func(v string) func(q *QSO) { return func(q *QSO) { q.Country = v } }
	state := func(v string) func(q *QSO) { return func(q *QSO) { q.State = v } }
	cont := func(v string) func(q *QSO) { return func(q *QSO) { q.Cont = v } }

	tests := []struct {
		name           string
		field          SortField
		rows           []sortRow // seeded in order, so IDs ascend
		want, reversed []string
	}{
		{
			name:  "date",
			field: SortDate,
			rows:  []sortRow{{"a", 0, nil}, {"b", 2, nil}, {"c", 1, nil}, {"d", 2, nil}},
			// Oldest first, then lowest ID; reversed, newest first.
			want:     []string{"a", "c", "b", "d"},
			reversed: []string{"d", "b", "c", "a"},
		},
		{
			name:  "call",
			field: SortCall,
			rows: []sortRow{
				{"a", 0, call("W1AW")}, {"b", 1, call("K1ABC")}, {"c", 2, call("W1AW")},
				{"d", 1, call("N0X")}, {"f", 2, call("W1AW")},
			},
			// Ties are newest first, then highest ID first, in both directions.
			want:     []string{"b", "d", "f", "c", "a"},
			reversed: []string{"f", "c", "a", "d", "b"},
		},
		{
			name:  "band in frequency order",
			field: SortBand,
			rows: []sortRow{
				{"a", 0, band("20m")}, {"b", 1, band("160m")}, {"c", 2, band("70cm")},
				{"d", 3, band("")}, {"e", 4, band("20m")}, {"g", 5, band("99m")}, {"h", 0, band("")},
			},
			// Unknown bands follow the known ones; empty bands are last.
			want:     []string{"b", "e", "a", "c", "g", "d", "h"},
			reversed: []string{"g", "c", "e", "a", "b", "d", "h"},
		},
		{
			name:  "mode by its displayed name",
			field: SortMode,
			rows: []sortRow{
				{"a", 0, mode("FT8", "")}, {"b", 1, mode("MFSK", "FT4")}, {"c", 2, mode("FM", "")},
				{"d", 3, mode("", "")}, {"e", 4, mode("CW", "")}, {"f", 5, mode("SSB", "USB")},
			},
			want:     []string{"e", "c", "b", "a", "f", "d"},
			reversed: []string{"f", "a", "b", "c", "e", "d"},
		},
		{
			name:  "country ignoring case",
			field: SortCountry,
			rows: []sortRow{
				{"a", 0, country("Canada")}, {"b", 1, country("ENGLAND")}, {"c", 2, country("")},
				{"d", 3, country("canada")}, {"e", 4, country("Belgium")},
			},
			want:     []string{"e", "d", "a", "b", "c"},
			reversed: []string{"b", "d", "a", "e", "c"},
		},
		{
			name:  "state ignoring case",
			field: SortState,
			rows: []sortRow{
				{"a", 0, state("TX")}, {"b", 1, state("")}, {"c", 2, state("ca")},
				{"d", 3, state("NY")}, {"e", 4, state("CA")},
			},
			want:     []string{"e", "c", "d", "a", "b"},
			reversed: []string{"a", "d", "e", "c", "b"},
		},
		{
			name:  "continent",
			field: SortContinent,
			rows: []sortRow{
				{"a", 0, cont("NA")}, {"b", 1, cont("EU")}, {"c", 2, cont("")},
				{"d", 3, cont("AS")}, {"e", 4, cont("NA")},
			},
			want:     []string{"d", "b", "e", "a", "c"},
			reversed: []string{"e", "a", "b", "d", "c"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			for _, r := range tt.rows {
				q := QSO{StartUTC: queryT0.Add(time.Duration(r.hours) * time.Hour), Call: "W1AW", Comment: r.label}
				if r.set != nil {
					r.set(&q)
				}
				seed(t, s, q)
			}

			got, err := s.Index(t.Context(), nil, Sort{Field: tt.field}, 0, 100)
			require.NoError(t, err)
			assert.Equal(t, tt.want, labels(got), "%s", tt.field)

			got, err = s.Index(t.Context(), nil, Sort{Field: tt.field, Reverse: true}, 0, 100)
			require.NoError(t, err)
			assert.Equal(t, tt.reversed, labels(got), "reverse-%s", tt.field)
		})
	}
}

// seedSortSet seeds QSOs with ties and empty values in every sortable field.
func seedSortSet(t *testing.T, s *SQLiteStore) {
	t.Helper()
	calls := []string{"W1AW", "K1ABC", "N0X", "W1AW", "DL1ABC", "K1ABC", "VK2XY", "W1AW", "G4ABC", "JA1ZZZ", "N0X"}
	bandList := []string{"20m", "40m", "", "20m", "70cm", "160m", "20m", "", "40m", "2m", "20m"}
	modes := [][2]string{{"FT8", ""}, {"MFSK", "FT4"}, {"CW", ""}, {"", ""}, {"SSB", "USB"}, {"FT8", ""}, {"FM", ""}, {"CW", ""}, {"", ""}, {"FT8", ""}, {"CW", ""}}
	countries := []string{"United States", "", "united states", "Germany", "Germany", "Japan", "", "Australia", "England", "Japan", "United States"}
	states := []string{"CT", "", "MN", "ct", "", "", "", "NY", "", "", "MN"}
	conts := []string{"NA", "NA", "", "NA", "EU", "AS", "OC", "", "EU", "AS", "NA"}
	hours := []int{0, 1, 1, 1, 2, 3, 3, 4, 5, 5, 6}
	for i := range calls {
		seed(t, s, QSO{
			StartUTC: queryT0.Add(time.Duration(hours[i]) * time.Hour),
			Call:     calls[i], Band: bandList[i], Mode: modes[i][0], Submode: modes[i][1],
			Country: countries[i], State: states[i], Cont: conts[i],
		})
	}
}

// allSorts returns every field in both directions.
func allSorts() []Sort {
	var out []Sort
	for _, f := range SortFields() {
		out = append(out, Sort{Field: f}, Sort{Field: f, Reverse: true})
	}
	return out
}

func TestSQLiteStoreIndexSortPages(t *testing.T) {
	s := newTestStore(t)
	seedSortSet(t, s)

	for _, sort := range allSorts() {
		t.Run(sort.String(), func(t *testing.T) {
			all, err := s.Index(t.Context(), nil, sort, 0, 100)
			require.NoError(t, err)
			require.Len(t, all, 11)

			for _, size := range []int{1, 2, 3, 4} {
				var got []int64
				for offset := 0; offset < len(all); offset += size {
					qs, err := s.Index(t.Context(), nil, sort, offset, size)
					require.NoError(t, err)
					got = append(got, ids(qs)...)
				}
				assert.Equal(t, ids(all), got, "page size %d", size)
			}
		})
	}
}

func TestSQLiteStoreIndexSortWithLimit(t *testing.T) {
	s := newTestStore(t)
	seedSortSet(t, s)
	e := Filter{Bands: []string{"20m"}}

	got, err := s.Index(t.Context(), e, Sort{Field: SortCall}, 0, 100)
	require.NoError(t, err)
	// 20m QSOs: W1AW (0h, 1h), VK2XY (3h), N0X (6h).
	assert.Equal(t, []string{"N0X", "VK2XY", "W1AW", "W1AW"}, calls(got))
	assert.True(t, got[2].StartUTC.After(got[3].StartUTC), "tied calls newest first")

	got, err = s.Index(t.Context(), e, Sort{Field: SortCall, Reverse: true}, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"W1AW", "VK2XY"}, calls(got))
}

func TestSQLiteStoreIndexOfSort(t *testing.T) {
	s := newTestStore(t)
	seedSortSet(t, s)
	limits := []Expr{nil, Filter{Bands: []string{"20m"}}, Not{X: Filter{Continents: []string{"NA"}}}}

	for _, sort := range allSorts() {
		for li, e := range limits {
			all, err := s.Index(t.Context(), e, sort, 0, 100)
			require.NoError(t, err)
			require.NotEmpty(t, all)
			for i, q := range all {
				got, err := s.IndexOf(t.Context(), e, sort, q.ID)
				require.NoError(t, err)
				assert.Equal(t, i, got, "%s limit %d QSO %d", sort, li, q.ID)
			}
		}
	}
}

func TestSQLiteStoreSearchSort(t *testing.T) {
	s := newTestStore(t)
	seedSortSet(t, s)
	within := Not{X: Filter{Bands: []string{"70cm"}}}
	matches := []Expr{nil, Filter{Bands: []string{"20m"}}, CallContains{Call: "K1"}, Filter{Continents: []string{"EU"}}}

	for _, sort := range allSorts() {
		t.Run(sort.String(), func(t *testing.T) {
			all, err := s.Index(t.Context(), within, sort, 0, 100)
			require.NoError(t, err)
			for mi, match := range matches {
				// The rows matching match, found by a linear scan of within's order.
				var hit []bool
				for _, q := range all {
					_, err := s.IndexOf(t.Context(), And{within, match}, Sort{}, q.ID)
					hit = append(hit, err == nil)
				}
				for from := -1; from <= len(all); from++ {
					want := -1
					for i := from + 1; i < len(all); i++ {
						if i >= 0 && hit[i] {
							want = i
							break
						}
					}
					got, err := s.Search(t.Context(), within, match, sort, from, false)
					checkSearch(t, want, got, err, "forward match %d from %d", mi, from)

					want = -1
					for i := min(from, len(all)) - 1; i >= 0; i-- {
						if hit[i] {
							want = i
							break
						}
					}
					got, err = s.Search(t.Context(), within, match, sort, from, true)
					checkSearch(t, want, got, err, "backward match %d from %d", mi, from)
				}
			}
		})
	}
}

// checkSearch asserts Search returned row want, or ErrNotFound when want is
// -1.
func checkSearch(t *testing.T, want, got int, err error, msg string, args ...any) {
	t.Helper()
	if want < 0 {
		assert.ErrorIs(t, err, ErrNotFound, append([]any{msg}, args...)...)
		return
	}
	if assert.NoError(t, err, append([]any{msg}, args...)...) {
		assert.Equal(t, want, got, append([]any{msg}, args...)...)
	}
}
