package config

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullConfig loads FullExample, a config that passes validation.
func fullConfig(t *testing.T) Config {
	t.Helper()
	c, _ := load(t, string(FullExample))
	return c
}

// validationKeys returns the Key of every *ValidationError joined in err, in
// order.
func validationKeys(t *testing.T, err error) []string {
	t.Helper()
	var errs []error
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	} else {
		errs = []error{err}
	}
	keys := make([]string, 0, len(errs))
	for _, e := range errs {
		var ve *ValidationError
		require.True(t, errors.As(e, &ve), "not a *ValidationError: %v", e)
		keys = append(keys, ve.Key)
	}
	return keys
}

func TestValidationErrorError(t *testing.T) {
	err := &ValidationError{Key: "rig.profiles.ic7300.port", Msg: "must be 1-65535"}
	assert.Equal(t, "rig.profiles.ic7300.port: must be 1-65535", err.Error())
}

func TestValidateFullExample(t *testing.T) {
	c := fullConfig(t)
	assert.NoError(t, c.Validate(""))
	assert.NoError(t, c.Validate("portable"))
	assert.NoError(t, c.Validate("PORTABLE"), "station names are case-insensitive")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name     string
		station  string
		mutate   func(c *Config)
		wantKeys []string
	}{
		{
			name:     "default_station must name a station",
			mutate:   func(c *Config) { c.DefaultStation = "missing" },
			wantKeys: []string{"default_station"},
		},
		{
			name:     "selected station must exist",
			station:  "missing",
			mutate:   func(c *Config) {},
			wantKeys: []string{"--station"},
		},
		{
			name: "selected station callsign required",
			mutate: func(c *Config) {
				s := c.Stations["home"]
				s.Callsign = ""
				c.Stations["home"] = s
			},
			wantKeys: []string{"stations.home.callsign"},
		},
		{
			name: "selected station gridsquare required",
			mutate: func(c *Config) {
				s := c.Stations["home"]
				s.Gridsquare = ""
				c.Stations["home"] = s
			},
			wantKeys: []string{"stations.home.gridsquare"},
		},
		{
			name: "unselected station may omit callsign and gridsquare",
			mutate: func(c *Config) {
				s := c.Stations["portable"]
				s.Callsign, s.Gridsquare = "", ""
				c.Stations["portable"] = s
			},
		},
		{
			name: "gridsquare must be Maidenhead on every station",
			mutate: func(c *Config) {
				s := c.Stations["portable"]
				s.Gridsquare = "ZZ99"
				c.Stations["portable"] = s
			},
			wantKeys: []string{"stations.portable.gridsquare"},
		},
		{
			name: "station rig_profile must name a profile",
			mutate: func(c *Config) {
				s := c.Stations["portable"]
				s.RigProfile = "missing"
				c.Stations["portable"] = s
			},
			wantKeys: []string{"stations.portable.rig_profile"},
		},
		{
			name:     "rig.profile must name a profile",
			mutate:   func(c *Config) { c.Rig.Profile = "missing" },
			wantKeys: []string{"rig.profile"},
		},
		{
			name:     "rig.profile may be empty",
			mutate:   func(c *Config) { c.Rig.Profile = "" },
			wantKeys: nil,
		},
		{
			name:     "poll_interval must be positive",
			mutate:   func(c *Config) { c.Rig.PollInterval = 0 },
			wantKeys: []string{"rig.poll_interval"},
		},
		{
			name:     "poll_interval negative",
			mutate:   func(c *Config) { c.Rig.PollInterval = -time.Second },
			wantKeys: []string{"rig.poll_interval"},
		},
		{
			name:     "backend must be none or rigctld",
			mutate:   func(c *Config) { setProfile(c, "portable", func(p *RigProfile) { p.Backend = "flrig" }) },
			wantKeys: []string{"rig.profiles.portable.backend"},
		},
		{
			name:     "backend is case-sensitive",
			mutate:   func(c *Config) { setProfile(c, "portable", func(p *RigProfile) { p.Backend = "Rigctld" }) },
			wantKeys: []string{"rig.profiles.portable.backend"},
		},
		{
			name:     "backend none is valid",
			mutate:   func(c *Config) { setProfile(c, "portable", func(p *RigProfile) { p.Backend = BackendNone }) },
			wantKeys: nil,
		},
		{
			name:     "port zero",
			mutate:   func(c *Config) { setProfile(c, "ic7300", func(p *RigProfile) { p.Port = 0 }) },
			wantKeys: []string{"rig.profiles.ic7300.port"},
		},
		{
			name:     "port too large",
			mutate:   func(c *Config) { setProfile(c, "ic7300", func(p *RigProfile) { p.Port = 65536 }) },
			wantKeys: []string{"rig.profiles.ic7300.port"},
		},
		{
			name:     "port 65535 is valid",
			mutate:   func(c *Config) { setProfile(c, "ic7300", func(p *RigProfile) { p.Port = 65535 }) },
			wantKeys: nil,
		},
		{
			name:     "empty spawn table needs model and device",
			mutate:   func(c *Config) { setProfile(c, "portable", func(p *RigProfile) { p.Spawn = &Spawn{} }) },
			wantKeys: []string{"rig.profiles.portable.spawn.model", "rig.profiles.portable.spawn.device"},
		},
		{
			name:     "spawn baud is optional",
			mutate:   func(c *Config) { setProfile(c, "ic7300", func(p *RigProfile) { p.Spawn.Baud = 0 }) },
			wantKeys: nil,
		},
		{
			name: "multiple failures in sorted key order",
			mutate: func(c *Config) {
				setProfile(c, "portable", func(p *RigProfile) { p.Port = 0 })
				setProfile(c, "ic7300", func(p *RigProfile) { p.Backend = "bogus" })
				for _, n := range []string{"portable", "home"} {
					s := c.Stations[n]
					s.Gridsquare = "bad"
					c.Stations[n] = s
				}
			},
			wantKeys: []string{
				"stations.home.gridsquare",
				"stations.portable.gridsquare",
				"rig.profiles.ic7300.backend",
				"rig.profiles.portable.port",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fullConfig(t)
			tt.mutate(&c)
			err := c.Validate(tt.station)
			if tt.wantKeys == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantKeys, validationKeys(t, err))
		})
	}
}

func TestValidateDefaultTemplateNeedsEditing(t *testing.T) {
	c, _ := load(t, string(DefaultTemplate))
	err := c.Validate("")
	require.Error(t, err)
	assert.Equal(t, []string{"stations.home.callsign", "stations.home.gridsquare"}, validationKeys(t, err))
}

// setProfile applies f to the named rig profile in c.
func setProfile(c *Config, name string, f func(p *RigProfile)) {
	p := c.Rig.Profiles[name]
	f(&p)
	c.Rig.Profiles[name] = p
}
