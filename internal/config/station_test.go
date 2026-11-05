package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStationName(t *testing.T) {
	stations := map[string]Station{
		"home":     {Name: "Home QTH", Callsign: "N0CALL", Gridsquare: "CM87wj"},
		"portable": {Name: "Portable", Callsign: "N0CALL/P", Gridsquare: "CM88"},
		"club1":    {Name: "Club A", Callsign: "W1AW", Gridsquare: "FN31pr"},
		"club2":    {Name: "Club B", Callsign: "w1aw", Gridsquare: "fn31PR"},
	}
	tests := []struct {
		name     string
		stations map[string]Station
		callsign string
		grid     string
		want     string
	}{
		{"one match", stations, "N0CALL", "CM87wj", "Home QTH"},
		{"case differences", stations, "n0call", "cm87WJ", "Home QTH"},
		{"space differences", stations, " N0CALL ", " CM87wj\t", "Home QTH"},
		{"callsign matches grid does not", stations, "N0CALL", "CM88", ""},
		{"grid is whole-string compared", stations, "N0CALL", "CM87", ""},
		{"no match", stations, "K1ABC", "FN42", ""},
		{"ambiguous", stations, "W1AW", "FN31pr", ""},
		{"empty callsign", stations, "", "CM87wj", ""},
		{"empty grid", stations, "N0CALL", "", ""},
		{"blank callsign", stations, "  ", "CM87wj", ""},
		{"nil stations", nil, "N0CALL", "CM87wj", ""},
		{"station with empty fields not matched by empty input", map[string]Station{"x": {Name: "X"}}, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StationName(tt.stations, tt.callsign, tt.grid))
		})
	}
}
