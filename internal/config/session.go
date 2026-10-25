package config

import (
	"errors"
	"maps"
	"slices"
	"strings"
)

// Overrides holds the session-override command-line flags. Zero values mean
// the flag was not given.
type Overrides struct {
	// Station selects a station, overriding default_station (--station).
	Station string
	// Grid replaces the station's gridsquare (--grid).
	Grid string
	// Operator is the guest operator callsign (--operator). It is only
	// used when OperatorSet is true.
	Operator string
	// OperatorSet reports whether --operator was given, so an explicit
	// empty value can be rejected.
	OperatorSet bool
	// POTA replaces the station's pota_references when non-empty (--pota).
	POTA []string
	// SOTA, IOTA and WWFF replace the station's xOTA references (--sota,
	// --iota, --wwff).
	SOTA, IOTA, WWFF string
	// Rig selects a rig profile, overriding the station's rig_profile and
	// rig.profile (--rig).
	Rig string
	// NoRig disables rig control for the session (--no-rig).
	NoRig bool
}

// Session is the result of applying Overrides to a Config.
type Session struct {
	// StationName is the lowercase key of the selected station.
	StationName string
	// Station is the selected station with overrides applied.
	Station Station
	// Operator is the operator callsign: --operator, or the station's
	// callsign.
	Operator string
	// RigProfileName is the resolved rig profile, or "" when none applies.
	RigProfileName string
	// RigEnabled reports whether rig control is on: a profile resolved and
	// its backend is not "none".
	RigEnabled bool
	// Config is the effective config: the input with overrides applied to
	// the selected station, DefaultStation set to it, and the rig selection
	// recorded so that loading it again resolves the same session. It
	// shares no mutable state with the input.
	Config Config
}

// Resolve validates the override flags against c and applies them to the
// selected station. Flag failures are *ValidationError values keyed by flag
// name (e.g. "--grid"), joined with errors.Join.
//
// Resolve does not run Config.Validate; callers should validate c first.
func Resolve(c Config, o Overrides) (Session, error) {
	name := strings.ToLower(o.Station)
	if name == "" {
		name = c.DefaultStation
	}
	rig := strings.ToLower(o.Rig)

	if err := checkOverrides(c, o, name, rig); err != nil {
		return Session{}, err
	}

	st := applyOverrides(c.Stations[name], o)
	eff := c
	eff.DefaultStation = name
	eff.Stations = maps.Clone(c.Stations)

	switch {
	case o.NoRig:
		st.RigProfile = ""
		eff.Rig.Profile = ""
	case rig != "":
		st.RigProfile = rig
	}
	eff.Stations[name] = st

	profile, enabled, err := resolveRig(eff, name)
	if err != nil {
		return Session{}, err
	}

	operator := st.Callsign
	if o.OperatorSet {
		operator = o.Operator
	}
	return Session{
		StationName:    name,
		Station:        st,
		Operator:       operator,
		RigProfileName: profile,
		RigEnabled:     enabled,
		Config:         eff,
	}, nil
}

// resolveRig picks the rig profile for station in the effective config c:
// the station's rig_profile, else rig.profile. It reports the profile name
// ("" for none) and whether rig control is enabled.
func resolveRig(c Config, station string) (string, bool, error) {
	profile := c.Stations[station].RigProfile
	key := "stations." + station + ".rig_profile"
	if profile == "" {
		profile, key = c.Rig.Profile, "rig.profile"
	}
	if profile == "" {
		return "", false, nil
	}
	p, ok := c.Rig.Profiles[profile]
	if !ok {
		return "", false, invalid(key, "no rig profile %q in [rig.profiles.*]", profile)
	}
	return profile, p.Backend != BackendNone, nil
}

// checkOverrides applies the flag validation rules. station and rig are the
// lowercased --station (or default) and --rig values.
func checkOverrides(c Config, o Overrides, station, rig string) error {
	var errs []error
	if _, ok := c.Stations[station]; !ok {
		errs = append(errs, invalid("--station", "no station %q in [stations.*]", station))
	}
	if o.Grid != "" && !ValidGridsquare(o.Grid) {
		errs = append(errs, invalid("--grid", "%q is not a 4, 6 or 8 character Maidenhead locator", o.Grid))
	}
	if o.OperatorSet && o.Operator == "" {
		errs = append(errs, invalid("--operator", "must not be empty"))
	}
	if rig != "" {
		if o.NoRig {
			errs = append(errs, invalid("--rig", "cannot be used with --no-rig"))
		} else if _, ok := c.Rig.Profiles[rig]; !ok {
			errs = append(errs, invalid("--rig", "no rig profile %q in [rig.profiles.*]", rig))
		}
	}
	return errors.Join(errs...)
}

// applyOverrides returns s with the station-field overrides applied. The
// returned POTA slice never aliases s's.
func applyOverrides(s Station, o Overrides) Station {
	s.POTAReferences = slices.Clone(s.POTAReferences)
	if o.Grid != "" {
		s.Gridsquare = o.Grid
	}
	if len(o.POTA) > 0 {
		s.POTAReferences = slices.Clone(o.POTA)
	}
	if o.SOTA != "" {
		s.SOTAReference = o.SOTA
	}
	if o.IOTA != "" {
		s.IOTAReference = o.IOTA
	}
	if o.WWFF != "" {
		s.WWFFReference = o.WWFF
	}
	return s
}
