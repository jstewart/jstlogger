package logbook

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStationGrid is the station locator used by newTestStore.
const testStationGrid = "FN42"

// newTestStore opens a SQLiteStore on a fresh database file under
// t.TempDir() and closes it when the test ends.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	return openTestStore(t, filepath.Join(t.TempDir(), "logbook.db"))
}

// openTestStore opens a SQLiteStore at path and closes it when the test ends.
func openTestStore(t *testing.T, path string) *SQLiteStore {
	t.Helper()
	s, err := Open(t.Context(), path, Options{StationGrid: testStationGrid})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	return s
}

// fixedClock sets s's clock to *at, so tests can advance time without sleeping.
func fixedClock(s *SQLiteStore, at *time.Time) {
	s.now = func() time.Time { return *at }
}

// valid wraps v in a valid sql.Null.
func valid[T any](v T) sql.Null[T] {
	return sql.Null[T]{V: v, Valid: true}
}

// truncateTimes returns q with every time truncated to whole seconds in UTC,
// matching what the store persists.
func truncateTimes(q QSO) QSO {
	tr := func(t time.Time) time.Time { return t.UTC().Truncate(time.Second) }
	trn := func(t sql.Null[time.Time]) sql.Null[time.Time] {
		if t.Valid {
			t.V = tr(t.V)
		}
		return t
	}
	q.StartUTC = tr(q.StartUTC)
	q.EndUTC = trn(q.EndUTC)
	q.CreatedAt = tr(q.CreatedAt)
	q.UpdatedAt = tr(q.UpdatedAt)
	for i := range q.Confirmations {
		q.Confirmations[i].SentDate = trn(q.Confirmations[i].SentDate)
		q.Confirmations[i].RcvdDate = trn(q.Confirmations[i].RcvdDate)
	}
	return q
}

// fullQSO returns a QSO with every promoted field set, side fields, two
// confirmations and tags. Times carry sub-second parts to check truncation.
func fullQSO() QSO {
	start := time.Date(2026, 10, 28, 14, 42, 17, 123456789, time.UTC)
	return QSO{
		StartUTC:        start,
		EndUTC:          valid(start.Add(3*time.Minute + 500*time.Millisecond)),
		Call:            "K1ABC",
		StationCallsign: "N0CALL",
		Operator:        "W1OP",
		FreqHz:          valid[int64](14_074_000),
		FreqRxHz:        valid[int64](14_075_000),
		Band:            "20m",
		Mode:            "MFSK",
		Submode:         "FT4",
		RSTSent:         "-10",
		RSTRcvd:         "-12",
		TxPwr:           valid(100.0),
		Name:            "Alice",
		QTH:             "Boston",
		Gridsquare:      "FN31PR",
		MyGridsquare:    "JO01",
		DXCC:            valid(291),
		Country:         "United States",
		Cont:            "NA",
		CQZ:             valid(5),
		ITUZ:            valid(8),
		ITURegion:       valid(2),
		DistanceKm:      valid(1.0), // recomputed from the grids
		State:           "MA",
		Cnty:            "MA,Suffolk",
		Comment:         "nice signal",
		Fields: map[string]string{
			"SOTA_REF":   "W1/HA-001",
			"CONTEST_ID": "CQ-WW-SSB",
		},
		Confirmations: []Confirmation{
			{Service: ServiceLoTW, Sent: "Y", SentDate: valid(start.Add(time.Hour)), Rcvd: "Y", RcvdDate: valid(start.Add(48 * time.Hour))},
			{Service: ServiceEQSL, Sent: "N", Rcvd: "R"},
		},
		Tags: []string{"pota", "DX"},
	}
}

func TestSQLiteStoreAddGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 28, 15, 0, 0, 987654321, time.UTC)
	fixedClock(s, &now)

	in := fullQSO()
	added, err := s.Add(ctx, in)
	require.NoError(t, err)

	assert.NotZero(t, added.ID)
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`), added.UUID)

	want, err := prepare(in, testStationGrid)
	require.NoError(t, err)
	want.ID = added.ID
	want.UUID = added.UUID
	want.CreatedAt = now
	want.UpdatedAt = now
	want = truncateTimes(want)
	// Confirmations come back ordered by service, tags by name.
	want.Confirmations = []Confirmation{want.Confirmations[1], want.Confirmations[0]}
	want.Tags = []string{"DX", "pota"}

	got, err := s.Get(ctx, added.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, got, added, "Add returns what Get returns")
	assert.NotEqual(t, 1.0, got.DistanceKm.V, "distance is recomputed from the grids")
	assert.Equal(t, "W1OP", got.Operator)
}

func TestSQLiteStoreAddKeepsGivenUUID(t *testing.T) {
	s := newTestStore(t)
	q := QSO{StartUTC: time.Now(), Call: "W1AW", UUID: "6f1c0a52-5d0f-4b8e-9c1e-1d2f3a4b5c6d"}

	added, err := s.Add(t.Context(), q)

	require.NoError(t, err)
	assert.Equal(t, q.UUID, added.UUID)
}

func TestSQLiteStoreNullRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	added, err := s.Add(ctx, QSO{StartUTC: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Call: "W1AW"})
	require.NoError(t, err)
	got, err := s.Get(ctx, added.ID)
	require.NoError(t, err)

	assert.False(t, got.EndUTC.Valid)
	assert.False(t, got.FreqHz.Valid)
	assert.False(t, got.FreqRxHz.Valid)
	assert.False(t, got.TxPwr.Valid)
	assert.False(t, got.DXCC.Valid)
	assert.False(t, got.CQZ.Valid)
	assert.False(t, got.ITUZ.Valid)
	assert.False(t, got.ITURegion.Valid)
	assert.False(t, got.DistanceKm.Valid)
	assert.Empty(t, got.Band)
	assert.Nil(t, got.Fields)
	assert.Nil(t, got.Confirmations)
	assert.Nil(t, got.Tags)
}

func TestSQLiteStorePersistsDerivedValues(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	added, err := s.Add(ctx, QSO{
		StartUTC:   time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Call:       "G4ABC",
		FreqHz:     valid[int64](7_074_000),
		Gridsquare: "IO91",
	})
	require.NoError(t, err)
	got, err := s.Get(ctx, added.ID)
	require.NoError(t, err)

	wantKm, err := GridDistanceKm("IO91", testStationGrid)
	require.NoError(t, err)
	assert.Equal(t, "40m", got.Band)
	assert.Equal(t, valid(wantKm), got.DistanceKm, "distance from the station grid")
	assert.Empty(t, got.MyGridsquare, "station grid is not stored in the QSO")
	assert.Equal(t, valid(1), got.ITURegion)
}

func TestSQLiteStoreUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC)
	fixedClock(s, &now)

	added, err := s.Add(ctx, fullQSO())
	require.NoError(t, err)

	now = now.Add(time.Hour)
	upd := added
	upd.UUID = "ignored"
	upd.CreatedAt = time.Time{}
	upd.Gridsquare = "PM95"
	upd.ITURegion = sql.Null[int]{}
	upd.ITUZ = sql.Null[int]{}
	upd.Fields = map[string]string{"SIG": "POTA"}
	upd.Confirmations = []Confirmation{{Service: ServiceCard, Sent: "Q"}}
	upd.Tags = []string{"jp"}
	require.NoError(t, s.Update(ctx, upd))

	got, err := s.Get(ctx, added.ID)
	require.NoError(t, err)

	wantKm, err := GridDistanceKm("PM95", "JO01")
	require.NoError(t, err)
	assert.Equal(t, valid(wantKm), got.DistanceKm, "distance follows the new grid")
	assert.NotEqual(t, added.DistanceKm, got.DistanceKm)
	assert.Equal(t, valid(3), got.ITURegion, "region is re-derived")
	assert.Equal(t, map[string]string{"SIG": "POTA"}, got.Fields)
	assert.Equal(t, []Confirmation{{Service: ServiceCard, Sent: "Q"}}, got.Confirmations)
	assert.Equal(t, []string{"jp"}, got.Tags)
	assert.Equal(t, added.UUID, got.UUID, "uuid is kept")
	assert.Equal(t, added.CreatedAt, got.CreatedAt, "created_at is kept")
	assert.Equal(t, now, got.UpdatedAt, "updated_at advances")

	assert.Equal(t, 1, countRows(t, s, "qso_adif_field"))
	assert.Equal(t, 1, countRows(t, s, "qso_confirmation"))
	assert.Equal(t, 1, countRows(t, s, "qso_tag"))
}

func TestSQLiteStoreUpdateClearsComputedDistance(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	added, err := s.Add(ctx, QSO{StartUTC: queryT0, Call: "G4ABC", Gridsquare: "JO01", MyGridsquare: "FN31"})
	require.NoError(t, err)
	require.True(t, added.DistanceKm.Valid)

	upd := added
	upd.Gridsquare = ""
	require.NoError(t, s.Update(ctx, upd))

	got, err := s.Get(ctx, added.ID)
	require.NoError(t, err)
	assert.False(t, got.DistanceKm.Valid, "a distance computed from a removed grid must not survive")
}

func TestSQLiteStoreUpdateKeepsImportedDistance(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	added, err := s.Add(ctx, QSO{StartUTC: queryT0, Call: "G4ABC", DistanceKm: valid(1234.0)})
	require.NoError(t, err)

	upd := added
	upd.Comment = "edited"
	require.NoError(t, s.Update(ctx, upd))

	got, err := s.Get(ctx, added.ID)
	require.NoError(t, err)
	assert.Equal(t, valid(1234.0), got.DistanceKm)
}

func TestSQLiteStoreDeleteCascades(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	added, err := s.Add(ctx, fullQSO())
	require.NoError(t, err)
	require.NoError(t, s.Delete(ctx, added.ID))

	_, err = s.Get(ctx, added.ID)
	require.ErrorIs(t, err, ErrNotFound)
	for _, table := range []string{"qso", "qso_adif_field", "qso_confirmation", "qso_tag"} {
		assert.Equal(t, 0, countRows(t, s, table), "table %s", table)
	}
}

func TestSQLiteStoreMissingIDNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	_, err := s.Get(ctx, 42)
	require.ErrorIs(t, err, ErrNotFound)

	err = s.Update(ctx, QSO{ID: 42, StartUTC: time.Now(), Call: "W1AW"})
	require.ErrorIs(t, err, ErrNotFound)

	err = s.Delete(ctx, 42)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestSQLiteStoreAddRejectsPromotedField(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	q := QSO{StartUTC: time.Now(), Call: "W1AW", Fields: map[string]string{"freq": "14.074"}}

	_, err := s.Add(ctx, q)

	require.ErrorIs(t, err, ErrPromotedField)
	assert.Equal(t, 0, countRows(t, s, "qso"))
}

func TestSQLiteStoreReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dir with space?#", "logbook.db")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	ctx := t.Context()

	first, err := Open(ctx, path, Options{})
	require.NoError(t, err)
	added, err := first.Add(ctx, fullQSO())
	require.NoError(t, err)
	require.NoError(t, first.Close())
	_, err = os.Stat(path)
	require.NoError(t, err, "database file is created at the escaped path")

	second := openTestStore(t, path)
	got, err := second.Get(ctx, added.ID)
	require.NoError(t, err)
	assert.Equal(t, added, got)

	var v int
	require.NoError(t, second.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v))
	assert.Equal(t, 2, v)
}

func TestSQLiteStoreEnablesPragmas(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	var fk int
	require.NoError(t, s.db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, 1, fk)
	var mode string
	require.NoError(t, s.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode)
	var timeout int
	require.NoError(t, s.db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&timeout))
	assert.Equal(t, 5000, timeout)
}

// countRows returns the number of rows in table, which must be a trusted
// constant.
func countRows(t *testing.T, s *SQLiteStore, table string) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
	return n
}
