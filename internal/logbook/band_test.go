package logbook

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBandForFreq(t *testing.T) {
	tests := []struct {
		name   string
		hz     int64
		want   string
		wantOK bool
	}{
		{name: "2190m lower edge", hz: 135_700, want: "2190m", wantOK: true},
		{name: "2190m upper edge", hz: 137_800, want: "2190m", wantOK: true},
		{name: "below 2190m", hz: 135_699, wantOK: false},
		{name: "160m lower edge", hz: 1_800_000, want: "160m", wantOK: true},
		{name: "160m upper edge", hz: 2_000_000, want: "160m", wantOK: true},
		{name: "just above 160m", hz: 2_000_001, wantOK: false},
		{name: "gap between 80m and 60m", hz: 4_500_000, wantOK: false},
		{name: "40m mid", hz: 7_074_000, want: "40m", wantOK: true},
		{name: "20m lower edge", hz: 14_000_000, want: "20m", wantOK: true},
		{name: "20m upper edge", hz: 14_350_000, want: "20m", wantOK: true},
		{name: "just below 20m", hz: 13_999_999, wantOK: false},
		{name: "just above 20m", hz: 14_350_001, wantOK: false},
		{name: "17m lower edge", hz: 18_068_000, want: "17m", wantOK: true},
		{name: "12m upper edge", hz: 24_990_000, want: "12m", wantOK: true},
		{name: "10m upper edge", hz: 29_700_000, want: "10m", wantOK: true},
		{name: "6m upper edge", hz: 54_000_000, want: "6m", wantOK: true},
		{name: "5m lower edge", hz: 54_000_001, want: "5m", wantOK: true},
		{name: "2m", hz: 144_174_000, want: "2m", wantOK: true},
		{name: "70cm upper edge", hz: 450_000_000, want: "70cm", wantOK: true},
		{name: "23cm", hz: 1_296_200_000, want: "23cm", wantOK: true},
		{name: "1mm upper edge", hz: 250_000_000_000, want: "1mm", wantOK: true},
		{name: "submm not covered", hz: 300_000_000_000, wantOK: false},
		{name: "zero", hz: 0, wantOK: false},
		{name: "negative", hz: -14_074_000, wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := BandForFreq(tt.hz)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
