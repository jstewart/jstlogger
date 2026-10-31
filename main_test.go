package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/config"
	"jstlogger/internal/logbook"
	"jstlogger/internal/rig"
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

// tuiCall records the arguments of a stubbed runTUI call.
type tuiCall struct {
	called  bool
	session config.Session
	store   logbook.Store
	logger  *slog.Logger
}

// stubTUI replaces runTUI for the duration of the test and records its
// call. The stub logs "tui started" so tests can see where logs go.
func stubTUI(t *testing.T, err error) *tuiCall {
	t.Helper()
	call := new(tuiCall)
	orig := runTUI
	runTUI = func(ctx context.Context, session config.Session, store logbook.Store, logger *slog.Logger) error {
		require.NotNil(t, ctx)
		*call = tuiCall{called: true, session: session, store: store, logger: logger}
		logger.Info("tui started")
		return err
	}
	t.Cleanup(func() { runTUI = orig })
	return call
}

// result captures one run invocation.
type result struct {
	code           int
	stdout, stderr string
	tuiCalled      bool
	tui            tuiCall
}

// invoke calls run with args and an environment pointing at empty temp
// XDG_CONFIG_HOME and XDG_DATA_HOME directories (unless vars is non-nil), with
// the TUI stubbed.
func invoke(t *testing.T, vars map[string]string, tuiErr error, args ...string) result {
	t.Helper()
	if vars == nil {
		vars = map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir()}
	}
	call := stubTUI(t, tuiErr)
	var stdout, stderr bytes.Buffer
	code := run(args, env(vars), &stdout, &stderr)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String(), tuiCalled: call.called, tui: *call}
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
		{name: "positional argument", args: []string{"check-config"}, wantCode: 2, wantStderr: `unexpected argument "check-config"`},
		{name: "argument after flag value", args: []string{"--grid", "CM87", "extra"}, wantCode: 2, wantStderr: `unexpected argument "extra"`},
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

func TestRunModeFlagsExclusive(t *testing.T) {
	modes := [][]string{
		{"--check-config"},
		{"--print-config"},
		{"--print-default-config"},
		{"--version"},
		{"--import-adif", "in.adi"},
		{"--export-adif", "out.adi"},
	}
	for i, a := range modes {
		for _, b := range modes[i+1:] {
			t.Run(a[0]+" "+b[0], func(t *testing.T) {
				r := invoke(t, nil, nil, append(slices.Clone(a), b...)...)
				assert.Equal(t, 2, r.code)
				assert.Contains(t, r.stderr, "mutually exclusive")
				assert.Contains(t, r.stderr, a[0])
				assert.Contains(t, r.stderr, b[0])
				assert.Empty(t, r.stdout)
				assert.False(t, r.tuiCalled)
			})
		}
	}
}

func TestRunADIFFlagErrors(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "tag alone", args: []string{"--tag", "pota"}, wantStderr: "--tag requires --import-adif or --export-adif"},
		{name: "tag with another mode", args: []string{"--tag", "pota", "--check-config"}, wantStderr: "--tag requires --import-adif or --export-adif"},
		{name: "empty import path", args: []string{"--import-adif", ""}, wantStderr: "--import-adif requires a path"},
		{name: "empty export path", args: []string{"--export-adif="}, wantStderr: "--export-adif requires a path"},
		{name: "import without path", args: []string{"--import-adif"}, wantStderr: "needs an argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := invoke(t, nil, nil, tt.args...)
			assert.Equal(t, 2, r.code)
			assert.Contains(t, r.stderr, tt.wantStderr)
			assert.Empty(t, r.stdout)
			assert.False(t, r.tuiCalled)
		})
	}
}

func TestRunHelpDescribesTag(t *testing.T) {
	r := invoke(t, nil, nil, "--help")
	assert.Equal(t, 0, r.code)
	for _, want := range []string{"--import-adif", "--export-adif", "--tag", "duplicates are skipped and not tagged", "all of these tags"} {
		assert.Contains(t, r.stderr, want)
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
			assert.Contains(t, r.stderr, "jstlogger: invalid config file "+want+":\n"+
				"  stations.home.callsign: required\n"+
				"  stations.home.gridsquare: required\n")
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
		// so are listed under its path; flag errors are listed under
		// "invalid flags".
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
			header := "jstlogger: invalid flags:\n"
			if tt.fromFile {
				header = "jstlogger: invalid config file " + p + ":\n"
			}
			assert.True(t, strings.HasPrefix(r.stderr, header), "stderr %q should start with %q", r.stderr, header)
			assert.Equal(t, 1, strings.Count(r.stderr, "jstlogger: "), "only the header is prefixed")
			for _, want := range tt.wantStderr {
				assert.Contains(t, r.stderr, "\n  "+want)
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

func TestRunPassesSessionToTUI(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantEnabled bool
		wantProfile string
	}{
		{name: "default profile", wantEnabled: true, wantProfile: "ic7300"},
		{name: "no rig", args: []string{"--no-rig"}, wantEnabled: false, wantProfile: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeConfig(t, validConfig)
			r := invoke(t, nil, nil, append([]string{"-c", p}, tt.args...)...)
			require.Equal(t, 0, r.code, r.stderr)
			require.True(t, r.tuiCalled)
			assert.Equal(t, tt.wantEnabled, r.tui.session.RigEnabled)
			assert.Equal(t, tt.wantProfile, r.tui.session.RigProfileName)
			assert.Equal(t, "N0CALL", r.tui.session.Operator)
		})
	}
}

func TestRunDebugLog(t *testing.T) {
	p := writeConfig(t, validConfig)
	state := t.TempDir()
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir(), "XDG_STATE_HOME": state}

	r := invoke(t, vars, nil, "-c", p, "--debug")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Empty(t, r.stderr)

	data, err := os.ReadFile(filepath.Join(state, "jstlogger", "debug.log"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `msg="tui started"`)
}

func TestRunNoDebugLog(t *testing.T) {
	p := writeConfig(t, validConfig)
	state := t.TempDir()
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir(), "XDG_STATE_HOME": state}

	r := invoke(t, vars, nil, "-c", p)
	require.Equal(t, 0, r.code, r.stderr)
	require.True(t, r.tuiCalled)
	assert.NoFileExists(t, filepath.Join(state, "jstlogger", "debug.log"))
	assert.Empty(t, r.stderr, "logs must never reach the terminal")
}

