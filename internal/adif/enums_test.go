package adif

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnumerations(t *testing.T) {
	tests := []struct {
		name string
		get  func() []Enum
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
			assert.Equal(t, tt.want, Codes(got), "the codes come in ADIF order")
			for _, e := range got {
				assert.NotEmpty(t, e.Description, "%s has a description", e.Code)
			}

			got[0].Code = "changed"
			assert.Equal(t, tt.want[0], tt.get()[0].Code, "the caller gets a copy")
		})
	}
}

func TestEnumerationDescriptions(t *testing.T) {
	assert.Contains(t, PropModes(), Enum{Code: "TEP", Description: "trans-equatorial"})
	assert.Contains(t, PropModes(), Enum{Code: "MS", Description: "meteor scatter"})
	assert.Contains(t, AntPaths(), Enum{Code: "G", Description: "grayline"})
	assert.Contains(t, QSLSentVias(), Enum{Code: "B", Description: "bureau"})
}

func TestCodes(t *testing.T) {
	assert.Equal(t, []string{"A", "B"}, Codes([]Enum{{Code: "A"}, {Code: "B", Description: "b"}}))
	assert.Empty(t, Codes(nil))
}
