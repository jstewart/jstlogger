package rig

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitTimeout bounds every wait for an event in these tests.
const waitTimeout = 3 * time.Second

// next returns the next event or fails the test.
func next(t *testing.T, ch <-chan any) any {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(waitTimeout):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}

// waitFor skips events until one of type T satisfies match (nil matches
// any) and returns it.
func waitFor[T any](t *testing.T, ch <-chan any, match func(T) bool) T {
	t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case ev := <-ch:
			if v, ok := ev.(T); ok && (match == nil || match(v)) {
				return v
			}
		case <-deadline:
			var zero T
			t.Fatalf("timed out waiting for %T", zero)
			return zero
		}
	}
}

// drain returns the events already buffered in ch.
func drain(ch <-chan any) []any {
	var evs []any
	for {
		select {
		case ev := <-ch:
			evs = append(evs, ev)
		default:
			return evs
		}
	}
}

// newTestRig returns a Rig with fast timeouts and backoff for tests.
func newTestRig(addr string, poll time.Duration) *Rig {
	r := New(Options{Addr: addr, PollInterval: poll})
	r.timeout = 500 * time.Millisecond
	r.backoffMin = 10 * time.Millisecond
	r.backoffMax = 40 * time.Millisecond
	return r
}

// startRig runs r in the background. Events arrive on the returned channel;
// Run's result arrives on the error channel. Run is stopped at cleanup.
func startRig(t *testing.T, r *Rig) (<-chan any, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan any, 1024)
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		done <- r.Run(ctx, func(ev any) { events <- ev })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(waitTimeout):
			t.Error("Run did not return after cancel")
		}
	})
	return events, cancel, done
}

// closedAddr returns an address nothing listens on.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func TestRunConnectsAndEmitsState(t *testing.T) {
	f := newFakeRigctld(t)
	events, _, _ := startRig(t, newTestRig(f.Addr(), time.Hour))

	assert.Equal(t, ConnectionChanged{Status: Connecting}, next(t, events))
	assert.Equal(t, ConnectionChanged{Status: Connected}, next(t, events))
	assert.Equal(t, StateChanged{State: f.State()}, next(t, events))
}

func TestRunNoDuplicateStateChanged(t *testing.T) {
	f := newFakeRigctld(t)
	events, _, _ := startRig(t, newTestRig(f.Addr(), 5*time.Millisecond))
	waitFor[StateChanged](t, events, nil)

	n := len(f.Commands())
	require.Eventually(t, func() bool { return len(f.Commands()) >= n+10 },
		waitTimeout, time.Millisecond, "rig should keep polling")
	for _, ev := range drain(events) {
		_, dup := ev.(StateChanged)
		assert.False(t, dup, "unexpected event %#v", ev)
	}
}

func TestRunReportsExternalChange(t *testing.T) {
	f := newFakeRigctld(t)
	events, _, _ := startRig(t, newTestRig(f.Addr(), 5*time.Millisecond))
	waitFor[StateChanged](t, events, nil)

	want := State{Freq: 7074000, Mode: "LSB", Passband: 1800}
	f.SetState(want)
	got := waitFor[StateChanged](t, events, nil)
	assert.Equal(t, want, got.State)
}

func TestSetFreq(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), time.Hour) // only a prompt poll can report the change
	events, _, _ := startRig(t, r)
	waitFor[StateChanged](t, events, nil)

	require.NoError(t, r.SetFreq(context.Background(), 14200000))
	assert.Equal(t, int64(14200000), f.State().Freq)
	assert.Equal(t, CommandResult{Command: "set_freq 14200000"}, waitFor[CommandResult](t, events, nil))
	got := waitFor[StateChanged](t, events, nil)
	assert.Equal(t, int64(14200000), got.State.Freq)
}

func TestSetMode(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), time.Hour)
	events, _, _ := startRig(t, r)
	waitFor[StateChanged](t, events, nil)

	require.NoError(t, r.SetMode(context.Background(), "pktusb", 3000))
	assert.Equal(t, CommandResult{Command: "set_mode PKTUSB 3000"}, waitFor[CommandResult](t, events, nil))
	got := waitFor[StateChanged](t, events, nil)
	assert.Equal(t, State{Freq: 14074000, Mode: "PKTUSB", Passband: 3000}, got.State)
}

func TestSetValidation(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), time.Hour)
	events, _, _ := startRig(t, r)
	waitFor[StateChanged](t, events, nil)
	n := len(f.Commands())

	assert.ErrorIs(t, r.SetFreq(context.Background(), 0), ErrInvalidArgument)
	assert.ErrorIs(t, r.SetMode(context.Background(), "BOGUS", 0), ErrInvalidArgument)
	assert.ErrorIs(t, r.SetMode(context.Background(), "USB", -5), ErrInvalidArgument)
	assert.Len(t, f.Commands(), n, "nothing should be sent")
}

