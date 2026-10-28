package rig

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperEnv makes the test binary act as a fake rigctld: "serve" listens on
// the -T/-t address, "exit" fails at once like rigctld with a bad device.
const helperEnv = "JSTLOGGER_FAKE_RIGCTLD"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "serve":
		os.Exit(fakeRigctldMain(os.Args[1:]))
	case "exit":
		fmt.Fprintln(os.Stderr, "rig_open: cannot open /dev/nope")
		os.Exit(3)
	}
	os.Exit(m.Run())
}

// fakeRigctldMain serves the fake protocol on the address given by -T and -t
// until SIGINT or SIGTERM.
func fakeRigctldMain(args []string) int {
	host, port := "127.0.0.1", ""
	if i := slices.Index(args, "-T"); i >= 0 && i+1 < len(args) {
		host = args[i+1]
	}
	if i := slices.Index(args, "-t"); i >= 0 && i+1 < len(args) {
		port = args[i+1]
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f := startFakeRigctld(ln)
	<-sig
	f.Close()
	return 0
}

func TestSpawnArgs(t *testing.T) {
	tests := []struct {
		name    string
		spawn   Spawn
		addr    string
		want    []string
		wantErr bool
	}{
		{
			name:  "all flags",
			spawn: Spawn{Model: 3073, Device: "/dev/ttyUSB0", Baud: 19200},
			addr:  "localhost:4532",
			want:  []string{"-m", "3073", "-r", "/dev/ttyUSB0", "-s", "19200", "-T", "localhost", "-t", "4532"},
		},
		{
			name:  "no baud",
			spawn: Spawn{Model: 3073, Device: "/dev/ttyUSB0"},
			addr:  "127.0.0.1:4533",
			want:  []string{"-m", "3073", "-r", "/dev/ttyUSB0", "-T", "127.0.0.1", "-t", "4533"},
		},
		{
			name:  "no device or host",
			spawn: Spawn{Model: 1},
			addr:  ":4532",
			want:  []string{"-m", "1", "-t", "4532"},
		},
		{
			name:  "ipv6",
			spawn: Spawn{Model: 1},
			addr:  "[::1]:4532",
			want:  []string{"-m", "1", "-T", "::1", "-t", "4532"},
		},
		{name: "no port", spawn: Spawn{Model: 1}, addr: "localhost", wantErr: true},
		{name: "empty port", spawn: Spawn{Model: 1}, addr: "localhost:", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := spawnArgs(tt.spawn, tt.addr)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// newSpawnRig returns a test Rig that spawns the test binary as rigctld.
func newSpawnRig(t *testing.T, mode string) (*Rig, string) {
	t.Helper()
	t.Setenv(helperEnv, mode)
	addr := closedAddr(t)
	r := New(Options{Addr: addr, PollInterval: 10 * time.Millisecond, Spawn: &Spawn{Model: 1}})
	r.timeout = 500 * time.Millisecond
	r.backoffMin = 10 * time.Millisecond
	r.backoffMax = 40 * time.Millisecond
	r.rigctldPath = os.Args[0]
	return r, addr
}

func TestRunSpawnLifecycle(t *testing.T) {
	r, addr := newSpawnRig(t, "serve")
	events, cancel, done := startRig(t, r)

	waitFor[ConnectionChanged](t, events, func(c ConnectionChanged) bool { return c.Status == Connected })
	st := waitFor[StateChanged](t, events, nil)
	assert.Equal(t, "USB", st.State.Mode)

	cancel()
	require.NoError(t, <-done)
	// Run waits for rigctld to exit, so its port is closed by now.
	_, err := net.DialTimeout("tcp", addr, time.Second)
	assert.Error(t, err, "spawned rigctld should have been stopped")
}

func TestRunSpawnUnexpectedExit(t *testing.T) {
	r, _ := newSpawnRig(t, "exit")
	events, cancel, done := startRig(t, r)

	dc := waitFor[ConnectionChanged](t, events, func(c ConnectionChanged) bool {
		return c.Status == Disconnected && c.Err != nil && strings.Contains(c.Err.Error(), "rigctld exited")
	})
	assert.Contains(t, dc.Err.Error(), "cannot open /dev/nope")

	// Run keeps retrying the connection rather than failing.
	waitFor[ConnectionChanged](t, events, func(c ConnectionChanged) bool { return c.Status == Connecting })
	cancel()
	assert.NoError(t, <-done)
}

func TestRunSpawnStartFailure(t *testing.T) {
	r := New(Options{Addr: "127.0.0.1:4532", PollInterval: time.Second, Spawn: &Spawn{Model: 1}})
	r.rigctldPath = "/nonexistent/rigctld"
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	err := r.Run(ctx, func(any) {})
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.Contains(t, err.Error(), "start rigctld")
}
