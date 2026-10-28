package logbook

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queryT0 is the base start time of the QSOs seeded by the query tests.
var queryT0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// seed adds each QSO to s and returns them as stored.
func seed(t *testing.T, s *SQLiteStore, qs ...QSO) []QSO {
	t.Helper()
	out := make([]QSO, 0, len(qs))
	for _, q := range qs {
		added, err := s.Add(t.Context(), q)
		require.NoError(t, err)
		out = append(out, added)
	}
	return out
}

// bulkInsert inserts n bare QSOs directly, one second apart, in a single
// transaction. It is much faster than Add for the page-size tests.
func bulkInsert(t *testing.T, s *SQLiteStore, n int) {
	t.Helper()
	ctx := t.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	for i := range n {
		ts := formatTime(queryT0.Add(time.Duration(i) * time.Second))
		_, err := tx.ExecContext(ctx,
			"INSERT INTO qso (uuid, start_utc, call, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			newUUID(), ts, fmt.Sprintf("W%dX", i), ts, ts)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
}

// calls returns the Call of each QSO.
func calls(qs []QSO) []string {
	out := make([]string, 0, len(qs))
	for _, q := range qs {
		out = append(out, q.Call)
	}
	return out
}

// seedFilterSet seeds four QSOs, A (oldest) to D (newest), that differ in
// every filterable dimension.
func seedFilterSet(t *testing.T, s *SQLiteStore) {
	t.Helper()
	seed(t, s,
		QSO{
			StartUTC: queryT0, Call: "A", Band: "20m", Mode: "SSB",
			DXCC: valid(291), Country: "United States", Cont: "NA", ITURegion: valid(2),
			DistanceKm: valid(100.0), Tags: []string{"pota", "dx"},
			Confirmations: []Confirmation{{Service: ServiceLoTW, Rcvd: "Y"}},
		},
		QSO{
			StartUTC: queryT0.Add(time.Hour), Call: "B", Band: "40m", Mode: "cw",
			DXCC: valid(223), Country: "England", Cont: "EU", ITURegion: valid(1),
			DistanceKm: valid(5000.0), Tags: []string{"pota"},
			Confirmations: []Confirmation{{Service: ServiceEQSL, Rcvd: "Y"}},
		},
		QSO{
			StartUTC: queryT0.Add(2 * time.Hour), Call: "C", Band: "20M", Mode: "FT8",
			DXCC: valid(339), Country: "Japan", Cont: "AS", ITURegion: valid(3),
			Confirmations: []Confirmation{{Service: ServiceLoTW, Sent: "Y", Rcvd: "N"}},
		},
		QSO{
			StartUTC: queryT0.Add(3 * time.Hour), Call: "D", Band: "15m", Mode: "SSB",
			DXCC: valid(291), Country: "united states", Cont: "na", ITURegion: valid(2),
			DistanceKm: valid(0.0), Tags: []string{"DX"},
		},
	)
}

func TestSQLiteStoreFilter(t *testing.T) {
	s := newTestStore(t)
	seedFilterSet(t, s)

	tests := []struct {
		name string
		f    Filter
		want []string
	}{
		{"no filter", Filter{}, []string{"D", "C", "B", "A"}},
		{"from inclusive", Filter{From: queryT0.Add(time.Hour)}, []string{"D", "C", "B"}},
		{"to exclusive", Filter{To: queryT0.Add(2 * time.Hour)}, []string{"B", "A"}},
		{"from and to", Filter{From: queryT0.Add(time.Hour), To: queryT0.Add(3 * time.Hour)}, []string{"C", "B"}},
		{"dxcc", Filter{DXCC: []int{291}}, []string{"D", "A"}},
		{"dxcc or", Filter{DXCC: []int{223, 339}}, []string{"C", "B"}},
		{"itu region", Filter{ITURegions: []int{1, 3}}, []string{"C", "B"}},
		{"mode ignores case", Filter{Modes: []string{"ssb"}}, []string{"D", "A"}},
		{"mode stored lower", Filter{Modes: []string{"CW"}}, []string{"B"}},
		{"band", Filter{Bands: []string{"20M"}}, []string{"C", "A"}},
		{"country ignores case", Filter{Countries: []string{"UNITED STATES"}}, []string{"D", "A"}},
		{"continent", Filter{Continents: []string{"eu", "As"}}, []string{"C", "B"}},
		{"one tag", Filter{Tags: []string{"pota"}}, []string{"B", "A"}},
		{"all tags", Filter{Tags: []string{"POTA", "dx"}}, []string{"A"}},
		{"min distance excludes null", Filter{MinDistanceKm: valid(100.0)}, []string{"B", "A"}},
		{"max distance excludes null", Filter{MaxDistanceKm: valid(100.0)}, []string{"D", "A"}},
		{"distance range", Filter{MinDistanceKm: valid(1.0), MaxDistanceKm: valid(1000.0)}, []string{"A"}},
		{"zero min distance", Filter{MinDistanceKm: valid(0.0)}, []string{"D", "B", "A"}},
		{"confirmed yes", Filter{Confirmed: ConfirmedYes}, []string{"B", "A"}},
		{"confirmed no", Filter{Confirmed: ConfirmedNo}, []string{"D", "C"}},
		{"confirmed yes lotw", Filter{Confirmed: ConfirmedYes, ConfirmedService: ServiceLoTW}, []string{"A"}},
		{"confirmed no lotw", Filter{Confirmed: ConfirmedNo, ConfirmedService: ServiceLoTW}, []string{"D", "C", "B"}},
		{"band and mode", Filter{Bands: []string{"20m"}, Modes: []string{"SSB"}}, []string{"A"}},
		{"dxcc and from", Filter{DXCC: []int{291}, From: queryT0.Add(time.Hour)}, []string{"D"}},
		{"no match", Filter{Bands: []string{"6m"}}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.List(t.Context(), tt.f, PageRequest{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, calls(page.QSOs))
			assert.False(t, page.HasMore)

			n, err := s.Count(t.Context(), tt.f)
			require.NoError(t, err)
			assert.Equal(t, len(tt.want), n, "Count matches List")
		})
	}
}

func TestSQLiteStoreInvalidFilter(t *testing.T) {
	s := newTestStore(t)
	tests := []struct {
		name string
		f    Filter
	}{
		{"unknown confirmed", Filter{Confirmed: Confirmed(7)}},
		{"service without confirmed", Filter{ConfirmedService: ServiceLoTW}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.List(t.Context(), tt.f, PageRequest{})
			require.ErrorIs(t, err, ErrInvalidFilter)
			_, err = s.Count(t.Context(), tt.f)
			require.ErrorIs(t, err, ErrInvalidFilter)
		})
	}
}

func TestFilterSQLKeepsValuesInArgs(t *testing.T) {
	const evil = "x'); DROP TABLE qso; --"
	f := Filter{
		From:             queryT0,
		To:               queryT0.Add(time.Hour),
		DXCC:             []int{98765},
		ITURegions:       []int{87654},
		Modes:            []string{evil + "mode"},
		Bands:            []string{evil + "band"},
		Countries:        []string{evil + "country"},
		Continents:       []string{evil + "cont"},
		Tags:             []string{evil + "tag1", evil + "tag2"},
		MinDistanceKm:    valid(12345.5),
		MaxDistanceKm:    valid(23456.5),
		Confirmed:        ConfirmedYes,
		ConfirmedService: evil + "service",
	}

	clause, args, err := filterSQL(f)
	require.NoError(t, err)

	for _, banned := range []string{"DROP", "x'", "98765", "87654", "12345", "23456", "2026"} {
		assert.NotContains(t, clause, banned)
	}
	assert.Equal(t, strings.Count(clause, "?"), len(args), "one arg per placeholder")
	assert.Contains(t, args, evil+"tag2")
	assert.Contains(t, args, evil+"service")
	assert.Contains(t, args, 98765)
}

func TestFilterSQLEmpty(t *testing.T) {
	clause, args, err := filterSQL(Filter{})
	require.NoError(t, err)
	assert.Empty(t, clause)
	assert.Empty(t, args)
}

func TestSQLiteStoreListEmpty(t *testing.T) {
	s := newTestStore(t)

	page, err := s.List(t.Context(), Filter{}, PageRequest{})

	require.NoError(t, err)
	assert.Equal(t, Page{}, page, "an empty result is the zero Page with nil QSOs")
	assert.Nil(t, page.QSOs)
}

func TestSQLiteStoreListPagesNewestFirst(t *testing.T) {
	s := newTestStore(t)
	// Three QSOs share a start time and straddle the boundary between the
	// second and third pages of two.
	starts := []time.Duration{0, time.Hour, time.Hour, time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour}
	var qs []QSO
	for i, d := range starts {
		qs = append(qs, QSO{StartUTC: queryT0.Add(d), Call: fmt.Sprintf("K%dA", i)})
	}
	added := seed(t, s, qs...)
	// Newest first, then highest ID first among equal start times.
	want := []int64{added[6].ID, added[5].ID, added[4].ID, added[3].ID, added[2].ID, added[1].ID, added[0].ID}

	var got []int64
	var after Cursor
	pages := 0
	for {
		page, err := s.List(t.Context(), Filter{}, PageRequest{After: after, Limit: 2})
		require.NoError(t, err)
		pages++
		require.LessOrEqual(t, pages, len(want), "paging terminates")
		for _, q := range page.QSOs {
			got = append(got, q.ID)
		}
		if !page.HasMore {
			assert.True(t, page.Next.IsZero(), "no cursor after the last page")
			assert.Len(t, page.QSOs, 1)
			break
		}
		last := page.QSOs[len(page.QSOs)-1]
		assert.Equal(t, Cursor{StartUTC: last.StartUTC, ID: last.ID}, page.Next)
		after = page.Next
	}
	assert.Equal(t, want, got, "every QSO exactly once, newest first")
	assert.Equal(t, 4, pages)
}

func TestSQLiteStoreListExactPage(t *testing.T) {
	s := newTestStore(t)
	seed(t, s, QSO{StartUTC: queryT0, Call: "A"}, QSO{StartUTC: queryT0.Add(time.Hour), Call: "B"})

	page, err := s.List(t.Context(), Filter{}, PageRequest{Limit: 2})

	require.NoError(t, err)
	assert.Equal(t, []string{"B", "A"}, calls(page.QSOs))
	assert.False(t, page.HasMore, "a full page with nothing after it has no more")
	assert.True(t, page.Next.IsZero())
}

func TestSQLiteStoreListLimits(t *testing.T) {
	s := newTestStore(t)
	bulkInsert(t, s, MaxPageSize+1)

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero defaults", 0, DefaultPageSize},
		{"negative defaults", -1, DefaultPageSize},
		{"capped", MaxPageSize * 2, MaxPageSize},
		{"explicit", 7, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := s.List(t.Context(), Filter{}, PageRequest{Limit: tt.limit})
			require.NoError(t, err)
			assert.Len(t, page.QSOs, tt.want)
			assert.True(t, page.HasMore)
		})
	}
	assert.Equal(t, 50, DefaultPageSize)
	assert.Equal(t, 500, MaxPageSize)
}

