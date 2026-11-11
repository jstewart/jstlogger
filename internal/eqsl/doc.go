// Package eqsl keeps a local copy of the eQSL.cc Authenticity Guaranteed
// (AG) member list and answers whether a callsign is an AG member.
//
// The list is a plain-text file published by eQSL.cc at DefaultURL: a
// header line ("List of AG members as of ..."), then one callsign per line.
// A Syncer downloads it, using conditional requests so an unchanged list is
// not downloaded again, and a Store keeps it in the eqsl_ag and eqsl_sync
// tables of the logbook database, created by the logbook's migrations.
package eqsl
