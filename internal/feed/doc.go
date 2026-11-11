// Package feed holds what the app's downloaded reference lists have in
// common: the metadata of the last successful download, a conditional GET
// that skips a list the server reports unchanged, a cap on the size of a
// downloaded body, and the text encoding of sync times in the logbook
// database.
//
// Each list keeps its own package, tables and parser; this package knows
// nothing about any of them and imports nothing from the app.
package feed
