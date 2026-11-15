package config

import (
	"fmt"
	"os"
	"runtime"
)

// Environment variables that override callbook passwords from the config
// file.
const (
	// EnvQRZPassword overrides callbook.qrz.password.
	EnvQRZPassword = "JSTLOGGER_QRZ_PASSWORD"
	// EnvHamQTHPassword overrides callbook.hamqth.password.
	EnvHamQTHPassword = "JSTLOGGER_HAMQTH_PASSWORD"
)

// ApplyEnv overrides config values from the environment, read through
// getenv. A non-empty EnvQRZPassword or EnvHamQTHPassword replaces the
// matching callbook password; an empty one leaves the file's value.
func (c *Config) ApplyEnv(getenv func(string) string) {
	if pw := getenv(EnvQRZPassword); pw != "" {
		c.Callbook.QRZ.Password = pw
	}
	if pw := getenv(EnvHamQTHPassword); pw != "" {
		c.Callbook.HamQTH.Password = pw
	}
}

// PermissionWarning returns a warning when the config file at path is
// readable by group or others and c, as loaded from that file before
// ApplyEnv, holds a callbook password. It returns "" otherwise, and always
// on Windows, where Unix file modes do not apply.
func PermissionWarning(path string, c Config) (string, error) {
	if runtime.GOOS == "windows" {
		return "", nil
	}
	if c.Callbook.QRZ.Password == "" && c.Callbook.HamQTH.Password == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("checking config file permissions: %w", err)
	}
	if info.Mode().Perm()&0o077 == 0 {
		return "", nil
	}
	return fmt.Sprintf("readable by group or others (mode %04o) and holds callbook passwords; run chmod 600 on it", info.Mode().Perm()), nil
}
