// Package lotw keeps a local copy of the ARRL Logbook of The World (LoTW)
// user activity feed and answers whether, and how recently, a callsign has
// uploaded to LoTW.
//
// The feed is a CSV file published by the ARRL at DefaultURL, one row per
// callsign in the form CALL,YYYY-MM-DD,HH:MM:SS, giving the UTC time of that
// callsign's most recent upload. A Syncer downloads it, using conditional
// requests so an unchanged feed is not downloaded again, and a Store keeps it
// in the lotw_user and lotw_sync tables of the logbook database, created by
// the logbook's migrations.
package lotw
