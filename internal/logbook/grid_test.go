package logbook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGridCenter(t *testing.T) {
	tests := []struct {
		name    string
		grid    string
		wantLat float64
		wantLon float64
	}{
		{name: "field", grid: "FN", wantLat: 45, wantLon: -70},
		{name: "square", grid: "FN31", wantLat: 41.5, wantLon: -73},
		{name: "subsquare", grid: "FN31pr", wantLat: 41 + 17.5/24, wantLon: -74 + 15.5/12},
		{name: "subsquare upper case", grid: "FN31PR", wantLat: 41 + 17.5/24, wantLon: -74 + 15.5/12},
		{name: "extended square", grid: "FN31pr45", wantLat: 41 + 17.0/24 + 5.5/240, wantLon: -74 + 15.0/12 + 4.5/120},
		{name: "lower case square", grid: "jo01", wantLat: 51.5, wantLon: 1},
		{name: "south-west corner", grid: "AA00aa", wantLat: -90 + 0.5/24, wantLon: -180 + 0.5/12},
		{name: "north-east corner", grid: "RR99xx", wantLat: 90 - 0.5/24, wantLon: 180 - 0.5/12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, err := GridCenter(tt.grid)
			require.NoError(t, err)
			assert.InDelta(t, tt.wantLat, lat, 1e-9)
			assert.InDelta(t, tt.wantLon, lon, 1e-9)
		})
	}
}

func TestGridCenterInvalid(t *testing.T) {
	tests := []struct {
		name string
		grid string
	}{
		{name: "empty", grid: ""},
		{name: "one char", grid: "F"},
		{name: "odd length", grid: "FN3"},
		{name: "too long", grid: "FN31pr45aa"},
		{name: "field out of range", grid: "SN31"},
		{name: "square not digit", grid: "FNA1"},
		{name: "subsquare out of range", grid: "FN31yr"},
		{name: "extended not digit", grid: "FN31prab"},
		{name: "non-ascii", grid: "FN3ı"},
		{name: "whitespace", grid: " FN31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := GridCenter(tt.grid)
			assert.ErrorIs(t, err, ErrInvalidGrid)
		})
	}
}

func TestGridCenterPrecision(t *testing.T) {
	lat4, lon4, err := GridCenter("FN31")
	require.NoError(t, err)
	lat6, lon6, err := GridCenter("FN31pr")
	require.NoError(t, err)

	// FN31pr lies in the north-east of FN31, so its centre is north and
	// east of the square's centre, but still inside the 2°x1° square.
	assert.Greater(t, lat6, lat4)
	assert.Greater(t, lon6, lon4)
	assert.Less(t, lat6-lat4, 0.5)
	assert.Less(t, lon6-lon4, 1.0)
	assert.Greater(t, GreatCircleKm(lat4, lon4, lat6, lon6), 20.0)
}

func TestGreatCircleKm(t *testing.T) {
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want, tol              float64
	}{
		{name: "same point", lat1: 51.5, lon1: -0.1, lat2: 51.5, lon2: -0.1, want: 0, tol: 1e-9},
		{name: "quarter of the equator", lat1: 0, lon1: 0, lat2: 0, lon2: 90, want: 6371.0088 * 3.141592653589793 / 2, tol: 1e-6},
		{name: "pole to pole", lat1: 90, lon1: 0, lat2: -90, lon2: 0, want: 6371.0088 * 3.141592653589793, tol: 1e-6},
		{name: "London to Paris", lat1: 51.5074, lon1: -0.1278, lat2: 48.8566, lon2: 2.3522, want: 343.5, tol: 2},
		{name: "New York to Los Angeles", lat1: 40.7128, lon1: -74.0060, lat2: 34.0522, lon2: -118.2437, want: 3936, tol: 10},
		{name: "across the antimeridian", lat1: 0, lon1: 179, lat2: 0, lon2: -179, want: 6371.0088 * 2 * 3.141592653589793 / 180, tol: 1e-6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GreatCircleKm(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			assert.InDelta(t, tt.want, got, tt.tol)
			// Distance is symmetric.
			assert.InDelta(t, got, GreatCircleKm(tt.lat2, tt.lon2, tt.lat1, tt.lon1), 1e-9)
		})
	}
}

func TestGridDistanceKm(t *testing.T) {
	tests := []struct {
		name      string
		a, b      string
		want, tol float64
	}{
		// Centre to centre; London itself (51.51N 0.13W) is ~5,410 km away,
		// but JO01's centre is at 51.5N 1E.
		{name: "FN31pr to JO01", a: "FN31pr", b: "JO01", want: 5489, tol: 5},
		{name: "case-insensitive", a: "fn31PR", b: "jo01", want: 5489, tol: 5},
		{name: "4-character home grid", a: "FN31", b: "JO01", want: 5520, tol: 5},
		{name: "same grid", a: "FN31", b: "fn31", want: 0, tol: 1e-6},
		{name: "adjacent squares", a: "FN31", b: "FN41", want: 166.6, tol: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GridDistanceKm(tt.a, tt.b)
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, tt.tol)
		})
	}
}

func TestGridDistanceKmInvalid(t *testing.T) {
	_, err := GridDistanceKm("FN31", "ZZ99")
	require.ErrorIs(t, err, ErrInvalidGrid)
	assert.Contains(t, err.Error(), "ZZ99")

	_, err = GridDistanceKm("", "FN31")
	assert.ErrorIs(t, err, ErrInvalidGrid)
}

func TestMilesKmConversion(t *testing.T) {
	tests := []struct {
		name  string
		km    float64
		miles float64
	}{
		{name: "zero", km: 0, miles: 0},
		{name: "one mile", km: 1.609344, miles: 1},
		{name: "marathon", km: 42.195, miles: 26.218757},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.miles, KmToMiles(tt.km), 1e-6)
			assert.InDelta(t, tt.km, MilesToKm(tt.miles), 1e-6)
		})
	}
}
