// Package highlight decides how a program such as WSJT-X should highlight
// the calls it decodes: by whether the logbook has QSOs with the call, and
// whether any are on the band and mode the program is on. It caches what
// it looks up, and tracks what it told each program, so a busy band costs
// neither a logbook query nor a packet for every decode. The package knows
// nothing about the network or the protocol the program speaks.
package highlight

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"net/netip"
	"strings"
	"time"

	"jstlogger/internal/callsign"
	"jstlogger/internal/logbook"
)

// Status is how a call stands in the logbook.
type Status int

const (
	// StatusNew means the logbook has no QSO with the call.
	StatusNew Status = iota
	// StatusWorked means the logbook has QSOs with the call, but none on the
	// band and mode the program is on, or that band or mode is unknown.
	StatusWorked
	// StatusWorkedBandMode means the logbook has a QSO with the call on the band
	// and mode the program is on.
	StatusWorkedBandMode
)

// String returns the status's name, as the config spells it.
func (s Status) String() string {
	switch s {
	case StatusNew:
		return "new"
	case StatusWorked:
		return "worked"
	case StatusWorkedBandMode:
		return "worked_band_mode"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// Colors is how a call is highlighted. A nil color leaves the program's
// own, so the zero Colors is no highlight.
type Colors struct {
	Background, Foreground color.Color
}

// IsZero reports whether c highlights nothing.
func (c Colors) IsZero() bool { return c.Background == nil && c.Foreground == nil }

// Scheme gives the colors of each status. A status it lacks is not
// highlighted.
type Scheme map[Status]Colors

// Counter counts the QSOs matching an expression; logbook.Store is one.
type Counter interface {
	Count(ctx context.Context, e logbook.Expr) (int, error)
}

// Client identifies a program that sends decodes: its client ID and the
// address it sends from.
type Client struct {
	ID   string
	Addr netip.AddrPort
}

// Highlight asks a program to highlight Call with Colors; the zero Colors
// clears its highlight.
type Highlight struct {
	Call   string
	Colors Colors
}

// lookup is what a status is looked up for: the base callsign, the band
// and the mode, "" when unknown.
type lookup struct {
	call, band, mode string
}

// cached is a looked-up status and when it expires.
type cached struct {
	status  Status
	expires time.Time
}

// sent is the highlight last sent to a client for a call: the status it
// was for, its colors, and when to send it again.
type sent struct {
	status  Status
	colors  Colors
	expires time.Time
}

// client is what the Highlighter knows of a program.
type client struct {
	// band and mode are what the program is on, from its last status;
	// "" until it sends one.
	band, mode string
	// sent is the highlight last sent for each call, as it was decoded.
	sent map[string]sent
}

// maxCached is the number of cached statuses past which expired ones are
// dropped.
const maxCached = 4096

// Highlighter decides which highlights to send to the programs that send
// it decodes. It is not safe for concurrent use.
type Highlighter struct {
	store   Counter
	scheme  Scheme
	ttl     time.Duration
	now     func() time.Time
	cache   map[lookup]cached
	clients map[Client]*client
}

// New returns a Highlighter that looks calls up in store and highlights
// them with scheme. A status it looks up is kept for ttl, and a highlight
// it sent is sent again after ttl, in case the program restarted and lost
// it.
func New(store Counter, scheme Scheme, ttl time.Duration) *Highlighter {
	return &Highlighter{
		store:   store,
		scheme:  scheme,
		ttl:     ttl,
		now:     time.Now,
		cache:   make(map[lookup]cached),
		clients: make(map[Client]*client),
	}
}

// Status records that c is on the dial frequency dialHz in mode. When the
// band or mode changes, every highlight c was sent is sent again on its
// next decode, as its status may differ there.
func (h *Highlighter) Status(c Client, dialHz uint64, mode string) {
	cl := h.client(c)
	var band string
	if hz, ok := toInt64(dialHz); ok {
		band, _ = logbook.BandForFreq(hz)
	}
	band = strings.ToLower(band)
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if band == cl.band && mode == cl.mode {
		return
	}
	cl.band, cl.mode = band, mode
	for call, s := range cl.sent {
		s.expires = time.Time{}
		cl.sent[call] = s
	}
}

// Decode returns the highlight to send to c for a message it decoded, and
// whether there is one to send. There is none when the message has no
// caller, when the caller's highlight is what was last sent to c, or when
// the caller has no colors and was sent none. An error looking the caller
// up is returned, wrapped, with no highlight.
func (h *Highlighter) Decode(ctx context.Context, c Client, message string) (Highlight, bool, error) {
	call := Caller(message)
	if call == "" {
		return Highlight{}, false, nil
	}
	cl := h.client(c)
	status, err := h.statusOf(ctx, call, cl.band, cl.mode)
	if err != nil {
		return Highlight{}, false, err
	}
	now := h.now()
	colors := h.scheme[status]
	prev, ok := cl.sent[call]
	switch {
	case ok && prev.status == status && now.Before(prev.expires):
		return Highlight{}, false, nil
	case colors.IsZero() && (!ok || prev.colors.IsZero()):
		cl.sent[call] = sent{status: status, expires: now.Add(h.ttl)}
		return Highlight{}, false, nil
	}
	cl.sent[call] = sent{status: status, colors: colors, expires: now.Add(h.ttl)}
	return Highlight{Call: call, Colors: colors}, true, nil
}

// Logged forgets what is known of call, which was just logged, so its
// next decode is looked up again and its highlight updated.
func (h *Highlighter) Logged(call string) {
	base := callsign.Base(call)
	for k := range h.cache {
		if k.call == base {
			delete(h.cache, k)
		}
	}
	for _, cl := range h.clients {
		for c, s := range cl.sent {
			if callsign.Base(c) == base {
				s.expires = time.Time{}
				cl.sent[c] = s
			}
		}
	}
}

// client returns what is known of c, adding it if new.
func (h *Highlighter) client(c Client) *client {
	cl, ok := h.clients[c]
	if !ok {
		cl = &client{sent: make(map[string]sent)}
		h.clients[c] = cl
	}
	return cl
}

// statusOf returns the status of the base callsign of call on band and in
// mode, from the cache when it has an unexpired one. A failed lookup is
// not cached.
func (h *Highlighter) statusOf(ctx context.Context, call, band, mode string) (Status, error) {
	k := lookup{call: callsign.Base(call), band: band, mode: mode}
	now := h.now()
	if c, ok := h.cache[k]; ok && now.Before(c.expires) {
		return c.status, nil
	}
	status, err := h.look(ctx, k)
	if err != nil {
		return StatusNew, fmt.Errorf("highlight: look up %s: %w", k.call, err)
	}
	if len(h.cache) >= maxCached {
		for k, c := range h.cache {
			if !now.Before(c.expires) {
				delete(h.cache, k)
			}
		}
	}
	h.cache[k] = cached{status: status, expires: now.Add(h.ttl)}
	return status, nil
}

// look looks k up in the logbook.
func (h *Highlighter) look(ctx context.Context, k lookup) (Status, error) {
	call := logbook.CallBaseIs{Call: k.call}
	n, err := h.store.Count(ctx, call)
	if err != nil {
		return StatusNew, err
	}
	if n == 0 {
		return StatusNew, nil
	}
	if k.band == "" || k.mode == "" {
		return StatusWorked, nil
	}
	n, err = h.store.Count(ctx, logbook.And{call, logbook.Filter{Bands: []string{k.band}}, logbook.ModeIs{Mode: k.mode}})
	if err != nil {
		return StatusNew, err
	}
	if n == 0 {
		return StatusWorked, nil
	}
	return StatusWorkedBandMode, nil
}

// toInt64 converts v to an int64, reporting false when it does not fit.
func toInt64(v uint64) (int64, bool) {
	if v > math.MaxInt64 {
		return 0, false
	}
	return int64(v), true
}

// Caller returns the call of the station that sent message, a decoded
// FT8 or FT4 style message, upper-cased and without the angle brackets of
// a hashed call: the call after CQ, QRZ or DE (and after a modifier such
// as DX, POTA or 290), or else the second word, as in "W9XYZ K1ABC -10".
// It returns "" when that word is not a callsign, as in free text or a
// hashed call the program could not resolve ("<...>").
func Caller(message string) string {
	words := strings.Fields(strings.ToUpper(message))
	if len(words) < 2 {
		return ""
	}
	cand := words[1]
	switch words[0] {
	case "CQ", "QRZ", "DE":
		if len(words) > 2 && !isCall(unbracket(words[1])) {
			cand = words[2]
		}
	}
	cand = unbracket(cand)
	if !isCall(cand) {
		return ""
	}
	return cand
}

// unbracket strips the angle brackets around a hashed call.
func unbracket(word string) string {
	return strings.TrimSuffix(strings.TrimPrefix(word, "<"), ">")
}

// isCall reports whether word looks like a callsign: its base callsign is
// 3 to 10 letters and digits with at least one of each.
func isCall(word string) bool {
	base := callsign.Base(word)
	if len(base) < 3 || len(base) > 10 {
		return false
	}
	var letter, digit bool
	for _, r := range base {
		switch {
		case r >= 'A' && r <= 'Z':
			letter = true
		case r >= '0' && r <= '9':
			digit = true
		default:
			return false
		}
	}
	return letter && digit
}
