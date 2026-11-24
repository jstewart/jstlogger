package listener

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// readBufSize is the largest packet the listener reads; it fits any UDP
// datagram.
const readBufSize = 64 << 10

// Options configures a Listener.
type Options struct {
	// Address is the IP literal and port to listen on, such as
	// "127.0.0.1:2237" (unicast) or "224.0.0.1:2237" (a multicast group).
	Address string
	// Interface optionally names the network interface for a multicast join,
	// such as "lo0". Empty lets the system choose.
	Interface string
	// Decoder turns each packet into events.
	Decoder Decoder
}

// Listener receives UDP packets on one address and decodes them into events.
// Only packets from this host are decoded.
type Listener struct {
	addr    netip.AddrPort
	iface   string
	decoder Decoder
	isLocal func(netip.Addr) bool
	onBind  func(net.Addr) // test hook, called after the socket is bound
}

// New returns a Listener for opts. It checks that Address is an IP literal
// with a port and that Decoder is set; it does not bind the socket.
func New(opts Options) (*Listener, error) {
	if opts.Decoder == nil {
		return nil, errors.New("listener: no decoder")
	}
	host, port, err := net.SplitHostPort(opts.Address)
	if err != nil {
		return nil, fmt.Errorf("listener: address %q: %w", opts.Address, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("listener: address %q: host must be an IP address: %w", opts.Address, err)
	}
	p, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("listener: address %q: port: %w", opts.Address, err)
	}
	return &Listener{
		addr:    netip.AddrPortFrom(ip.Unmap(), uint16(p)),
		iface:   opts.Interface,
		decoder: opts.Decoder,
		isLocal: isLocalAddr,
	}, nil
}

// Run binds the socket, then reads packets until ctx is canceled, passing
// events to emit from Run's goroutine. It returns nil when ctx is canceled
// and a wrapped error when it can't bind (bad interface name, port in use,
// ...) or reading fails.
func (l *Listener) Run(ctx context.Context, emit func(any)) error {
	conn, err := l.listen()
	if err != nil {
		return fmt.Errorf("listener: listen on %s: %w", l.addr, err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer func() {
		if stop() {
			_ = conn.Close()
		}
	}()
	if l.onBind != nil {
		l.onBind(conn.LocalAddr())
	}

	buf := make([]byte, readBufSize)
	for {
		n, src, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("listener: read from %s: %w", l.addr, err)
		}
		if !l.isLocal(src.Addr().Unmap()) {
			emit(PacketDropped{From: src.String(), Err: ErrForeignSender})
			continue
		}
		events, err := l.decoder.Decode(buf[:n])
		if err != nil {
			emit(PacketDropped{From: src.String(), Err: err})
			continue
		}
		for _, ev := range events {
			emit(ev)
		}
	}
}

// listen binds the UDP socket, joining the multicast group when the address
// is one.
func (l *Listener) listen() (*net.UDPConn, error) {
	udp := net.UDPAddrFromAddrPort(l.addr)
	if !l.addr.Addr().IsMulticast() {
		return net.ListenUDP("udp", udp)
	}
	var ifi *net.Interface
	if l.iface != "" {
		var err error
		if ifi, err = net.InterfaceByName(l.iface); err != nil {
			return nil, fmt.Errorf("interface %q: %w", l.iface, err)
		}
	}
	return net.ListenMulticastUDP("udp", ifi, udp)
}

// isLocalAddr reports whether ip is loopback or one of this host's
// interface addresses. An IPv6 zone is ignored, as interface addresses
// carry none.
func isLocalAddr(ip netip.Addr) bool {
	ip = ip.WithZone("").Unmap()
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if local, ok := netip.AddrFromSlice(ipnet.IP); ok && local.Unmap() == ip {
			return true
		}
	}
	return false
}
