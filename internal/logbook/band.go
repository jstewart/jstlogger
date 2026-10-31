package logbook

// band is one entry of the ADIF Band enumeration; edges are inclusive Hz.
type band struct {
	name   string
	lowHz  int64
	highHz int64
}

// bands is the ADIF 3.1.4 Band enumeration from 2190m to 1mm, in ascending
// frequency order. The open-ended "submm" band is not included.
var bands = []band{
	{"2190m", 135_700, 137_800},
	{"630m", 472_000, 479_000},
	{"560m", 501_000, 504_000},
	{"160m", 1_800_000, 2_000_000},
	{"80m", 3_500_000, 4_000_000},
	{"60m", 5_060_000, 5_450_000},
	{"40m", 7_000_000, 7_300_000},
	{"30m", 10_100_000, 10_150_000},
	{"20m", 14_000_000, 14_350_000},
	{"17m", 18_068_000, 18_168_000},
	{"15m", 21_000_000, 21_450_000},
	{"12m", 24_890_000, 24_990_000},
	{"10m", 28_000_000, 29_700_000},
	{"8m", 40_000_000, 45_000_000},
	{"6m", 50_000_000, 54_000_000},
	{"5m", 54_000_001, 69_900_000},
	{"4m", 70_000_000, 71_000_000},
	{"2m", 144_000_000, 148_000_000},
	{"1.25m", 222_000_000, 225_000_000},
	{"70cm", 420_000_000, 450_000_000},
	{"33cm", 902_000_000, 928_000_000},
	{"23cm", 1_240_000_000, 1_300_000_000},
	{"13cm", 2_300_000_000, 2_450_000_000},
	{"9cm", 3_300_000_000, 3_500_000_000},
	{"6cm", 5_650_000_000, 5_925_000_000},
	{"3cm", 10_000_000_000, 10_500_000_000},
	{"1.25cm", 24_000_000_000, 24_250_000_000},
	{"6mm", 47_000_000_000, 47_200_000_000},
	{"4mm", 75_500_000_000, 81_000_000_000},
	{"2.5mm", 119_980_000_000, 123_000_000_000},
	{"2mm", 134_000_000_000, 149_000_000_000},
	{"1mm", 241_000_000_000, 250_000_000_000},
}

// BandForFreq returns the ADIF band name (e.g. "20m") containing hz, using
// the ADIF 3.1 Band enumeration with inclusive edges. It reports false when
// hz is outside every band.
func BandForFreq(hz int64) (string, bool) {
	for _, b := range bands {
		if hz >= b.lowHz && hz <= b.highHz {
			return b.name, true
		}
	}
	return "", false
}
