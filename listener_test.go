package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image/color"
	"log/slog"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"jstlogger/internal/callbook"
	cbmock "jstlogger/internal/callbook/mock"
	"jstlogger/internal/callinfo"
	"jstlogger/internal/config"
	"jstlogger/internal/dxcc"
	"jstlogger/internal/highlight"
	"jstlogger/internal/listener"
	"jstlogger/internal/logbook"
	"jstlogger/internal/tui"
)

// testWSJTXADIF is the ADIF WSJT-X sends in a LoggedADIF message.
const testWSJTXADIF = "\n<adif_ver:5>3.1.0\n<programid:6>WSJT-X\n<EOH>\n" +
	"<call:4>K1AB <gridsquare:4>FN42 <mode:3>FT8 <rst_sent:3>-10 <rst_rcvd:3>-12 " +
	"<qso_date:8>20261123 <time_on:6>130102 <qso_date_off:8>20261123 <time_off:6>130215 " +
	"<band:3>20m <freq:9>14.075123 <station_callsign:5>W1XYZ <my_gridsquare:6>FN42ab <EOR>"

// wsjtxLoggedADIF returns a WSJT-X LoggedADIF (type 12) packet.
func wsjtxLoggedADIF(id, adif string) []byte {
	var b []byte
	b = binary.BigEndian.AppendUint32(b, 0xADBCCBDA)
	b = binary.BigEndian.AppendUint32(b, 3)
	b = binary.BigEndian.AppendUint32(b, 12)
	for _, s := range []string{id, adif} {
		b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
		b = append(b, s...)
	}
	return b
}

// freeUDPAddr returns a loopback address with a UDP port that was free a
// moment ago.
func freeUDPAddr(t *testing.T) string {
	t.Helper()
	probe, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	addr := probe.LocalAddr().String()
	require.NoError(t, probe.Close())
	return addr
}

// lockedBuffer is a bytes.Buffer that the listener's goroutine can write
// to while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// openTestLogbook opens an empty logbook in a temp dir.
func openTestLogbook(t *testing.T) *logbook.SQLiteStore {
	t.Helper()
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	return store
}

// startListener runs runListener in the background until the test ends.
func startListener(t *testing.T, name string, cfg config.Listener, store logbook.Store, info callinfo.Looker, logger *slog.Logger, send func(tea.Msg)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runListener(ctx, name, cfg, store, info, logger, send)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("runListener did not stop when ctx was canceled")
		}
	})
}

// sendUDP sends b to addr from loopback.
func sendUDP(t *testing.T, addr string, b []byte) {
	t.Helper()
	conn, err := net.Dial("udp", addr)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Write(b)
	require.NoError(t, err)
}

func TestRunListenerLogsQSOsOnce(t *testing.T) {
	store := openTestLogbook(t)
	cb := cbmock.NewMockLooker(gomock.NewController(t))
	// Once for each time WSJT-X sends the QSO.
	cb.EXPECT().Lookup(gomock.Any(), "K1AB").Return(callbook.Result{Name: "Alice", QTH: "Boston", Service: callbook.ServiceQRZ}, nil).MinTimes(1)
	info := callinfo.Looker{DXCC: dxcc.Default(), Callbook: cb, Logbook: store}
	addr := freeUDPAddr(t)
	send, sent := recordSends()
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	startListener(t, "wsjtx", config.Listener{Protocol: "wsjtx", Address: addr, Tags: []string{"wsjtx", "ft8"}}, store, info, logger, send)

	// The listener binds in the background, so send until the QSO lands.
	packet := wsjtxLoggedADIF("WSJT-X", testWSJTXADIF)
	require.Eventually(t, func() bool {
		sendUDP(t, addr, packet)
		return len(sent()) > 0
	}, 3*time.Second, 20*time.Millisecond)

	// WSJT-X sending the QSO again doesn't log it twice.
	sendUDP(t, addr, packet)
	sendUDP(t, addr, wsjtxLoggedADIF("WSJT-X", "garbage <call:2>K1"))
	require.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "dropped")
	}, 3*time.Second, 20*time.Millisecond, "the bad packet is logged")

	n, err := store.Count(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "one QSO is stored")
	assert.Equal(t, []tea.Msg{tui.LogbookChanged{}}, sent(), "the UI hears only about the added QSO")

	qsos, err := store.Index(t.Context(), nil, logbook.Sort{}, 0, 10)
	require.NoError(t, err)
	require.Len(t, qsos, 1)
	q, err := store.Get(t.Context(), qsos[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "K1AB", q.Call)
	assert.Equal(t, "FT8", q.Mode)
	assert.ElementsMatch(t, []string{"wsjtx", "ft8"}, q.Tags)
	assert.Equal(t, 291, q.DXCC.V, "the DXCC entity is filled in")
	assert.Equal(t, "NA", q.Cont)
	assert.Equal(t, "Alice", q.Name, "the callbook's name is filled in")
	assert.Equal(t, "Boston", q.QTH)
	assert.Equal(t, "FN42", q.Gridsquare, "WSJT-X's grid is kept")
	assert.Contains(t, logs.String(), "K1AB")
	assert.Contains(t, logs.String(), "listener=wsjtx")
}

