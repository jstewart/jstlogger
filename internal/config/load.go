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
// Station and rig profile names, and the values that refer to them, are
// lowercased.
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

	if err := v.ReadInConfig(); err != nil {
		return Config{}, res, fmt.Errorf("reading config %s: %w", path, err)
	}

	poll, err := parsePollInterval(v.Get("rig.poll_interval"))
	if err != nil {
		return Config{}, res, fmt.Errorf("config %s: %w", path, err)
	}
	v.Set("rig.poll_interval", poll)

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

// parsePollInterval converts the raw rig.poll_interval value to a duration.
// The file must hold a duration string; the code default is a time.Duration.
func parsePollInterval(raw any) (time.Duration, error) {
	switch val := raw.(type) {
	case time.Duration:
		return val, nil
	case string:
		d, err := time.ParseDuration(val)
		if err != nil {
			return 0, fmt.Errorf("rig.poll_interval: %w", err)
		}
		return d, nil
	default:
		return 0, fmt.Errorf(`rig.poll_interval: want a duration string such as "1s", got %v`, raw)
	}
}

// applyEntryDefaults fills per-entry defaults that viper cannot express
// (the table key is dynamic), recovers empty tables that viper drops from
// Unmarshal, and lowercases values that refer to station or profile names.
func applyEntryDefaults(v *viper.Viper, c *Config) {
	c.DefaultStation = strings.ToLower(c.DefaultStation)
	c.Rig.Profile = strings.ToLower(c.Rig.Profile)

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
}
