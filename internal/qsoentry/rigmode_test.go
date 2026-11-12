package qsoentry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRigMode(t *testing.T) {
	tests := []struct {
		rigMode               string
		dataMode, dataSubmode string
		mode, submode         string
	}{
		{rigMode: "USB", mode: "SSB", submode: "USB"},
		{rigMode: "LSB", mode: "SSB", submode: "LSB"},
		{rigMode: "CW", mode: "CW"},
		{rigMode: "CWR", mode: "CW"},
		{rigMode: "AM", mode: "AM"},
		{rigMode: "FM", mode: "FM"},
		{rigMode: "WFM", mode: "FM"},
		{rigMode: "RTTY", mode: "RTTY"},
		{rigMode: "RTTYR", mode: "RTTY"},
		{rigMode: "usb", mode: "SSB", submode: "USB"},
		{rigMode: " CW ", mode: "CW"},
		{rigMode: "PKTUSB", dataMode: "MFSK", dataSubmode: "FT4", mode: "MFSK", submode: "FT4"},
		{rigMode: "PKTLSB", dataMode: "FT8", mode: "FT8"},
		{rigMode: "PKTFM", dataMode: "PSK", dataSubmode: "PSK31", mode: "PSK", submode: "PSK31"},
		{rigMode: "PKTUSB"},
		{rigMode: "USB", dataMode: "FT8", mode: "SSB", submode: "USB"},
		{rigMode: "FAX"},
		{rigMode: ""},
	}
	for _, tt := range tests {
		t.Run(tt.rigMode+"/"+tt.dataMode, func(t *testing.T) {
			mode, submode := RigMode(tt.rigMode, tt.dataMode, tt.dataSubmode)
			assert.Equal(t, tt.mode, mode, "mode")
			assert.Equal(t, tt.submode, submode, "submode")
		})
	}
}

func TestIsDataMode(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{"FT8", true},
		{"MFSK", true},
		{"psk", true},
		{"SSB", false},
		{"CW", false},
		{"AM", false},
		{"FM", false},
		{"RTTY", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDataMode(tt.mode))
		})
	}
}
