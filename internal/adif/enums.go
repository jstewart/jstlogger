package adif

import "slices"

// Enum is one value of an ADIF enumeration: the code ADIF stores, such as
// "G", and what it means, such as "grayline".
type Enum struct {
	Code        string
	Description string
}

// Codes returns the codes of enums, in order.
func Codes(enums []Enum) []string {
	out := make([]string, len(enums))
	for i, e := range enums {
		out[i] = e.Code
	}
	return out
}

// propModes is the ADIF 3.1.5 Propagation_Mode enumeration (PROP_MODE), in
// ADIF's order.
var propModes = []Enum{
	{"AS", "aircraft scatter"},
	{"AUE", "aurora-E"},
	{"AUR", "aurora"},
	{"BS", "back scatter"},
	{"ECH", "EchoLink"},
	{"EME", "Earth-Moon-Earth"},
	{"ES", "sporadic E"},
	{"F2", "F2 reflection"},
	{"FAI", "field-aligned irregularities"},
	{"GWAVE", "ground wave"},
	{"INTERNET", "Internet-assisted"},
	{"ION", "ionoscatter"},
	{"IRL", "IRLP"},
	{"LOS", "line of sight"},
	{"MS", "meteor scatter"},
	{"RPT", "terrestrial or atmospheric repeater or transponder"},
	{"RS", "rain scatter"},
	{"SAT", "satellite"},
	{"TEP", "trans-equatorial"},
	{"TR", "tropospheric ducting"},
}

// antPaths is the ADIF Ant_Path enumeration (ANT_PATH), in ADIF's order.
var antPaths = []Enum{
	{"G", "grayline"},
	{"O", "other"},
	{"S", "short path"},
	{"L", "long path"},
}

// qslSentStatuses is the ADIF QSL_Sent enumeration (QSL_SENT,
// LOTW_QSL_SENT, EQSL_QSL_SENT and the like), in ADIF's order.
var qslSentStatuses = []Enum{
	{"Y", "yes, sent"},
	{"N", "no, do not send"},
	{"R", "requested by the other station"},
	{"Q", "queued to send"},
	{"I", "ignore or invalid"},
}

// qslSentVias is the ADIF QSL_Via enumeration (QSL_SENT_VIA), in ADIF's
// order. M is deprecated in ADIF, and kept so old logs round-trip.
var qslSentVias = []Enum{
	{"B", "bureau"},
	{"D", "direct"},
	{"E", "electronic"},
	{"M", "manager (deprecated)"},
}

// PropModes returns the ADIF propagation modes, in ADIF's order.
func PropModes() []Enum {
	return slices.Clone(propModes)
}

// AntPaths returns the ADIF antenna paths, in ADIF's order.
func AntPaths() []Enum {
	return slices.Clone(antPaths)
}

// QSLSentStatuses returns the ADIF QSL sent statuses, in ADIF's order.
func QSLSentStatuses() []Enum {
	return slices.Clone(qslSentStatuses)
}

// QSLSentVias returns the ADIF means of sending a QSL, in ADIF's order.
func QSLSentVias() []Enum {
	return slices.Clone(qslSentVias)
}
