package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEnv returns a getenv func backed by env.
func fakeEnv(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

func TestDefaultPath(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "XDG_CONFIG_HOME set",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg", "HOME": "/home/me"},
			want: "/xdg/jstlogger/config.toml",
		},
		{
			name: "XDG_CONFIG_HOME unset falls back to HOME",
			env:  map[string]string{"HOME": "/home/me"},
			want: "/home/me/.config/jstlogger/config.toml",
		},
		{
			name: "XDG_CONFIG_HOME empty falls back to HOME",
			env:  map[string]string{"XDG_CONFIG_HOME": "", "HOME": "/home/me"},
			want: "/home/me/.config/jstlogger/config.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultPath(fakeEnv(tt.env))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultPathNoHome(t *testing.T) {
	_, err := DefaultPath(fakeEnv(nil))
	assert.Error(t, err)
}

func TestDebugLogPath(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "XDG_STATE_HOME set",
			env:  map[string]string{"XDG_STATE_HOME": "/xdg", "HOME": "/home/me"},
			want: "/xdg/jstlogger/debug.log",
		},
		{
			name: "XDG_STATE_HOME unset falls back to HOME",
			env:  map[string]string{"HOME": "/home/me"},
			want: "/home/me/.local/state/jstlogger/debug.log",
		},
		{
			name: "XDG_STATE_HOME empty falls back to HOME",
			env:  map[string]string{"XDG_STATE_HOME": "", "HOME": "/home/me"},
			want: "/home/me/.local/state/jstlogger/debug.log",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DebugLogPath(fakeEnv(tt.env))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDebugLogPathNoHome(t *testing.T) {
	_, err := DebugLogPath(fakeEnv(nil))
	assert.Error(t, err)
}

func TestLogbookPath(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "XDG_DATA_HOME set",
			env:  map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/home/me"},
			want: "/xdg/jstlogger/logbook.db",
		},
		{
			name: "XDG_DATA_HOME unset falls back to HOME",
			env:  map[string]string{"HOME": "/home/me"},
			want: "/home/me/.local/share/jstlogger/logbook.db",
		},
		{
			name: "XDG_DATA_HOME empty falls back to HOME",
			env:  map[string]string{"XDG_DATA_HOME": "", "HOME": "/home/me"},
			want: "/home/me/.local/share/jstlogger/logbook.db",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LogbookPath(fakeEnv(tt.env))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLogbookPathNoHome(t *testing.T) {
	_, err := LogbookPath(fakeEnv(nil))
	assert.Error(t, err)
}
