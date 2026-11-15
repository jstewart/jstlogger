package callbook

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChainQRZThenHamQTH checks the issue's acceptance criterion: QRZ.com
// answers first, and HamQTH fills in when QRZ.com does not list the call.
func TestChainQRZThenHamQTH(t *testing.T) {
	tests := []struct {
		name           string
		qrzSearch      string
		want           Result
		hamqthSearched int
	}{
		{
			name:      "qrz has the call",
			qrzSearch: "qrz_search_ok.xml",
			want: Result{
				Call:    "W1AW",
				Name:    "Hiram",
				QTH:     "Newington",
				Grid:    "FN31pr",
				State:   "CT",
				County:  "Hartford",
				Email:   "w1aw@arrl.org",
				Service: ServiceQRZ,
			},
			hamqthSearched: 0,
		},
		{
			name:      "qrz does not list the call",
			qrzSearch: "qrz_not_found.xml",
			want: Result{
				Call:    "W1AW",
				Name:    "Bob",
				QTH:     "Newington",
				Grid:    "FN31pr",
				State:   "CT",
				County:  "Hartford",
				Email:   "k1abc@example.com",
				Service: ServiceHamQTH,
			},
			hamqthSearched: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qf := &qrzServer{login: "qrz_login_ok.xml", searches: []string{tt.qrzSearch}}
			hf := &hamqthServer{login: "hamqth_login_ok.xml", searches: []string{"hamqth_search_us.xml"}}
			c := Chain{newQRZ(t, qf), newHamQTH(t, hf)}

			r, err := c.Lookup(t.Context(), "W1AW")
			require.NoError(t, err)
			assert.Equal(t, tt.want, r)

			_, searched := hf.counts()
			assert.Len(t, searched, tt.hamqthSearched)
		})
	}
}
