// Package dxcc resolves amateur radio callsigns to their DXCC entity, with
// the entity's continent, CQ and ITU zones and location.
//
// Country data from AD1C's Big CTY country file,
// https://www.country-files.com/big-cty/, under the terms in
// cty-copyright.txt. Big CTY is the variant meant for everyday logging: it
// lists exception calls back to 2000 and every prefix that needs a CQ or
// ITU zone override, where the contest file lists only recent calls and
// leaves most zone overrides to the logging software. The package embeds a
// snapshot of Big CTY's cty.csv; Default parses it, and Parse reads any
// other copy. Refresh the snapshot with `just cty`.
//
// AD1C releases a new file every few weeks. A Syncer downloads the latest
// release from DefaultURL, using conditional requests so an unchanged file
// is not downloaded again, and a Store keeps its cty.csv in the dxcc_cty
// table of the logbook database, created by the logbook's migrations.
// Store.Table parses the stored copy, falling back to the snapshot until
// the first sync.
//
// cty.csv lists the current entity for each prefix and exception call, with
// no dates, so a call resolves to its current entity even when an older
// QSO with it counted for a different one.
//
// cty.csv's entity names are AD1C's ("United States"), not ARRL's.
// EntityName gives the name ADIF lists for an entity code ("UNITED STATES
// OF AMERICA"), the form ADIF files and LoTW use for a QSO's country, from
// a copy of ADIF's DXCC_Entity_Code enumeration that includes deleted
// entities.
package dxcc