func TestRunDebugLogError(t *testing.T) {
	p := writeConfig(t, validConfig)
	state := t.TempDir()
	// A file where the jstlogger directory should be makes the log unopenable.
	require.NoError(t, os.WriteFile(filepath.Join(state, "jstlogger"), nil, 0o644))
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": t.TempDir(), "XDG_STATE_HOME": state}

	r := invoke(t, vars, nil, "-c", p, "--debug")
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, "debug log")
	assert.False(t, r.tuiCalled)
}

func TestRunOpensLogbook(t *testing.T) {
	p := writeConfig(t, validConfig)
	data := t.TempDir()
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": data}

	r := invoke(t, vars, nil, "-c", p)
	require.Equal(t, 0, r.code, r.stderr)
	require.True(t, r.tuiCalled)
	assert.NotNil(t, r.tui.store)
	assert.FileExists(t, filepath.Join(data, "jstlogger", "logbook.db"))
}

func TestRunLogbookPathFromConfig(t *testing.T) {
	db := filepath.Join(t.TempDir(), "my.db")
	p := writeConfig(t, validConfig+"\n[logbook]\npath = \""+db+"\"\n")

	r := invoke(t, nil, nil, "-c", p)
	require.Equal(t, 0, r.code, r.stderr)
	assert.FileExists(t, db)
	assert.Equal(t, db, r.tui.session.Config.Logbook.Path)
}

func TestRunPrintConfigShowsLogbookPath(t *testing.T) {
	p := writeConfig(t, validConfig)
	data := t.TempDir()
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": data}

	r := invoke(t, vars, nil, "-c", p, "--print-config")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Contains(t, r.stdout, filepath.Join(data, "jstlogger", "logbook.db"))
	assert.NoFileExists(t, filepath.Join(data, "jstlogger", "logbook.db"), "--print-config must not create the logbook")
}

func TestRunLogbookPathError(t *testing.T) {
	p := writeConfig(t, validConfig)
	// Neither XDG_DATA_HOME nor HOME is set, so there is no default path.
	r := invoke(t, map[string]string{"XDG_CONFIG_HOME": t.TempDir()}, nil, "-c", p)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, "logbook")
	assert.False(t, r.tuiCalled)
}

func TestRunLogbookOpenError(t *testing.T) {
	p := writeConfig(t, validConfig)
	data := t.TempDir()
	// A file where the jstlogger directory should be makes the logbook unopenable.
	require.NoError(t, os.WriteFile(filepath.Join(data, "jstlogger"), nil, 0o644))
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": data}

	r := invoke(t, vars, nil, "-c", p)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, "opening logbook")
	assert.False(t, r.tuiCalled)
}

func TestRigOptions(t *testing.T) {
	base := config.Config{Rig: config.Rig{
		PollInterval: 2 * time.Second,
		Profiles: map[string]config.RigProfile{
			"ic7300": {Backend: config.BackendRigctld, Host: "localhost", Port: 4532},
			"spawned": {
				Backend: config.BackendRigctld, Host: "127.0.0.1", Port: 4533,
				Spawn: &config.Spawn{Model: 3073, Device: "/dev/ttyUSB0", Baud: 19200},
			},
			"ipv6": {Backend: config.BackendRigctld, Host: "::1", Port: 4532},
		},
	}}
	tests := []struct {
		name    string
		session config.Session
		want    rig.Options
		wantOK  bool
	}{
		{
			name:    "disabled",
			session: config.Session{RigEnabled: false, RigProfileName: "ic7300", Config: base},
		},
		{
			name:    "rigctld",
			session: config.Session{RigEnabled: true, RigProfileName: "ic7300", Config: base},
			want:    rig.Options{Addr: "localhost:4532", PollInterval: 2 * time.Second},
			wantOK:  true,
		},
		{
			name:    "spawn",
			session: config.Session{RigEnabled: true, RigProfileName: "spawned", Config: base},
			want: rig.Options{
				Addr: "127.0.0.1:4533", PollInterval: 2 * time.Second,
				Spawn: &rig.Spawn{Model: 3073, Device: "/dev/ttyUSB0", Baud: 19200},
			},
			wantOK: true,
		},
		{
			name:    "ipv6 host",
			session: config.Session{RigEnabled: true, RigProfileName: "ipv6", Config: base},
			want:    rig.Options{Addr: "[::1]:4532", PollInterval: 2 * time.Second},
			wantOK:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := rigOptions(tt.session)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
