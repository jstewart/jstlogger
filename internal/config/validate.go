package config

import (
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"strings"

	"jstlogger/internal/indexfmt"
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
	errs = append(errs, c.validateLogbook()...)
	errs = append(errs, c.validateUI()...)
	errs = append(errs, c.validateIndex()...)
	errs = append(errs, c.validateCallbook()...)
	errs = append(errs, c.validateListeners()...)
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

// validateLogbook checks the [logbook] table.
func (c Config) validateLogbook() []error {
	switch c.Logbook.DistanceUnit {
	case DistanceUnitKm, DistanceUnitMi:
		return nil
	default:
		return []error{invalid("logbook.distance_unit", "must be %q or %q, got %q", DistanceUnitKm, DistanceUnitMi, c.Logbook.DistanceUnit)}
	}
}

// validateUI checks the [ui] table.
func (c Config) validateUI() []error {
	switch c.UI.Background {
	case BackgroundAuto, BackgroundDark, BackgroundLight:
		return nil
	default:
		return []error{invalid("ui.background", "must be %q, %q or %q, got %q", BackgroundAuto, BackgroundDark, BackgroundLight, c.UI.Background)}
	}
}

// validateIndex checks that index.format parses as an index format and
// that index.pager_index_lines is not negative.
func (c Config) validateIndex() []error {
	var errs []error
	if _, err := indexfmt.Parse(c.Index.Format); err != nil {
		errs = append(errs, invalid("index.format", "%v", err))
	}
	if c.Index.PagerIndexLines < 0 {
		errs = append(errs, invalid("index.pager_index_lines", "must be 0 or more, got %d", c.Index.PagerIndexLines))
	}
	return errs
}

// validateCallbook checks the [callbook] table: every listed service must be
// known, listed once and have a username and password. Credentials for
// unlisted services are not required.
func (c Config) validateCallbook() []error {
	var errs []error
	var seen []string
	for _, svc := range c.Callbook.Services {
		var acct CallbookAccount
		switch svc {
		case CallbookQRZ:
			acct = CallbookAccount{Username: c.Callbook.QRZ.Username, Password: c.Callbook.QRZ.Password}
		case CallbookHamQTH:
			acct = c.Callbook.HamQTH
		default:
			errs = append(errs, invalid("callbook.services", "unknown service %q (want %s or %s)", svc, CallbookQRZ, CallbookHamQTH))
			continue
		}
		if slices.Contains(seen, svc) {
			errs = append(errs, invalid("callbook.services", "service %q is listed more than once", svc))
			continue
		}
		seen = append(seen, svc)

		prefix := "callbook." + svc + "."
		if acct.Username == "" {
			errs = append(errs, invalid(prefix+"username", "required when %q is in callbook.services", svc))
		}
		if acct.Password == "" {
			errs = append(errs, invalid(prefix+"password", "required when %q is in callbook.services", svc))
		}
	}
	if c.Callbook.CacheTTL < 0 {
		errs = append(errs, invalid("callbook.cache_ttl", "must be 0 or more, got %s", c.Callbook.CacheTTL))
	}
	return errs
}

// validateListeners checks every [listeners.<name>] table. The address must
// be a loopback or multicast IP literal with a port, so other hosts on the
// network cannot write to the log. Whether the interface exists is checked
// when the listener binds, not here.
func (c Config) validateListeners() []error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(c.Listeners)) {
		l := c.Listeners[name]
		prefix := "listeners." + name + "."
		if l.Protocol != ProtocolWSJTX {
			errs = append(errs, invalid(prefix+"protocol", "must be %q, got %q", ProtocolWSJTX, l.Protocol))
		}
		addr, err := netip.ParseAddrPort(l.Address)
		switch {
		case err != nil:
			errs = append(errs, invalid(prefix+"address", "want an IP address and port such as %q, got %q", DefaultListenerAddress, l.Address))
		case addr.Port() == 0:
			errs = append(errs, invalid(prefix+"address", "port must be 1-65535, got %q", l.Address))
		case !addr.Addr().IsLoopback() && !addr.Addr().IsMulticast():
			errs = append(errs, invalid(prefix+"address", "must be a loopback or multicast address, got %q", l.Address))
		}
		if l.Interface != "" && (err != nil || !addr.Addr().IsMulticast()) {
			errs = append(errs, invalid(prefix+"interface", "only allowed with a multicast address, got address %q", l.Address))
		}
	}
	return errs
}
