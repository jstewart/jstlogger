package logbook

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseQSO returns a minimal valid QSO for prepare tests.
func baseQSO(t *testing.T) QSO {
	t.Helper()
	return QSO{
		StartUTC: time.Date(2026, 10, 28, 14, 42, 17, 0, time.UTC),
		Call:     "W1AW",
	}
}

func TestPrepareRequiresStartUTC(t *testing.T) {
	_, err := prepare(QSO{Call: "W1AW"}, "")
	assert.ErrorIs(t, err, ErrMissingStart)
}

func TestPrepareNormalizesCase(t *testing.T) {
	q := baseQSO(t)
	q.Call = " k1abc/p "
	q.StationCallsign = "n0call"
	q.Operator = " w1op "
	q.Gridsquare = "fn31pr"
	q.MyGridsquare = " jo01 "
	q.Cont = "na"
	q.Band = " 20M "
	q.Mode = " ft8 "
	q.Submode = "ft4"
	q.Confirmations = []Confirmation{{Service: " LoTW ", Rcvd: "Y"}}

	got, err := prepare(q, "")
	require.NoError(t, err)
	assert.Equal(t, "K1ABC/P", got.Call)
	assert.Equal(t, "N0CALL", got.StationCallsign)
	assert.Equal(t, "W1OP", got.Operator)
	assert.Equal(t, "FN31PR", got.Gridsquare)
	assert.Equal(t, "JO01", got.MyGridsquare)
	assert.Equal(t, "NA", got.Cont)
	assert.Equal(t, "20m", got.Band)
	assert.Equal(t, "FT8", got.Mode)
	assert.Equal(t, "FT4", got.Submode)
	assert.Equal(t, ServiceLoTW, got.Confirmations[0].Service)
}

func TestPrepareConvertsTimesToUTC(t *testing.T) {
	est := time.FixedZone("EST", -5*60*60)
	start := time.Date(2026, 10, 28, 9, 0, 0, 0, est)
	end := start.Add(5 * time.Minute)

	q := baseQSO(t)
	q.StartUTC = start
	q.EndUTC = sql.Null[time.Time]{V: end, Valid: true}
	q.Confirmations = []Confirmation{{
		Service:  ServiceLoTW,
		Rcvd:     "Y",
		RcvdDate: sql.Null[time.Time]{V: end, Valid: true},
		SentDate: sql.Null[time.Time]{V: start, Valid: true},
	}}

	got, err := prepare(q, "")
	require.NoError(t, err)
	assert.Equal(t, time.UTC, got.StartUTC.Location())
	assert.True(t, got.StartUTC.Equal(start))
	assert.Equal(t, 14, got.StartUTC.Hour())
	require.True(t, got.EndUTC.Valid)
	assert.Equal(t, time.UTC, got.EndUTC.V.Location())
	assert.True(t, got.EndUTC.V.Equal(end))
	require.Len(t, got.Confirmations, 1)
	assert.Equal(t, time.UTC, got.Confirmations[0].RcvdDate.V.Location())
	assert.Equal(t, time.UTC, got.Confirmations[0].SentDate.V.Location())

	// The caller's confirmation slice is untouched.
	assert.Equal(t, est, q.Confirmations[0].RcvdDate.V.Location())
}

func TestPrepareBand(t *testing.T) {
	tests := []struct {
		name string
		band string
		freq sql.Null[int64]
		want string
	}{
		{name: "derived from freq", freq: sql.Null[int64]{V: 14_074_000, Valid: true}, want: "20m"},
		{name: "explicit band kept", band: "40M", freq: sql.Null[int64]{V: 14_074_000, Valid: true}, want: "40m"},
		{name: "freq outside any band", freq: sql.Null[int64]{V: 4_500_000, Valid: true}, want: ""},
		{name: "no freq", want: ""},
		{name: "invalid freq ignored", freq: sql.Null[int64]{V: 14_074_000}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.Band = tt.band
			q.FreqHz = tt.freq
			got, err := prepare(q, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Band)
		})
	}
}

func TestPrepareITURegion(t *testing.T) {
	tests := []struct {
		name     string
		ituz     sql.Null[int]
		grid     string
		explicit sql.Null[int]
		want     sql.Null[int]
	}{
		{name: "from known zone", ituz: sql.Null[int]{V: 45, Valid: true}, want: sql.Null[int]{V: 3, Valid: true}},
		{name: "zone wins over grid", ituz: sql.Null[int]{V: 27, Valid: true}, grid: "FN31", want: sql.Null[int]{V: 1, Valid: true}},
		{name: "straddling zone falls back to grid", ituz: sql.Null[int]{V: 61, Valid: true}, grid: "BL11", want: sql.Null[int]{V: 2, Valid: true}},
		{name: "grid only", grid: "PM95", want: sql.Null[int]{V: 3, Valid: true}},
		{name: "invalid grid and no zone", grid: "ZZ99", want: sql.Null[int]{}},
		{name: "nothing known", want: sql.Null[int]{}},
		{name: "stale value re-derived", explicit: sql.Null[int]{V: 2, Valid: true}, ituz: sql.Null[int]{V: 27, Valid: true}, want: sql.Null[int]{V: 1, Valid: true}},
		{name: "stale value cleared", explicit: sql.Null[int]{V: 2, Valid: true}, want: sql.Null[int]{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.ITUZ = tt.ituz
			q.Gridsquare = tt.grid
			q.ITURegion = tt.explicit
			got, err := prepare(q, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.ITURegion)
		})
	}
}

