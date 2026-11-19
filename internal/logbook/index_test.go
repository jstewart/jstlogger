package logbook

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ids returns the ID of each QSO.
func ids(qs []QSO) []int64 {
	out := make([]int64, 0, len(qs))
	for _, q := range qs {
		out = append(out, q.ID)
	}
	return out
}

// seedIndexSet seeds seven QSOs, three of them sharing a start time, and
// returns their IDs newest first (StartUTC descending, then ID descending).
func seedIndexSet(t *testing.T, s *SQLiteStore) []int64 {
	t.Helper()
	starts := []time.Duration{0, time.Hour, time.Hour, time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour}
	var qs []QSO
	for i, d := range starts {
		band := "20m"
		if i%2 == 1 {
			band = "40m"
		}
		qs = append(qs, QSO{StartUTC: queryT0.Add(d), Call: fmt.Sprintf("K%dA", i), Band: band})
	}
	added := seed(t, s, qs...)
	out := make([]int64, 0, len(added))
	for i := len(added) - 1; i >= 0; i-- {
		out = append(out, added[i].ID)
	}
	return out
}

func TestSQLiteStoreIndex(t *testing.T) {
	s := newTestStore(t)
	want := seedIndexSet(t, s)

	tests := []struct {
		name          string
		offset, limit int
		want          []int64
	}{
		{"first rows", 0, 3, want[:3]},
		{"middle across equal start times", 2, 3, want[2:5]},
		{"last rows", 5, 10, want[5:]},
		{"everything", 0, 100, want},
		{"past the end", 7, 3, nil},
		{"zero limit", 0, 0, nil},
		{"negative limit", 0, -1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Index(t.Context(), Filter{}, Sort{}, tt.offset, tt.limit)
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tt.want, ids(got))
		})
	}
}

func TestSQLiteStoreIndexPagesMatchList(t *testing.T) {
	s := newTestStore(t)
	want := seedIndexSet(t, s)

	var got []int64
	for offset := 0; offset < len(want); offset += 2 {
		qs, err := s.Index(t.Context(), Filter{}, Sort{}, offset, 2)
		require.NoError(t, err)
		got = append(got, ids(qs)...)
	}

	assert.Equal(t, want, got, "every QSO exactly once, newest first")
	page, err := s.List(t.Context(), Filter{}, PageRequest{Limit: len(want)})
	require.NoError(t, err)
	assert.Equal(t, ids(page.QSOs), got, "Index orders QSOs as List does")
}

func TestSQLiteStoreIndexFilter(t *testing.T) {
	s := newTestStore(t)
	seedFilterSet(t, s)

	got, err := s.Index(t.Context(), Filter{Countries: []string{"UNITED STATES"}}, Sort{}, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, []string{"D", "A"}, calls(got))
}

func TestSQLiteStoreIndexLoadsOnlyQSORow(t *testing.T) {
	s := newTestStore(t)
	added := seed(t, s, fullQSO())

	got, err := s.Index(t.Context(), Filter{}, Sort{}, 0, 1)

	require.NoError(t, err)
	require.Len(t, got, 1)
	want := added[0]
	require.NotEmpty(t, want.Fields)
	require.NotEmpty(t, want.Confirmations)
	require.NotEmpty(t, want.Tags)
	want.Fields, want.Confirmations, want.Tags = nil, nil, nil
	assert.Equal(t, want, got[0], "every qso column, but no fields, confirmations or tags")
}

func TestSQLiteStoreIndexCapsLimit(t *testing.T) {
	s := newTestStore(t)
	bulkInsert(t, s, MaxPageSize+1)

	got, err := s.Index(t.Context(), Filter{}, Sort{}, 0, MaxPageSize*2)

	require.NoError(t, err)
	assert.Len(t, got, MaxPageSize)
}

