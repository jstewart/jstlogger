package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/config"
	"jstlogger/internal/logbook"
)

func TestOpenLogbookCreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "logbook.db")
	session := config.Session{Config: config.Config{Logbook: config.Logbook{Path: path}}}

	store, err := openLogbook(context.Background(), session)
	require.NoError(t, err)
	require.NoError(t, store.Close())

	assert.FileExists(t, path)
}

func TestOpenLogbookUsesStationGrid(t *testing.T) {
	session := config.Session{
		Station: config.Station{Gridsquare: "FN31pr"},
		Config:  config.Config{Logbook: config.Logbook{Path: filepath.Join(t.TempDir(), "logbook.db")}},
	}
	store, err := openLogbook(context.Background(), session)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	q, err := store.Add(context.Background(), logbook.QSO{
		StartUTC:   time.Date(2026, 10, 28, 14, 42, 0, 0, time.UTC),
		Call:       "G4ABC",
		Gridsquare: "JO01",
	})
	require.NoError(t, err)

	require.True(t, q.DistanceKm.Valid, "distance should be computed from the station grid")
	assert.InDelta(t, 5489, q.DistanceKm.V, 50)
	assert.Empty(t, q.MyGridsquare, "the station grid must not be stored on the QSO")
}

func TestOpenLogbookError(t *testing.T) {
	dir := t.TempDir()
	// A file where the directory should be makes the logbook unopenable.
	blocker := filepath.Join(dir, "jstlogger")
	require.NoError(t, os.WriteFile(blocker, nil, 0o644))
	session := config.Session{Config: config.Config{Logbook: config.Logbook{Path: filepath.Join(blocker, "logbook.db")}}}

	_, err := openLogbook(context.Background(), session)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening logbook")
}
