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
			name:     "distance_unit mi is valid",
			mutate:   func(c *Config) { c.Logbook.DistanceUnit = DistanceUnitMi },
			wantKeys: nil,
		},
		{
			name:     "distance_unit must be km or mi",
			mutate:   func(c *Config) { c.Logbook.DistanceUnit = "furlongs" },
			wantKeys: []string{"logbook.distance_unit"},
		},
		{
			name:     "distance_unit must not be empty",
			mutate:   func(c *Config) { c.Logbook.DistanceUnit = "" },
			wantKeys: []string{"logbook.distance_unit"},
		},
		{
			name:     "background light is valid",
			mutate:   func(c *Config) { c.UI.Background = BackgroundLight },
			wantKeys: nil,
		},
		{
			name:     "background must be auto, dark or light",
			mutate:   func(c *Config) { c.UI.Background = "sepia" },
			wantKeys: []string{"ui.background"},
		},
		{
			name:     "background must not be empty",
			mutate:   func(c *Config) { c.UI.Background = "" },
			wantKeys: []string{"ui.background"},
		},
		{
			name:     "custom index format is valid",
			mutate:   func(c *Config) { c.Index.Format = "%-10c %-*C" },
			wantKeys: nil,
		},
		{
			name:     "index format rejects an unknown expando",
			mutate:   func(c *Config) { c.Index.Format = "%Z %q" },
			wantKeys: []string{"index.format"},
		},
		{
			name:     "index format rejects a bad strftime conversion",
			mutate:   func(c *Config) { c.Index.Format = "%{%Q}" },
			wantKeys: []string{"index.format"},
		},
		{
			name:     "index format allows only one flexible column",
			mutate:   func(c *Config) { c.Index.Format = "%-*c %-*C" },
			wantKeys: []string{"index.format"},
		},
		{
			name:     "pager_index_lines may be zero",
			mutate:   func(c *Config) { c.Index.PagerIndexLines = 0 },
			wantKeys: nil,
		},
		{
			name:     "pager_index_lines must not be negative",
			mutate:   func(c *Config) { c.Index.PagerIndexLines = -1 },
			wantKeys: []string{"index.pager_index_lines"},
		},
		{
			name: "multiple failures in sorted key order",
			mutate: func(c *Config) {
				setProfile(c, "portable", func(p *RigProfile) { p.Port = 0 })
				setProfile(c, "ic7300", func(p *RigProfile) { p.Backend = "bogus" })
				c.Logbook.DistanceUnit = "bogus"
				c.Index.Format = "%q"
				c.Index.PagerIndexLines = -1
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
				"logbook.distance_unit",
				"index.format",
				"index.pager_index_lines",
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

func TestValidateCallbook(t *testing.T) {
	both := func(c *Config) {
		c.Callbook.Services = []string{CallbookQRZ, CallbookHamQTH}
		c.Callbook.QRZ = QRZAccount{Username: "W1AW", Password: "secret"}
		c.Callbook.HamQTH = CallbookAccount{Username: "w1aw", Password: "other"}
	}
	tests := []struct {
		name     string
		mutate   func(c *Config)
		wantKeys []string
	}{
		{
			name:   "empty services disables lookups",
			mutate: func(c *Config) { c.Callbook = Callbook{} },
		},
		{
			name:   "both services with credentials",
			mutate: both,
		},
		{
			name: "credentials for unlisted services are not required",
			mutate: func(c *Config) {
				c.Callbook.Services = []string{CallbookHamQTH}
				c.Callbook.HamQTH = CallbookAccount{Username: "w1aw", Password: "other"}
			},
		},
		{
			name: "zero cache_ttl is valid",
			mutate: func(c *Config) {
				both(c)
				c.Callbook.CacheTTL = 0
			},
		},
		{
			name: "unknown service",
			mutate: func(c *Config) {
				both(c)
				c.Callbook.Services = append(c.Callbook.Services, "foo")
			},
			wantKeys: []string{"callbook.services"},
		},
		{
			name: "duplicate service",
			mutate: func(c *Config) {
				both(c)
				c.Callbook.Services = append(c.Callbook.Services, CallbookQRZ)
			},
			wantKeys: []string{"callbook.services"},
		},
		{
			name: "missing qrz credentials",
			mutate: func(c *Config) {
				both(c)
				c.Callbook.QRZ = QRZAccount{}
			},
			wantKeys: []string{"callbook.qrz.username", "callbook.qrz.password"},
		},
		{
			name: "missing hamqth credentials",
			mutate: func(c *Config) {
				both(c)
				c.Callbook.HamQTH = CallbookAccount{}
			},
			wantKeys: []string{"callbook.hamqth.username", "callbook.hamqth.password"},
		},
		{
			name: "negative cache_ttl",
			mutate: func(c *Config) {
				c.Callbook.CacheTTL = -time.Hour
			},
			wantKeys: []string{"callbook.cache_ttl"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fullConfig(t)
			tt.mutate(&c)
			err := c.Validate("")
			if tt.wantKeys == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantKeys, validationKeys(t, err))
		})
	}
}

func TestValidateCallbookUnknownServiceMessage(t *testing.T) {
	c := fullConfig(t)
	c.Callbook.Services = []string{"foo"}
	err := c.Validate("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `callbook.services: unknown service "foo" (want qrz or hamqth)`)
}

func TestValidateIndexMessage(t *testing.T) {
	c := fullConfig(t)
	c.Index.Format = "%Z %q"
	err := c.Validate("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `index.format: indexfmt: unknown expando`)
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

func TestValidateListeners(t *testing.T) {
	tests := []struct {
		name     string
		listener Listener
		wantKeys []string
	}{
		{
			name:     "loopback IPv4",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "127.0.0.1:2237"},
		},
		{
			name:     "loopback IPv6",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "[::1]:2237"},
		},
		{
			name:     "multicast with interface",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "224.0.0.1:2237", Interface: "lo0"},
		},
		{
			name:     "multicast without interface",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "239.255.0.1:2237", Tags: []string{"wsjtx"}},
		},
		{
			name:     "unknown protocol",
			listener: Listener{Protocol: "js8call", Address: "127.0.0.1:2237"},
			wantKeys: []string{"listeners.wsjtx.protocol"},
		},
		{
			name:     "empty protocol",
			listener: Listener{Address: "127.0.0.1:2237"},
			wantKeys: []string{"listeners.wsjtx.protocol"},
		},
		{
			name:     "hostname",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "localhost:2237"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "unspecified IPv4",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "0.0.0.0:2237"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "unspecified IPv6",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "[::]:2237"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "LAN unicast",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "192.168.1.5:2237"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "missing port",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "127.0.0.1"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "empty address",
			listener: Listener{Protocol: ProtocolWSJTX},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "port zero",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "127.0.0.1:0"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "port too large",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "127.0.0.1:70000"},
			wantKeys: []string{"listeners.wsjtx.address"},
		},
		{
			name:     "interface with unicast address",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "127.0.0.1:2237", Interface: "lo0"},
			wantKeys: []string{"listeners.wsjtx.interface"},
		},
		{
			name:     "every rule fails",
			listener: Listener{Protocol: "bogus", Address: "10.0.0.1:2237", Interface: "en0"},
			wantKeys: []string{"listeners.wsjtx.protocol", "listeners.wsjtx.address", "listeners.wsjtx.interface"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fullConfig(t)
			c.Listeners = map[string]Listener{"wsjtx": tt.listener}
			err := c.Validate("")
			if tt.wantKeys == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantKeys, validationKeys(t, err))
		})
	}
}

