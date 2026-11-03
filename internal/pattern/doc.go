// Package pattern parses mutt-style QSO patterns into logbook expressions,
// so users can select QSOs with a short typed query.
//
// A pattern is built from terms, each a '~' followed by a case-sensitive
// letter and, for most letters, a value:
//
//	~A            every QSO
//	~c CALL       call contains CALL
//	~b BAND       on band BAND
//	~m MODE       mode or submode is MODE
//	~y TAG        carries tag TAG
//	~C COUNTRY    country is COUNTRY, ignoring case
//	~s STATE      state or province is STATE (ADIF code, e.g. MA)
//	~o CONT       on continent CONT: AF, AN, AS, EU, NA, OC or SA
//	~d RANGE      started within RANGE (UTC dates, YYYY-MM-DD):
//	              D (that day), D- (from D), -D (through D), D1-D2 (D1 through D2)
//
// Terms combine with '!' (not), juxtaposition (and) and '|' (or), in that
// order of precedence, and parentheses group:
//
//	~c W1 (~b 20m | ~b 40m) !~m FT8
//
// A value is a bare word ending at whitespace, '(', ')' or '|', or a string
// in double or single quotes in which a backslash escapes the next
// character, so values can contain spaces: ~y "field day". Space between a
// term and its value is optional. As in mutt, a value may start with '~',
// so ~c ~b 20m looks for calls containing "~B" AND'd with the bare word
// 20m, which is an error.
//
// Input with none of the pattern syntax ~ ! | ( ) " ' is a bare value,
// matched against the default fields as if typed as
//
//	~c VALUE | ~b VALUE | ~m VALUE | ~C VALUE | ~s VALUE | ~o VALUE
//
// so W1AW finds the call and 20m the band, as in mutt's simple search. A
// bare value that is not a continent code just matches no continent.
//
// Parse reports malformed patterns with errors that wrap ErrSyntax and name
// the 1-based column of the offending character.
package pattern
