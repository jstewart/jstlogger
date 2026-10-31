package logbook

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importQSO returns a minimal QSO with a full natural key, i minutes after
// queryT0.
func importQSO(call string, i int) QSO {
	return QSO{
		StartUTC:        queryT0.Add(time.Duration(i) * time.Minute),
		Call:            call,
		Band:            "20m",
		Mode:            "FT8",
		StationCallsign: "N0CALL",
	}
}

// eachAll collects every QSO matching f through Each.
func eachAll(t *testing.T, s *SQLiteStore, f Filter) []QSO {
	t.Helper()
	var out []QSO
	require.NoError(t, s.Each(t.Context(), f, func(q QSO) error {
		out = append(out, q)
		return nil
	}))
	return out
}

func TestSQLiteStoreImportAdds(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	fixedClock(s, &now)
	withUUID := importQSO("K1ABC", 1)
	withUUID.UUID = "6f1c0a52-5d0f-4b8e-9c1e-1d2f3a4b5c6d"
	withChildren := importQSO("G4ABC", 2)
	withChildren.Fields = map[string]string{"SOTA_REF": "G/LD-001"}
	withChildren.Confirmations = []Confirmation{{Service: ServiceLoTW, Rcvd: "Y"}}
	withChildren.Tags = []string{"sota"}

	res, err := s.Import(ctx, []QSO{importQSO("W1AW", 0), withUUID, withChildren}, nil)

	require.NoError(t, err)
	assert.Equal(t, ImportResult{Added: 3}, res)
	got := eachAll(t, s, Filter{})
	require.Len(t, got, 3)
	assert.Equal(t, []string{"W1AW", "K1ABC", "G4ABC"}, calls(got))
	assert.NotEmpty(t, got[0].UUID, "a missing UUID is generated")
	assert.Equal(t, withUUID.UUID, got[1].UUID, "a given UUID is kept")
	assert.Equal(t, map[string]string{"SOTA_REF": "G/LD-001"}, got[2].Fields)
	assert.Equal(t, []Confirmation{{Service: ServiceLoTW, Rcvd: "Y"}}, got[2].Confirmations)
	assert.Equal(t, []string{"sota"}, got[2].Tags)
	for _, q := range got {
		assert.Equal(t, now, q.CreatedAt)
		assert.Equal(t, now, q.UpdatedAt)
	}
}

func TestSQLiteStoreImportEmpty(t *testing.T) {
	s := newTestStore(t)

	res, err := s.Import(t.Context(), nil, []string{"x"})

	require.NoError(t, err)
	assert.Equal(t, ImportResult{}, res)
	assert.Equal(t, 0, countRows(t, s, "tag"))
}

func TestSQLiteStoreImportIsAtomic(t *testing.T) {
	s := newTestStore(t)
	bad := importQSO("K1ABC", 1)
	bad.StartUTC = time.Time{}

	_, err := s.Import(t.Context(), []QSO{importQSO("W1AW", 0), importQSO("G4ABC", 2), bad}, []string{"batch"})

	require.ErrorIs(t, err, ErrMissingStart)
	assert.Contains(t, err.Error(), "qso 3")
	assert.Equal(t, 0, countRows(t, s, "qso"))
	assert.Equal(t, 0, countRows(t, s, "tag"))
}

