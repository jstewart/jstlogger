// Package config loads, defaults and validates jstlogger's TOML configuration.
//
// Defaults live in code; the user's config file holds only overrides. The
// package is independent of the TUI.
package config

import (
	"time"

	"jstlogger/internal/indexfmt"
)

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
	// DefaultDistanceUnit is the default logbook.distance_unit.
	DefaultDistanceUnit = DistanceUnitMi
	// DefaultIndexFormat is the default index.format, reproducing the
	// built-in QSO index columns.
	DefaultIndexFormat = indexfmt.Default
	// DefaultPagerIndexLines is the default index.pager_index_lines.
	DefaultPagerIndexLines = 10
	// DefaultIndexSort is the default index.sort: date, newest first.
	DefaultIndexSort = "reverse-date"
	// DefaultBackground is the default ui.background: ask the terminal.
	DefaultBackground = BackgroundAuto
	// DefaultCallbookCacheTTL is the default callbook.cache_ttl.
	DefaultCallbookCacheTTL = 168 * time.Hour
)

// Callbook services accepted in callbook.services.
const (
	// CallbookQRZ is the QRZ.com XML data service.
	CallbookQRZ = "qrz"
	// CallbookHamQTH is the HamQTH.com XML service.
	CallbookHamQTH = "hamqth"
)

// Distance units accepted in logbook.distance_unit.
const (
	// DistanceUnitKm shows distances in kilometers.
	DistanceUnitKm = "km"
	// DistanceUnitMi shows distances in statute miles.
	DistanceUnitMi = "mi"
)

// Terminal backgrounds accepted in ui.background.
const (
	// BackgroundAuto asks the terminal for its background color, and
	// assumes a dark background until, or unless, it answers.
	BackgroundAuto = "auto"
	// BackgroundDark uses the colors for a dark background.
	BackgroundDark = "dark"
	// BackgroundLight uses the colors for a light background.
	BackgroundLight = "light"
)

// Config is the merged jstlogger configuration (defaults + file).
type Config struct {
	// DefaultStation names the station used when --station is not given.
	DefaultStation string `mapstructure:"default_station" toml:"default_station"`
	// Stations holds the [stations.<name>] tables, keyed by lowercase name.
	Stations map[string]Station `mapstructure:"stations" toml:"stations"`
	// Rig is the [rig] table.
	Rig Rig `mapstructure:"rig" toml:"rig"`
	// Logbook is the [logbook] table.
	Logbook Logbook `mapstructure:"logbook" toml:"logbook"`
	// Index is the [index] table.
	Index Index `mapstructure:"index" toml:"index"`
	// Callbook is the [callbook] table.
	Callbook Callbook `mapstructure:"callbook" toml:"callbook"`
	// UI is the [ui] table.
	UI UI `mapstructure:"ui" toml:"ui"`
}

// UI is the [ui] table, which configures how the TUI looks.
type UI struct {
	// Background is the terminal's background, which picks the colors:
	// BackgroundAuto, BackgroundDark or BackgroundLight. Load lowercases it.
	Background string `mapstructure:"background" toml:"background"`
}

// Callbook is the [callbook] table, which configures looking up the call
// being logged in an online callbook.
type Callbook struct {
	// Services lists the callbook services to try, in order, until one
	// finds the call: CallbookQRZ or CallbookHamQTH. Empty disables
	// lookups. Load trims and lowercases each entry.
	Services []string `mapstructure:"services" toml:"services"`
	// CacheTTL is how long a found call is kept. 0 keeps it for the
	// session only.
	CacheTTL time.Duration `mapstructure:"cache_ttl" toml:"cache_ttl"`
	// QRZ is the [callbook.qrz] table.
	QRZ QRZAccount `mapstructure:"qrz" toml:"qrz"`
	// HamQTH is the [callbook.hamqth] table.
	HamQTH CallbookAccount `mapstructure:"hamqth" toml:"hamqth"`
}

// CallbookAccount holds the login for one callbook service.
type CallbookAccount struct {
	// Username is the account's login name.
	Username string `mapstructure:"username" toml:"username"`
	// Password is the account's password. An environment variable may
	// override it; see Config.ApplyEnv.
	Password string `mapstructure:"password" toml:"password"`
}

// QRZAccount is the [callbook.qrz] table: the QRZ.com login plus its
// display options.
type QRZAccount struct {
	// Username is the QRZ.com login name.
	Username string `mapstructure:"username" toml:"username"`
	// Password is the QRZ.com password. EnvQRZPassword may override it.
	Password string `mapstructure:"password" toml:"password"`
	// FullName adds the last name to the first name in looked-up names.
	FullName bool `mapstructure:"full_name" toml:"full_name"`
}

// Index is the [index] table, which configures the QSO index.
type Index struct {
	// Format is the index format string that chooses the index columns; see
	// package indexfmt for the syntax. Load keeps it verbatim because
	// spacing is significant.
	Format string `mapstructure:"format" toml:"format"`
	// PagerIndexLines is the number of index rows kept above the QSO pager,
	// as NeoMutt's pager_index_lines.
	PagerIndexLines int `mapstructure:"pager_index_lines" toml:"pager_index_lines"`
	// Sort is the order the index starts in, as logbook.ParseSort accepts
	// it, e.g. "band" or "reverse-call". Load replaces an unknown sort with
	// DefaultIndexSort and warns.
	Sort string `mapstructure:"sort" toml:"sort"`
}

// Logbook is the [logbook] table.
type Logbook struct {
	// Path is the logbook database file. Empty means the default from
	// LogbookPath.
	Path string `mapstructure:"path" toml:"path"`
	// DistanceUnit is the unit distances are shown in: DistanceUnitKm or
	// DistanceUnitMi. Load lowercases it.
	DistanceUnit string `mapstructure:"distance_unit" toml:"distance_unit"`
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
	// Name is the rig's name as shown in the header. Empty means the
	// profile's name is shown instead.
	Name    string `mapstructure:"name" toml:"name"`
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
