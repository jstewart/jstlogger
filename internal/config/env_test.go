package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyEnv(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want Callbook
	}{
		{
			name: "no env keeps the file's passwords",
			want: Callbook{
				QRZ:    QRZAccount{Username: "W1AW", Password: "file-qrz"},
				HamQTH: CallbookAccount{Username: "w1aw", Password: "file-hamqth"},
			},
		},
		{
			name: "env overrides both passwords",
			vars: map[string]string{EnvQRZPassword: "env-qrz", EnvHamQTHPassword: "env-hamqth"},
			want: Callbook{
				QRZ:    QRZAccount{Username: "W1AW", Password: "env-qrz"},
				HamQTH: CallbookAccount{Username: "w1aw", Password: "env-hamqth"},
			},
		},
		{
			name: "empty env value is ignored",
			vars: map[string]string{EnvQRZPassword: "", EnvHamQTHPassword: "env-hamqth"},
			want: Callbook{
				QRZ:    QRZAccount{Username: "W1AW", Password: "file-qrz"},
				HamQTH: CallbookAccount{Username: "w1aw", Password: "env-hamqth"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Callbook: Callbook{
				QRZ:    QRZAccount{Username: "W1AW", Password: "file-qrz"},
				HamQTH: CallbookAccount{Username: "w1aw", Password: "file-hamqth"},
			}}
			c.ApplyEnv(func(k string) string { return tt.vars[k] })
			assert.Equal(t, tt.want, c.Callbook)
		})
	}
}

func TestPermissionWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	withQRZ := Config{Callbook: Callbook{QRZ: QRZAccount{Password: "secret"}}}
	withHamQTH := Config{Callbook: Callbook{HamQTH: CallbookAccount{Password: "secret"}}}
	tests := []struct {
		name     string
		mode     fs.FileMode
		config   Config
		wantWarn bool
	}{
		{name: "0644 with qrz password", mode: 0o644, config: withQRZ, wantWarn: true},
		{name: "0640 with hamqth password", mode: 0o640, config: withHamQTH, wantWarn: true},
		{name: "0604 with qrz password", mode: 0o604, config: withQRZ, wantWarn: true},
		{name: "0600 with password", mode: 0o600, config: withQRZ},
		{name: "0644 without passwords", mode: 0o644, config: Config{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			require.NoError(t, os.WriteFile(p, nil, 0o600))
			require.NoError(t, os.Chmod(p, tt.mode))

			msg, err := PermissionWarning(p, tt.config)
			require.NoError(t, err)
			if tt.wantWarn {
				assert.Contains(t, msg, "readable by group or others")
				assert.Contains(t, msg, "chmod 600")
			} else {
				assert.Empty(t, msg)
			}
		})
	}
}

func TestPermissionWarningStatError(t *testing.T) {
	withQRZ := Config{Callbook: Callbook{QRZ: QRZAccount{Password: "secret"}}}
	_, err := PermissionWarning(filepath.Join(t.TempDir(), "missing.toml"), withQRZ)
	if runtime.GOOS == "windows" {
		assert.NoError(t, err)
		return
	}
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}