func TestRunListenerLogsSetupErrors(t *testing.T) {
	busy, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = busy.Close() })

	tests := []struct {
		name string
		cfg  config.Listener
		want string
	}{
		{"unknown protocol", config.Listener{Protocol: "n1mm", Address: "127.0.0.1:2237"}, "unknown protocol"},
		{"bad address", config.Listener{Protocol: "wsjtx", Address: "localhost"}, "address"},
		{"port in use", config.Listener{Protocol: "wsjtx", Address: busy.LocalAddr().String()}, "listen on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs lockedBuffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			send, sent := recordSends()

			// runListener returns on its own, without ctx being canceled.
			runListener(t.Context(), "wsjtx", tt.cfg, openTestLogbook(t), callinfo.Looker{}, logger, send)

			assert.Contains(t, logs.String(), "level=ERROR")
			assert.Contains(t, logs.String(), tt.want)
			assert.Empty(t, sent())
		})
	}
}

func TestHandleListenerEventStoresAfterCancel(t *testing.T) {
	store := openTestLogbook(t)
	send, sent := recordSends()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	q := logbook.QSO{Call: "K1AB", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), Band: "20m", Mode: "FT8"}
	handleListenerEvent(ctx, slog.New(slog.DiscardHandler), store, callinfo.Looker{}, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q})

	n, err := store.Count(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "a QSO that arrives as the app quits is stored")
	assert.Equal(t, []tea.Msg{tui.LogbookChanged{}}, sent())
}

func TestHandleListenerEventImportError(t *testing.T) {
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	require.NoError(t, store.Close(), "imports into a closed logbook fail")
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	send, sent := recordSends()

	q := logbook.QSO{Call: "K1AB", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), Band: "20m", Mode: "FT8"}
	handleListenerEvent(t.Context(), logger, store, callinfo.Looker{}, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q})

	assert.Contains(t, logs.String(), "level=ERROR")
	assert.Contains(t, logs.String(), "K1AB")
	assert.Empty(t, sent())
}

func TestHandleListenerEventLogsDroppedFields(t *testing.T) {
	store := openTestLogbook(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	send, _ := recordSends()

	q := logbook.QSO{Call: "K1AB", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), Band: "20m", Mode: "FT8"}
	dropped := []error{&logbook.FieldError{Field: "FREQ_RX", Value: "14.074 MHz"}}
	handleListenerEvent(t.Context(), logger, store, callinfo.Looker{}, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q, Dropped: dropped})

	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "FREQ_RX")
	assert.Contains(t, logs.String(), "K1AB")
	n, err := store.Count(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the QSO is still stored")
}

func TestHandleListenerEventStoresAfterFailedLookup(t *testing.T) {
	store := openTestLogbook(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	send, sent := recordSends()
	cb := cbmock.NewMockLooker(gomock.NewController(t))
	cb.EXPECT().Lookup(gomock.Any(), "K1AB").Return(callbook.Result{}, fmt.Errorf("qrz: %w", callbook.ErrAuth))
	info := callinfo.Looker{DXCC: dxcc.Default(), Callbook: cb, Logbook: store}

	q := logbook.QSO{Call: "K1AB", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), Band: "20m", Mode: "FT8"}
	handleListenerEvent(t.Context(), logger, store, info, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q})

	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "lookup failed")
	assert.Equal(t, []tea.Msg{tui.LogbookChanged{}}, sent())
	qsos, err := store.Index(t.Context(), nil, logbook.Sort{}, 0, 10)
	require.NoError(t, err)
	require.Len(t, qsos, 1)
	got, err := store.Get(t.Context(), qsos[0].ID)
	require.NoError(t, err)
	assert.Equal(t, 291, got.DXCC.V, "the QSO is stored with what was found")
}

// wsjtxStatus returns a WSJT-X Status (type 1) packet, cut after the mode.
func wsjtxStatus(id string, dialHz uint64, mode string) []byte {
	b := wsjtxHeader(1, id)
	b = binary.BigEndian.AppendUint64(b, dialHz)
	return appendQString(b, mode)
}

