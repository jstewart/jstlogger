// Package callinfo finds what is known about a contacted station: its DXCC
// entity, continent and zones from the country file, and its operator's
// name, QTH, grid, state, county and email from a callbook or, failing
// that, from the latest earlier QSO with the call.
//
// A Looker's Details answers the New QSO screen, which shows the details
// as they are found. Fill fills them into a QSO that arrived complete, such
// as one a UDP listener received, leaving the values it already has alone.
package callinfo
