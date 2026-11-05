// Package callsign normalizes amateur radio callsigns and builds links to
// the online callsign lookup services.
//
// A logged call may carry a prefix or suffix, such as VE3/W1AW or W1AW/P,
// but lookup services list the operator under the base callsign. Base
// strips those parts, and the lookup URL builders (QRZURL, HamQTHURL and
// EQSLURL) look up the base callsign.
package callsign
