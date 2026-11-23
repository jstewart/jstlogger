package listener

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitTimeout bounds every wait in these tests.
const waitTimeout = 3 * time.Second

// waitFor skips events until one of type T arrives and returns it.
func waitFor[T any](t *testing.T, ch <-chan any) T {
	t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case ev := <-ch:
			if v, ok := ev.(T); ok {
				return v
			}
		case <-deadline:
			var zero T
			t.Fatalf("timed out waiting for %T", zero)
			return zero
		}
	}
}

// running is a Listener started by startListener.
type running struct {
	events <-chan any
	done   <-chan error
	addr   net.Addr
	cancel context.CancelFunc
}

// startListener runs l in the background and waits for it to bind. Run is
// stopped at cleanup.
func startListener(t *testing.T, l *Listener) running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan any, 64)
	done := make(chan error, 1)
	bound := make(chan net.Addr, 1)
	l.onBind = func(a net.Addr) { bound <- a }
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		done <- l.Run(ctx, func(ev any) { events <- ev })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(waitTimeout):
			t.Error("Run did not return after cancel")
		}
	})
	select {
	case a := <-bound:
		return running{events: events, done: done, addr: a, cancel: cancel}
	case err := <-done:
		t.Fatalf("Run returned before binding: %v", err)
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for bind")
	}
	return running{}
}

// send writes one datagram to addr from loopback.
func send(t *testing.T, addr net.Addr, b []byte) {
	t.Helper()
	ua, ok := addr.(*net.UDPAddr)
	require.True(t, ok)
	dst := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: ua.Port}
	conn, err := net.DialUDP("udp", nil, dst)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Write(b)
	require.NoError(t, err)
}

func newTestListener(t *testing.T, addr string) *Listener {
	t.Helper()
	l, err := New(Options{Address: addr, Decoder: WSJTX{}})
	require.NoError(t, err)
	return l
}

func TestNewValidates(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"no decoder", Options{Address: "127.0.0.1:2237"}},
		{"hostname", Options{Address: "localhost:2237", Decoder: WSJTX{}}},
		{"no port", Options{Address: "127.0.0.1", Decoder: WSJTX{}}},
		{"bad port", Options{Address: "127.0.0.1:x", Decoder: WSJTX{}}},
		{"port out of range", Options{Address: "127.0.0.1:70000", Decoder: WSJTX{}}},
		{"empty", Options{Decoder: WSJTX{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			assert.Error(t, err)
		})
	}
	_, err := New(Options{Address: "224.0.0.1:2237", Interface: "lo0", Decoder: WSJTX{}})
	assert.NoError(t, err)
}

func TestListenerReceivesQSO(t *testing.T) {
	r := startListener(t, newTestListener(t, "127.0.0.1:0"))
	send(t, r.addr, loggedADIF("WSJT-X", sampleADIF))
	got := waitFor[QSOReceived](t, r.events)
	assert.Equal(t, "WSJT-X", got.Source)
	assert.Equal(t, "K1AB", got.QSO.Call)
}

func TestListenerDropsGarbageAndContinues(t *testing.T) {
	r := startListener(t, newTestListener(t, "127.0.0.1:0"))
	send(t, r.addr, []byte("garbage!!!!!"))
	dropped := waitFor[PacketDropped](t, r.events)
	assert.ErrorIs(t, dropped.Err, ErrBadMagic)
	assert.Contains(t, dropped.From, "127.0.0.1")

	send(t, r.addr, loggedADIF("WSJT-X", sampleADIF))
	got := waitFor[QSOReceived](t, r.events)
	assert.Equal(t, "K1AB", got.QSO.Call)
}

func TestListenerDropsForeignSender(t *testing.T) {
	l := newTestListener(t, "127.0.0.1:0")
	l.isLocal = func(netip.Addr) bool { return false }
	r := startListener(t, l)
	send(t, r.addr, loggedADIF("WSJT-X", sampleADIF))
	dropped := waitFor[PacketDropped](t, r.events)
	assert.ErrorIs(t, dropped.Err, ErrForeignSender)
	assert.Contains(t, dropped.From, "127.0.0.1")
}

func TestListenerStopsOnCancel(t *testing.T) {
	r := startListener(t, newTestListener(t, "127.0.0.1:0"))
	r.cancel()
	select {
	case err := <-r.done:
		assert.NoError(t, err)
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after cancel")
	}
}

func TestListenerPortInUse(t *testing.T) {
	busy, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = busy.Close() })

	l := newTestListener(t, busy.LocalAddr().String())
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	err = l.Run(ctx, func(any) {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listener: listen on")
}

func TestListenerBadInterface(t *testing.T) {
	l, err := New(Options{Address: "224.0.0.1:0", Interface: "nosuchif0", Decoder: WSJTX{}})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	assert.Error(t, l.Run(ctx, func(any) {}))
}

func TestIsLocalAddr(t *testing.T) {
	assert.True(t, isLocalAddr(netip.MustParseAddr("127.0.0.1")), "IPv4 loopback")
	assert.True(t, isLocalAddr(netip.MustParseAddr("::1")), "IPv6 loopback")
	assert.False(t, isLocalAddr(netip.MustParseAddr("192.0.2.1")), "a TEST-NET-1 address is no host's")
	assert.False(t, isLocalAddr(netip.MustParseAddr("2001:db8::1")), "a documentation address is no host's")

	addrs, err := net.InterfaceAddrs()
	require.NoError(t, err)
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip, ok := netip.AddrFromSlice(ipnet.IP)
		require.True(t, ok)
		assert.True(t, isLocalAddr(ip.Unmap()), "this host's address %s", ip)
		if ip.Is6() && ip.IsLinkLocalUnicast() {
			// A link-local sender's source address carries a zone.
			zoned := ip.WithZone("en0")
			assert.True(t, isLocalAddr(zoned), "this host's address %s", zoned)
		}
	}
}
