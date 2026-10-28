package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"jstlogger/internal/config"
	"jstlogger/internal/logbook"
)

// openLogbook opens the logbook at the session's logbook path, creating its
// directory if needed and applying any pending migrations. The station's
// grid is used for distances when a QSO has no MY_GRIDSQUARE. The caller
// must close the store.
func openLogbook(ctx context.Context, session config.Session) (*logbook.SQLiteStore, error) {
	path := session.Config.Logbook.Path
	// The log is personal data, so keep the directory private.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("opening logbook: %w", err)
	}
	store, err := logbook.Open(ctx, path, logbook.Options{StationGrid: session.Station.Gridsquare})
	if err != nil {
		return nil, fmt.Errorf("opening logbook: %w", err)
	}
	return store, nil
}
