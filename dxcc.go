package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"jstlogger/internal/config"
	"jstlogger/internal/dxcc"
)

// dxccArchiveURL is the address of the Big CTY country file archive. Tests
// replace it.
var dxccArchiveURL = dxcc.DefaultURL

// dxccDownloadTimeout bounds one download of the Big CTY archive, which is
// a few hundred kilobytes.
const dxccDownloadTimeout = time.Minute

// newDXCCSyncer returns a syncer downloading the Big CTY country file into
// store.
func newDXCCSyncer(store *dxcc.Store) *dxcc.Syncer {
	return &dxcc.Syncer{
		Store:  store,
		Client: &http.Client{Timeout: dxccDownloadTimeout},
		URL:    dxccArchiveURL,
	}
}

// loadDXCCTable returns the country file stored in the logbook, or the
// built-in snapshot until the first sync. A stored copy that can't be read
// is logged to logger and the snapshot is used instead, so the logger still
// starts.
func loadDXCCTable(ctx context.Context, store *dxcc.Store, logger *slog.Logger) *dxcc.Table {
	table, err := store.Table(ctx)
	if err != nil {
		logger.Error("dxcc load", "err", err)
		return dxcc.Default()
	}
	return table
}

// runDXCCSync opens the session's logbook, downloads the Big CTY country
// file into it whether or not the stored copy is fresh, prints a summary to
// stdout, and closes the logbook. This is the --sync-dxcc mode.
func runDXCCSync(ctx context.Context, session config.Session, stdout io.Writer) error {
	store, err := openLogbook(ctx, session)
	if err != nil {
		return err
	}
	res, err := newDXCCSyncer(dxcc.NewStore(store.DB())).Sync(ctx)
	if err != nil {
		err = fmt.Errorf("syncing DXCC: %w", err)
	} else if _, werr := fmt.Fprintln(stdout, dxccSummary(res)); werr != nil {
		err = fmt.Errorf("syncing DXCC: %w", werr)
	}
	if cerr := store.Close(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("closing logbook: %w", cerr))
	}
	return err
}

// dxccSummary describes a completed sync in one line.
func dxccSummary(res dxcc.Result) string {
	if res.NotModified {
		return "DXCC: country file unchanged"
	}
	return fmt.Sprintf("DXCC: %d %s, %d exception %s", res.Prefixes, plural(res.Prefixes, "prefix", "prefixes"),
		res.Calls, plural(res.Calls, "call", "calls"))
}

// syncDXCCInBackground syncs the Big CTY country file when the stored copy
// is older than dxcc.MaxAge and logs the outcome to logger. Nothing is sent
// to the UI: it keeps the table loaded at startup, and a newly synced copy
// is used from the next launch.
func syncDXCCInBackground(ctx context.Context, s *dxcc.Syncer, logger *slog.Logger) {
	res, synced, err := s.SyncIfStale(ctx, dxcc.MaxAge)
	switch {
	case err != nil && ctx.Err() != nil && errors.Is(err, context.Canceled):
		logger.Info("dxcc sync canceled")
	case err != nil:
		logger.Error("dxcc sync", "err", err)
	case !synced:
		logger.Debug("dxcc sync not needed")
	default:
		logger.Info("dxcc sync", "prefixes", res.Prefixes, "calls", res.Calls, "not_modified", res.NotModified)
	}
}
