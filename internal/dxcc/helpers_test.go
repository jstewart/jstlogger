package dxcc

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// newTestDB opens a migrated logbook database under t.TempDir() and closes
// it when the test ends.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	lb, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "log.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, lb.Close()) })
	return lb.DB()
}

// newTestStore returns a Store on a fresh migrated database.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	return NewStore(newTestDB(t))
}

// fixtureCSV returns the hand-made testdata/cty.csv.
func fixtureCSV(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/cty.csv")
	require.NoError(t, err)
	return b
}

// zipOf returns a zip archive holding files, keyed by name.
func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// utc builds a UTC time.
func utc(year int, month time.Month, day, hour, minute, sec int) time.Time {
	return time.Date(year, month, day, hour, minute, sec, 0, time.UTC)
}