func TestSetFreqRPRTError(t *testing.T) {
	f := newFakeRigctld(t)
	f.SetError("set_freq", -9)
	r := newTestRig(f.Addr(), time.Hour)
	events, _, _ := startRig(t, r)
	waitFor[StateChanged](t, events, nil)

	err := r.SetFreq(context.Background(), 7000000)
	var rerr *RPRTError
	require.ErrorAs(t, err, &rerr)
	assert.Equal(t, -9, rerr.Code)
	res := waitFor[CommandResult](t, events, nil)
	assert.Equal(t, "set_freq 7000000", res.Command)
	assert.ErrorAs(t, res.Err, &rerr)

	// The connection survives an RPRT error.
	require.NoError(t, r.SetMode(context.Background(), "CW", 500))
	for _, ev := range drain(events) {
		if cc, ok := ev.(ConnectionChanged); ok {
			t.Errorf("unexpected %#v", cc)
		}
	}
}

func TestRunReconnectsAfterDrop(t *testing.T) {
	f := newFakeRigctld(t)
	events, _, _ := startRig(t, newTestRig(f.Addr(), 5*time.Millisecond))
	waitFor[StateChanged](t, events, nil)

	f.DropConns()
	dc := waitFor[ConnectionChanged](t, events, nil)
	assert.Equal(t, Disconnected, dc.Status)
	assert.Error(t, dc.Err)
	assert.Equal(t, 10*time.Millisecond, dc.RetryIn)
	assert.Equal(t, ConnectionChanged{Status: Connecting}, next(t, events))
	assert.Equal(t, ConnectionChanged{Status: Connected}, next(t, events))
	// The unchanged state is re-emitted so the UI refreshes.
	assert.Equal(t, StateChanged{State: f.State()}, next(t, events))
}

func TestRunDisconnectsOnTimeout(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), 5*time.Millisecond)
	r.timeout = 50 * time.Millisecond
	events, _, _ := startRig(t, r)
	waitFor[StateChanged](t, events, nil)

	f.SetHang(true)
	dc := waitFor[ConnectionChanged](t, events, func(c ConnectionChanged) bool { return c.Status == Disconnected })
	var nerr net.Error
	assert.True(t, errors.As(dc.Err, &nerr) && nerr.Timeout(), "want timeout, got %v", dc.Err)
}

func TestRunBackoff(t *testing.T) {
	events, _, _ := startRig(t, newTestRig(closedAddr(t), time.Hour))

	var got []time.Duration
	for len(got) < 5 {
		dc := waitFor[ConnectionChanged](t, events, func(c ConnectionChanged) bool { return c.Status == Disconnected })
		require.Error(t, dc.Err)
		got = append(got, dc.RetryIn)
	}
	ms := time.Millisecond
	assert.Equal(t, []time.Duration{10 * ms, 20 * ms, 40 * ms, 40 * ms, 40 * ms}, got)
}

func TestSetFreqUnreachable(t *testing.T) {
	r := newTestRig(closedAddr(t), time.Hour)
	r.backoffMin, r.backoffMax = time.Minute, time.Minute
	events, _, _ := startRig(t, r)
	waitFor[ConnectionChanged](t, events, func(c ConnectionChanged) bool { return c.Status == Disconnected })

	start := time.Now()
	err := r.SetFreq(context.Background(), 14074000)
	assert.ErrorIs(t, err, ErrNotConnected)
	assert.Less(t, time.Since(start), time.Second)
}

func TestSetFreqWithoutRun(t *testing.T) {
	r := newTestRig(closedAddr(t), time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, r.SetFreq(ctx, 14074000), context.DeadlineExceeded)
}

func TestSetFreqAfterRunExits(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), time.Hour)
	events, cancel, done := startRig(t, r)
	waitFor[StateChanged](t, events, nil)
	cancel()
	require.NoError(t, <-done)

	assert.ErrorIs(t, r.SetFreq(context.Background(), 14074000), ErrNotConnected)
}

func TestRunReturnsNilOnCancel(t *testing.T) {
	f := newFakeRigctld(t)
	events, cancel, done := startRig(t, newTestRig(f.Addr(), 5*time.Millisecond))
	waitFor[StateChanged](t, events, nil)

	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return")
	}
}

func TestRunReturnsNilOnCancelWhileHung(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), 5*time.Millisecond)
	r.timeout = time.Minute
	events, cancel, done := startRig(t, r)
	waitFor[StateChanged](t, events, nil)
	f.SetHang(true)
	n := len(f.Commands())
	require.Eventually(t, func() bool { return len(f.Commands()) > n }, waitTimeout, time.Millisecond)

	start := time.Now()
	cancel()
	assert.NoError(t, <-done)
	assert.Less(t, time.Since(start), time.Second, "cancel should interrupt a pending read")
}

func TestRunTwice(t *testing.T) {
	f := newFakeRigctld(t)
	r := newTestRig(f.Addr(), time.Hour)
	events, _, _ := startRig(t, r)
	waitFor[StateChanged](t, events, nil)
	assert.Error(t, r.Run(context.Background(), func(any) {}))
}