func TestValidateListenersSortedByName(t *testing.T) {
	c := fullConfig(t)
	c.Listeners = map[string]Listener{
		"zeta":  {Protocol: "bogus", Address: "127.0.0.1:2237"},
		"alpha": {Protocol: "bogus", Address: "127.0.0.1:2237"},
	}
	err := c.Validate("")
	require.Error(t, err)
	assert.Equal(t, []string{"listeners.alpha.protocol", "listeners.zeta.protocol"}, validationKeys(t, err))
}

func TestValidateListenerMessages(t *testing.T) {
	tests := []struct {
		name     string
		listener Listener
		want     string
	}{
		{
			name:     "protocol lists the allowed protocols",
			listener: Listener{Protocol: "js8call", Address: "127.0.0.1:2237"},
			want:     `listeners.wsjtx.protocol: must be "wsjtx", got "js8call"`,
		},
		{
			name:     "address must be loopback or multicast",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "0.0.0.0:2237"},
			want:     "must be a loopback or multicast address",
		},
		{
			name:     "interface needs multicast",
			listener: Listener{Protocol: ProtocolWSJTX, Address: "127.0.0.1:2237", Interface: "lo0"},
			want:     "listeners.wsjtx.interface: only allowed with a multicast address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fullConfig(t)
			c.Listeners = map[string]Listener{"wsjtx": tt.listener}
			err := c.Validate("")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}
