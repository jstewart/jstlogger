package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/config"
	"jstlogger/internal/rig"
)

func TestOpenDebugLog(t *testing.T) {
	state := t.TempDir()
	logger, closeLog, err := openDebugLog(env(map[string]string{"XDG_STATE_HOME": state}))
	require.NoError(t, err)
	logger.Info("hello")
	require.NoError(t, closeLog())

	data, err := os.ReadFile(filepath.Join(state, "jstlogger", "debug.log"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "msg=hello")
}

func TestOpenDebugLogAppends(t *testing.T) {
	getenv := env(map[string]string{"XDG_STATE_HOME": t.TempDir()})
	for _, msg := range []string{"first", "second"} {
		logger, closeLog, err := openDebugLog(getenv)
		require.NoError(t, err)
		logger.Info(msg)
		require.NoError(t, closeLog())
	}

	path, err := config.DebugLogPath(getenv)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "msg=first")
	assert.Contains(t, string(data), "msg=second")
}

func TestOpenDebugLogNoLocation(t *testing.T) {
	_, _, err := openDebugLog(env(nil))
	assert.ErrorContains(t, err, "debug log")
}

func TestOpenDebugLogUnwritable(t *testing.T) {
	// A file where the jstlogger directory should be makes MkdirAll fail.
	state := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(state, "jstlogger"), nil, 0o644))
	_, _, err := openDebugLog(env(map[string]string{"XDG_STATE_HOME": state}))
	assert.ErrorContains(t, err, "debug log")
}

func TestLogEvent(t *testing.T) {
	tests := []struct {
		name  string
		event any
		want  []string
	}{
		{
			name:  "state",
			event: rig.StateChanged{State: rig.State{Freq: 14074000, Mode: "USB", Passband: 2400}},
			want:  []string{"level=INFO", `msg="rig state"`, "freq=14074000", "mode=USB", "passband=2400"},
		},
		{
			name:  "connected",
			event: rig.ConnectionChanged{Status: rig.Connected},
			want:  []string{"level=INFO", `msg="rig connection"`, "status=connected"},
		},
		{
			name:  "disconnected",
			event: rig.ConnectionChanged{Status: rig.Disconnected, Err: errors.New("refused"), RetryIn: 4 * time.Second},
			want:  []string{"level=WARN", "status=disconnected", "err=refused", "retry_in=4s"},
		},
		{
			name:  "command ok",
			event: rig.CommandResult{Command: "set_freq 14074000"},
			want:  []string{"level=INFO", `msg="rig command"`, `command="set_freq 14074000"`},
		},
		{
			name:  "command failed",
			event: rig.CommandResult{Command: "set_mode FOO 0", Err: errors.New("invalid mode")},
			want:  []string{"level=ERROR", `command="set_mode FOO 0"`, `err="invalid mode"`},
		},
		{
			name:  "unknown event",
			event: 42,
			want:  []string{"level=DEBUG", `msg="rig event"`, "event=42"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			logEvent(logger, tt.event)
			for _, w := range tt.want {
				assert.Contains(t, buf.String(), w)
			}
		})
	}
}
