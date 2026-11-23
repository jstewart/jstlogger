package main

import (
	"context"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/config"
	"jstlogger/internal/listener"
	"jstlogger/internal/logbook"
	"jstlogger/internal/tui"
)

// runListener runs the listener configured as [listeners.<name>] until ctx
// is canceled. Each QSO it receives is imported into store with the
// listener's tags, as it was sent, and the UI is sent tui.LogbookChanged
// when the QSO is new. Problems are logged to logger and never stop the
// app: a listener that can't start returns at once, and one that receives
// a bad packet or fails to store a QSO keeps running.
func runListener(ctx context.Context, name string, cfg config.Listener, store logbook.Store, logger *slog.Logger, send func(tea.Msg)) {
	logger = logger.With("listener", name)
	dec, err := listener.NewDecoder(cfg.Protocol)
	if err != nil {
		logger.Error("listener not started", "err", err)
		return
	}
	l, err := listener.New(listener.Options{Address: cfg.Address, Interface: cfg.Interface, Decoder: dec})
	if err != nil {
		logger.Error("listener not started", "err", err)
		return
	}
	logger.Info("listener starting", "protocol", cfg.Protocol, "address", cfg.Address, "interface", cfg.Interface)
	err = l.Run(ctx, func(event any) {
		handleListenerEvent(ctx, logger, store, cfg.Tags, send, event)
	})
	if err != nil {
		logger.Error("listener stopped", "err", err)
		return
	}
	logger.Info("listener stopped")
}

// handleListenerEvent stores a QSO a listener received, with tags, and
// tells the UI when it was added, or logs a packet the listener dropped.
// A QSO already in the logbook is skipped as a duplicate.
func handleListenerEvent(ctx context.Context, logger *slog.Logger, store logbook.Store, tags []string, send func(tea.Msg), event any) {
	switch ev := event.(type) {
	case listener.QSOReceived:
		q := ev.QSO
		attrs := []any{"source", ev.Source, "call", q.Call, "start", q.StartUTC, "band", q.Band, "mode", q.Mode}
		res, err := store.Import(ctx, []logbook.QSO{q}, tags)
		switch {
		case err != nil:
			logger.Error("listener qso not stored", append(attrs, "err", err)...)
		case res.Added == 0:
			logger.Info("listener qso skipped as a duplicate", attrs...)
		default:
			logger.Info("listener qso stored", attrs...)
			send(tui.LogbookChanged{})
		}
	case listener.PacketDropped:
		logger.Warn("listener packet dropped", "from", ev.From, "err", ev.Err)
	default:
		logger.Debug("listener event", "event", ev)
	}
}
