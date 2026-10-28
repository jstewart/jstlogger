package rig

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeRigctld is an in-process rigctld that speaks the extended response
// protocol over TCP. It is safe for concurrent use.
type fakeRigctld struct {
	ln net.Listener
	wg sync.WaitGroup

	mu     sync.Mutex
	state  State
	cmds   []string
	errs   map[string]int // command name -> RPRT code to return
	hang   bool           // read commands but never reply
	conns  map[net.Conn]struct{}
	closed bool
}

// newFakeRigctld starts a fake rigctld on 127.0.0.1:0 and stops it when the
// test ends.
func newFakeRigctld(t *testing.T) *fakeRigctld {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := startFakeRigctld(ln)
	t.Cleanup(f.Close)
	return f
}

// startFakeRigctld serves the fake protocol on ln until Close.
func startFakeRigctld(ln net.Listener) *fakeRigctld {
	f := &fakeRigctld{
		ln:    ln,
		state: State{Freq: 14074000, Mode: "USB", Passband: 2400},
		errs:  map[string]int{},
		conns: map[net.Conn]struct{}{},
	}
	f.wg.Go(f.accept)
	return f
}

// Addr is the host:port the fake listens on.
func (f *fakeRigctld) Addr() string { return f.ln.Addr().String() }

// SetState replaces the rig state, as if the operator turned the dial.
func (f *fakeRigctld) SetState(s State) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = s
}

// State returns the current rig state.
func (f *fakeRigctld) State() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

// Commands returns the command lines received so far.
func (f *fakeRigctld) Commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cmds...)
}

// SetError makes cmd (e.g. "set_freq") answer with RPRT code; 0 clears it.
func (f *fakeRigctld) SetError(cmd string, code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if code == 0 {
		delete(f.errs, cmd)
		return
	}
	f.errs[cmd] = code
}

// SetHang makes the fake stop replying to commands.
func (f *fakeRigctld) SetHang(hang bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hang = hang
}

// DropConns closes every open client connection; new ones are still accepted.
func (f *fakeRigctld) DropConns() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for c := range f.conns {
		_ = c.Close()
	}
}

// Close stops the listener and drops all connections.
func (f *fakeRigctld) Close() {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	_ = f.ln.Close()
	f.DropConns()
	f.wg.Wait()
}

func (f *fakeRigctld) accept() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		if f.closed {
			f.mu.Unlock()
			_ = c.Close()
			return
		}
		f.conns[c] = struct{}{}
		f.mu.Unlock()
		f.wg.Go(func() { f.serve(c) })
	}
}

func (f *fakeRigctld) serve(c net.Conn) {
	defer func() {
		f.mu.Lock()
		delete(f.conns, c)
		f.mu.Unlock()
		_ = c.Close()
	}()
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		reply, ok := f.handle(strings.TrimRight(line, "\r\n"))
		if !ok {
			continue // like rigctld: unknown commands get no reply
		}
		if _, err := c.Write([]byte(reply)); err != nil {
			return
		}
	}
}

// handle applies one command line and returns the reply, or ok=false for
// no reply.
func (f *fakeRigctld) handle(line string) (reply string, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, line)
	if f.hang {
		return "", false
	}
	cmdline, found := strings.CutPrefix(line, `+\`)
	if !found {
		return "", false
	}
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return "", false
	}
	name, args := fields[0], fields[1:]
	echo := name + ":"
	if len(args) > 0 {
		echo += " " + strings.Join(args, " ")
	}
	if code, bad := f.errs[name]; bad {
		return fmt.Sprintf("%s\nRPRT %d\n", echo, code), true
	}
	switch name {
	case "get_freq":
		return fmt.Sprintf("%s\nFrequency: %d\nRPRT 0\n", echo, f.state.Freq), true
	case "get_mode":
		return fmt.Sprintf("%s\nMode: %s\nPassband: %d\nRPRT 0\n", echo, f.state.Mode, f.state.Passband), true
	case "set_freq":
		if len(args) != 1 {
			return "", false
		}
		hz, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return echo + "\nRPRT -1\n", true
		}
		f.state.Freq = hz
		return echo + "\nRPRT 0\n", true
	case "set_mode":
		if len(args) != 2 {
			return "", false
		}
		pb, err := strconv.Atoi(args[1])
		if err != nil {
			return echo + "\nRPRT -1\n", true
		}
		f.state.Mode, f.state.Passband = args[0], pb
		return echo + "\nRPRT 0\n", true
	}
	return "", false
}
