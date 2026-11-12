package tui

import (
	"slices"

	"jstlogger/internal/qsoentry"
)

// WithStations sets the station profiles the New QSO screen picks our side
// of each QSO from. The first is the one each QSO starts with.
func WithStations(stations []qsoentry.Station) Option {
	return func(m *Model) {
		m.stations = stations
	}
}

// setStations offers stations in the station picker, by name, and fills in
// the Station tab from the first.
func (e *entry) setStations(stations []qsoentry.Station) {
	e.stations = stations
	names := make([]string, len(stations))
	for i, st := range stations {
		names[i] = st.Name
	}
	e.pickers[qsoentry.StationName] = newPicker(names)
	e.fillStation()
}

// fillStation fills in the Station tab from the profile picked, replacing
// what the last profile filled in or was typed. The rig and power are not
// part of a profile and are left as they are.
func (e *entry) fillStation() {
	i := e.pickers[qsoentry.StationName].i
	if i >= len(e.stations) {
		return
	}
	f := e.form()
	e.stations[i].Fill(&f)
	for _, k := range slices.Concat(entryLayouts[entryTabStation].rows...) {
		// The picker already shows the profile picked, which setting it
		// by name would not when two profiles share a name.
		if k != qsoentry.StationName {
			e.setValue(k, f[k])
		}
	}
}