// wsjtxDecode returns a WSJT-X Decode (type 2) packet for message.
func wsjtxDecode(id, message string) []byte {
	b := wsjtxHeader(2, id)
	b = append(b, 1)                        // new
	b = append(b, make([]byte, 4+4+8+4)...) // time, SNR, delta time, delta frequency
	b = appendQString(b, "~")               // mode
	b = appendQString(b, message)
	return append(b, 0, 0) // low confidence, off air
}

// wsjtxHeader starts a WSJT-X packet of msgType from id.
func wsjtxHeader(msgType uint32, id string) []byte {
	var b []byte
	b = binary.BigEndian.AppendUint32(b, 0xADBCCBDA)
	b = binary.BigEndian.AppendUint32(b, 3)
	b = binary.BigEndian.AppendUint32(b, msgType)
	return appendQString(b, id)
}

// appendQString appends s as a QByteArray.
func appendQString(b []byte, s string) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

// fakeWSJTX is a WSJT-X instance on loopback that sends to a listener and
// reads its replies.
type fakeWSJTX struct {
	t    *testing.T
	conn *net.UDPConn
}

// newFakeWSJTX returns a fakeWSJTX sending to addr.
func newFakeWSJTX(t *testing.T, addr string) fakeWSJTX {
	t.Helper()
	raddr, err := net.ResolveUDPAddr("udp", addr)
	require.NoError(t, err)
	conn, err := net.DialUDP("udp", nil, raddr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return fakeWSJTX{t: t, conn: conn}
}

// send sends packet, ignoring a refusal while the listener is not yet
// bound.
func (w fakeWSJTX) send(packet []byte) {
	_, _ = w.conn.Write(packet)
}

// read returns the next packet sent back, or nil after wait.
func (w fakeWSJTX) read(wait time.Duration) []byte {
	require.NoError(w.t, w.conn.SetReadDeadline(time.Now().Add(wait)))
	buf := make([]byte, 1024)
	n, err := w.conn.Read(buf)
	if err != nil {
		return nil
	}
	return buf[:n]
}

func TestRunListenerHighlightsWorkedCalls(t *testing.T) {
	store := openTestLogbook(t)
	q := logbook.QSO{Call: "K1AB", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), Band: "20m", Mode: "FT8"}
	_, err := store.Import(t.Context(), []logbook.QSO{q}, nil)
	require.NoError(t, err)
	addr := freeUDPAddr(t)
	send, _ := recordSends()
	cfg := config.Listener{Protocol: "wsjtx", Address: addr, Highlight: true, Colors: config.DefaultHighlightColors}
	startListener(t, "wsjtx", cfg, store, callinfo.Looker{}, slog.New(slog.DiscardHandler), send)

	wsjtx := newFakeWSJTX(t, addr)
	var reply []byte
	require.Eventually(t, func() bool {
		wsjtx.send(wsjtxStatus("WSJT-X", 14_074_000, "FT8"))
		wsjtx.send(wsjtxDecode("WSJT-X", "CQ K1AB FN42"))
		reply = wsjtx.read(50 * time.Millisecond)
		return reply != nil
	}, 3*time.Second, 10*time.Millisecond)

	gray, err := config.ParseColor(config.DefaultHighlightColors.WorkedBandMode.Foreground)
	require.NoError(t, err)
	assert.Equal(t, listener.WSJTXHighlightCallsign("WSJT-X", "K1AB", nil, gray, false), reply)

	// A call worked on another band is highlighted as worked.
	wsjtx.send(wsjtxStatus("WSJT-X", 7_074_000, "FT8"))
	wsjtx.send(wsjtxDecode("WSJT-X", "W9XYZ K1AB -10"))
	yellow, err := config.ParseColor(config.DefaultHighlightColors.Worked.Background)
	require.NoError(t, err)
	assert.Equal(t, listener.WSJTXHighlightCallsign("WSJT-X", "K1AB", yellow, nil, false), wsjtx.read(3*time.Second))
}

func TestRunListenerHighlightsLoggedCall(t *testing.T) {
	store := openTestLogbook(t)
	addr := freeUDPAddr(t)
	send, _ := recordSends()
	cfg := config.Listener{Protocol: "wsjtx", Address: addr, Highlight: true, Colors: config.DefaultHighlightColors}
	startListener(t, "wsjtx", cfg, store, callinfo.Looker{DXCC: dxcc.Default(), Logbook: store}, slog.New(slog.DiscardHandler), send)

	wsjtx := newFakeWSJTX(t, addr)
	// A new call isn't highlighted by default, so nothing comes back
	// until the QSO is logged.
	require.Eventually(t, func() bool {
		wsjtx.send(wsjtxStatus("WSJT-X", 14_074_000, "FT8"))
		wsjtx.send(wsjtxDecode("WSJT-X", "CQ K1AB FN42"))
		wsjtx.send(wsjtxLoggedADIF("WSJT-X", testWSJTXADIF))
		wsjtx.send(wsjtxDecode("WSJT-X", "W1XYZ K1AB RR73"))
		return wsjtx.read(50*time.Millisecond) != nil
	}, 3*time.Second, 10*time.Millisecond)
}

