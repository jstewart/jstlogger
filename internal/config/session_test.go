package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolve runs Resolve on FullExample with o and requires success.
func resolve(t *testing.T, o Overrides) Session {
	t.Helper()
	s, err := Resolve(fullConfig(t), o)
	require.NoError(t, err)
	return s
}

func TestResolveDefaults(t *testing.T) {
	c := fullConfig(t)
	s, err := Resolve(c, Overrides{})
	require.NoError(t, err)

	assert.Equal(t, "home", s.StationName)
	assert.Equal(t, c.Stations["home"], s.Station)
	assert.Equal(t, "N0CALL", s.Operator, "operator defaults to the station callsign")
	assert.Equal(t, "ic7300", s.RigProfileName)
	assert.True(t, s.RigEnabled)
	assert.Equal(t, c, s.Config, "no overrides: effective config equals the input")
}

func TestResolveSelectsStation(t *testing.T) {
	s := resolve(t, Overrides{Station: "Portable"})

	assert.Equal(t, "portable", s.StationName)
	assert.Equal(t, "N0CALL/P", s.Station.Callsign)
	assert.Equal(t, "N0CALL/P", s.Operator)
	assert.Equal(t, "portable", s.RigProfileName, "station rig_profile wins over rig.profile")
	assert.Equal(t, "portable", s.Config.DefaultStation, "effective config selects the session station")
}

func TestResolveOverridesReplaceStationFields(t *testing.T) {
	s := resolve(t, Overrides{
		Station:     "portable",
		Grid:        "FN31pr",
		Operator:    "K1ABC",
		OperatorSet: true,
		POTA:        []string{"US-0001"},
		SOTA:        "W1/AM-001",
		IOTA:        "NA-001",
		WWFF:        "KFF-0001",
	})

	assert.Equal(t, "K1ABC", s.Operator)
	want := fullConfig(t).Stations["portable"]
	want.Gridsquare = "FN31pr"
	want.POTAReferences = []string{"US-0001"}
	want.SOTAReference = "W1/AM-001"
	want.IOTAReference = "NA-001"
	want.WWFFReference = "KFF-0001"
	assert.Equal(t, want, s.Station)
	assert.Equal(t, want, s.Config.Stations["portable"])
	assert.Equal(t, "N0CALL/P", s.Station.Callsign, "--operator does not change the station callsign")
}

func TestResolveDoesNotAliasInput(t *testing.T) {
	c := fullConfig(t)
	orig := fullConfig(t)

	s, err := Resolve(c, Overrides{Station: "portable", Grid: "FN31"})
	require.NoError(t, err)
	s.Config.Stations["portable"].POTAReferences[0] = "XX-9999"
	s.Config.Stations["new"] = Station{}
	s.Station.POTAReferences[1] = "XX-0000"

	assert.Equal(t, orig, c, "input config must not be modified")
}

func TestResolveRig(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(c *Config)
		o           Overrides
		wantProfile string
		wantEnabled bool
		wantRigCfg  string // effective rig.profile
	}{
		{
			name:        "station rig_profile",
			wantProfile: "ic7300", wantEnabled: true, wantRigCfg: "ic7300",
		},
		{
			name: "falls back to rig.profile",
			mutate: func(c *Config) {
				s := c.Stations["home"]
				s.RigProfile = ""
				c.Stations["home"] = s
				c.Rig.Profile = "portable"
			},
			wantProfile: "portable", wantEnabled: true, wantRigCfg: "portable",
		},
		{
			name: "no profile anywhere disables rig",
			mutate: func(c *Config) {
				s := c.Stations["home"]
				s.RigProfile = ""
				c.Stations["home"] = s
				c.Rig.Profile = ""
			},
			wantProfile: "", wantEnabled: false, wantRigCfg: "",
		},
		{
			name:        "--rig overrides station and rig.profile",
			o:           Overrides{Rig: "PORTABLE"},
			wantProfile: "portable", wantEnabled: true, wantRigCfg: "ic7300",
		},
		{
			name:        "--no-rig disables rig",
			o:           Overrides{NoRig: true},
			wantProfile: "", wantEnabled: false, wantRigCfg: "",
		},
		{
			name:        "backend none disables rig",
			mutate:      func(c *Config) { setProfile(c, "ic7300", func(p *RigProfile) { p.Backend = BackendNone }) },
			wantProfile: "ic7300", wantEnabled: false, wantRigCfg: "ic7300",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fullConfig(t)
			if tt.mutate != nil {
				tt.mutate(&c)
			}
			s, err := Resolve(c, tt.o)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProfile, s.RigProfileName)
			assert.Equal(t, tt.wantEnabled, s.RigEnabled)
			assert.Equal(t, tt.wantRigCfg, s.Config.Rig.Profile)
		})
	}
}

func TestResolveRigOverrideRoundTrips(t *testing.T) {
	// The effective config must select the same rig when re-resolved, as
	// it does after --print-config is loaded again.
	for _, o := range []Overrides{{Rig: "portable"}, {NoRig: true}} {
		s := resolve(t, o)
		again, err := Resolve(s.Config, Overrides{})
		require.NoError(t, err)
		assert.Equal(t, s.RigProfileName, again.RigProfileName)
		assert.Equal(t, s.RigEnabled, again.RigEnabled)
		assert.NoError(t, s.Config.Validate(""))
	}
}

func TestResolveFlagErrors(t *testing.T) {
	tests := []struct {
		name     string
		o        Overrides
		wantKeys []string
	}{
		{name: "unknown station", o: Overrides{Station: "missing"}, wantKeys: []string{"--station"}},
		{name: "--rig with --no-rig", o: Overrides{Rig: "ic7300", NoRig: true}, wantKeys: []string{"--rig"}},
		{name: "unknown --rig", o: Overrides{Rig: "missing"}, wantKeys: []string{"--rig"}},
		{name: "invalid --grid", o: Overrides{Grid: "ZZ99"}, wantKeys: []string{"--grid"}},
		{name: "empty --operator", o: Overrides{OperatorSet: true}, wantKeys: []string{"--operator"}},
		{
			name:     "several at once",
			o:        Overrides{Grid: "bad", Operator: "", OperatorSet: true, Rig: "missing"},
			wantKeys: []string{"--grid", "--operator", "--rig"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(fullConfig(t), tt.o)
			require.Error(t, err)
			assert.Equal(t, tt.wantKeys, validationKeys(t, err))
		})
	}
}

func TestResolveUnknownRigProfileReference(t *testing.T) {
	c := fullConfig(t)
	c.Rig.Profile = "missing"
	s := c.Stations["home"]
	s.RigProfile = ""
	c.Stations["home"] = s

	_, err := Resolve(c, Overrides{})
	require.Error(t, err)
	assert.Equal(t, []string{"rig.profile"}, validationKeys(t, err))
}
