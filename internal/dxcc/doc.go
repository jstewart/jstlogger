// Package dxcc resolves amateur radio callsigns to their DXCC entity, with
// the entity's continent, CQ and ITU zones and location.
//
// Country data from AD1C's Big CTY country files, https://www.country-files.com.
// The package embeds a snapshot of cty.csv; Default parses it, and Parse
// reads any other copy. Refresh the snapshot with `just cty`.
//
// cty.csv lists the current entity for each prefix and exception call, with
// no dates, so a call resolves to its current entity even when an older
// QSO with it counted for a different one.
package dxcc
