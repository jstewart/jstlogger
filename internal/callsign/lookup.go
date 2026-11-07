package callsign

import "net/url"

// QRZURL returns the QRZ.com page for the base callsign of call, or "" when
// call has no base callsign.
func QRZURL(call string) string {
	base := Base(call)
	if base == "" {
		return ""
	}
	return "https://www.qrz.com/db/" + url.PathEscape(base)
}

// HamQTHURL returns the HamQTH page for the base callsign of call, or ""
// when call has no base callsign.
func HamQTHURL(call string) string {
	base := Base(call)
	if base == "" {
		return ""
	}
	return "https://www.hamqth.com/" + url.PathEscape(base)
}

// EQSLURL returns the eQSL member profile for the base callsign of call, or
// "" when call has no base callsign.
//
// eQSL takes the callsign as the whole query string, with no parameter
// name: Member.cfm?W1AW shows the profile, while Member.cfm?CALLSIGN=W1AW
// reports that the user is not found.
func EQSLURL(call string) string {
	base := Base(call)
	if base == "" {
		return ""
	}
	return "https://www.eqsl.cc/Member.cfm?" + url.QueryEscape(base)
}
