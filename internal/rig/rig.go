package rig

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

// Default reconnect backoff: it doubles from min up to max and resets after
// a successful connect.
const (
	defaultBackoffMin = time.Second
	defaultBackoffMax = 30 * time.Second
)

// ErrNotConnected is returned (possibly wrapped) by SetFreq and SetMode when
// there is no connection to rigctld or Run has exited.
var ErrNotConnected = errors.New("rig: not connected")

// Options configures a Rig.
type Options struct {
	// Addr is the host:port of rigctld.
	Addr string
	// PollInterval is how often the rig is polled; it must be > 0.
	PollInterval time.Duration
	// Spawn, if non-nil, makes Run start rigctld itself. Nil means connect to
	// an already-running rigctld.
	Spawn *Spawn
}

// Spawn describes how to start rigctld.
type Spawn struct {
	// Model is the Hamlib model number (-m).
	Model int
	// Device is the serial device (-r).
	Device string
	// Baud is the serial speed (-s); 0 omits the flag.
	Baud int
}

// Rig controls a radio through rigctld. A single goroutine inside Run owns
// the connection; SetFreq and SetMode hand it requests. Create one with New.
type Rig struct {
	opts    Options
	reqs    chan request
	done    chan struct{} // closed when Run returns
	started atomic.Bool

	// Overridable in tests.
	timeout     time.Duration // per request and per dial
	backoffMin  time.Duration
	backoffMax  time.Duration
	rigctldPath string
	dial        func(ctx context.Context, addr string) (net.Conn, error)
}

// request is a command handed to the actor goroutine.
type request struct {
	command string // e.g. "set_freq 14074000", for CommandResult
	do      func(ctx context.Context, c *client) error
	reply   chan error // buffered; receives exactly one value
}

// New returns a Rig for opts. Call Run to connect.
func New(opts Options) *Rig {
	r := &Rig{
		opts:        opts,
		reqs:        make(chan request),
		done:        make(chan struct{}),
		timeout:     defaultRequestTimeout,
		backoffMin:  defaultBackoffMin,
		backoffMax:  defaultBackoffMax,
		rigctldPath: "rigctld",
	}
	r.dial = func(ctx context.Context, addr string) (net.Conn, error) {
		d := net.Dialer{Timeout: r.timeout}
		return d.DialContext(ctx, "tcp", addr)
	}
	return r
}

// Run connects to rigctld (starting it first if Options.Spawn is set), polls
// the rig and executes commands until ctx is done, reconnecting with
// backoff when the connection fails. Events (StateChanged,
// ConnectionChanged, CommandResult) are passed to emit, always from Run's
// goroutine. Run returns nil when ctx is cancelled and an error only on
// fatal setup failures. It may be called only once.
func (r *Rig) Run(ctx context.Context, emit func(any)) error {
	if !r.started.CompareAndSwap(false, true) {
		return errors.New("rig: Run called more than once")
	}
	defer close(r.done)
	if r.opts.PollInterval <= 0 {
		return fmt.Errorf("rig: poll interval %v must be positive", r.opts.PollInterval)
	}
	a := &actor{r: r, emit: emit}
	if r.opts.Spawn != nil {
		exit, err := r.startRigctld(ctx)
		if err != nil {
			return err
		}
		a.procExit = exit
		// loop returns only once ctx is done, which stops rigctld; wait for
		// it unless its exit was already seen.
		defer func() {
			if a.procExit != nil {
				<-a.procExit //nolint:errcheck // rigctld's exit error is expected: ctx stopped it
			}
		}()
	}
	return a.loop(ctx)
}

// SetFreq tunes the rig to hz. It returns once rigctld has answered, ctx is
// done or Run has exited.
func (r *Rig) SetFreq(ctx context.Context, hz int64) error {
	if err := validateFreq(hz); err != nil {
		return err
	}
	return r.send(ctx, request{
		command: "set_freq " + strconv.FormatInt(hz, 10),
		do:      func(ctx context.Context, c *client) error { return c.setFreq(ctx, hz) },
	})
}

