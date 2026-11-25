package listener

import (
	"encoding/binary"
	"image/color"
	"math"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// u64 appends a big-endian quint64.
func (p packet) u64(v uint64) packet { return binary.BigEndian.AppendUint64(p, v) }

// boolean appends a bool as one byte.
func (p packet) boolean(v bool) packet {
	if v {
		return append(p, 1)
	}
	return append(p, 0)
}

// f64 appends a big-endian double.
func (p packet) f64(v float64) packet { return p.u64(math.Float64bits(v)) }

// status builds a Status (type 1) message with the fields this package
// reads, followed by a few of the ones it ignores.
func status(id string, dialHz uint64, mode string) packet {
	return header(1, id).u64(dialHz).str(mode).str("K1AB").str("-10").str(mode).boolean(false)
}

// decode builds a Decode (type 2) message.
func decode(id, message string) packet {
	return header(2, id).
		boolean(true).       // new
		u32(13 * 3600_000).  // time: ms since midnight
		u32(uint32(0xFFF6)). // snr
		f64(0.2).            // delta time
		u32(1234).           // delta frequency
		str("~").            // mode
		str(message).
		boolean(false). // low confidence
		boolean(false)  // off air
}

func TestWSJTXDecodeStatus(t *testing.T) {
	evs, err := WSJTX{}.Decode(status("WSJT-X", 14_074_000, "FT8"))
	require.NoError(t, err)
	require.Len(t, evs, 1)
	assert.Equal(t, StatusReceived{Source: "WSJT-X", DialHz: 14_074_000, Mode: "FT8"}, evs[0])

	p := status("WSJT-X", 14_074_000, "FT8")
	_, err = WSJTX{}.Decode(p[:len(header(1, "WSJT-X"))+5])
	require.ErrorIs(t, err, ErrTruncated)
}

func TestWSJTXDecodeDecode(t *testing.T) {
	evs, err := WSJTX{}.Decode(decode("JTDX", "CQ K1AB FN42"))
	require.NoError(t, err)
	require.Len(t, evs, 1)
	assert.Equal(t, DecodeReceived{Source: "JTDX", Message: "CQ K1AB FN42"}, evs[0])

	p := decode("JTDX", "CQ K1AB FN42")
	for _, cut := range []int{len(header(2, "JTDX")) + 3, len(p) - 4} {
		_, err = WSJTX{}.Decode(p[:cut])
		require.ErrorIs(t, err, ErrTruncated, "cut at %d", cut)
	}
}

func TestWSJTXHighlightCallsign(t *testing.T) {
	got := WSJTXHighlightCallsign("WSJT-X", "K1AB", color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}, nil, false)
	want := header(13, "WSJT-X").str("K1AB")
	// Background: spec Rgb, alpha, red, green, blue, pad.
	want = append(want, 1)
	want = binary.BigEndian.AppendUint16(want, 0xffff)
	want = binary.BigEndian.AppendUint16(want, 0x1212)
	want = binary.BigEndian.AppendUint16(want, 0x3434)
	want = binary.BigEndian.AppendUint16(want, 0x5656)
	want = binary.BigEndian.AppendUint16(want, 0)
	// Foreground: an invalid color, which leaves it as it was.
	want = append(want, 0, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0)
	want = want.boolean(false)
	assert.Equal(t, []byte(want), got)
}

func TestListenerAddressesEvents(t *testing.T) {
	r := startListener(t, newTestListener(t, "127.0.0.1:0"))
	conn := dial(t, r.addr)
	_, err := conn.Write(status("WSJT-X", 7_074_000, "FT8"))
	require.NoError(t, err)
	_, err = conn.Write(decode("WSJT-X", "CQ K1AB FN42"))
	require.NoError(t, err)

	from := conn.LocalAddr().(*net.UDPAddr).AddrPort()
	st := waitFor[StatusReceived](t, r.events)
	assert.Equal(t, from, st.From)
	dec := waitFor[DecodeReceived](t, r.events)
	assert.Equal(t, from, dec.From)
	assert.Equal(t, "CQ K1AB FN42", dec.Message)
}

func TestListenerSendReplies(t *testing.T) {
	l := newTestListener(t, "127.0.0.1:0")
	r := startListener(t, l)
	conn := dial(t, r.addr)
	from := conn.LocalAddr().(*net.UDPAddr).AddrPort()

	require.NoError(t, l.Send(t.Context(), from, []byte("reply")))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(waitTimeout)))
	buf := make([]byte, 64)
	n, src, err := conn.ReadFromUDPAddrPort(buf)
	require.NoError(t, err)
	assert.Equal(t, "reply", string(buf[:n]))
	assert.Equal(t, r.addr.(*net.UDPAddr).AddrPort().Port(), src.Port(), "sent from the listening socket")
}

func TestListenerSendBeforeRun(t *testing.T) {
	l := newTestListener(t, "127.0.0.1:0")
	err := l.Send(t.Context(), netip.MustParseAddrPort("127.0.0.1:9"), []byte("x"))
	require.ErrorIs(t, err, ErrNotRunning)
}

// dial returns a UDP socket on loopback connected to addr.
func dial(t *testing.T, addr net.Addr) *net.UDPConn {
	t.Helper()
	ua, ok := addr.(*net.UDPAddr)
	require.True(t, ok)
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: ua.Port})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
