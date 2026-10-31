// Package logbook stores QSOs, their confirmations and tags.
//
// The data model follows ADIF so that import and export are lossless:
// frequently filtered or displayed fields are promoted to typed QSO fields,
// and every other ADIF field travels in QSO.Fields.
package logbook