func TestSQLiteStoreIndexErrors(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Index(t.Context(), Filter{}, Sort{}, -1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative offset")

	_, err = s.Index(t.Context(), Filter{Confirmed: Confirmed(7)}, Sort{}, 0, 10)
	require.ErrorIs(t, err, ErrInvalidFilter)
}

func TestSQLiteStoreIndexUsesStartIndex(t *testing.T) {
	s := newTestStore(t)
	query, args, err := indexQuery(Filter{}, Sort{}, 1000, 500)
	require.NoError(t, err)

	joined := queryPlan(t, s, query, args)
	assert.Contains(t, joined, "qso_by_start", "rows are read in index order")
	assert.NotContains(t, joined, "TEMP B-TREE", "no sort step")
}

func TestSQLiteStoreIndexOf(t *testing.T) {
	s := newTestStore(t)
	want := seedIndexSet(t, s)

	for i, id := range want {
		got, err := s.IndexOf(t.Context(), Filter{}, Sort{}, id)
		require.NoError(t, err)
		assert.Equal(t, i, got, "QSO %d", id)
	}
}

func TestSQLiteStoreIndexOfFilter(t *testing.T) {
	s := newTestStore(t)
	want := seedIndexSet(t, s)
	f := Filter{Bands: []string{"40m"}}
	matching, err := s.Index(t.Context(), f, Sort{}, 0, 10)
	require.NoError(t, err)
	require.Len(t, matching, 3)

	for i, q := range matching {
		got, err := s.IndexOf(t.Context(), f, Sort{}, q.ID)
		require.NoError(t, err)
		assert.Equal(t, i, got, "QSO %d", q.ID)
	}

	// The newest QSO is on 20m.
	_, err = s.IndexOf(t.Context(), f, Sort{}, want[0])
	require.ErrorIs(t, err, ErrNotFound, "a QSO that does not match the filter")
}

func TestSQLiteStoreIndexOfErrors(t *testing.T) {
	s := newTestStore(t)

	_, err := s.IndexOf(t.Context(), Filter{}, Sort{}, 42)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = s.IndexOf(t.Context(), Filter{Confirmed: Confirmed(7)}, Sort{}, 42)
	require.ErrorIs(t, err, ErrInvalidFilter)
}

func TestSQLiteStoreSearch(t *testing.T) {
	s := newTestStore(t)
	seedIndexSet(t, s)
	// Overall rows, newest first: 0 K6A 20m, 1 K5A 40m, 2 K4A 20m,
	// 3 K3A 40m, 4 K2A 20m, 5 K1A 40m, 6 K0A 20m; rows 3 to 5 share a
	// start time.
	b20 := Filter{Bands: []string{"20m"}}
	b40 := Filter{Bands: []string{"40m"}}

	tests := []struct {
		name          string
		within, match Expr
		from          int
		backward      bool
		want          int
		wantErr       error
	}{
		{"forward from a middle row", nil, b40, 2, false, 3, nil},
		{"backward from a middle row", nil, b40, 4, true, 3, nil},
		{"forward across equal start times", nil, b40, 3, false, 5, nil},
		{"forward into equal start times", nil, b20, 3, false, 4, nil},
		{"backward across equal start times", nil, b40, 5, true, 3, nil},
		{"forward from -1", nil, b20, -1, false, 0, nil},
		{"forward from -1 skips non-matching", nil, b40, -1, false, 1, nil},
		{"backward from the last row", nil, b20, 6, true, 4, nil},
		{"backward from past the end", nil, b20, 7, true, 6, nil},
		{"backward from far past the end", nil, b40, 100, true, 5, nil},
		{"no further match forward", nil, b40, 5, false, 0, ErrNotFound},
		{"no further match backward", nil, b40, 1, true, 0, ErrNotFound},
		{"anchor itself not returned forward", nil, CallContains{Call: "K3A"}, 3, false, 0, ErrNotFound},
		{"anchor itself not returned backward", nil, CallContains{Call: "K3A"}, 3, true, 0, ErrNotFound},
		{"forward from past the end", nil, nil, 7, false, 0, ErrNotFound},
		{"backward from the first row", nil, nil, 0, true, 0, ErrNotFound},
		{"backward from -1", nil, nil, -1, true, 0, ErrNotFound},
		{"nil within and match forward", nil, nil, 2, false, 3, nil},
		{"nil within and match backward", nil, nil, 2, true, 1, nil},
		{"within: row under within", b20, CallContains{Call: "K2A"}, 0, false, 2, nil},
		{"within: backward from past the end", b20, CallContains{Call: "K4A"}, 10, true, 1, nil},
		{"within: nil match forward", b20, nil, 1, false, 2, nil},
		{"within: nil match backward", b40, nil, 1, true, 0, nil},
		{"within: from -1", b40, CallContains{Call: "K1"}, -1, false, 2, nil},
		{"within: match outside within", b20, b40, -1, false, 0, ErrNotFound},
		{"within: forward from past the end", b40, nil, 3, false, 0, ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Search(t.Context(), tt.within, tt.match, Sort{}, tt.from, tt.backward)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)

			// Index at the returned row is a QSO matching both.
			qs, err := s.Index(t.Context(), tt.within, Sort{}, got, 1)
			require.NoError(t, err)
			require.Len(t, qs, 1)
			n, err := s.IndexOf(t.Context(), And{tt.within, tt.match}, Sort{}, qs[0].ID)
			require.NoError(t, err, "QSO %d matches within and match", qs[0].ID)
			assert.GreaterOrEqual(t, n, 0)
		})
	}
}