// roundedDistance returns the distance between two grids rounded to 0.1 km,
// as the store calculates it.
func roundedDistance(t *testing.T, grid, myGrid string) float64 {
	t.Helper()
	km, err := GridDistanceKm(grid, myGrid)
	require.NoError(t, err)
	return math.Round(km*10) / 10
}

func TestPrepareDistance(t *testing.T) {
	imported := sql.Null[float64]{V: 1234.56, Valid: true}
	calculated := roundedDistance(t, "JO01", "FN31PR")
	tests := []struct {
		name        string
		grid, mine  string
		stationGrid string
		supplied    sql.Null[float64]
		want        sql.Null[float64]
	}{
		{name: "calculated from both grids", grid: "JO01", mine: "FN31pr", want: valid(calculated)},
		{name: "imported wins over calculated", grid: "JO01", mine: "FN31pr", supplied: imported, want: imported},
		{name: "station grid fallback", grid: "JO01", stationGrid: "fn31pr", want: valid(calculated)},
		{name: "my grid preferred over station grid", grid: "JO01", mine: "JO01", stationGrid: "FN31pr", want: valid(0.0)},
		{name: "imported kept without their grid", mine: "FN31pr", supplied: imported, want: imported},
		{name: "imported kept without my grid", grid: "JO01", supplied: imported, want: imported},
		{name: "imported kept when grid invalid", grid: "ZZ99", mine: "FN31", supplied: imported, want: imported},
		{name: "zero distance import kept", grid: "JO01", mine: "FN31pr", supplied: valid(0.0), want: valid(0.0)},
		{name: "nothing known", grid: "JO01", want: sql.Null[float64]{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.Gridsquare = tt.grid
			q.MyGridsquare = tt.mine
			q.DistanceKm = tt.supplied
			got, err := prepare(q, tt.stationGrid)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.DistanceKm)
		})
	}
}

func TestPrepareDoesNotStoreStationGrid(t *testing.T) {
	q := baseQSO(t)
	q.Gridsquare = "JO01"
	got, err := prepare(q, "FN31pr")
	require.NoError(t, err)
	assert.Empty(t, got.MyGridsquare)
	assert.True(t, got.DistanceKm.Valid)
}

func TestPrepareFields(t *testing.T) {
	q := baseQSO(t)
	q.Fields = map[string]string{
		" contest_id ": " ARRL-FD ",
		"App_N1MM_Id":  "abc",
		"USERDEF1":     "x",
	}
	got, err := prepare(q, "")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"CONTEST_ID":  "ARRL-FD",
		"APP_N1MM_ID": "abc",
		"USERDEF1":    "x",
	}, got.Fields)

	// The caller's map is untouched.
	assert.Equal(t, " ARRL-FD ", q.Fields[" contest_id "])
	assert.Len(t, q.Fields, 3)
}

func TestPrepareFieldsRejectsPromoted(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "call", key: "CALL"},
		{name: "lower-case freq", key: "freq"},
		{name: "padded gridsquare", key: " gridsquare "},
		{name: "operator", key: "OPERATOR"},
		{name: "jstlogger uuid", key: "app_jstlogger_uuid"},
		{name: "distance", key: "DISTANCE"},
		{name: "lotw received", key: "LOTW_QSL_RCVD"},
		{name: "eqsl sent date", key: "EQSL_QSLSDATE"},
		{name: "card received date", key: "QSLRDATE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.Fields = map[string]string{tt.key: "x"}
			_, err := prepare(q, "")
			assert.ErrorIs(t, err, ErrPromotedField)
		})
	}
}

func TestPrepareFieldsRejectsBadNames(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]string
	}{
		{name: "empty name", fields: map[string]string{"  ": "x"}},
		{name: "duplicate after normalization", fields: map[string]string{"contest_id": "a", "CONTEST_ID": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.Fields = tt.fields
			_, err := prepare(q, "")
			assert.ErrorIs(t, err, ErrInvalidField)
		})
	}
}

func TestPrepareTags(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{name: "nil", tags: nil, want: nil},
		{name: "trimmed", tags: []string{" rare ", "needs-card"}, want: []string{"rare", "needs-card"}},
		{name: "empties dropped", tags: []string{"", "  ", "rare"}, want: []string{"rare"}},
		{name: "case-insensitive dedupe keeps first", tags: []string{"Rare", "rare", " RARE ", "fd"}, want: []string{"Rare", "fd"}},
		{name: "all empty", tags: []string{" ", ""}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.Tags = tt.tags
			var orig []string
			if tt.tags != nil {
				orig = append([]string{}, tt.tags...)
			}
			got, err := prepare(q, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Tags)
			assert.Equal(t, orig, q.Tags, "caller's slice must be untouched")
		})
	}
}

func TestPromotedFieldsAreUpperCase(t *testing.T) {
	require.NotEmpty(t, promotedFields)
	for _, name := range promotedFields {
		assert.Regexp(t, `^[A-Z_]+$`, name)
	}
}

func TestCursorIsZero(t *testing.T) {
	assert.True(t, Cursor{}.IsZero())
	assert.False(t, Cursor{ID: 1}.IsZero())
	assert.False(t, Cursor{StartUTC: time.Unix(0, 0)}.IsZero())
}

func TestPrepareRejectsBadConfirmations(t *testing.T) {
	tests := []struct {
		name  string
		confs []Confirmation
	}{
		{"empty service", []Confirmation{{Service: " ", Rcvd: "Y"}}},
		{"duplicate service", []Confirmation{{Service: ServiceLoTW}, {Service: " LoTW "}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := baseQSO(t)
			q.Confirmations = tt.confs
			_, err := prepare(q, "")
			assert.ErrorIs(t, err, ErrInvalidConfirmation)
		})
	}
}
