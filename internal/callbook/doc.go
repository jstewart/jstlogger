// Package callbook looks up callsigns in online callbooks (QRZ.com, HamQTH).
//
// A Looker answers a lookup with a Result: the operator's name, QTH, grid,
// state, county and email. Service clients implement Looker for each
// callbook. Chain tries several Lookers in order and returns the first
// success, so a second service can fill in when the first is down or does
// not list the call. Cache sits in front of a Looker and remembers
// successful results, for a TTL or for the session, so logging the same
// station again does not spend another lookup.
//
// Lookers report failures with the sentinel errors ErrNotFound, ErrAuth
// and ErrRateLimit, or a *NetworkError when a service could not be reached
// or gave an unexpected reply. Use errors.Is and errors.As to tell them
// apart.
//
// A portable call such as VE3/W1AW or W1AW/P is looked up under its base
// callsign. The location fields are then dropped from the Result, since
// they describe where the operator lives, not where the station is.
package callbook
