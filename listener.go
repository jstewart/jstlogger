package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	tea "charm.land/bubbletea/v2"

	"jstlogger/internal/callinfo"
	"jstlogger/internal/config"
	"jstlogger/internal/highlight"
	"jstlogger/internal/listener"
	"jstlogger/internal/logbook"
	"jstlogger/internal/tui"
)

const (
	// listenerImportTimeout bounds storing one QSO a listener received.
	listenerImportTimeout = 10 * time.Second
	// highlightTTL is how long a decoded call's status is cached, and how
	// often a highlight is sent again in case WSJT-X restarted.
	highlightTTL = 2 * time.Minute
	// highlightTimeout bounds looking up a decoded call and sending its
	// highlight.
	highlightTimeout = 2 * time.Second
)

// runListener runs the listener configured as [listeners.<name>] until ctx
// is canceled. Each QSO it receives has the fields it leaves empty filled
// in by info, such as its DXCC entity and the operator's name, and is
// imported into store with the listener's tags, and the UI is sent
// tui.LogbookChanged when the QSO is new. When cfg.Highlight is set, the
// calls the sender decodes are highlighted in it by how they stand in
// store. Problems are logged to logger and never stop the app: a listener
// that can't start returns at once, one with bad highlight colors runs
// without highlighting, and one that receives a bad packet, fails to store
// a QSO or fails to highlight a call keeps running.
func runListener(ctx context.Context, name string, cfg config.Listener, store logbook.Store, info callinfo.Looker, logger *slog.Logger, send func(tea.Msg)) {
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
	var hl *highlight.Highlighter
	if cfg.Highlight {
		scheme, err := highlightScheme(cfg.Colors)
		if err != nil {
			logger.Error("listener highlighting off", "err", err)
		} else {
			hl = highlight.New(store, scheme, highlightTTL)
		}
	}
	logger.Info("listener starting", "protocol", cfg.Protocol, "address", cfg.Address, "interface", cfg.Interface, "highlight", hl != nil)
	// Run calls this from its one goroutine, which is what keeps hl, which
	// is not safe for concurrent use, safe.
	err = l.Run(ctx, func(event any) {
		handleListenerEvent(ctx, logger, store, info, cfg.Tags, send, event)
		if hl != nil {
			highlightListenerEvent(ctx, logger, hl, l.Send, event)
		}
	})
	if err != nil {
		logger.Error("listener stopped", "err", err)
		return
	}
	logger.Info("listener stopped")
}

// handleListenerEvent stores a QSO a listener received, with tags and the
// fields it leaves empty filled in by info, and tells the UI when it was
// added, or logs a packet the listener dropped. Each field the listener
// left out of the QSO, and a lookup that failed, is logged as a warning;
// the QSO is stored with what was found. A QSO already in the logbook is
// skipped as a duplicate.
func handleListenerEvent(ctx context.Context, logger *slog.Logger, store logbook.Store, info callinfo.Looker, tags []string, send func(tea.Msg), event any) {
	switch ev := event.(type) {
	case listener.QSOReceived:
		q := ev.QSO
		attrs := []any{"source", ev.Source, "call", q.Call, "start", q.StartUTC, "band", q.Band, "mode", q.Mode}
		for _, err := range ev.Dropped {
			logger.Warn("listener qso field dropped", append(attrs, "err", err)...)
		}
		// The lookups stop as the app quits; the QSO is stored with what
		// they found by then.
		q, err := info.Fill(ctx, q)
		if err != nil {
			logger.Warn("listener qso lookup failed", append(attrs, "err", err)...)
		}
		// The sender already counts the QSO as logged, so a QSO that
		// arrives as the app quits is still stored.
		importCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), listenerImportTimeout)
		res, err := store.Import(importCtx, []logbook.QSO{q}, tags)
		cancel()
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
	case listener.StatusReceived, listener.DecodeReceived:
		// Handled by highlightListenerEvent when highlighting, and too
		// frequent to log.
	default:
		logger.Debug("listener event", "event", ev)
	}
}

// highlightListenerEvent feeds an event a highlighting listener received to
// hl: a sender's status sets the band and mode its decodes are looked up
// on, a decode is answered with the highlight hl picks for its caller,
// sent back to the sender with reply, and a received QSO, which
// handleListenerEvent has stored, has its call looked up afresh. A failed
// lookup or send is logged as a warning. Decodes that arrive as the app
// quits are dropped.
func highlightListenerEvent(ctx context.Context, logger *slog.Logger, hl *highlight.Highlighter, reply func(context.Context, netip.AddrPort, []byte) error, event any) {
	switch ev := event.(type) {
	case listener.StatusReceived:
		hl.Status(highlight.Client{ID: ev.Source, Addr: ev.From}, ev.DialHz, ev.Mode)
	case listener.DecodeReceived:
		if ctx.Err() != nil {
			return // the app is quitting
		}
		ctx, cancel := context.WithTimeout(ctx, highlightTimeout)
		defer cancel()
		h, ok, err := hl.Decode(ctx, highlight.Client{ID: ev.Source, Addr: ev.From}, ev.Message)
		if err != nil {
			logger.Warn("listener highlight lookup failed", "source", ev.Source, "message", ev.Message, "err", err)
			return
		}
		if !ok {
			return
		}
		packet := listener.WSJTXHighlightCallsign(ev.Source, h.Call, h.Colors.Background, h.Colors.Foreground, false)
		if err := reply(ctx, ev.From, packet); err != nil {
			logger.Warn("listener highlight not sent", "source", ev.Source, "call", h.Call, "err", err)
			return
		}
		logger.Debug("listener highlight sent", "source", ev.Source, "call", h.Call)
	case listener.QSOReceived:
		hl.Logged(ev.QSO.Call)
	}
}

// highlightScheme returns the highlight colors c configures.
func highlightScheme(c config.HighlightColors) (highlight.Scheme, error) {
	scheme := make(highlight.Scheme, 3)
	for _, s := range []struct {
		status highlight.Status
		colors config.HighlightColor
	}{
		{highlight.StatusNew, c.New},
		{highlight.StatusWorked, c.Worked},
		{highlight.StatusWorkedBandMode, c.WorkedBandMode},
	} {
		bg, err := config.ParseColor(s.colors.Background)
		if err != nil {
			return nil, fmt.Errorf("%s background: %w", s.status, err)
		}
		fg, err := config.ParseColor(s.colors.Foreground)
		if err != nil {
			return nil, fmt.Errorf("%s foreground: %w", s.status, err)
		}
		scheme[s.status] = highlight.Colors{Background: bg, Foreground: fg}
	}
	return scheme, nil
}
