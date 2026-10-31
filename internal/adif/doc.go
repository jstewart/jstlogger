// Package adif reads and writes ADIF (Amateur Data Interchange Format) files
// in the tagged ADI encoding, and compares record sets semantically.
//
// The package is a pure format layer: it knows nothing about QSOs, storage or
// the rest of jstlogger. A Record is an ordered list of raw field name/value
// pairs; interpreting those values is left to callers.
//
// Parse reads a whole ADI file, NewWriter writes one, and Diff reports the
// differences between two record sets, which is useful for verifying that an
// import followed by an export preserves the data.
package adif
