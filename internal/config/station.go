package config

import "strings"

// StationName returns the Name of the one station whose Callsign and
// Gridsquare equal callsign and grid, compared ignoring case and surrounding
// whitespace. It returns "" when callsign or grid is empty, when no station
// matches, or when more than one does, since the match is then ambiguous.
//
// It is the fallback for QSOs without a stored station name, inferring the
// station from the QSO's station callsign and grid; a stored name is read
// from logbook.StationField.
func StationName(stations map[string]Station, callsign, grid string) string {
	callsign = strings.TrimSpace(callsign)
	grid = strings.TrimSpace(grid)
	if callsign == "" || grid == "" {
		return ""
	}
	name, matches := "", 0
	for _, st := range stations {
		if strings.EqualFold(strings.TrimSpace(st.Callsign), callsign) &&
			strings.EqualFold(strings.TrimSpace(st.Gridsquare), grid) {
			name = st.Name
			matches++
		}
	}
	if matches != 1 {
		return ""
	}
	return name
}
