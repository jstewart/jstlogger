package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/config"
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
func startListener(t *testing.T, name string, cfg config.Listener, store logbook.Store, logger *slog.Logger, send func(tea.Msg)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		runListener(ctx, name, cfg, store, logger, send)
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
	addr := freeUDPAddr(t)
	send, sent := recordSends()
	var logs lockedBuffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	startListener(t, "wsjtx", config.Listener{Protocol: "wsjtx", Address: addr, Tags: []string{"wsjtx", "ft8"}}, store, logger, send)

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
			runListener(t.Context(), "wsjtx", tt.cfg, openTestLogbook(t), logger, send)

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
	handleListenerEvent(ctx, slog.New(slog.DiscardHandler), store, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q})

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
	handleListenerEvent(t.Context(), logger, store, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q})

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
	handleListenerEvent(t.Context(), logger, store, nil, send, listener.QSOReceived{Source: "WSJT-X", QSO: q, Dropped: dropped})

	assert.Contains(t, logs.String(), "level=WARN")
	assert.Contains(t, logs.String(), "FREQ_RX")
	assert.Contains(t, logs.String(), "K1AB")
	n, err := store.Count(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the QSO is still stored")
}
