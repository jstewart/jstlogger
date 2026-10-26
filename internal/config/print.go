package config

import (
	"fmt"
	"io"

	"github.com/pelletier/go-toml/v2"
)

// tomlConfig mirrors Config for encoding, with rig.poll_interval as a
// duration string so the output can be loaded again.
type tomlConfig struct {
	DefaultStation string                 `toml:"default_station"`
	Stations       map[string]tomlStation `toml:"stations"`
	Rig            tomlRig                `toml:"rig"`
}

// tomlStation mirrors Station for encoding. It must stay convertible to and
// from Station; it differs only in omitting an unset pota_references, which
// would otherwise print as [] and load back as an empty, non-nil slice.
type tomlStation struct {
	Name           string   `toml:"name"`
	Callsign       string   `toml:"callsign"`
	Gridsquare     string   `toml:"gridsquare"`
	DXCC           int      `toml:"dxcc"`
	CQZone         int      `toml:"cq_zone"`
	ITUZone        int      `toml:"itu_zone"`
	State          string   `toml:"state"`
	City           string   `toml:"city"`
	RigProfile     string   `toml:"rig_profile"`
	POTAReferences []string `toml:"pota_references,omitempty"`
	SOTAReference  string   `toml:"sota_reference"`
	IOTAReference  string   `toml:"iota_reference"`
	WWFFReference  string   `toml:"wwff_reference"`
}

// tomlRig mirrors Rig for encoding.
type tomlRig struct {
	Profile      string                `toml:"profile"`
	PollInterval string                `toml:"poll_interval"`
	Profiles     map[string]RigProfile `toml:"profiles"`
}

// WriteTOML writes c to w as TOML that Load accepts, with rig.poll_interval
// as a duration string (e.g. "1s"). Absent [spawn] tables are omitted.
func WriteTOML(w io.Writer, c Config) error {
	tc := tomlConfig{
		DefaultStation: c.DefaultStation,
		Stations:       make(map[string]tomlStation, len(c.Stations)),
		Rig: tomlRig{
			Profile:      c.Rig.Profile,
			PollInterval: c.Rig.PollInterval.String(),
			Profiles:     c.Rig.Profiles,
		},
	}
	for name, s := range c.Stations {
		tc.Stations[name] = tomlStation(s)
	}
	if err := toml.NewEncoder(w).Encode(tc); err != nil {
		return fmt.Errorf("writing config as TOML: %w", err)
	}
	return nil
}
