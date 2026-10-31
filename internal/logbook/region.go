package logbook

// ituZoneRegion maps an ITU zone to its ITU region, only for zones that lie
// entirely within one region. Index 0 is unused; 0 means "not known".
//
// Sources:
//   - Zone membership: the ARRL DXCC List, current entities, February 2020
//     (https://www.arrl.org/files/file/DXCC/2020%20Current_Deleted.txt). Its
//     multi-zone footnotes were taken from the same list as mirrored at
//     https://www.qsl.net/kl7jef/dxccentitylist.htm (e.g. China 33, 42-44;
//     Antarctica 67, 69-74).
//   - Region boundaries: ITU Radio Regulations Article 5 (RR 5.3-5.9), Lines
//     A, B and C. Region 1 also includes the whole of Armenia, Azerbaijan,
//     Georgia, Kazakhstan, Kyrgyzstan, Mongolia, Russia, Tajikistan,
//     Turkmenistan, Turkey, Ukraine and Uzbekistan, and excludes Iran.
//
// A zone is listed only when every DXCC entity in it falls in the same
// region. Left out:
//   - 33: China (R3) with Mongolia and Asiatic Russia (R1).
//   - 53: Mauritius, Madagascar etc. (R1) with Rodrigues, east of 60°E (R3).
//   - 61: Hawaii (R2) with Baker & Howland and Palmyra, south of 10°N (R3).
//   - 63: French Polynesia (R3) with Easter I., east of 120°W (R2).
//   - 67-74: the Antarctic sectors, whose boundaries we can't place; 68 also
//     spans 60°E (Crozet R1, Kerguelen R3).
//   - 75: Greenland and Canada (R2) with Asiatic Russia and Franz Josef Land
//     (R1).
//   - 76-89: ocean zones with no DXCC entity.
var ituZoneRegion = [91]int{
	// Region 2: North, Central and South America, Greenland.
	1: 2, 2: 2, 3: 2, 4: 2, 5: 2, 6: 2, 7: 2, 8: 2, 9: 2, 10: 2,
	11: 2, 12: 2, 13: 2, 14: 2, 15: 2, 16: 2,
	// Region 1: Europe, Africa, the Middle East, Russia, Central Asia,
	// Mongolia and the South Atlantic islands east of Line B.
	17: 1, 18: 1, 19: 1, 20: 1, 21: 1, 22: 1, 23: 1, 24: 1, 25: 1, 26: 1,
	27: 1, 28: 1, 29: 1, 30: 1, 31: 1, 32: 1, 34: 1, 35: 1, 36: 1, 37: 1,
	38: 1, 39: 1, 46: 1, 47: 1, 48: 1, 52: 1, 57: 1, 66: 1,
	// Region 3: Iran, South and East Asia, Oceania west of Line C.
	40: 3, 41: 3, 42: 3, 43: 3, 44: 3, 45: 3, 49: 3, 50: 3, 51: 3, 54: 3,
	55: 3, 56: 3, 58: 3, 59: 3, 60: 3, 62: 3, 64: 3, 65: 3, 90: 3,
}

// ITURegionForZone returns the ITU region (1, 2 or 3) for an ITU zone
// (1-90). It is best effort: it reports false for a zone that straddles a
// region boundary or that cannot be placed confidently, and for numbers out
// of range.
func ITURegionForZone(zone int) (int, bool) {
	if zone < 1 || zone >= len(ituZoneRegion) {
		return 0, false
	}
	r := ituZoneRegion[zone]
	return r, r != 0
}

// ITURegionForLocation returns an approximate ITU region (1, 2 or 3) for a
// point in decimal degrees. It is a coarse heuristic using meridians, not
// the real ITU Lines A, B and C, so it is wrong near the boundaries (for
// example Iran, which is in Region 3, or Pacific islands west of 170°W):
//
//   - Region 2 if -170 <= lon <= -25 (the Americas, Greenland, Hawaii).
//   - Region 1 if -25 < lon <= 60, or lon > 60 and lat >= 50 (Europe,
//     Africa, the Middle East, Asiatic Russia).
//   - Region 3 otherwise.
func ITURegionForLocation(lat, lon float64) int {
	switch {
	case lon >= -170 && lon <= -25:
		return 2
	case lon > -25 && lon <= 60, lon > 60 && lat >= 50:
		return 1
	default:
		return 3
	}
}