func TestRunListenerWithoutHighlightDoesNotReply(t *testing.T) {
	store := openTestLogbook(t)
	q := logbook.QSO{Call: "K1AB", StartUTC: time.Date(2026, 11, 23, 13, 1, 2, 0, time.UTC), Band: "20m", Mode: "FT8"}
	_, err := store.Import(t.Context(), []logbook.QSO{q}, nil)
	require.NoError(t, err)
	addr := freeUDPAddr(t)
	send, sent := recordSends()
	startListener(t, "wsjtx", config.Listener{Protocol: "wsjtx", Address: addr}, store, callinfo.Looker{}, slog.New(slog.DiscardHandler), send)

	wsjtx := newFakeWSJTX(t, addr)
	// Logging a QSO shows the listener is up.
	require.Eventually(t, func() bool {
		wsjtx.send(wsjtxDecode("WSJT-X", "CQ K1AB FN42"))
		wsjtx.send(wsjtxLoggedADIF("WSJT-X", strings.Replace(testWSJTXADIF, "130102", "140102", 1)))
		return len(sent()) > 0
	}, 3*time.Second, 10*time.Millisecond)
	assert.Nil(t, wsjtx.read(100*time.Millisecond))
}

func TestHighlightScheme(t *testing.T) {
	scheme, err := highlightScheme(config.HighlightColors{
		New:            config.HighlightColor{Background: "#00ff00", Foreground: "#000000"},
		WorkedBandMode: config.HighlightColor{Foreground: "#808080"},
	})
	require.NoError(t, err)
	assert.Equal(t, highlight.Scheme{
		highlight.StatusNew:            {Background: color.RGBA{G: 0xff, A: 0xff}, Foreground: color.RGBA{A: 0xff}},
		highlight.StatusWorked:         {},
		highlight.StatusWorkedBandMode: {Foreground: color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}},
	}, scheme)

	_, err = highlightScheme(config.HighlightColors{Worked: config.HighlightColor{Background: "yellow"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worked")
}

func TestRunListenerBadHighlightColorsStillLogs(t *testing.T) {
	store := openTestLogbook(t)
	addr := freeUDPAddr(t)
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	send, sent := recordSends()
	cfg := config.Listener{Protocol: "wsjtx", Address: addr, Highlight: true, Colors: config.HighlightColors{Worked: config.HighlightColor{Background: "yellow"}}}
	startListener(t, "wsjtx", cfg, store, callinfo.Looker{}, logger, send)

	require.Eventually(t, func() bool {
		sendUDP(t, addr, wsjtxLoggedADIF("WSJT-X", testWSJTXADIF))
		return len(sent()) > 0
	}, 3*time.Second, 20*time.Millisecond)
	assert.Contains(t, logs.String(), "highlighting off")
}

func TestHighlightListenerEventQuietOnQuit(t *testing.T) {
	store := openTestLogbook(t)
	scheme, err := highlightScheme(config.DefaultHighlightColors)
	require.NoError(t, err)
	hl := highlight.New(store, scheme, highlightTTL)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reply := func(context.Context, netip.AddrPort, []byte) error { return ctx.Err() }
	ev := listener.DecodeReceived{Source: "WSJT-X", From: netip.MustParseAddrPort("127.0.0.1:50000"), Message: "CQ K1AB FN42"}
	highlightListenerEvent(ctx, logger, hl, reply, ev)
	assert.Empty(t, logs.String(), "a lookup cut short as the app quits is not a failure")
}

func TestHighlightListenerEventLogsFailures(t *testing.T) {
	store, err := logbook.Open(t.Context(), filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	require.NoError(t, store.Close(), "lookups in a closed logbook fail")
	hl := highlight.New(store, highlight.Scheme{}, highlightTTL)
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	ev := listener.DecodeReceived{Source: "WSJT-X", From: netip.MustParseAddrPort("127.0.0.1:50000"), Message: "CQ K1AB FN42"}
	highlightListenerEvent(t.Context(), logger, hl, nil, ev)
	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "lookup failed")
}
