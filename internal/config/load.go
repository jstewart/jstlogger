package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"jstlogger/internal/logbook"
)

// LoadResult describes what Load did besides producing a Config.
type LoadResult struct {
	// Path is the config file that was loaded.
	Path string
	// Created reports whether Load wrote DefaultTemplate to Path because the
	// file did not exist (first run).
	Created bool
	// Warnings lists non-fatal problems, such as unknown keys, sorted.
	Warnings []string
}

// unusedKeyReplacer turns mapstructure's "stations[home].colour" notation
// into the dotted "stations.home.colour" form used in the config file.
var unusedKeyReplacer = strings.NewReplacer("[", ".", "]", "")

// Load reads the TOML config file at path and merges it over the defaults in
// code. It does not validate the result; call Config.Validate for that.
//
// If the file does not exist and explicit is false (no -c flag), Load creates
// its directory (0755), writes DefaultTemplate (0644) and loads that. If
// explicit is true, a missing file is an error wrapping fs.ErrNotExist. An
// existing file is never modified.
//
// Unknown keys are reported in LoadResult.Warnings rather than as errors.
// Station, rig profile and listener names, and the values that refer to
// them, are lowercased.
func Load(path string, explicit bool) (Config, LoadResult, error) {
	res := LoadResult{Path: path}

	created, err := ensureFile(path, explicit)
	if err != nil {
		return Config{}, res, err
	}
	res.Created = created

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	v.SetDefault("default_station", DefaultStationName)
	v.SetDefault("rig.poll_interval", DefaultPollInterval)
	v.SetDefault("logbook.distance_unit", DefaultDistanceUnit)
	v.SetDefault("index.format", DefaultIndexFormat)
	v.SetDefault("index.pager_index_lines", DefaultPagerIndexLines)
	v.SetDefault("index.sort", DefaultIndexSort)
	v.SetDefault("callbook.cache_ttl", DefaultCallbookCacheTTL)
	v.SetDefault("ui.background", DefaultBackground)

	if err := v.ReadInConfig(); err != nil {
		return Config{}, res, fmt.Errorf("reading config %s: %w", path, err)
	}

	poll, err := parseDuration("rig.poll_interval", `"1s"`, v.Get("rig.poll_interval"))
	if err != nil {
		return Config{}, res, fmt.Errorf("config %s: %w", path, err)
	}
	v.Set("rig.poll_interval", poll)

	ttl, err := parseDuration("callbook.cache_ttl", `"168h"`, v.Get("callbook.cache_ttl"))
	if err != nil {
		return Config{}, res, fmt.Errorf("config %s: %w", path, err)
	}
	v.Set("callbook.cache_ttl", ttl)

	lines, err := parsePagerIndexLines(v.Get("index.pager_index_lines"))
	if err != nil {
		return Config{}, res, fmt.Errorf("config %s: %w", path, err)
	}
	v.Set("index.pager_index_lines", lines)

	var c Config
	var md mapstructure.Metadata
	if err := v.Unmarshal(&c, func(dc *mapstructure.DecoderConfig) {
		dc.Metadata = &md
	}); err != nil {
		return Config{}, res, fmt.Errorf("decoding config %s: %w", path, err)
	}

	for _, k := range md.Unused {
		res.Warnings = append(res.Warnings, fmt.Sprintf("unknown key %q", unusedKeyReplacer.Replace(k)))
	}
	// An unknown sort only changes the starting order, so it is not fatal.
	if _, err := logbook.ParseSort(c.Index.Sort); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("index.sort: unknown sort %q; using %q", c.Index.Sort, DefaultIndexSort))
		c.Index.Sort = DefaultIndexSort
	}
	slices.Sort(res.Warnings)

	applyEntryDefaults(v, &c)
	return c, res, nil
}

