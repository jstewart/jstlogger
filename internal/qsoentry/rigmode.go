package qsoentry

import (
	"slices"
	"strings"
)

// rigModes maps the Hamlib modes that name an ADIF mode to that mode and
// submode.
var rigModes = map[string][2]string{
	"USB":   {"SSB", "USB"},
	"LSB":   {"SSB", "LSB"},
	"CW":    {"CW", ""},
	"CWR":   {"CW", ""},
	"AM":    {"AM", ""},
	"FM":    {"FM", ""},
	"WFM":   {"FM", ""},
	"RTTY":  {"RTTY", ""},
	"RTTYR": {"RTTY", ""},
}

// packetModes are the Hamlib modes that carry a data mode, such as FT8,
// which the rig can't name.
var packetModes = []string{"PKTUSB", "PKTLSB", "PKTFM"}

// RigMode returns the ADIF mode and submode for the Hamlib mode rigMode, as
// rigctld reports it, ignoring case. A packet mode such as PKTUSB carries a
// data mode the rig can't name, so it returns dataMode and dataSubmode, the
// data mode last used, which may be empty. It returns "" for other Hamlib
// modes.
func RigMode(rigMode, dataMode, dataSubmode string) (mode, submode string) {
	rigMode = strings.ToUpper(strings.TrimSpace(rigMode))
	if m, ok := rigModes[rigMode]; ok {
		return m[0], m[1]
	}
	if slices.Contains(packetModes, rigMode) {
		return dataMode, dataSubmode
	}
	return "", ""
}

// IsDataMode reports whether mode, ignoring case, is an ADIF mode that a
// rig in a packet mode would be carrying: any mode RigMode can't name from
// the rig alone.
func IsDataMode(mode string) bool {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == "" {
		return false
	}
	for _, m := range rigModes {
		if m[0] == mode {
			return false
		}
	}
	return true
}
