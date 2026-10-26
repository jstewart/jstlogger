package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeConfig writes content to config.toml in a fresh temp dir and returns
// its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// load runs Load on content as an explicit config file and requires success.
func load(t *testing.T, content string) (Config, LoadResult) {
	t.Helper()
	p := writeConfig(t, content)
	c, res, err := Load(p, true)
	require.NoError(t, err)
	return c, res
}

func TestLoadFirstRunCreatesTemplate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "xdg", "jstlogger")
	p := filepath.Join(dir, "config.toml")

	c, res, err := Load(p, false)
	require.NoError(t, err)

	assert.True(t, res.Created)
	assert.Equal(t, p, res.Path)
	assert.Empty(t, res.Warnings)

	got, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, DefaultTemplate, got)

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o644), fileInfo.Mode().Perm())

	assert.Equal(t, DefaultStationName, c.DefaultStation)
	assert.Equal(t, map[string]Station{"home": {Name: "Home QTH"}}, c.Stations)
	assert.Equal(t, DefaultPollInterval, c.Rig.PollInterval)
}

func TestLoadExplicitMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")

	_, _, err := Load(p, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.Contains(t, err.Error(), p)

	_, statErr := os.Stat(p)
	assert.True(t, errors.Is(statErr, fs.ErrNotExist), "explicit path must not be created")
}

func TestLoadNeverOverwrites(t *testing.T) {
	const content = "# my config\ndefault_station = \"home\"\n\n[stations.home]\ncallsign = \"N0CALL\"\n"
	p := writeConfig(t, content)

	for _, explicit := range []bool{false, true} {
		_, res, err := Load(p, explicit)
		require.NoError(t, err)
		assert.False(t, res.Created)

		got, err := os.ReadFile(p)
		require.NoError(t, err)
		assert.Equal(t, content, string(got))
	}
}

func TestLoadDefaultsInCode(t *testing.T) {
	c, _ := load(t, "[stations.home]\ncallsign = \"N0CALL\"\n")

	assert.Equal(t, DefaultStationName, c.DefaultStation)
	assert.Equal(t, DefaultPollInterval, c.Rig.PollInterval)
	assert.Empty(t, c.Rig.Profile)
	assert.Empty(t, c.Rig.Profiles)
}

func TestLoadFileOverridesDefaults(t *testing.T) {
	c, _ := load(t, `
default_station = "portable"

[stations.portable]
callsign = "N0CALL/P"

[rig]
poll_interval = "500ms"
`)

	assert.Equal(t, "portable", c.DefaultStation)
	assert.Equal(t, 500*time.Millisecond, c.Rig.PollInterval)
}

func TestLoadUnknownKeyWarnings(t *testing.T) {
	_, res := load(t, `
colour = "red"

[stations.home]
callsign = "N0CALL"
colour   = "blue"

[rig]
bogus = 1

[rig.profiles.ic7300]
hots = "localhost"

  [rig.profiles.ic7300.spawn]
  modle = 3073
`)

	assert.Equal(t, []string{
		`unknown key "colour"`,
		`unknown key "rig.bogus"`,
		`unknown key "rig.profiles.ic7300.hots"`,
		`unknown key "rig.profiles.ic7300.spawn.modle"`,
		`unknown key "stations.home.colour"`,
	}, res.Warnings)
}

func TestLoadNoWarningsForKnownKeys(t *testing.T) {
	_, res := load(t, string(FullExample))
	assert.Empty(t, res.Warnings)
}

func TestLoadEmptyTables(t *testing.T) {
	c, res := load(t, `
[stations.home]

[rig.profiles.ic7300]

[rig.profiles.spawned]
  [rig.profiles.spawned.spawn]
`)

	assert.Empty(t, res.Warnings)
	assert.Equal(t, map[string]Station{"home": {Name: "home"}}, c.Stations)
	assert.Equal(t, map[string]RigProfile{
		"ic7300":  {Backend: BackendNone, Host: DefaultHost, Port: DefaultPort},
		"spawned": {Backend: BackendNone, Host: DefaultHost, Port: DefaultPort, Spawn: &Spawn{}},
	}, c.Rig.Profiles)
}

func TestLoadPerEntryDefaults(t *testing.T) {
	c, _ := load(t, `
[stations.home]
callsign = "N0CALL"

[stations.named]
name = "Named QTH"

[rig.profiles.backend_only]
backend = "rigctld"

[rig.profiles.explicit]
backend = "rigctld"
host    = ""
port    = 0
`)

	assert.Equal(t, "home", c.Stations["home"].Name, "name defaults to the table key")
	assert.Equal(t, "Named QTH", c.Stations["named"].Name)

	assert.Equal(t, RigProfile{Backend: BackendRigctld, Host: DefaultHost, Port: DefaultPort},
		c.Rig.Profiles["backend_only"])
	assert.Equal(t, RigProfile{Backend: BackendRigctld, Host: "", Port: 0},
		c.Rig.Profiles["explicit"], "explicitly set values are kept so validation can reject them")
}

func TestLoadLowercasesNamesAndReferences(t *testing.T) {
	c, _ := load(t, `
default_station = "HOME"

[stations.Home]
callsign    = "N0call"
rig_profile = "IC7300"

[rig]
profile = "Portable"

[rig.profiles.IC7300]
backend = "rigctld"
`)

	assert.Equal(t, "home", c.DefaultStation)
	assert.Equal(t, "portable", c.Rig.Profile)
	require.Contains(t, c.Stations, "home")
	assert.Equal(t, "ic7300", c.Stations["home"].RigProfile)
	assert.Equal(t, "N0call", c.Stations["home"].Callsign, "other values keep their case")
	assert.Contains(t, c.Rig.Profiles, "ic7300")
}

func TestLoadPollInterval(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Duration
		wantErr []string
	}{
		{name: "seconds", value: `"2s"`, want: 2 * time.Second},
		{name: "milliseconds", value: `"250ms"`, want: 250 * time.Millisecond},
		{name: "zero parses (validation rejects it)", value: `"0s"`, want: 0},
		{name: "invalid string", value: `"fast"`, wantErr: []string{"rig.poll_interval", `"fast"`}},
		{name: "bare number", value: `5`, wantErr: []string{"rig.poll_interval", "5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeConfig(t, "[rig]\npoll_interval = "+tt.value+"\n")
			c, _, err := Load(p, true)
			if tt.wantErr != nil {
				require.Error(t, err)
				for _, s := range tt.wantErr {
					assert.Contains(t, err.Error(), s)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, c.Rig.PollInterval)
		})
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	p := writeConfig(t, "default_station = \n")
	_, _, err := Load(p, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), p)
}