func TestSQLiteStoreListPagesWithFilter(t *testing.T) {
	s := newTestStore(t)
	var qs []QSO
	for i := range 6 {
		band := "20m"
		if i%2 == 1 {
			band = "40m"
		}
		qs = append(qs, QSO{StartUTC: queryT0.Add(time.Duration(i) * time.Minute), Call: fmt.Sprintf("K%dB", i), Band: band})
	}
	seed(t, s, qs...)
	f := Filter{Bands: []string{"40m"}}

	first, err := s.List(t.Context(), f, PageRequest{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"K5B", "K3B"}, calls(first.QSOs))
	assert.True(t, first.HasMore)

	second, err := s.List(t.Context(), f, PageRequest{After: first.Next, Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, []string{"K1B"}, calls(second.QSOs))
	assert.False(t, second.HasMore)
	assert.True(t, second.Next.IsZero())
}

func TestSQLiteStoreListLoadsChildren(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	other := QSO{
		StartUTC: queryT0, Call: "W1AW",
		Fields:        map[string]string{"SIG": "POTA"},
		Confirmations: []Confirmation{{Service: ServiceCard, Rcvd: "Y"}},
		Tags:          []string{"other"},
	}
	added := seed(t, s, fullQSO(), other, QSO{StartUTC: queryT0.Add(-time.Hour), Call: "N0NE"})

	page, err := s.List(ctx, Filter{}, PageRequest{})
	require.NoError(t, err)
	require.Len(t, page.QSOs, 3)

	for i, q := range page.QSOs {
		want, err := s.Get(ctx, q.ID)
		require.NoError(t, err)
		assert.Equal(t, want, q, "List row %d matches Get", i)
	}
	assert.Equal(t, added[0], page.QSOs[0])
	assert.Equal(t, []string{"DX", "pota"}, page.QSOs[0].Tags)
	assert.Len(t, page.QSOs[0].Confirmations, 2)
	assert.Equal(t, map[string]string{"SIG": "POTA"}, page.QSOs[1].Fields)
	assert.Nil(t, page.QSOs[2].Fields)
	assert.Nil(t, page.QSOs[2].Confirmations)
	assert.Nil(t, page.QSOs[2].Tags)
}
