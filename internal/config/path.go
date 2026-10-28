package config

import (
	"errors"
	"path/filepath"
)

// DefaultPath returns the default config file location,
// $XDG_CONFIG_HOME/jstlogger/config.toml, falling back to
// $HOME/.config/jstlogger/config.toml when XDG_CONFIG_HOME is unset or empty.
// It deliberately ignores os.UserConfigDir so the path is the same on every
// platform. getenv is usually os.Getenv; tests inject their own.
func DefaultPath(getenv func(string) string) (string, error) {
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		home := getenv("HOME")
		if home == "" {
			return "", errors.New("cannot determine config directory: neither $XDG_CONFIG_HOME nor $HOME is set")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "jstlogger", "config.toml"), nil
}

// DebugLogPath returns the location of the --debug log,
// $XDG_STATE_HOME/jstlogger/debug.log, falling back to
// $HOME/.local/state/jstlogger/debug.log when XDG_STATE_HOME is unset or
// empty. getenv is usually os.Getenv; tests inject their own.
func DebugLogPath(getenv func(string) string) (string, error) {
	base := getenv("XDG_STATE_HOME")
	if base == "" {
		home := getenv("HOME")
		if home == "" {
			return "", errors.New("cannot determine state directory: neither $XDG_STATE_HOME nor $HOME is set")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "jstlogger", "debug.log"), nil
}

// LogbookPath returns the default logbook database location,
// $XDG_DATA_HOME/jstlogger/logbook.db, falling back to
// $HOME/.local/share/jstlogger/logbook.db when XDG_DATA_HOME is unset or
// empty. getenv is usually os.Getenv; tests inject their own.
func LogbookPath(getenv func(string) string) (string, error) {
	base := getenv("XDG_DATA_HOME")
	if base == "" {
		home := getenv("HOME")
		if home == "" {
			return "", errors.New("cannot determine data directory: neither $XDG_DATA_HOME nor $HOME is set")
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "jstlogger", "logbook.db"), nil
}