func TestSQLiteStoreSearchErrors(t *testing.T) {
	s := newTestStore(t)
	seedIndexSet(t, s)
	invalid := Filter{ConfirmedService: ServiceLoTW}

	_, err := s.Search(t.Context(), invalid, nil, Sort{}, 0, false)
	require.ErrorIs(t, err, ErrInvalidFilter, "invalid within")

	_, err = s.Search(t.Context(), nil, invalid, Sort{}, 0, false)
	require.ErrorIs(t, err, ErrInvalidFilter, "invalid match")

	_, err = s.Search(t.Context(), nil, invalid, Sort{}, 0, true)
	require.ErrorIs(t, err, ErrInvalidFilter, "invalid match backward")
}

// BenchmarkSQLiteStoreIndexSort times Index at a deep offset, IndexOf of a
// QSO deep in the order and Search both ways from a deep row, under every
// sort, over 100,000 QSOs.
func BenchmarkSQLiteStoreIndexSort(b *testing.B) {
	if testing.Short() {
		b.Skip("seeds 100,000 QSOs")
	}
	const n = 100_000
	s, err := Open(b.Context(), filepath.Join(b.TempDir(), "logbook.db"), Options{})
	require.NoError(b, err)
	b.Cleanup(func() { assert.NoError(b, s.Close()) })

	ctx := b.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	require.NoError(b, err)
	modes := [][2]string{{"FT8", ""}, {"MFSK", "FT4"}, {"CW", ""}, {"SSB", "USB"}, {"FM", ""}, {"", ""}}
	conts := []string{"NA", "EU", "AS", "OC", "SA", "AF", ""}
	for i := range n {
		ts := formatTime(queryT0.Add(time.Duration(i) * time.Minute))
		m := modes[i%len(modes)]
		_, err := tx.ExecContext(ctx,
			"INSERT INTO qso (uuid, start_utc, call, band, mode, submode, country, state, cont, created_at, updated_at)"+
				" VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			newUUID(), ts, fmt.Sprintf("W%dX", i%5000), bands[i%len(bands)].name, m[0], m[1],
			fmt.Sprintf("Country %d", i%340), fmt.Sprintf("S%d", i%60), conts[i%len(conts)], ts, ts)
		require.NoError(b, err)
	}
	require.NoError(b, tx.Commit())

	for _, sort := range allSorts() {
		b.Run(sort.String()+"/Index", func(b *testing.B) {
			for b.Loop() {
				_, err := s.Index(ctx, nil, sort, n*3/4, 50)
				require.NoError(b, err)
			}
		})
		deep, err := s.Index(ctx, nil, sort, n*3/4, 1)
		require.NoError(b, err)
		require.Len(b, deep, 1)
		b.Run(sort.String()+"/IndexOf", func(b *testing.B) {
			for b.Loop() {
				_, err := s.IndexOf(ctx, nil, sort, deep[0].ID)
				require.NoError(b, err)
			}
		})
		b.Run(sort.String()+"/Search", func(b *testing.B) {
			// Depending on the sort there may be no match on one side.
			match := Filter{Continents: []string{"SA"}}
			for b.Loop() {
				_, err := s.Search(ctx, nil, match, sort, n*3/4, false)
				if !errors.Is(err, ErrNotFound) {
					require.NoError(b, err)
				}
				_, err = s.Search(ctx, nil, match, sort, n*3/4, true)
				if !errors.Is(err, ErrNotFound) {
					require.NoError(b, err)
				}
			}
		})
	}
}

// queryPlan returns the detail lines of SQLite's query plan for query.
func queryPlan(t *testing.T, s *SQLiteStore, query string, args []any) string {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, args...)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()
	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &notUsed, &detail))
		plan = append(plan, detail)
	}
	require.NoError(t, rows.Err())
	return strings.Join(plan, "\n")
}

func TestSQLiteStoreIndexSortUsesIndex(t *testing.T) {
	s := newTestStore(t)
	for _, sort := range allSorts() {
		t.Run(sort.String(), func(t *testing.T) {
			query, args, err := indexQuery(nil, sort, 1000, 500)
			require.NoError(t, err)

			plan := queryPlan(t, s, query, args)
			assert.Contains(t, plan, "USING INDEX", "rows are read in index order")
			assert.NotContains(t, plan, "TEMP B-TREE", "no sort step")
		})
	}
}