// ensureFile makes sure path exists, writing DefaultTemplate when it is
// missing and not explicit. It reports whether it created the file.
func ensureFile(path string, explicit bool) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("checking config file: %w", err)
	}
	if explicit {
		return false, fmt.Errorf("config file not found: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating config directory: %w", err)
	}
	// O_EXCL: never overwrite a file that appeared since the Stat above.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("creating config file: %w", err)
	}
	_, werr := f.Write(DefaultTemplate)
	if err := errors.Join(werr, f.Close()); err != nil {
		return false, fmt.Errorf("writing config file %s: %w", path, err)
	}
	return true, nil
}

// parseDuration converts the raw value of the duration option key to a
// duration. The file must hold a duration string such as example; the code
// default is a time.Duration.
func parseDuration(key, example string, raw any) (time.Duration, error) {
	switch val := raw.(type) {
	case time.Duration:
		return val, nil
	case string:
		d, err := time.ParseDuration(val)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", key, err)
		}
		return d, nil
	default:
		return 0, fmt.Errorf("%s: want a duration string such as %s, got %v", key, example, raw)
	}
}

// parsePagerIndexLines checks that the raw index.pager_index_lines value is
// a whole number. Decoding alone would cut a fraction short or accept a
// numeric string.
func parsePagerIndexLines(raw any) (int, error) {
	switch val := raw.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	default:
		return 0, fmt.Errorf("index.pager_index_lines: want a whole number of rows, got %v", raw)
	}
}

// applyEntryDefaults fills per-entry defaults that viper cannot express
// (the table key is dynamic), recovers empty tables that viper drops from
// Unmarshal (including listener tables, whose address defaults to
// DefaultListenerAddress), lowercases values that refer to station or
// profile names, and normalizes logbook.distance_unit, callbook.services
// and ui.background.
func applyEntryDefaults(v *viper.Viper, c *Config) {
	c.DefaultStation = strings.ToLower(c.DefaultStation)
	c.Rig.Profile = strings.ToLower(c.Rig.Profile)
	c.Logbook.DistanceUnit = strings.ToLower(strings.TrimSpace(c.Logbook.DistanceUnit))
	c.UI.Background = strings.ToLower(strings.TrimSpace(c.UI.Background))
	for i, s := range c.Callbook.Services {
		c.Callbook.Services[i] = strings.ToLower(strings.TrimSpace(s))
	}

	for name := range v.GetStringMap("stations") {
		if _, ok := c.Stations[name]; !ok {
			if c.Stations == nil {
				c.Stations = make(map[string]Station)
			}
			c.Stations[name] = Station{}
		}
	}
	for name, s := range c.Stations {
		if s.Name == "" {
			s.Name = name
		}
		s.RigProfile = strings.ToLower(s.RigProfile)
		c.Stations[name] = s
	}

	for name := range v.GetStringMap("rig.profiles") {
		if _, ok := c.Rig.Profiles[name]; !ok {
			if c.Rig.Profiles == nil {
				c.Rig.Profiles = make(map[string]RigProfile)
			}
			c.Rig.Profiles[name] = RigProfile{}
		}
	}
	for name, p := range c.Rig.Profiles {
		key := "rig.profiles." + name + "."
		if !v.IsSet(key + "backend") {
			p.Backend = BackendNone
		}
		if !v.IsSet(key + "host") {
			p.Host = DefaultHost
		}
		if !v.IsSet(key + "port") {
			p.Port = DefaultPort
		}
		if p.Spawn == nil && v.IsSet(key+"spawn") {
			p.Spawn = &Spawn{}
		}
		c.Rig.Profiles[name] = p
	}

	for name := range v.GetStringMap("listeners") {
		if _, ok := c.Listeners[name]; !ok {
			if c.Listeners == nil {
				c.Listeners = make(map[string]Listener)
			}
			c.Listeners[name] = Listener{}
		}
	}
	for name, l := range c.Listeners {
		if !v.IsSet("listeners." + name + ".address") {
			l.Address = DefaultListenerAddress
		}
		c.Listeners[name] = l
	}
}
