package adif

import "slices"

// propModes is the ADIF 3.1.5 Propagation_Mode enumeration (PROP_MODE), in
// ADIF's order.
var propModes = []string{
	"AS",       // aircraft scatter
	"AUE",      // aurora-E
	"AUR",      // aurora
	"BS",       // back scatter
	"ECH",      // EchoLink
	"EME",      // Earth-Moon-Earth
	"ES",       // sporadic E
	"F2",       // F2 reflection
	"FAI",      // field-aligned irregularities
	"GWAVE",    // ground wave
	"INTERNET", // Internet-assisted
	"ION",      // ionoscatter
	"IRL",      // IRLP
	"LOS",      // line of sight
	"MS",       // meteor scatter
	"RPT",      // terrestrial or atmospheric repeater or transponder
	"RS",       // rain scatter
	"SAT",      // satellite
	"TEP",      // trans-equatorial
	"TR",       // tropospheric ducting
}

// antPaths is the ADIF Ant_Path enumeration (ANT_PATH), in ADIF's order.
var antPaths = []string{
	"G", // grayline
	"O", // other
	"S", // short path
	"L", // long path
}

// qslSentStatuses is the ADIF QSL_Sent enumeration (QSL_SENT,
// LOTW_QSL_SENT, EQSL_QSL_SENT and the like), in ADIF's order.
var qslSentStatuses = []string{
	"Y", // yes: an outgoing QSL has been sent
	"N", // no: do not send an outgoing QSL
	"R", // requested: the contacted station has requested a QSL
	"Q", // queued: an outgoing QSL has been queued to send
	"I", // ignore or invalid
}

// qslSentVias is the ADIF QSL_Via enumeration (QSL_SENT_VIA), in ADIF's
// order.
var qslSentVias = []string{
	"B", // bureau
	"D", // direct
	"E", // electronic
	"M", // manager (deprecated in ADIF, kept so old logs round-trip)
}

// PropModes returns the ADIF propagation modes, in ADIF's order.
func PropModes() []string {
	return slices.Clone(propModes)
}

// AntPaths returns the ADIF antenna paths, in ADIF's order.
func AntPaths() []string {
	return slices.Clone(antPaths)
}

// QSLSentStatuses returns the ADIF QSL sent statuses, in ADIF's order.
func QSLSentStatuses() []string {
	return slices.Clone(qslSentStatuses)
}

// QSLSentVias returns the ADIF means of sending a QSL, in ADIF's order.
func QSLSentVias() []string {
	return slices.Clone(qslSentVias)
}
