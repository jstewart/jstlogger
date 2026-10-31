package logbook

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/migrations"
)

// openRawDB opens a bare SQLite database file under t.TempDir() with no
// migrations applied, and closes it when the test ends.
func openRawDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "raw.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.PingContext(t.Context()))
	return db
}

// userVersion reads PRAGMA user_version.
func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	require.NoError(t, db.QueryRowContext(t.Context(), "PRAGMA user_version").Scan(&v))
	return v
}

// schemaObjectExists reports whether sqlite_master has an object of the given
// type and name.
func schemaObjectExists(t *testing.T, db *sql.DB, typ, name string) bool {
	t.Helper()
	var n int
	err := db.QueryRowContext(t.Context(),
		"SELECT count(*) FROM sqlite_master WHERE type = ? AND name = ?", typ, name).Scan(&n)
	require.NoError(t, err)
	return n > 0
}

// testMigrations is a valid two-step synthetic migration set.
func testMigrations() fstest.MapFS {
	return fstest.MapFS{
		"0001_create_a.sql": {Data: []byte("CREATE TABLE a (x INTEGER);")},
		"0002_create_b.sql": {Data: []byte("CREATE TABLE b (y TEXT); INSERT INTO a (x) VALUES (1);")},
		"README.md":         {Data: []byte("not a migration")},
	}
}

func TestMigrateEmptyToLatest(t *testing.T) {
	db := openRawDB(t)

	require.NoError(t, migrate(t.Context(), db, testMigrations()))

	assert.Equal(t, 2, userVersion(t, db))
	assert.True(t, schemaObjectExists(t, db, "table", "a"))
	assert.True(t, schemaObjectExists(t, db, "table", "b"))
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openRawDB(t)
	ctx := t.Context()

	require.NoError(t, migrate(ctx, db, testMigrations()))
	require.NoError(t, migrate(ctx, db, testMigrations()))

	assert.Equal(t, 2, userVersion(t, db))
	var n int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM a").Scan(&n))
	assert.Equal(t, 1, n, "second run must not re-apply migration 2")
}

func TestMigrateAppliesOnlyPending(t *testing.T) {
	db := openRawDB(t)
	ctx := t.Context()
	fsys := testMigrations()
	first := fstest.MapFS{"0001_create_a.sql": fsys["0001_create_a.sql"]}

	require.NoError(t, migrate(ctx, db, first))
	assert.Equal(t, 1, userVersion(t, db))

	require.NoError(t, migrate(ctx, db, fsys))
	assert.Equal(t, 2, userVersion(t, db))
	assert.True(t, schemaObjectExists(t, db, "table", "b"))
}

func TestMigrateFailureRollsBack(t *testing.T) {
	db := openRawDB(t)
	fsys := fstest.MapFS{
		"0001_ok.sql":  {Data: []byte("CREATE TABLE a (x INTEGER);")},
		"0002_bad.sql": {Data: []byte("CREATE TABLE b (y TEXT); INSERT INTO missing VALUES (1);")},
	}

	err := migrate(t.Context(), db, fsys)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "0002_bad.sql")
	assert.Equal(t, 1, userVersion(t, db))
	assert.True(t, schemaObjectExists(t, db, "table", "a"))
	assert.False(t, schemaObjectExists(t, db, "table", "b"), "failed migration must roll back")
}

func TestMigrateSchemaTooNew(t *testing.T) {
	db := openRawDB(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, "PRAGMA user_version = 5")
	require.NoError(t, err)

	err = migrate(ctx, db, testMigrations())

	require.ErrorIs(t, err, ErrSchemaTooNew)
	assert.Equal(t, 5, userVersion(t, db))
	assert.False(t, schemaObjectExists(t, db, "table", "a"))
}

func TestMigrateNegativeVersion(t *testing.T) {
	db := openRawDB(t)
	ctx := t.Context()
	_, err := db.ExecContext(ctx, "PRAGMA user_version = -1")
	require.NoError(t, err)

	err = migrate(ctx, db, testMigrations())

	require.Error(t, err)
	assert.False(t, schemaObjectExists(t, db, "table", "a"))
}

func TestMigrateRejectsBadSets(t *testing.T) {
	sql1 := &fstest.MapFile{Data: []byte("CREATE TABLE a (x INTEGER);")}
	tests := []struct {
		name string
		fsys fstest.MapFS
		want string
	}{
		{"gap", fstest.MapFS{"0001_a.sql": sql1, "0003_c.sql": sql1}, "missing migration 2"},
		{"duplicate", fstest.MapFS{"0001_a.sql": sql1, "0001_b.sql": sql1}, "duplicate"},
		{"no number", fstest.MapFS{"init.sql": sql1}, "init.sql"},
		{"no description", fstest.MapFS{"0001.sql": sql1}, "0001.sql"},
		{"zero", fstest.MapFS{"0000_zero.sql": sql1}, "0000_zero.sql"},
		{"not numeric", fstest.MapFS{"00a1_x.sql": sql1}, "00a1_x.sql"},
		{"starts at two", fstest.MapFS{"0002_b.sql": sql1}, "missing migration 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openRawDB(t)

			err := migrate(t.Context(), db, tt.fsys)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
			assert.Equal(t, 0, userVersion(t, db))
			assert.False(t, schemaObjectExists(t, db, "table", "a"), "nothing may be applied")
		})
	}
}

func TestMigrateRealMigrations(t *testing.T) {
	db := openRawDB(t)

	require.NoError(t, migrate(context.Background(), db, migrations.FS))

	assert.Equal(t, 2, userVersion(t, db))
	for _, table := range []string{"qso", "qso_adif_field", "qso_confirmation", "tag", "qso_tag"} {
		assert.True(t, schemaObjectExists(t, db, "table", table), "table %s", table)
	}
	for _, index := range []string{
		"qso_by_start", "qso_by_call", "qso_by_dxcc", "qso_by_band", "qso_by_mode",
		"qso_by_cont", "qso_by_itu_region", "qso_by_distance", "qso_worked_before",
		"qso_confirmation_by_rcvd", "qso_tag_by_tag",
	} {
		assert.True(t, schemaObjectExists(t, db, "index", index), "index %s", index)
	}
}

func TestMigrateAddsOperatorColumn(t *testing.T) {
	db := openRawDB(t)
	ctx := t.Context()
	body, err := fs.ReadFile(migrations.FS, "0001_init.sql")
	require.NoError(t, err)
	require.NoError(t, migrate(ctx, db, fstest.MapFS{"0001_init.sql": {Data: body}}))
	_, err = db.ExecContext(ctx,
		"INSERT INTO qso (uuid, start_utc, call, created_at, updated_at) VALUES ('u', 't', 'W1AW', 't', 't')")
	require.NoError(t, err)

	require.NoError(t, migrate(ctx, db, migrations.FS))

	var operator string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT operator FROM qso").Scan(&operator))
	assert.Empty(t, operator, "existing rows default to no operator")
}
