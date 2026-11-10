package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/config"
	"jstlogger/internal/lotw"
	"jstlogger/internal/tui"
)

// lotwFeedURL is the address of the LoTW activity feed. Tests replace it.
var lotwFeedURL = lotw.DefaultURL

// lotwDownloadTimeout bounds one download of the LoTW activity feed, which
// is several megabytes.
const lotwDownloadTimeout = 2 * time.Minute

// newLoTWSyncer returns a syncer downloading the LoTW activity feed into
// store.
func newLoTWSyncer(store *lotw.Store) *lotw.Syncer {
	return &lotw.Syncer{
		Store:  store,
		Client: &http.Client{Timeout: lotwDownloadTimeout},
		URL:    lotwFeedURL,
	}
}

// runLoTWSync opens the session's logbook, downloads the LoTW activity feed
// into it whether or not the stored copy is fresh, prints a summary to
// stdout, and closes the logbook. This is the --sync-lotw mode.
func runLoTWSync(ctx context.Context, session config.Session, stdout io.Writer) error {
	store, err := openLogbook(ctx, session)
	if err != nil {
		return err
	}
	res, err := newLoTWSyncer(lotw.NewStore(store.DB())).Sync(ctx)
	if err != nil {
		err = fmt.Errorf("syncing LoTW: %w", err)
	} else if _, werr := fmt.Fprintln(stdout, lotwSummary(res)); werr != nil {
		err = fmt.Errorf("syncing LoTW: %w", werr)
	}
	if cerr := store.Close(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("closing logbook: %w", cerr))
	}
	return err
}

// lotwSummary describes a completed sync in one line.
func lotwSummary(res lotw.Result) string {
	if res.NotModified {
		return "LoTW: feed unchanged"
	}
	s := fmt.Sprintf("LoTW: %d %s", res.Users, plural(res.Users, "user", "users"))
	if res.Skipped > 0 {
		s += fmt.Sprintf(" (%d %s skipped)", res.Skipped, plural(res.Skipped, "row", "rows"))
	}
	return s
}

// plural returns one when n is 1 and many otherwise.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// syncLoTWInBackground syncs the LoTW activity feed when the stored copy is
// older than lotw.MaxAge, logs the outcome to logger, and sends it to the UI
// with send. Nothing is sent when no sync was needed, or when ctx was
// canceled because the UI exited.
func syncLoTWInBackground(ctx context.Context, s *lotw.Syncer, logger *slog.Logger, send func(tea.Msg)) {
	res, synced, err := s.SyncIfStale(ctx, lotw.MaxAge)
	switch {
	case err != nil && ctx.Err() != nil && errors.Is(err, context.Canceled):
		logger.Info("lotw sync canceled")
		return
	case err != nil:
		logger.Error("lotw sync", "err", err)
	case !synced:
		logger.Debug("lotw sync not needed")
		return
	default:
		logger.Info("lotw sync", "users", res.Users, "skipped", res.Skipped, "not_modified", res.NotModified)
	}
	send(tui.LoTWSynced{Err: err})
}
