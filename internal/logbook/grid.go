package logbook

import (
	"errors"
	"fmt"
	"math"
)

// ErrInvalidGrid is returned (wrapped) for a malformed Maidenhead locator.
var ErrInvalidGrid = errors.New("logbook: invalid maidenhead locator")

// earthRadiusKm is the IUGG mean Earth radius.
const earthRadiusKm = 6371.0088

// kmPerMile is the length of an international mile in km.
const kmPerMile = 1.609344

// gridPairs describes each Maidenhead character pair: its upper bound (the
// lower bound is 'A' for letters, '0' for digits) and its cell size in
// degrees of longitude and latitude.
var gridPairs = [...]struct {
	hi       byte
	lon, lat float64
}{
	{'R', 20, 10},               // field
	{'9', 2, 1},                 // square
	{'X', 2.0 / 24, 1.0 / 24},   // subsquare
	{'9', 2.0 / 240, 1.0 / 240}, // extended square
}

// GridCenter returns the centre, in decimal degrees, of a 2, 4, 6 or 8
// character Maidenhead locator, ignoring case. The centre is that of the
// most precise square given. Malformed input returns an error wrapping
// ErrInvalidGrid.
func GridCenter(grid string) (lat, lon float64, err error) {
	if n := len(grid); n == 0 || n > 2*len(gridPairs) || n%2 != 0 {
		return 0, 0, fmt.Errorf("grid %q: %w", grid, ErrInvalidGrid)
	}
	lon, lat = -180, -90
	for i := range len(grid) {
		c := grid[i]
		// ASCII-only upper-casing, so non-ASCII input can't slip through.
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		p := gridPairs[i/2]
		lo := byte('0')
		if p.hi > '9' {
			lo = 'A'
		}
		if c < lo || c > p.hi {
			return 0, 0, fmt.Errorf("grid %q: %w", grid, ErrInvalidGrid)
		}
		v := float64(c - lo)
		if i%2 == 0 {
			lon += v * p.lon
		} else {
			lat += v * p.lat
		}
	}
	// Centre of the most precise cell given.
	last := gridPairs[len(grid)/2-1]
	return lat + last.lat/2, lon + last.lon/2, nil
}

// GreatCircleKm returns the great-circle distance in km between two points
// given in decimal degrees, using the haversine formula on a sphere of the
// mean Earth radius.
func GreatCircleKm(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	φ1, φ2 := lat1*rad, lat2*rad
	dφ := φ2 - φ1
	dλ := (lon2 - lon1) * rad
	h := math.Pow(math.Sin(dφ/2), 2) + math.Cos(φ1)*math.Cos(φ2)*math.Pow(math.Sin(dλ/2), 2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(min(h, 1)))
}

// GridDistanceKm returns the great-circle distance in km between the centres
// of two Maidenhead locators.
func GridDistanceKm(a, b string) (float64, error) {
	lat1, lon1, err := GridCenter(a)
	if err != nil {
		return 0, fmt.Errorf("logbook: grid distance: %w", err)
	}
	lat2, lon2, err := GridCenter(b)
	if err != nil {
		return 0, fmt.Errorf("logbook: grid distance: %w", err)
	}
	return GreatCircleKm(lat1, lon1, lat2, lon2), nil
}

// KmToMiles converts kilometers to international miles.
func KmToMiles(km float64) float64 { return km / kmPerMile }

// MilesToKm converts international miles to kilometers.
func MilesToKm(miles float64) float64 { return miles * kmPerMile }
