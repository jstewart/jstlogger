package logbook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestITURegionForZone(t *testing.T) {
	tests := []struct {
		name   string
		zone   int
		want   int
		wantOK bool
	}{
		{name: "Alaska", zone: 1, want: 2, wantOK: true},
		{name: "eastern US", zone: 8, want: 2, wantOK: true},
		{name: "Caribbean", zone: 11, want: 2, wantOK: true},
		{name: "Argentina and Falklands", zone: 16, want: 2, wantOK: true},
		{name: "Iceland", zone: 17, want: 1, wantOK: true},
		{name: "western Europe", zone: 27, want: 1, wantOK: true},
		{name: "Middle East", zone: 39, want: 1, wantOK: true},
		{name: "west Africa", zone: 46, want: 1, wantOK: true},
		{name: "Iran and Afghanistan", zone: 40, want: 3, wantOK: true},
		{name: "India", zone: 41, want: 3, wantOK: true},
		{name: "Japan", zone: 45, want: 3, wantOK: true},
		{name: "New Zealand", zone: 60, want: 3, wantOK: true},
		{name: "Minami Torishima", zone: 90, want: 3, wantOK: true},
		{name: "China, Mongolia and Russia straddle", zone: 33, wantOK: false},
		{name: "Mauritius and Rodrigues straddle", zone: 53, wantOK: false},
		{name: "Hawaii and Baker Is. straddle", zone: 61, wantOK: false},
		{name: "Easter I. and French Polynesia straddle", zone: 63, wantOK: false},
		{name: "Antarctic sector", zone: 71, wantOK: false},
		{name: "Greenland and Franz Josef Land straddle", zone: 75, wantOK: false},
		{name: "ocean zone", zone: 80, wantOK: false},
		{name: "zero", zone: 0, wantOK: false},
		{name: "out of range", zone: 91, wantOK: false},
		{name: "negative", zone: -1, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ITURegionForZone(tt.zone)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestITURegionForZoneTableIsValid(t *testing.T) {
	for zone := range 92 {
		region, ok := ITURegionForZone(zone)
		if !ok {
			assert.Zero(t, region, "zone %d", zone)
			continue
		}
		assert.True(t, zone >= 1 && zone <= 90, "zone %d", zone)
		assert.Contains(t, []int{1, 2, 3}, region, "zone %d", zone)
	}
}

func TestITURegionForLocation(t *testing.T) {
	tests := []struct {
		name     string
		lat, lon float64
		want     int
	}{
		{name: "New York", lat: 40.7, lon: -74.0, want: 2},
		{name: "Honolulu", lat: 21.3, lon: -157.8, want: 2},
		{name: "Nuuk", lat: 64.2, lon: -51.7, want: 2},
		{name: "Buenos Aires", lat: -34.6, lon: -58.4, want: 2},
		{name: "western edge of region 2", lat: 0, lon: -170, want: 2},
		{name: "eastern edge of region 2", lat: 0, lon: -25, want: 2},
		{name: "just east of -25", lat: 0, lon: -24.9, want: 1},
		{name: "London", lat: 51.5, lon: -0.1, want: 1},
		{name: "Cape Town", lat: -33.9, lon: 18.4, want: 1},
		{name: "Riyadh", lat: 24.7, lon: 46.7, want: 1},
		{name: "60E boundary", lat: 0, lon: 60, want: 1},
		{name: "Novosibirsk", lat: 55.0, lon: 82.9, want: 1},
		{name: "Anadyr", lat: 64.7, lon: 177.5, want: 1},
		{name: "Tokyo", lat: 35.7, lon: 139.7, want: 3},
		{name: "Delhi", lat: 28.6, lon: 77.2, want: 3},
		{name: "Sydney", lat: -33.9, lon: 151.2, want: 3},
		{name: "Samoa", lat: -13.8, lon: -172.1, want: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ITURegionForLocation(tt.lat, tt.lon))
		})
	}
}
