package logbook

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedExprSet seeds five QSOs, oldest first, and returns their IDs keyed by
// call. Their calls and modes exercise CallContains and ModeIs.
func seedExprSet(t *testing.T, s *SQLiteStore) map[string]int64 {
	t.Helper()
	added := seed(t, s,
		QSO{StartUTC: queryT0, Call: "W1AW", Band: "20m", Mode: "SSB"},
		QSO{StartUTC: queryT0.Add(time.Hour), Call: "K1ABC", Band: "40m", Mode: "CW"},
		QSO{StartUTC: queryT0.Add(2 * time.Hour), Call: "JA1XYZ", Band: "20m", Mode: "MFSK", Submode: "FT4"},
		QSO{StartUTC: queryT0.Add(3 * time.Hour), Call: "G4AW", Band: "15m", Mode: "FT8"},
		QSO{StartUTC: queryT0.Add(4 * time.Hour), Call: "VE3AWX", Band: "40m", Mode: "SSB"},
	)
	out := make(map[string]int64, len(added))
	for _, q := range added {
		out[q.Call] = q.ID
	}
	return out
}

// idsOf returns the IDs of the given calls in byCall, in the given order.
func idsOf(byCall map[string]int64, calls ...string) []int64 {
	out := make([]int64, 0, len(calls))
	for _, c := range calls {
		out = append(out, byCall[c])
	}
	return out
}

func TestSQLiteStoreExpr(t *testing.T) {
	s := newTestStore(t)
	byCall := seedExprSet(t, s)

	all := []string{"VE3AWX", "G4AW", "JA1XYZ", "K1ABC", "W1AW"}
	band40 := Filter{Bands: []string{"40m"}}
	band20 := Filter{Bands: []string{"20m"}}

	tests := []struct {
		name string
		e    Expr
		want []string // newest first
	}{
		{"nil", nil, all},
		{"All", All{}, all},
		{"zero Filter", Filter{}, all},
		{"empty And", And{}, all},
		{"empty Or", Or{}, nil},
		{"Not All", Not{X: All{}}, nil},
		{"Not nil", Not{}, nil},
		{"And with nil child", And{nil, band40}, []string{"VE3AWX", "K1ABC"}},
		{"Or with nil child", Or{nil, band40}, all},
		{"Filter", band40, []string{"VE3AWX", "K1ABC"}},
		{"Or", Or{band40, band20}, []string{"VE3AWX", "JA1XYZ", "K1ABC", "W1AW"}},
		{"And", And{CallContains{"AW"}, band40}, []string{"VE3AWX"}},
		{"Not", Not{X: band40}, []string{"G4AW", "JA1XYZ", "W1AW"}},
		{"Not Or", Not{X: Or{band40, band20}}, []string{"G4AW"}},
		{
			"nested",
			Or{And{CallContains{"aw"}, Not{X: ModeIs{"ssb"}}}, ModeIs{"cw"}},
			[]string{"G4AW", "K1ABC"},
		},
		{"double Not", Not{X: Not{X: ModeIs{"SSB"}}}, []string{"VE3AWX", "W1AW"}},
		{"CallContains", CallContains{"AW"}, []string{"VE3AWX", "G4AW", "W1AW"}},
		{"CallContains ignores case and space", CallContains{" 1a "}, []string{"K1ABC", "W1AW"}},
		{"CallContains empty", CallContains{}, all},
		{"CallContains no match", CallContains{"ZZ"}, nil},
		{"ModeIs mode", ModeIs{"ssb"}, []string{"VE3AWX", "W1AW"}},
		{"ModeIs submode", ModeIs{" ft4 "}, []string{"JA1XYZ"}},
		{"ModeIs parent mode", ModeIs{"MFSK"}, []string{"JA1XYZ"}},
		{"ModeIs empty", ModeIs{" "}, all},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.IDs(t.Context(), tt.e)
			require.NoError(t, err)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, idsOf(byCall, tt.want...), got)
		})
	}
}

func TestSQLiteStoreCallContainsLiteral(t *testing.T) {
	s := newTestStore(t)
	added := seed(t, s,
		QSO{StartUTC: queryT0, Call: "W1AW"},
		QSO{StartUTC: queryT0.Add(time.Hour), Call: `K1%B`},
		QSO{StartUTC: queryT0.Add(2 * time.Hour), Call: "K1_C"},
		QSO{StartUTC: queryT0.Add(3 * time.Hour), Call: `K1\D`},
	)

	tests := []struct {
		call string
		want []int64
	}{
		{"%", []int64{added[1].ID}},
		{"_", []int64{added[2].ID}},
		{`\`, []int64{added[3].ID}},
		{"1%", []int64{added[1].ID}},
	}
	for _, tt := range tests {
		t.Run(tt.call, func(t *testing.T) {
			got, err := s.IDs(t.Context(), CallContains{tt.call})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExprKeepsValuesInArgs(t *testing.T) {
	const hostile = "x' OR 1=1 --"
	clause, args, err := exprSQL(Or{CallContains{hostile}, Not{X: ModeIs{hostile}}})
	require.NoError(t, err)
	assert.NotContains(t, clause, "OR 1=1")
	assert.Len(t, args, 3)
}

func TestSQLiteStoreExprInvalidFilter(t *testing.T) {
	s := newTestStore(t)
	bad := Filter{Confirmed: Confirmed(7)}

	tests := []struct {
		name string
		e    Expr
	}{
		{"Filter", bad},
		{"Or", Or{All{}, bad}},
		{"And", And{CallContains{"A"}, bad}},
		{"Not", Not{X: bad}},
		{"nested", Not{X: Or{And{bad}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Count(t.Context(), tt.e)
			require.ErrorIs(t, err, ErrInvalidFilter)
			_, err = s.Index(t.Context(), tt.e, 0, 10)
			require.ErrorIs(t, err, ErrInvalidFilter)
			_, err = s.IndexOf(t.Context(), tt.e, 1)
			require.ErrorIs(t, err, ErrInvalidFilter)
			_, err = s.IDs(t.Context(), tt.e)
			require.ErrorIs(t, err, ErrInvalidFilter)
		})
	}
}

func TestSQLiteStoreExprMethodsAgree(t *testing.T) {
	s := newTestStore(t)
	seedIndexSet(t, s)
	e := Or{Filter{Bands: []string{"40m"}}, CallContains{"K0"}}

	gotIDs, err := s.IDs(t.Context(), e)
	require.NoError(t, err)
	require.Len(t, gotIDs, 4)

	n, err := s.Count(t.Context(), e)
	require.NoError(t, err)
	assert.Equal(t, len(gotIDs), n)

	qs, err := s.Index(t.Context(), e, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, gotIDs, ids(qs))

	for i, id := range gotIDs {
		got, err := s.IndexOf(t.Context(), e, id)
		require.NoError(t, err)
		assert.Equal(t, i, got, "QSO %d", id)
	}
}

func TestSQLiteStoreIDs(t *testing.T) {
	s := newTestStore(t)

	got, err := s.IDs(t.Context(), nil)
	require.NoError(t, err)
	assert.Nil(t, got, "nil on an empty logbook")

	want := seedIndexSet(t, s)
	got, err = s.IDs(t.Context(), All{})
	require.NoError(t, err)
	assert.Equal(t, want, got, "newest first, ties by ID descending")
}
