//go:build unix

package listener

import (
	"context"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loopbackInterface returns the name of the loopback interface, such as
// "lo0" on macOS. It skips the test when that interface can't carry
// multicast, as Linux's lo can't by default.
func loopbackInterface(t *testing.T) string {
	t.Helper()
	ifis, err := net.Interfaces()
	require.NoError(t, err)
	for _, ifi := range ifis {
		if ifi.Flags&net.FlagLoopback != 0 && ifi.Flags&net.FlagUp != 0 && ifi.Flags&net.FlagMulticast != 0 {
			return ifi.Name
		}
	}
	t.Skip("no multicast-capable loopback interface")
	return ""
}

// sendOnLoopback sends b to group out of the loopback interface, as WSJT-X
// does with its outgoing interface set to lo0.
func sendOnLoopback(t *testing.T, group *net.UDPAddr, b []byte) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	raw, err := conn.SyscallConn()
	require.NoError(t, err)
	var sockErr error
	require.NoError(t, raw.Control(func(fd uintptr) {
		sockErr = syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, [4]byte{127, 0, 0, 1})
	}))
	require.NoError(t, sockErr)
	_, err = conn.WriteToUDP(b, group)
	require.NoError(t, err)
}

// TestListenerMulticast joins a group on the loopback interface twice, as
// jstlogger and GridTracker2 do, and checks that both listeners get the
// QSO.
func TestListenerMulticast(t *testing.T) {
	iface := loopbackInterface(t)
	probe, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	port := probe.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, probe.Close())
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 1), Port: port}

	ctx, cancel := context.WithCancel(context.Background())
	var chans []chan any
	var dones []chan error
	for range 2 {
		l, err := New(Options{Address: group.String(), Interface: iface, Decoder: WSJTX{}})
		require.NoError(t, err)
		events := make(chan any, 64)
		done := make(chan error, 1)
		bound := make(chan net.Addr, 1)
		l.onBind = func(a net.Addr) { bound <- a }
		go func() { done <- l.Run(ctx, func(ev any) { events <- ev }) }()
		select {
		case <-bound:
		case err := <-done:
			cancel()
			t.Skipf("multicast join on %s failed: %v", iface, err)
		case <-time.After(waitTimeout):
			t.Fatal("timed out waiting for bind")
		}
		chans = append(chans, events)
		dones = append(dones, done)
	}
	t.Cleanup(func() {
		cancel()
		for _, done := range dones {
			assert.NoError(t, <-done)
		}
	})

	sendOnLoopback(t, group, loggedADIF("WSJT-X", sampleADIF))

	for i, events := range chans {
		select {
		case ev := <-events:
			got, ok := ev.(QSOReceived)
			require.True(t, ok, "listener %d: event %#v", i, ev)
			assert.Equal(t, "K1AB", got.QSO.Call, "listener %d", i)
		case <-time.After(waitTimeout):
			t.Fatalf("listener %d: no QSO received", i)
		}
	}
}
