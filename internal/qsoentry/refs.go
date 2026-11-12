package qsoentry

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

var (
	// digits matches an unsigned whole number.
	digits = regexp.MustCompile(`^\d+$`)
	// decimal matches an unsigned decimal number without an exponent.
	decimal = regexp.MustCompile(`^(\d+\.?\d*|\.\d+)$`)
	// iotaRef matches an IOTA reference, e.g. NA-001.
	iotaRef = regexp.MustCompile(`^(AF|AN|AS|EU|NA|OC|SA)-\d{3}$`)
	// sotaRef matches a SOTA summit reference, e.g. W1/HA-001.
	sotaRef = regexp.MustCompile(`^[A-Z0-9]{1,4}/[A-Z]{2}-\d{3}$`)
	// wwffRef matches a WWFF reference, e.g. KFF-1234.
	wwffRef = regexp.MustCompile(`^[A-Z0-9]{1,4}FF-\d{4}$`)
	// potaRef matches one POTA park reference with an optional location,
	// e.g. K-1234 or K-1234@US-MA.
	potaRef = regexp.MustCompile(`^[A-Z0-9]{1,4}-\d{4,5}(@[A-Z0-9-]+)?$`)
)

// continents are the ADIF continent codes.
var continents = []string{"AF", "AN", "AS", "EU", "NA", "OC", "SA"}

// parseIntRange parses s as a whole number from lo to hi, reporting false
// when it is not one.
func parseIntRange(s string, lo, hi int) (int, bool) {
	if !digits.MatchString(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < lo || n > hi {
		return 0, false
	}
	return n, true
}

// parseDXCC parses a DXCC entity code, 0 to 999; 0 means no entity.
func parseDXCC(s string) (int, bool) {
	return parseIntRange(s, 0, 999)
}

// parseCQZone parses a CQ zone, 1 to 40.
func parseCQZone(s string) (int, bool) {
	return parseIntRange(s, 1, 40)
}

// parseITUZone parses an ITU zone, 1 to 90.
func parseITUZone(s string) (int, bool) {
	return parseIntRange(s, 1, 90)
}

// parsePower parses a transmit power, a positive decimal number of watts.
func parsePower(s string) (float64, bool) {
	if !decimal.MatchString(s) {
		return 0, false
	}
	w, err := strconv.ParseFloat(s, 64)
	if err != nil || w <= 0 {
		return 0, false
	}
	return w, true
}

// isContinent reports whether s is an ADIF continent code, ignoring case.
func isContinent(s string) bool {
	return slices.Contains(continents, strings.ToUpper(s))
}

// isIOTA reports whether s is an IOTA reference, ignoring case.
func isIOTA(s string) bool {
	return iotaRef.MatchString(strings.ToUpper(s))
}

// isSOTA reports whether s is a SOTA summit reference, ignoring case.
func isSOTA(s string) bool {
	return sotaRef.MatchString(strings.ToUpper(s))
}

// isWWFF reports whether s is a WWFF reference, ignoring case.
func isWWFF(s string) bool {
	return wwffRef.MatchString(strings.ToUpper(s))
}

// isPOTA reports whether s is a single POTA park reference, ignoring case.
func isPOTA(s string) bool {
	return potaRef.MatchString(strings.ToUpper(s))
}

// normalizePOTA splits a comma-separated list of POTA references as
// splitTags does, upper-cases each and joins them with commas. When one is
// not a POTA reference, it returns that reference, upper-cased, as bad.
func normalizePOTA(s string) (refs, bad string) {
	parts := splitTags(strings.ToUpper(s))
	for _, p := range parts {
		if !isPOTA(p) {
			return "", p
		}
	}
	return strings.Join(parts, ","), ""
}

// isEmail reports whether s looks like an email address: a single @ with
// text on both sides, and no spaces.
func isEmail(s string) bool {
	local, domain, ok := strings.Cut(s, "@")
	return ok && local != "" && domain != "" &&
		!strings.Contains(domain, "@") && !strings.ContainsFunc(s, unicode.IsSpace)
}
