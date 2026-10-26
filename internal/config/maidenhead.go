package config

// ValidGridsquare reports whether s is a 4, 6 or 8 character Maidenhead
// locator, ignoring case. Character pairs must be, in order: field (A–R),
// square (0–9), subsquare (A–X) and extended square (0–9).
func ValidGridsquare(s string) bool {
	if len(s) != 4 && len(s) != 6 && len(s) != 8 {
		return false
	}
	// Upper bound for each pair; letters start at 'A', digits at '0'.
	limits := [...]byte{'R', '9', 'X', '9'}
	for i := range len(s) {
		c := s[i]
		// ASCII-only upper-casing: strings.ToUpper could change the byte
		// length of non-ASCII input and let it slip through.
		if 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		hi := limits[i/2]
		lo := byte('0')
		if hi > '9' {
			lo = 'A'
		}
		if c < lo || c > hi {
			return false
		}
	}
	return true
}
