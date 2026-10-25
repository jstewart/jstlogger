package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig is a minimal config that passes validation, with a rig profile
// so --rig and --no-rig can be exercised.
const validConfig = `
default_station = "home"

[stations.home]
callsign   = "N0CALL"
gridsquare = "CM87"
pota_references = ["US-0001"]

[stations.rover]
callsign   = "N0CALL"
gridsquare = "DM04"

[rig]
profile = "ic7300"

[rig.profiles.ic7300]
backend = "rigctld"
`

// env returns a getenv func backed by vars.
func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// writeConfig writes content to a temp config.toml and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// stubTUI replaces runTUI for the duration of the test and reports whether
// it was called.
func stubTUI(t *testing.T, err error) *bool {
	t.Helper()
	called := new(bool)
	orig := runTUI
	runTUI = func() error { *called = true; return err }
	t.Cleanup(func() { runTUI = orig })
	return called
}

// result captures one run invocation.
type result struct {
	code           int
	stdout, stderr string
	tuiCalled      bool
}

// invoke calls run with args and an environment pointing at an empty temp
// XDG_CONFIG_HOME (unless vars is non-nil), with the TUI stubbed.
func invoke(t *testing.T, vars map[string]string, tuiErr error, args ...string) result {
	t.Helper()
	if vars == nil {
		vars = map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
	}
	called := stubTUI(t, tuiErr)
	var stdout, stderr bytes.Buffer
	code := run(args, env(vars), &stdout, &stderr)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String(), tuiCalled: *called}
}

func TestRunNoFilesystemModes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStdout []string
	}{
		{name: "version", args: []string{"--version"}, wantStdout: []string{"jstlogger dev\n"}},
		{name: "print default config", args: []string{"--print-default-config"}, wantStdout: []string{"default_station", "[rig.profiles."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			xdg := t.TempDir()
			r := invoke(t, map[string]string{"XDG_CONFIG_HOME": xdg}, nil, tt.args...)
			assert.Equal(t, 0, r.code, r.stderr)
			for _, want := range tt.wantStdout {
				assert.Contains(t, r.stdout, want)
			}
			assert.Empty(t, r.stderr)
			assert.False(t, r.tuiCalled)
			assert.NoFileExists(t, filepath.Join(xdg, "jstlogger", "config.toml"))
		})
	}
}

func TestRunHelp(t *testing.T) {
	r := invoke(t, nil, nil, "--help")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stderr, "--print-default-config")
	assert.Contains(t, r.stderr, "-c, --config")
	assert.False(t, r.tuiCalled)
}

func TestRunFlagErrors(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "unknown flag", args: []string{"--bogus"}, wantCode: 2, wantStderr: "unknown flag"},
		{name: "mode flags exclusive", args: []string{"--version", "--check-config"}, wantCode: 2, wantStderr: "mutually exclusive"},
		{name: "three mode flags", args: []string{"--print-config", "--print-default-config", "--version"}, wantCode: 2, wantStderr: "mutually exclusive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := invoke(t, nil, nil, tt.args...)
			assert.Equal(t, tt.wantCode, r.code)
			assert.Contains(t, r.stderr, tt.wantStderr)
			assert.Empty(t, r.stdout)
			assert.False(t, r.tuiCalled)
		})
	}
}

func TestRunFirstRunCreatesConfig(t *testing.T) {
	tests := []struct {
		name     string
		vars     func(dir string) map[string]string
		wantPath func(dir string) string
	}{
		{
			name: "XDG_CONFIG_HOME",
			vars: func(dir string) map[string]string {
				return map[string]string{"XDG_CONFIG_HOME": dir, "HOME": "/nonexistent"}
			},
			wantPath: func(dir string) string { return filepath.Join(dir, "jstlogger", "config.toml") },
		},
		{
			name:     "HOME fallback",
			vars:     func(dir string) map[string]string { return map[string]string{"HOME": dir} },
			wantPath: func(dir string) string { return filepath.Join(dir, ".config", "jstlogger", "config.toml") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			want := tt.wantPath(dir)
			r := invoke(t, tt.vars(dir), nil, "--check-config")
			assert.Equal(t, 1, r.code)
			assert.FileExists(t, want)
			assert.Contains(t, r.stderr, "created "+want)
			assert.Contains(t, r.stderr, "jstlogger: "+want+": stations.home.callsign: required\n")
			assert.Contains(t, r.stderr, "jstlogger: "+want+": stations.home.gridsquare: required\n")
			assert.Contains(t, r.stderr, "edit "+want+" to set callsign and gridsquare")
			assert.Empty(t, r.stdout)
			assert.False(t, r.tuiCalled)
		})
	}
}

