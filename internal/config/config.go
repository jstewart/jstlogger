// Package config loads, defaults and validates jstlogger's TOML configuration.
//
// Defaults live in code; the user's config file holds only overrides. The
// package is independent of the TUI.
package config

import "time"

// Rig backends accepted in a rig profile's backend option.
const (
	// BackendNone disables rig control for the profile.
	BackendNone = "none"
	// BackendRigctld talks to Hamlib's rigctld over TCP.
	BackendRigctld = "rigctld"
)

// Defaults applied in code when the config file leaves an option unset.
const (
	// DefaultHost is the default rig profile host.
	DefaultHost = "localhost"
	// DefaultPort is the default rig profile port (rigctld's default).
	DefaultPort = 4532
	// DefaultPollInterval is the default rig.poll_interval.
	DefaultPollInterval = time.Second
	// DefaultStationName is the default value of default_station.
	DefaultStationName = "home"
)

// Config is the merged jstlogger configuration (defaults + file).
type Config struct {
	// DefaultStation names the station used when --station is not given.
	DefaultStation string `mapstructure:"default_station" toml:"default_station"`
	// Stations holds the [stations.<name>] tables, keyed by lowercase name.
	Stations map[string]Station `mapstructure:"stations" toml:"stations"`
	// Rig is the [rig] table.
	Rig Rig `mapstructure:"rig" toml:"rig"`
}

// Station describes one operating location. Field names follow the ADIF
// MY_* fields. A zero value (0 or "") means unset.
type Station struct {
	Name           string   `mapstructure:"name" toml:"name"`
	Callsign       string   `mapstructure:"callsign" toml:"callsign"`
	Gridsquare     string   `mapstructure:"gridsquare" toml:"gridsquare"`
	DXCC           int      `mapstructure:"dxcc" toml:"dxcc"`
	CQZone         int      `mapstructure:"cq_zone" toml:"cq_zone"`
	ITUZone        int      `mapstructure:"itu_zone" toml:"itu_zone"`
	State          string   `mapstructure:"state" toml:"state"`
	City           string   `mapstructure:"city" toml:"city"`
	RigProfile     string   `mapstructure:"rig_profile" toml:"rig_profile"`
	POTAReferences []string `mapstructure:"pota_references" toml:"pota_references"`
	SOTAReference  string   `mapstructure:"sota_reference" toml:"sota_reference"`
	IOTAReference  string   `mapstructure:"iota_reference" toml:"iota_reference"`
	WWFFReference  string   `mapstructure:"wwff_reference" toml:"wwff_reference"`
}

// Rig is the [rig] table.
type Rig struct {
	// Profile names the rig profile used when the station sets none.
	// Empty means no rig control.
	Profile string `mapstructure:"profile" toml:"profile"`
	// PollInterval is how often the rig is polled.
	PollInterval time.Duration `mapstructure:"poll_interval" toml:"poll_interval"`
	// Profiles holds the [rig.profiles.<name>] tables, keyed by lowercase name.
	Profiles map[string]RigProfile `mapstructure:"profiles" toml:"profiles"`
}

// RigProfile describes how to reach one rig.
type RigProfile struct {
	Backend string `mapstructure:"backend" toml:"backend"`
	Host    string `mapstructure:"host" toml:"host"`
	Port    int    `mapstructure:"port" toml:"port"`
	// Spawn is nil when the [spawn] table is absent, meaning jstlogger
	// connects to an already-running rigctld.
	Spawn *Spawn `mapstructure:"spawn" toml:"spawn,omitempty"`
}

// Spawn is the optional [rig.profiles.<name>.spawn] table that lets
// jstlogger start rigctld itself.
type Spawn struct {
	// Model is the Hamlib model number (see `rigctl -l`).
	Model int `mapstructure:"model" toml:"model"`
	// Device is the serial device of the rig.
	Device string `mapstructure:"device" toml:"device"`
	// Baud is the serial speed; 0 means the rig's default.
	Baud int `mapstructure:"baud" toml:"baud"`
}