func TestSQLiteStoreImportRollsBackOnInsertFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	// A valid QSO cannot fail to insert, so make the database reject one.
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_fail BEFORE INSERT ON qso
		WHEN NEW.call = 'FAIL' BEGIN SELECT RAISE(ABORT, 'rejected'); END`)
	require.NoError(t, err)
	first := importQSO("W1AW", 0)
	first.Fields = map[string]string{"SIG": "POTA"}

	_, err = s.Import(ctx, []QSO{first, importQSO("FAIL", 1)}, []string{"batch"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "qso 2")
	for _, table := range []string{"qso", "qso_adif_field", "tag", "qso_tag"} {
		assert.Equal(t, 0, countRows(t, s, table), "table %s", table)
	}
}

func TestSQLiteStoreImportSkipsDuplicates(t *testing.T) {
	const uuid = "6f1c0a52-5d0f-4b8e-9c1e-1d2f3a4b5c6d"
	stored := importQSO("W1AW", 0)
	stored.UUID = uuid

	tests := []struct {
		name string
		qsos func() []QSO
		want ImportResult
	}{
		{
			name: "same uuid as stored",
			qsos: func() []QSO {
				q := importQSO("K1ABC", 5)
				q.UUID = uuid
				return []QSO{q}
			},
			want: ImportResult{Skipped: 1},
		},
		{
			name: "same natural key as stored",
			qsos: func() []QSO { return []QSO{importQSO("W1AW", 0)} },
			want: ImportResult{Skipped: 1},
		},
		{
			name: "natural key matches after normalization",
			qsos: func() []QSO {
				q := importQSO(" w1aw ", 0)
				q.StartUTC = q.StartUTC.Add(400 * time.Millisecond).In(time.FixedZone("EST", -5*60*60))
				q.Band = "20M"
				q.Mode = "ft8"
				q.StationCallsign = "n0call"
				return []QSO{q}
			},
			want: ImportResult{Skipped: 1},
		},
		{
			name: "band derived from frequency",
			qsos: func() []QSO {
				q := importQSO("W1AW", 0)
				q.Band = ""
				q.FreqHz = valid[int64](14_074_000)
				return []QSO{q}
			},
			want: ImportResult{Skipped: 1},
		},
		{
			name: "different station callsign is new",
			qsos: func() []QSO {
				q := importQSO("W1AW", 0)
				q.StationCallsign = "N0CALL/P"
				return []QSO{q}
			},
			want: ImportResult{Added: 1},
		},
		{
			name: "next second is new",
			qsos: func() []QSO {
				q := importQSO("W1AW", 0)
				q.StartUTC = q.StartUTC.Add(time.Second)
				return []QSO{q}
			},
			want: ImportResult{Added: 1},
		},
		{
			name: "natural key repeated within the batch",
			qsos: func() []QSO { return []QSO{importQSO("K1ABC", 5), importQSO("k1abc", 5)} },
			want: ImportResult{Added: 1, Skipped: 1},
		},
		{
			name: "uuid repeated within the batch",
			qsos: func() []QSO {
				a, b := importQSO("K1ABC", 5), importQSO("G4ABC", 6)
				a.UUID, b.UUID = "a-uuid", "a-uuid"
				return []QSO{a, b}
			},
			want: ImportResult{Added: 1, Skipped: 1},
		},
		{
			name: "uuid of a skipped duplicate in the batch",
			qsos: func() []QSO {
				a, b := importQSO("W1AW", 0), importQSO("G4ABC", 6)
				a.UUID, b.UUID = "b-uuid", "b-uuid"
				return []QSO{a, b}
			},
			want: ImportResult{Skipped: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			_, err := s.Import(ctx, []QSO{stored}, nil)
			require.NoError(t, err)

			res, err := s.Import(ctx, tt.qsos(), nil)

			require.NoError(t, err)
			assert.Equal(t, tt.want, res)
			assert.Equal(t, 1+tt.want.Added, countRows(t, s, "qso"))
		})
	}
}

func TestSQLiteStoreImportTagsOnlyAddedQSOs(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	first := importQSO("W1AW", 0)
	first.Tags = []string{"old"}
	_, err := s.Import(ctx, []QSO{first}, nil)
	require.NoError(t, err)
	fresh := importQSO("K1ABC", 1)
	fresh.Tags = []string{"own", "Field-Day"}
	callerTags := fresh.Tags

	res, err := s.Import(ctx, []QSO{importQSO("W1AW", 0), fresh}, []string{"field-day", " contest "})

	require.NoError(t, err)
	assert.Equal(t, ImportResult{Added: 1, Skipped: 1}, res)
	got := eachAll(t, s, Filter{})
	require.Len(t, got, 2)
	assert.Equal(t, []string{"old"}, got[0].Tags, "a skipped duplicate is not tagged")
	assert.Equal(t, []string{"contest", "Field-Day", "own"}, got[1].Tags)
	assert.Equal(t, []string{"own", "Field-Day"}, callerTags, "caller's tags are untouched")
}

func TestSQLiteStoreEachOldestFirstAcrossBatches(t *testing.T) {
	s := newTestStore(t)
	old := eachBatchSize
	eachBatchSize = 3
	t.Cleanup(func() { eachBatchSize = old })
	// Two QSOs share a start time, so the ID breaks the tie.
	var qs []QSO
	for i := range 7 {
		qs = append(qs, importQSO(fmt.Sprintf("K%dA", i), i))
	}
	tie := importQSO("TIE", 3)
	qs = append(qs, tie)
	_, err := s.Import(t.Context(), qs, []string{"t"})
	require.NoError(t, err)

	got := eachAll(t, s, Filter{})

	assert.Equal(t, []string{"K0A", "K1A", "K2A", "K3A", "TIE", "K4A", "K5A", "K6A"}, calls(got))
	for _, q := range got {
		assert.Equal(t, []string{"t"}, q.Tags, "children are loaded")
	}
}

func TestSQLiteStoreEachExactBatch(t *testing.T) {
	s := newTestStore(t)
	old := eachBatchSize
	eachBatchSize = 2
	t.Cleanup(func() { eachBatchSize = old })
	_, err := s.Import(t.Context(), []QSO{importQSO("A", 0), importQSO("B", 1)}, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"A", "B"}, calls(eachAll(t, s, Filter{})))
}

func TestSQLiteStoreEachEmpty(t *testing.T) {
	s := newTestStore(t)

	assert.Empty(t, eachAll(t, s, Filter{}))
}

func TestSQLiteStoreEachFiltersByAllTags(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	a, b, c := importQSO("A", 0), importQSO("B", 1), importQSO("C", 2)
	a.Tags = []string{"pota", "dx"}
	b.Tags = []string{"pota"}
	c.Tags = []string{"DX", "POTA", "cw"}
	_, err := s.Import(ctx, []QSO{a, b, c}, nil)
	require.NoError(t, err)

	got := eachAll(t, s, Filter{Tags: []string{"pota", "dx"}})

	assert.Equal(t, []string{"A", "C"}, calls(got))
}

func TestSQLiteStoreEachInvalidFilter(t *testing.T) {
	s := newTestStore(t)

	err := s.Each(t.Context(), Filter{Confirmed: Confirmed(9)}, func(QSO) error { return nil })

	require.ErrorIs(t, err, ErrInvalidFilter)
}

func TestSQLiteStoreEachStopsOnError(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Import(t.Context(), []QSO{importQSO("A", 0), importQSO("B", 1), importQSO("C", 2)}, nil)
	require.NoError(t, err)
	errStop := errors.New("stop")

	var seen []string
	err = s.Each(t.Context(), Filter{}, func(q QSO) error {
		seen = append(seen, q.Call)
		if q.Call == "B" {
			return errStop
		}
		return nil
	})

	require.ErrorIs(t, err, errStop)
	assert.Equal(t, []string{"A", "B"}, seen)
}

func TestSQLiteStoreEachRespectsContext(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Import(t.Context(), []QSO{importQSO("A", 0)}, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err = s.Each(ctx, Filter{}, func(QSO) error { return nil })

	require.ErrorIs(t, err, context.Canceled)
}
