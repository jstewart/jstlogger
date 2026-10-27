package rig

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Modes lists the Hamlib mode names accepted by SetMode.
var Modes = []string{
	"USB", "LSB", "CW", "CWR", "RTTY", "RTTYR", "AM", "FM", "WFM", "AMS",
	"PKTLSB", "PKTUSB", "PKTFM", "PKTAM", "ECSSUSB", "ECSSLSB", "FAX",
	"SAM", "SAL", "SAH", "DSB", "FMN",
}

// ErrInvalidArgument is returned (wrapped) when a command argument fails
// client-side validation. Nothing is sent to rigctld in that case.
var ErrInvalidArgument = errors.New("invalid argument")

// defaultRequestTimeout bounds every request to rigctld.
const defaultRequestTimeout = 2 * time.Second

// maxReplyLines bounds a reply so a confused server cannot keep us reading.
const maxReplyLines = 32

// RPRTError is a non-zero RPRT code returned by rigctld. The connection is
// still usable after one.
type RPRTError struct {
	// Code is the Hamlib error code (negative).
	Code int
}

// rprtNames maps Hamlib error codes to their descriptions.
var rprtNames = map[int]string{
	-1:  "invalid parameter",
	-2:  "invalid configuration",
	-3:  "out of memory",
	-4:  "not implemented",
	-5:  "timeout",
	-6:  "I/O error",
	-7:  "internal error",
	-8:  "protocol error",
	-9:  "command rejected",
	-10: "argument truncated",
	-11: "not available",
	-12: "VFO not targetable",
}

// Error describes the Hamlib error.
func (e *RPRTError) Error() string {
	if name, ok := rprtNames[e.Code]; ok {
		return fmt.Sprintf("rigctld: %s (RPRT %d)", name, e.Code)
	}
	return fmt.Sprintf("rigctld: error %d", e.Code)
}

// isConnErr reports whether err leaves the connection unusable. Validation
// and RPRT errors do not; I/O, timeout and framing errors do.
func isConnErr(err error) bool {
	if err == nil {
		return false
	}
	var rerr *RPRTError
	return !errors.As(err, &rerr) && !errors.Is(err, ErrInvalidArgument)
}

// validateFreq checks a frequency in hertz.
func validateFreq(hz int64) error {
	if hz <= 0 {
		return fmt.Errorf("frequency %d Hz: %w", hz, ErrInvalidArgument)
	}
	return nil
}

// normalizeMode upper-cases mode and checks it and passband.
func normalizeMode(mode string, passband int) (string, error) {
	m := strings.ToUpper(strings.TrimSpace(mode))
	if !slices.Contains(Modes, m) {
		return "", fmt.Errorf("mode %q: %w", mode, ErrInvalidArgument)
	}
	if passband < 0 {
		return "", fmt.Errorf("passband %d Hz: %w", passband, ErrInvalidArgument)
	}
	return m, nil
}

// client speaks rigctld's extended response protocol on one connection. It
// is not safe for concurrent use.
type client struct {
	conn    net.Conn
	r       *bufio.Reader
	timeout time.Duration
}

// newClient wraps conn; timeout bounds each request.
func newClient(conn net.Conn, timeout time.Duration) *client {
	return &client{conn: conn, r: bufio.NewReader(conn), timeout: timeout}
}

// Close closes the connection.
func (c *client) Close() error {
	return c.conn.Close()
}

// do sends `+\cmd args` and returns the reply's Key: value pairs. Only
// well-formed commands may be sent: rigctld does not answer unknown ones.
func (c *client) do(ctx context.Context, cmd string, args ...string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", cmd, err)
	}
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("%s: set deadline: %w", cmd, err)
	}

	line := `+\` + strings.Join(append([]string{cmd}, args...), " ") + "\n"
	if _, err := c.conn.Write([]byte(line)); err != nil {
		return nil, fmt.Errorf("%s: write: %w", cmd, err)
	}

	vals := map[string]string{}
	for i := range maxReplyLines {
		raw, err := c.r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("%s: read: %w", cmd, err)
		}
		l := strings.TrimRight(raw, "\r\n")
		if code, ok := strings.CutPrefix(l, "RPRT "); ok {
			n, err := strconv.Atoi(strings.TrimSpace(code))
			if err != nil {
				return nil, fmt.Errorf("%s: bad RPRT line %q: %w", cmd, l, err)
			}
			if n != 0 {
				return nil, fmt.Errorf("%s: %w", cmd, &RPRTError{Code: n})
			}
			return vals, nil
		}
		if i == 0 {
			// The echo of the command, e.g. "get_freq:" or "set_freq: 7074000".
			if !strings.HasPrefix(l, cmd+":") {
				return nil, fmt.Errorf("%s: unexpected reply %q", cmd, l)
			}
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			return nil, fmt.Errorf("%s: unexpected reply line %q", cmd, l)
		}
		vals[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return nil, fmt.Errorf("%s: reply longer than %d lines", cmd, maxReplyLines)
}

// field returns vals[key] or an error naming the missing key.
func field(cmd string, vals map[string]string, key string) (string, error) {
	v, ok := vals[key]
	if !ok {
		return "", fmt.Errorf("%s: reply has no %s", cmd, key)
	}
	return v, nil
}

// getFreq returns the dial frequency in hertz.
func (c *client) getFreq(ctx context.Context) (int64, error) {
	vals, err := c.do(ctx, "get_freq")
	if err != nil {
		return 0, err
	}
	v, err := field("get_freq", vals, "Frequency")
	if err != nil {
		return 0, err
	}
	if hz, err := strconv.ParseInt(v, 10, 64); err == nil {
		return hz, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("get_freq: parse frequency %q: %w", v, err)
	}
	return int64(f), nil
}

// getMode returns the mode and passband in hertz.
func (c *client) getMode(ctx context.Context) (string, int, error) {
	vals, err := c.do(ctx, "get_mode")
	if err != nil {
		return "", 0, err
	}
	mode, err := field("get_mode", vals, "Mode")
	if err != nil {
		return "", 0, err
	}
	v, err := field("get_mode", vals, "Passband")
	if err != nil {
		return "", 0, err
	}
	pb, err := strconv.Atoi(v)
	if err != nil {
		return "", 0, fmt.Errorf("get_mode: parse passband %q: %w", v, err)
	}
	return mode, pb, nil
}

// getState polls frequency and mode.
func (c *client) getState(ctx context.Context) (State, error) {
	hz, err := c.getFreq(ctx)
	if err != nil {
		return State{}, err
	}
	mode, pb, err := c.getMode(ctx)
	if err != nil {
		return State{}, err
	}
	return State{Freq: hz, Mode: mode, Passband: pb}, nil
}

// setFreq tunes to hz after validating it.
func (c *client) setFreq(ctx context.Context, hz int64) error {
	if err := validateFreq(hz); err != nil {
		return err
	}
	_, err := c.do(ctx, "set_freq", strconv.FormatInt(hz, 10))
	return err
}

// setMode sets mode and passband after validating them.
func (c *client) setMode(ctx context.Context, mode string, passband int) error {
	m, err := normalizeMode(mode, passband)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, "set_mode", m, strconv.Itoa(passband))
	return err
}
