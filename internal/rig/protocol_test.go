package rig

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pipeClient returns a client whose server answers the first request line
// with reply. The received line is sent on the returned channel.
func pipeClient(t *testing.T, reply string) (*client, <-chan string) {
	t.Helper()
	cc, sc := net.Pipe()
	got := make(chan string, 1)
	go func() {
		defer sc.Close()
		line, err := bufio.NewReader(sc).ReadString('\n')
		if err != nil {
			return
		}
		got <- line
		_, _ = sc.Write([]byte(reply))
	}()
	c := newClient(cc, time.Second)
	t.Cleanup(func() { _ = c.Close() })
	return c, got
}

func TestClientGetFreq(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		want    int64
		wantErr bool
	}{
		{"ok", "get_freq:\nFrequency: 14074000\nRPRT 0\n", 14074000, false},
		{"float", "get_freq:\nFrequency: 7074000.000000\nRPRT 0\n", 7074000, false},
		{"rprt error", "get_freq:\nRPRT -5\n", 0, true},
		{"missing key", "get_freq:\nRPRT 0\n", 0, true},
		{"bad value", "get_freq:\nFrequency: abc\nRPRT 0\n", 0, true},
		{"wrong echo", "get_mode:\nFrequency: 1\nRPRT 0\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, sent := pipeClient(t, tt.reply)
			got, err := c.getFreq(context.Background())
			assert.Equal(t, "+\\get_freq\n", <-sent)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClientGetMode(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		mode    string
		pb      int
		wantErr bool
	}{
		{"ok", "get_mode:\nMode: USB\nPassband: 2400\nRPRT 0\n", "USB", 2400, false},
		{"rprt error", "get_mode:\nRPRT -11\n", "", 0, true},
		{"bare rprt", "RPRT -4\n", "", 0, true},
		{"missing passband", "get_mode:\nMode: USB\nRPRT 0\n", "", 0, true},
		{"bad passband", "get_mode:\nMode: USB\nPassband: x\nRPRT 0\n", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, sent := pipeClient(t, tt.reply)
			mode, pb, err := c.getMode(context.Background())
			assert.Equal(t, "+\\get_mode\n", <-sent)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.mode, mode)
			assert.Equal(t, tt.pb, pb)
		})
	}
}

func TestClientSet(t *testing.T) {
	t.Run("set_freq", func(t *testing.T) {
		c, sent := pipeClient(t, "set_freq: 7074000\nRPRT 0\n")
		require.NoError(t, c.setFreq(context.Background(), 7074000))
		assert.Equal(t, "+\\set_freq 7074000\n", <-sent)
	})
	t.Run("set_mode normalises case", func(t *testing.T) {
		c, sent := pipeClient(t, "set_mode: LSB 1800\nRPRT 0\n")
		require.NoError(t, c.setMode(context.Background(), "lsb", 1800))
		assert.Equal(t, "+\\set_mode LSB 1800\n", <-sent)
	})
}

func TestRPRTError(t *testing.T) {
	c, _ := pipeClient(t, "set_freq: 7074000\nRPRT -9\n")
	err := c.setFreq(context.Background(), 7074000)
	var rerr *RPRTError
	require.ErrorAs(t, err, &rerr)
	assert.Equal(t, -9, rerr.Code)
	assert.False(t, isConnErr(err), "an RPRT error does not break the connection")

	tests := []struct {
		code int
		want string
	}{
		{-1, "invalid parameter"},
		{-4, "not implemented"},
		{-5, "timeout"},
		{-6, "I/O error"},
		{-8, "protocol error"},
		{-9, "command rejected"},
		{-11, "not available"},
		{-42, "error -42"},
	}
	for _, tt := range tests {
		assert.Contains(t, (&RPRTError{Code: tt.code}).Error(), tt.want)
	}
}

func TestClientValidation(t *testing.T) {
	tests := []struct {
		name string
		call func(c *client) error
	}{
		{"zero freq", func(c *client) error { return c.setFreq(context.Background(), 0) }},
		{"negative freq", func(c *client) error { return c.setFreq(context.Background(), -1) }},
		{"unknown mode", func(c *client) error { return c.setMode(context.Background(), "XYZ", 0) }},
		{"empty mode", func(c *client) error { return c.setMode(context.Background(), "", 0) }},
		{"negative passband", func(c *client) error { return c.setMode(context.Background(), "USB", -1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeRigctld(t)
			conn, err := net.Dial("tcp", f.Addr())
			require.NoError(t, err)
			c := newClient(conn, time.Second)
			defer c.Close()

			err = tt.call(c)
			require.ErrorIs(t, err, ErrInvalidArgument)
			assert.False(t, isConnErr(err))
			// A follow-up request proves nothing was sent before it.
			_, err = c.getFreq(context.Background())
			require.NoError(t, err)
			assert.Equal(t, []string{`+\get_freq`}, f.Commands())
		})
	}
}

func TestClientTimeout(t *testing.T) {
	f := newFakeRigctld(t)
	f.SetHang(true)
	conn, err := net.Dial("tcp", f.Addr())
	require.NoError(t, err)
	c := newClient(conn, 50*time.Millisecond)
	defer c.Close()

	start := time.Now()
	_, err = c.getFreq(context.Background())
	require.Error(t, err)
	assert.True(t, isConnErr(err))
	var nerr net.Error
	assert.True(t, errors.As(err, &nerr) && nerr.Timeout(), "want a timeout, got %v", err)
	assert.Less(t, time.Since(start), time.Second)
}

func TestClientContextDeadline(t *testing.T) {
	f := newFakeRigctld(t)
	f.SetHang(true)
	conn, err := net.Dial("tcp", f.Addr())
	require.NoError(t, err)
	c := newClient(conn, time.Minute)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = c.getFreq(ctx)
	require.Error(t, err)
	assert.True(t, isConnErr(err))
}

func TestModesContainsCommon(t *testing.T) {
	for _, m := range []string{"USB", "LSB", "CW", "PKTUSB", "FM", "AM", "RTTY"} {
		assert.Contains(t, Modes, m)
	}
}
