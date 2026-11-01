package logbook

import (
	"fmt"
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
			got, err := s.Index(t.Context(), Filter{}, tt.offset, tt.limit)
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
		qs, err := s.Index(t.Context(), Filter{}, offset, 2)
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

	got, err := s.Index(t.Context(), Filter{Countries: []string{"UNITED STATES"}}, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, []string{"D", "A"}, calls(got))
}

func TestSQLiteStoreIndexLoadsOnlyQSORow(t *testing.T) {
	s := newTestStore(t)
	added := seed(t, s, fullQSO())

	got, err := s.Index(t.Context(), Filter{}, 0, 1)

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

	got, err := s.Index(t.Context(), Filter{}, 0, MaxPageSize*2)

	require.NoError(t, err)
	assert.Len(t, got, MaxPageSize)
}

func TestSQLiteStoreIndexErrors(t *testing.T) {
	s := newTestStore(t)

	_, err := s.Index(t.Context(), Filter{}, -1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "negative offset")

	_, err = s.Index(t.Context(), Filter{Confirmed: Confirmed(7)}, 0, 10)
	require.ErrorIs(t, err, ErrInvalidFilter)
}

func TestSQLiteStoreIndexUsesStartIndex(t *testing.T) {
	s := newTestStore(t)
	query, args, err := indexQuery(Filter{}, 1000, 500)
	require.NoError(t, err)

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

	joined := strings.Join(plan, "\n")
	assert.Contains(t, joined, "qso_by_start", "rows are read in index order")
	assert.NotContains(t, joined, "TEMP B-TREE", "no sort step")
}

func TestSQLiteStoreIndexOf(t *testing.T) {
	s := newTestStore(t)
	want := seedIndexSet(t, s)

	for i, id := range want {
		got, err := s.IndexOf(t.Context(), Filter{}, id)
		require.NoError(t, err)
		assert.Equal(t, i, got, "QSO %d", id)
	}
}

func TestSQLiteStoreIndexOfFilter(t *testing.T) {
	s := newTestStore(t)
	want := seedIndexSet(t, s)
	f := Filter{Bands: []string{"40m"}}
	matching, err := s.Index(t.Context(), f, 0, 10)
	require.NoError(t, err)
	require.Len(t, matching, 3)

	for i, q := range matching {
		got, err := s.IndexOf(t.Context(), f, q.ID)
		require.NoError(t, err)
		assert.Equal(t, i, got, "QSO %d", q.ID)
	}

	// The newest QSO is on 20m.
	_, err = s.IndexOf(t.Context(), f, want[0])
	require.ErrorIs(t, err, ErrNotFound, "a QSO that does not match the filter")
}

func TestSQLiteStoreIndexOfErrors(t *testing.T) {
	s := newTestStore(t)

	_, err := s.IndexOf(t.Context(), Filter{}, 42)
	require.ErrorIs(t, err, ErrNotFound)

	_, err = s.IndexOf(t.Context(), Filter{Confirmed: Confirmed(7)}, 42)
	require.ErrorIs(t, err, ErrInvalidFilter)
}
