package rig

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rigctldWaitDelay is how long rigctld gets to exit after SIGINT before it
// is killed.
const rigctldWaitDelay = 2 * time.Second

// stderrTail is how many trailing bytes of rigctld's stderr are kept for
// the exit error.
const stderrTail = 512

// spawnArgs builds the rigctld command line for s listening on addr.
func spawnArgs(s Spawn, addr string) ([]string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("rigctld address %q: %w", addr, err)
	}
	if port == "" {
		return nil, fmt.Errorf("rigctld address %q: missing port", addr)
	}
	args := []string{"-m", strconv.Itoa(s.Model)}
	if s.Device != "" {
		args = append(args, "-r", s.Device)
	}
	if s.Baud > 0 {
		args = append(args, "-s", strconv.Itoa(s.Baud))
	}
	if host != "" {
		args = append(args, "-T", host)
	}
	return append(args, "-t", port), nil
}

// startRigctld starts rigctld as configured in Options.Spawn. The process is
// interrupted when ctx is done. The returned channel receives exactly one
// error describing its exit.
func (r *Rig) startRigctld(ctx context.Context) (<-chan error, error) {
	args, err := spawnArgs(*r.opts.Spawn, r.opts.Addr)
	if err != nil {
		return nil, fmt.Errorf("start rigctld: %w", err)
	}
	cmd := exec.CommandContext(ctx, r.rigctldPath, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = rigctldWaitDelay
	stderr := &tailBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start rigctld: %w", err)
	}

	exit := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err == nil {
			err = errors.New("exit status 0")
		}
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			err = fmt.Errorf("%w: %s", err, msg)
		}
		exit <- fmt.Errorf("rigctld exited: %w", err)
	}()
	return exit, nil
}

// tailBuffer is an io.Writer that keeps the last stderrTail bytes written.
// It is safe for concurrent use.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

// Write appends p, discarding the oldest bytes beyond stderrTail.
func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if extra := len(b.buf) - stderrTail; extra > 0 {
		b.buf = append(b.buf[:0], b.buf[extra:]...)
	}
	return len(p), nil
}

// String returns the bytes kept.
func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