// SetMode sets the rig's mode (one of Modes, case-insensitive) and passband
// in hertz (0 for the rig's default).
func (r *Rig) SetMode(ctx context.Context, mode string, passband int) error {
	m, err := normalizeMode(mode, passband)
	if err != nil {
		return err
	}
	return r.send(ctx, request{
		command: fmt.Sprintf("set_mode %s %d", m, passband),
		do:      func(ctx context.Context, c *client) error { return c.setMode(ctx, m, passband) },
	})
}

// send hands req to the actor and waits for its reply.
func (r *Rig) send(ctx context.Context, req request) error {
	req.reply = make(chan error, 1)
	select {
	case r.reqs <- req:
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", req.command, ctx.Err())
	case <-r.done:
		return fmt.Errorf("%s: %w: rig control stopped", req.command, ErrNotConnected)
	}
	// The actor always replies to a request it has accepted.
	select {
	case err := <-req.reply:
		return err
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", req.command, ctx.Err())
	}
}

// actor is the state owned by Run's goroutine.
type actor struct {
	r    *Rig
	emit func(any)
	last State // last state emitted
	// procExit delivers the spawned rigctld's exit; nil when not spawned or
	// once the exit has been seen.
	procExit <-chan error
}

// loop connects, serves and reconnects until ctx is done.
func (a *actor) loop(ctx context.Context) error {
	backoff := a.r.backoffMin
	for {
		a.emit(ConnectionChanged{Status: Connecting})
		conn, err := a.r.dial(ctx, a.r.opts.Addr)
		if err == nil {
			backoff = a.r.backoffMin
			a.emit(ConnectionChanged{Status: Connected})
			err = a.serve(ctx, newClient(conn, a.r.timeout))
		} else {
			err = fmt.Errorf("connect to rigctld at %s: %w", a.r.opts.Addr, err)
		}
		if ctx.Err() != nil {
			return nil
		}
		a.emit(ConnectionChanged{Status: Disconnected, Err: err, RetryIn: backoff})
		if !a.sleep(ctx, backoff) {
			return nil
		}
		backoff = min(backoff*2, a.r.backoffMax)
	}
}

// serve polls and runs requests on c until an error or ctx is done. It
// closes c before returning.
func (a *actor) serve(ctx context.Context, c *client) (err error) {
	// Closing on cancel interrupts a pending read.
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer func() {
		if stop() {
			if cerr := c.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("close rigctld connection: %w", cerr)
			}
		}
	}()

	if err := a.poll(ctx, c, true); err != nil {
		return err
	}
	ticker := time.NewTicker(a.r.opts.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := a.poll(ctx, c, false); err != nil {
				return err
			}
		case req := <-a.r.reqs:
			err := req.do(ctx, c)
			a.emit(CommandResult{Command: req.command, Err: err})
			req.reply <- err
			if isConnErr(err) {
				return err
			}
			if err := a.poll(ctx, c, false); err != nil {
				return err
			}
		case err := <-a.procExit:
			a.procExit = nil
			return err
		}
	}
}

// poll reads the rig state and emits it if it changed or force is set. Any
// error, including an RPRT error, is returned so the caller reconnects: a
// rig that cannot report its state is treated as unavailable.
func (a *actor) poll(ctx context.Context, c *client, force bool) error {
	s, err := c.getState(ctx)
	if err != nil {
		return fmt.Errorf("poll rig: %w", err)
	}
	if force || s != a.last {
		a.last = s
		a.emit(StateChanged{State: s})
	}
	return nil
}

// sleep waits d while rejecting requests, since there is no connection. It
// returns false if ctx is done first.
func (a *actor) sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		case req := <-a.r.reqs:
			err := fmt.Errorf("%s: %w", req.command, ErrNotConnected)
			a.emit(CommandResult{Command: req.command, Err: err})
			req.reply <- err
		case err := <-a.procExit:
			a.procExit = nil
			a.emit(ConnectionChanged{Status: Disconnected, Err: err, RetryIn: max(d-time.Since(start), 0)})
		}
	}
}