func TestRunNoConfigLocation(t *testing.T) {
	r := invoke(t, map[string]string{}, nil, "--check-config")
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, "jstlogger: ")
	assert.Contains(t, r.stderr, "cannot determine config directory")
}

func TestRunExplicitMissingConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	for _, flag := range []string{"-c", "--config"} {
		t.Run(flag, func(t *testing.T) {
			r := invoke(t, nil, nil, flag, p, "--check-config")
			assert.Equal(t, 1, r.code)
			assert.Contains(t, r.stderr, "jstlogger: ")
			assert.Contains(t, r.stderr, "not found")
			assert.NoFileExists(t, p)
			assert.False(t, r.tuiCalled)
		})
	}
}

func TestRunCheckConfig(t *testing.T) {
	p := writeConfig(t, validConfig)
	r := invoke(t, nil, nil, "-c", p, "--check-config")
	assert.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "config OK: "+p+"\n", r.stdout)
	assert.Empty(t, r.stderr)
	assert.False(t, r.tuiCalled)
}

func TestRunPrintConfigOverrides(t *testing.T) {
	p := writeConfig(t, validConfig)
	r := invoke(t, nil, nil, "-c", p, "--print-config",
		"-s", "rover", "--grid", "FN31pr", "--pota", "US-1,US-2", "--pota", "US-3", "--no-rig")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Empty(t, r.stderr)
	assert.Contains(t, r.stdout, `default_station = 'rover'`)
	assert.Contains(t, r.stdout, `gridsquare = 'FN31pr'`)
	assert.Contains(t, r.stdout, `pota_references = ['US-1', 'US-2', 'US-3']`)
	assert.Contains(t, r.stdout, `profile = ''`)
	assert.False(t, r.tuiCalled)
}

func TestRunValidationAndOverrideErrors(t *testing.T) {
	tests := []struct {
		name   string
		config string
		args   []string
		// fromFile reports whether the errors concern the config file and
		// so are prefixed with its path; flag errors are not.
		fromFile   bool
		wantStderr []string
	}{
		{
			name:       "rig and no-rig",
			config:     validConfig,
			args:       []string{"--rig", "ic7300", "--no-rig"},
			wantStderr: []string{"--rig: cannot be used with --no-rig"},
		},
		{
			name:       "bad grid",
			config:     validConfig,
			args:       []string{"--grid", "ZZ99"},
			wantStderr: []string{`--grid: "ZZ99" is not`},
		},
		{
			name:       "unknown station",
			config:     validConfig,
			args:       []string{"-s", "nope"},
			fromFile:   true,
			wantStderr: []string{`--station: no station "nope"`},
		},
		{
			name:       "invalid file",
			config:     "[stations.home]\ncallsign = \"N0CALL\"\n",
			fromFile:   true,
			wantStderr: []string{"stations.home.gridsquare: required"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeConfig(t, tt.config)
			args := append([]string{"-c", p, "--check-config"}, tt.args...)
			r := invoke(t, nil, nil, args...)
			assert.Equal(t, 1, r.code)
			prefix := "jstlogger: "
			if tt.fromFile {
				prefix += p + ": "
			}
			for _, want := range tt.wantStderr {
				assert.Contains(t, r.stderr, prefix+want)
			}
			assert.NotContains(t, r.stderr, "edit ")
			assert.Empty(t, r.stdout)
			assert.False(t, r.tuiCalled)
		})
	}
}

func TestRunUnknownKeyWarning(t *testing.T) {
	p := writeConfig(t, "bogus_key = 1\n"+validConfig)
	r := invoke(t, nil, nil, "-c", p, "--check-config")
	assert.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stderr, "jstlogger: warning: "+p+`: unknown key "bogus_key"`)
	assert.Equal(t, "config OK: "+p+"\n", r.stdout)
}

func TestRunStartsTUI(t *testing.T) {
	tests := []struct {
		name     string
		tuiErr   error
		wantCode int
	}{
		{name: "success", wantCode: 0},
		{name: "tui error", tuiErr: errors.New("boom"), wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeConfig(t, validConfig)
			r := invoke(t, nil, tt.tuiErr, "-c", p)
			assert.Equal(t, tt.wantCode, r.code, r.stderr)
			assert.True(t, r.tuiCalled)
			assert.Empty(t, r.stdout)
			if tt.tuiErr != nil {
				assert.Contains(t, r.stderr, "jstlogger: ")
				assert.Contains(t, r.stderr, "boom")
			} else {
				assert.Empty(t, r.stderr)
			}
		})
	}
}
