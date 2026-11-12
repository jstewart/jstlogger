package adif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnumerations(t *testing.T) {
	tests := []struct {
		name string
		get  func() []string
		want []string
	}{
		{"PropModes", PropModes, []string{
			"AS", "AUE", "AUR", "BS", "ECH", "EME", "ES", "F2", "FAI", "GWAVE",
			"INTERNET", "ION", "IRL", "LOS", "MS", "RPT", "RS", "SAT", "TEP", "TR",
		}},
		{"AntPaths", AntPaths, []string{"G", "O", "S", "L"}},
		{"QSLSentStatuses", QSLSentStatuses, []string{"Y", "N", "R", "Q", "I"}},
		{"QSLSentVias", QSLSentVias, []string{"B", "D", "E", "M"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.get()
			assert.Equal(t, tt.want, got, "the codes come in ADIF order")

			got[0] = "changed"
			assert.Equal(t, tt.want[0], tt.get()[0], "the caller gets a copy")
		})
	}
}
