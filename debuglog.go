package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"jstlogger/internal/config"
	"jstlogger/internal/rig"
)

// openDebugLog opens the --debug log for appending, creating it and its
// directory if needed, and returns a logger writing to it. The caller must
// call closeLog when done.
func openDebugLog(getenv func(string) string) (logger *slog.Logger, closeLog func() error, err error) {
	path, err := config.DebugLogPath(getenv)
	if err != nil {
		return nil, nil, fmt.Errorf("opening debug log: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("opening debug log: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("opening debug log: %w", err)
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(h), f.Close, nil
}

// logEvent writes one rig event to logger.
func logEvent(logger *slog.Logger, event any) {
	switch ev := event.(type) {
	case rig.StateChanged:
		logger.Info("rig state", "freq", ev.State.Freq, "mode", ev.State.Mode, "passband", ev.State.Passband)
	case rig.ConnectionChanged:
		if ev.Status == rig.Disconnected {
			logger.Warn("rig connection", "status", ev.Status, "err", ev.Err, "retry_in", ev.RetryIn)
			return
		}
		logger.Info("rig connection", "status", ev.Status)
	case rig.CommandResult:
		if ev.Err != nil {
			logger.Error("rig command", "command", ev.Command, "err", ev.Err)
			return
		}
		logger.Info("rig command", "command", ev.Command)
	default:
		logger.Debug("rig event", "event", ev)
	}
}
