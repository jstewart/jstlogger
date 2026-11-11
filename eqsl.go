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
	"jstlogger/internal/eqsl"
	"jstlogger/internal/tui"
)

// eqslListURL is the address of the eQSL AG member list. Tests replace it.
var eqslListURL = eqsl.DefaultURL

// eqslDownloadTimeout bounds one download of the eQSL AG member list, which
// is a few megabytes.
const eqslDownloadTimeout = 2 * time.Minute

// newEQSLSyncer returns a syncer downloading the eQSL AG member list into
// store.
func newEQSLSyncer(store *eqsl.Store) *eqsl.Syncer {
	return &eqsl.Syncer{
		Store:  store,
		Client: &http.Client{Timeout: eqslDownloadTimeout},
		URL:    eqslListURL,
	}
}

// runEQSLSync opens the session's logbook, downloads the eQSL AG member list
// into it whether or not the stored copy is fresh, prints a summary to
// stdout, and closes the logbook. This is the --sync-eqsl mode.
func runEQSLSync(ctx context.Context, session config.Session, stdout io.Writer) error {
	store, err := openLogbook(ctx, session)
	if err != nil {
		return err
	}
	res, err := newEQSLSyncer(eqsl.NewStore(store.DB())).Sync(ctx)
	if err != nil {
		err = fmt.Errorf("syncing eQSL: %w", err)
	} else if _, werr := fmt.Fprintln(stdout, eqslSummary(res)); werr != nil {
		err = fmt.Errorf("syncing eQSL: %w", werr)
	}
	if cerr := store.Close(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("closing logbook: %w", cerr))
	}
	return err
}

// eqslSummary describes a completed sync in one line.
func eqslSummary(res eqsl.Result) string {
	if res.NotModified {
		return "eQSL: AG list unchanged"
	}
	s := fmt.Sprintf("eQSL: %d AG %s", res.Members, plural(res.Members, "member", "members"))
	if res.Skipped > 0 {
		s += fmt.Sprintf(" (%d %s skipped)", res.Skipped, plural(res.Skipped, "row", "rows"))
	}
	return s
}

// syncEQSLInBackground syncs the eQSL AG member list when the stored copy is
// older than eqsl.MaxAge, logs the outcome to logger, and sends it to the UI
// with send. Nothing is sent when no sync was needed, or when ctx was
// canceled because the UI exited.
func syncEQSLInBackground(ctx context.Context, s *eqsl.Syncer, logger *slog.Logger, send func(tea.Msg)) {
	res, synced, err := s.SyncIfStale(ctx, eqsl.MaxAge)
	switch {
	case err != nil && ctx.Err() != nil && errors.Is(err, context.Canceled):
		logger.Info("eqsl sync canceled")
		return
	case err != nil:
		logger.Error("eqsl sync", "err", err)
	case !synced:
		logger.Debug("eqsl sync not needed")
		return
	default:
		logger.Info("eqsl sync", "members", res.Members, "skipped", res.Skipped, "not_modified", res.NotModified)
	}
	send(tui.EQSLSynced{Err: err})
}
