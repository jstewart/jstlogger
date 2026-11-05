package callsign

import (
	"slices"
	"strings"
)

// modifiers are the slash-separated callsign parts that describe how or
// where a station operates rather than who it is: portable, mobile,
// maritime mobile, aeronautical mobile, low power, alternate station,
// repeater and lighthouse.
var modifiers = []string{"P", "M", "MM", "AM", "QRP", "QRPP", "A", "B", "R", "LH"}

// Base returns the base callsign of call, the one lookup services list the
// operator under. It trims surrounding spaces, upper-cases the call and
// splits it on "/". It then drops empty parts, known modifiers (P, M, MM,
// AM, QRP, QRPP, A, B, R and LH) and single digits, which name a call
// area as in W1AW/7.
//
// Of the parts left, Base returns the longest, on the heuristic that a
// country prefix such as VE3 or KH6 is shorter than the callsign it is
// attached to. When parts tie for longest, it returns the last of them.
// Base returns "" when call is empty or no part is left.
func Base(call string) string {
	var base string
	for part := range strings.SplitSeq(strings.ToUpper(strings.TrimSpace(call)), "/") {
		if part == "" || isCallArea(part) || slices.Contains(modifiers, part) {
			continue
		}
		if len(part) >= len(base) {
			base = part
		}
	}
	return base
}

// isCallArea reports whether part is a single digit naming a call area.
func isCallArea(part string) bool {
	return len(part) == 1 && part[0] >= '0' && part[0] <= '9'
}
