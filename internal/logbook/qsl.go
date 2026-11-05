package logbook

import "strings"

// qslSentWords maps the ADIF QSL_SENT enumeration to display words.
var qslSentWords = map[string]string{
	"Y": "Yes",
	"N": "No",
	"R": "Requested",
	"Q": "Queued",
	"I": "Ignored",
}

// qslRcvdWords maps the ADIF QSL_RCVD enumeration to display words.
var qslRcvdWords = map[string]string{
	"Y": "Yes",
	"N": "No",
	"R": "Requested",
	"I": "Ignored",
	"V": "Verified",
}

// QSLSentText returns the display word for an ADIF QSL_SENT value such as
// Confirmation.Sent, e.g. "Q" becomes "Queued". Input is trimmed and matched
// ignoring case; "" stays "" and an unknown value is returned trimmed.
func QSLSentText(s string) string {
	return qslText(qslSentWords, s)
}

// QSLRcvdText returns the display word for an ADIF QSL_RCVD value such as
// Confirmation.Rcvd, e.g. "V" becomes "Verified". Input is trimmed and
// matched ignoring case; "" stays "" and an unknown value is returned
// trimmed.
func QSLRcvdText(s string) string {
	return qslText(qslRcvdWords, s)
}

// qslText looks up the trimmed, upper-cased s in words, falling back to the
// trimmed s.
func qslText(words map[string]string, s string) string {
	s = strings.TrimSpace(s)
	if w, ok := words[strings.ToUpper(s)]; ok {
		return w
	}
	return s
}
