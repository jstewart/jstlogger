package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ValidationError reports one invalid option. Key is the dotted option name
// as written in the config file (e.g. "stations.home.gridsquare") or, for
// session overrides, the flag (e.g. "--grid").
type ValidationError struct {
	Key string
	Msg string
}

// Error returns "<key>: <msg>".
func (e *ValidationError) Error() string {
	return e.Key + ": " + e.Msg
}

// invalid returns a *ValidationError for key with a formatted message.
func invalid(key, format string, args ...any) error {
	return &ValidationError{Key: key, Msg: fmt.Sprintf(format, args...)}
}

// Validate checks c against every rule in the config spec, using station as
// the selected station ("" means DefaultStation; case-insensitive). The
// callsign and gridsquare requirements apply to the selected station only;
// every other rule applies to all entries.
//
// Each failure is a *ValidationError; several are joined with errors.Join in
// a deterministic order. Validate returns nil if c is valid.
func (c Config) Validate(station string) error {
	var errs []error
	errs = append(errs, c.validateSelected(strings.ToLower(station))...)
	errs = append(errs, c.validateStations()...)
	errs = append(errs, c.validateRig()...)
	return errors.Join(errs...)
}

// validateSelected checks default_station and the selected station's
// required fields.
func (c Config) validateSelected(station string) []error {
	var errs []error
	if _, ok := c.Stations[c.DefaultStation]; !ok {
		errs = append(errs, invalid("default_station", "no station %q in [stations.*]", c.DefaultStation))
	}

	key := "default_station"
	if station == "" {
		station = c.DefaultStation
	} else {
		key = "--station"
	}
	s, ok := c.Stations[station]
	if !ok {
		if key == "--station" {
			errs = append(errs, invalid(key, "no station %q in [stations.*]", station))
		}
		return errs
	}

	prefix := "stations." + station + "."
	if s.Callsign == "" {
		errs = append(errs, invalid(prefix+"callsign", "required"))
	}
	if s.Gridsquare == "" {
		errs = append(errs, invalid(prefix+"gridsquare", "required"))
	}
	return errs
}

// validateStations checks the rules that apply to every station.
func (c Config) validateStations() []error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(c.Stations)) {
		s := c.Stations[name]
		prefix := "stations." + name + "."
		if s.Gridsquare != "" && !ValidGridsquare(s.Gridsquare) {
			errs = append(errs, invalid(prefix+"gridsquare", "%q is not a 4, 6 or 8 character Maidenhead locator", s.Gridsquare))
		}
		if s.RigProfile != "" {
			if _, ok := c.Rig.Profiles[s.RigProfile]; !ok {
				errs = append(errs, invalid(prefix+"rig_profile", "no rig profile %q in [rig.profiles.*]", s.RigProfile))
			}
		}
	}
	return errs
}

// validateRig checks the [rig] table and every rig profile.
func (c Config) validateRig() []error {
	var errs []error
	if c.Rig.Profile != "" {
		if _, ok := c.Rig.Profiles[c.Rig.Profile]; !ok {
			errs = append(errs, invalid("rig.profile", "no rig profile %q in [rig.profiles.*]", c.Rig.Profile))
		}
	}
	if c.Rig.PollInterval <= 0 {
		errs = append(errs, invalid("rig.poll_interval", "must be greater than 0, got %s", c.Rig.PollInterval))
	}
	for _, name := range slices.Sorted(maps.Keys(c.Rig.Profiles)) {
		errs = append(errs, validateProfile("rig.profiles."+name+".", c.Rig.Profiles[name])...)
	}
	return errs
}

// validateProfile checks one rig profile whose keys start with prefix.
func validateProfile(prefix string, p RigProfile) []error {
	var errs []error
	if p.Backend != BackendNone && p.Backend != BackendRigctld {
		errs = append(errs, invalid(prefix+"backend", "must be %q or %q, got %q", BackendNone, BackendRigctld, p.Backend))
	}
	if p.Port < 1 || p.Port > 65535 {
		errs = append(errs, invalid(prefix+"port", "must be 1-65535, got %d", p.Port))
	}
	if p.Spawn != nil {
		if p.Spawn.Model == 0 {
			errs = append(errs, invalid(prefix+"spawn.model", "required when [spawn] is present"))
		}
		if p.Spawn.Device == "" {
			errs = append(errs, invalid(prefix+"spawn.device", "required when [spawn] is present"))
		}
	}
	return errs
}
