package adif

import (
	"slices"
	"strings"
)

// mode is one entry of the ADIF Mode enumeration and the submodes ADIF's
// Submode enumeration places under it.
type mode struct {
	name     string
	submodes []string
}

// modes is the ADIF 3.1.5 Mode enumeration, without the import-only modes,
// in ADIF's alphabetical order, each with its submodes.
var modes = []mode{
	{"AM", nil},
	{"ARDOP", nil},
	{"ATV", nil},
	{"CHIP", []string{"CHIP128", "CHIP64"}},
	{"CLO", nil},
	{"CONTESTI", nil},
	{"CW", []string{"PCW"}},
	{"DIGITALVOICE", []string{"C4FM", "DMR", "DSTAR", "FREEDV", "M17"}},
	{"DOMINO", []string{"DOM-M", "DOM11", "DOM16", "DOM22", "DOM4", "DOM44", "DOM5", "DOM8", "DOM88", "DOMINOEX", "DOMINOF"}},
	{"DYNAMIC", []string{"VARA FM 1200", "VARA FM 9600", "VARA HF", "VARA SATELLITE"}},
	{"FAX", nil},
	{"FM", nil},
	{"FSK441", nil},
	{"FT8", nil},
	{"HELL", []string{"FMHELL", "FSKHELL", "HELL80", "HELLX5", "HELLX9", "HFSK", "PSKHELL", "SLOWHELL"}},
	{"ISCAT", []string{"ISCAT-A", "ISCAT-B"}},
	{"JT4", []string{"JT4A", "JT4B", "JT4C", "JT4D", "JT4E", "JT4F", "JT4G"}},
	{"JT44", nil},
	{"JT65", []string{"JT65A", "JT65B", "JT65B2", "JT65C", "JT65C2"}},
	{"JT9", []string{"JT9-1", "JT9-10", "JT9-2", "JT9-30", "JT9-5", "JT9A", "JT9B", "JT9C", "JT9D", "JT9E", "JT9E FAST", "JT9F", "JT9F FAST", "JT9G", "JT9G FAST", "JT9H", "JT9H FAST"}},
	{"MFSK", []string{"FSQCALL", "FST4", "FST4W", "FT4", "JS8", "JTMS", "MFSK11", "MFSK128", "MFSK128L", "MFSK16", "MFSK22", "MFSK31", "MFSK32", "MFSK4", "MFSK64", "MFSK64L", "MFSK8", "Q65"}},
	{"MSK144", nil},
	{"MT63", nil},
	{"OLIVIA", []string{"OLIVIA 16/1000", "OLIVIA 16/500", "OLIVIA 32/1000", "OLIVIA 4/125", "OLIVIA 4/250", "OLIVIA 8/250", "OLIVIA 8/500"}},
	{"OPERA", []string{"OPERA-BEACON", "OPERA-QSO"}},
	{"PAC", []string{"PAC2", "PAC3", "PAC4"}},
	{"PAX", []string{"PAX2"}},
	{"PKT", nil},
	{"PSK", []string{
		"8PSK1000", "8PSK1000F", "8PSK1200F", "8PSK125", "8PSK125F", "8PSK125FL",
		"8PSK250", "8PSK250F", "8PSK250FL", "8PSK500", "8PSK500F", "FSK31",
		"PSK10", "PSK1000", "PSK1000RC2", "PSK125", "PSK125RC10", "PSK125RC12", "PSK125RC16",
		"PSK125RC4", "PSK125RC5", "PSK250", "PSK250RC2", "PSK250RC3", "PSK250RC5",
		"PSK250RC6", "PSK250RC7", "PSK31", "PSK500", "PSK500RC2", "PSK500RC3",
		"PSK500RC4", "PSK63", "PSK63F", "PSK63RC10", "PSK63RC20", "PSK63RC32",
		"PSK63RC4", "PSK63RC5", "PSK800RC2", "PSKAM10", "PSKAM31",
		"PSKAM50", "PSKFEC31", "QPSK125", "QPSK250", "QPSK31", "QPSK500",
		"QPSK63", "SIM31",
	}},
	{"Q15", nil},
	{"QRA64", []string{"QRA64A", "QRA64B", "QRA64C", "QRA64D", "QRA64E"}},
	{"ROS", []string{"ROS-EME", "ROS-HF", "ROS-MF"}},
	{"RTTY", []string{"ASCI"}},
	{"RTTYM", nil},
	{"SSB", []string{"LSB", "USB"}},
	{"SSTV", nil},
	{"T10", nil},
	{"THOR", []string{"THOR-M", "THOR100", "THOR11", "THOR16", "THOR22", "THOR25X4", "THOR4", "THOR5", "THOR50X1", "THOR50X2", "THOR8"}},
	{"THRB", []string{"THRBX", "THRBX1", "THRBX2", "THRBX4", "THROB1", "THROB2", "THROB4"}},
	{"TOR", []string{"AMTORFEC", "GTOR", "NAVTEX", "SITORB"}},
	{"V4", nil},
	{"VOI", nil},
	{"WINMOR", nil},
	{"WSPR", nil},
}

// Modes returns the names of the ADIF modes, in alphabetical order.
func Modes() []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = m.name
	}
	return out
}

// Submodes returns the ADIF submodes of the named mode, matched ignoring
// case, or nil when the mode has none or is not an ADIF mode.
func Submodes(name string) []string {
	i := slices.IndexFunc(modes, func(m mode) bool { return strings.EqualFold(m.name, name) })
	if i < 0 {
		return nil
	}
	return slices.Clone(modes[i].submodes)
}
