// Package fle parses Fast Log Entry text, in the Simple FLE syntax of
// Wavelog, into QSOs. Each line either sets what the QSOs after it share,
// such as the date, band, frequency and mode, or logs one QSO from a time,
// a callsign and optional reports, grid, references, name, comment and QSL
// message. A line that can't be read is set aside with the reason, and
// parsing goes on with the next one. The package knows nothing about the
// terminal UI.
package fle

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

// Options sets what Parse needs from outside the text.
type Options struct {
	// Now is the current time. The date is Now's UTC date until a line
	// sets another, and a QSO can't start after Now.
	Now time.Time
	// Station is our side of each QSO, copied in with Station.Fill.
	Station qsoentry.Station
}

// QSO is a QSO parsed from the text.
type QSO struct {
	// Line is the 1-based number of the line the QSO is on.
	Line int
	// QSO is the QSO, as qsoentry.Build returns it.
	QSO logbook.QSO
}

// Ignored is a line Parse could not read, which it skipped.
type Ignored struct {
	// Line is the 1-based number of the line.
	Line int
	// Text is the line as written, without trailing spaces.
	Text string
	// Reason says why the line was skipped.
	Reason string
}

// Result is what Parse read from the text: the QSOs, and the lines it
// skipped, each in the order of the text.
type Result struct {
	QSOs    []QSO
	Ignored []Ignored
}

// oneDay is the length of a day in UTC, which has no daylight saving.
const oneDay = 24 * time.Hour

var (
	// freqNumber matches a frequency in MHz, a decimal number with a dot.
	freqNumber = regexp.MustCompile(`^(\d+\.\d*|\.\d+)$`)
	// callsign matches a callsign: letters, digits and slashes.
	callsign = regexp.MustCompile(`^[A-Z0-9/]+$`)
	// report matches a signal report, a whole number with an optional sign.
	report = regexp.MustCompile(`^[+-]?\d+$`)
	// grid matches a Maidenhead locator of 4 to 10 characters.
	grid = regexp.MustCompile(`^[A-R]{2}\d{2}([A-X]{2}(\d{2}([A-X]{2})?)?)?$`)
	// wwffRef matches a WWFF reference, e.g. KFF-1234.
	wwffRef = regexp.MustCompile(`^[A-Z0-9]{1,4}FF-\d{4}$`)
	// sotaRef matches a SOTA summit reference, e.g. W1/HA-001.
	sotaRef = regexp.MustCompile(`^[A-Z0-9]{1,4}/[A-Z]{2}-\d{3}$`)
	// iotaRef matches an IOTA reference, e.g. NA-001.
	iotaRef = regexp.MustCompile(`^(AF|AN|AS|EU|NA|OC|SA)-\d{3}$`)
	// potaRef matches one POTA park reference with an optional location,
	// e.g. K-1234 or K-1234@US-MA.
	potaRef = regexp.MustCompile(`^[A-Z0-9]{1,4}-\d{4,5}(@[A-Z0-9-]+)?$`)
)

// state is what carries from one line to the next.
type state struct {
	// date is the date of the next QSO, midnight UTC.
	date time.Time
	// band, freq, mode and submode are as the form takes them; freq is in
	// MHz as typed.
	band, freq, mode, submode string
	// haveTime is set once a QSO has had a full time, and hour and minute
	// are the time of the last QSO.
	haveTime     bool
	hour, minute int
}

// Parse reads text, one line at a time, into QSOs. Each line is either a
// line of operators or a QSO line.
//
// Operators set what the QSOs after them share. Every token on the line
// must be one, or the line changes nothing:
//
//   - a band, such as 20m, which clears the frequency
//   - a frequency in MHz, such as 14.074, which sets the band
//   - an ADIF mode or submode, such as cw or ft4
//   - "date YYYY-MM-DD"
//   - "day +", which moves the date forward a day for each +
//
// A QSO line starts with a time, in one of three forms:
//
//   - HHMM, the full time
//   - two digits, the minute, in the hour of the QSO before
//   - one digit, the last digit of the minute, keeping the tens of the
//     QSO before
//
// A two- or one-digit time earlier than the QSO before moves on to the
// next hour, or to the next ten minutes for one digit, and past midnight
// to the next day. A full HHMM never changes the date.
//
// The callsign comes next, then optionally the report sent and the report
// received. A missing report is the mode's usual one, and a single digit
// is the signal part of it. After those come, in any order:
//
//   - a grid, optionally after #
//   - a POTA, WWFF, SOTA or IOTA reference
//   - the name, after @
//   - a comment, in <>
//   - a QSL message, in []
//
// Each QSO is built with qsoentry.Build. Blank lines are skipped. A line
// that can't be read, or whose QSO Build rejects, is Ignored and changes
// nothing that carries to later lines.
func Parse(text string, opts Options) Result {
	var r Result
	st := state{date: opts.Now.UTC().Truncate(oneDay)}
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		next, q, err := st.line(line, opts)
		if err != nil {
			r.Ignored = append(r.Ignored, Ignored{Line: i + 1, Text: line, Reason: err.Error()})
			continue
		}
		st = next
		if q != nil {
			r.QSOs = append(r.QSOs, QSO{Line: i + 1, QSO: *q})
		}
	}
	return r
}

// line reads one line, returning the state after it and the QSO it logs,
// or nil when it is a line of operators.
func (s state) line(line string, opts Options) (state, *logbook.QSO, error) {
	toks, err := tokenize(line)
	if err != nil {
		return s, nil, err
	}
	if !allDigits(toks[0]) {
		next, err := s.operators(toks)
		return next, nil, err
	}
	next, q, err := s.qso(toks, opts)
	if err != nil {
		return s, nil, err
	}
	return next, &q, nil
}

// closers are the brackets a token may start with, which run to their
// closing bracket, spaces and all.
var closers = map[byte]byte{'<': '>', '[': ']'}

// tokenize splits line at spaces, keeping a token in <> or [] whole.
func tokenize(line string) ([]string, error) {
	var toks []string
	for i := 0; i < len(line); {
		c := line[i]
		if isSpace(c) {
			i++
			continue
		}
		if closer, ok := closers[c]; ok {
			j := strings.IndexByte(line[i+1:], closer)
			if j < 0 {
				return nil, fmt.Errorf("%c is not closed with %c", c, closer)
			}
			end := i + 1 + j + 1
			toks = append(toks, line[i:end])
			i = end
			continue
		}
		j := i
		for j < len(line) && !isSpace(line[j]) {
			j++
		}
		toks = append(toks, line[i:j])
		i = j
	}
	return toks, nil
}

// isSpace reports whether c is an ASCII space or tab.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f'
}

// operators applies a line of operators to s.
func (s state) operators(toks []string) (state, error) {
	next := s
	for i := 0; i < len(toks); i++ {
		tok := strings.ToLower(toks[i])
		switch {
		case tok == "date":
			i++
			if i >= len(toks) {
				return s, errors.New("date needs a YYYY-MM-DD date")
			}
			d, err := time.Parse(qsoentry.DateLayout, toks[i])
			if err != nil {
				return s, fmt.Errorf("date %q is not YYYY-MM-DD", toks[i])
			}
			next.date = d
		case tok == "day":
			i++
			if i >= len(toks) || strings.Trim(toks[i], "+") != "" {
				return s, errors.New("day needs a + for each day forward")
			}
			next.date = next.date.AddDate(0, 0, len(toks[i]))
		case logbook.IsBand(tok):
			next.band, next.freq = tok, ""
		case freqNumber.MatchString(tok):
			hz, err := qsoentry.ParseFreq(tok)
			if err != nil {
				return s, fmt.Errorf("frequency %s: %w", tok, err)
			}
			band, ok := logbook.BandForFreq(hz)
			if !ok {
				return s, fmt.Errorf("%s MHz is not in any band", tok)
			}
			next.band, next.freq = band, tok
		case adif.ModeOf(tok) != "":
			mode, upper := adif.ModeOf(tok), strings.ToUpper(tok)
			next.mode, next.submode = mode, ""
			if mode != upper {
				next.submode = upper
			}
		default:
			return s, fmt.Errorf("%q is not a band, frequency, mode, date or day", toks[i])
		}
	}
	return next, nil
}

// qso reads a QSO line, returning the state after it and the QSO.
func (s state) qso(toks []string, opts Options) (state, logbook.QSO, error) {
	next := s
	if err := next.clock(toks[0]); err != nil {
		return s, logbook.QSO{}, err
	}
	if len(toks) < 2 {
		return s, logbook.QSO{}, errors.New("a callsign must follow the time")
	}
	call := strings.ToUpper(toks[1])
	if !isCallsign(call) {
		return s, logbook.QSO{}, fmt.Errorf("%q is not a callsign", toks[1])
	}

	var f qsoentry.Form
	opts.Station.Fill(&f)
	f[qsoentry.Date] = next.date.Format(qsoentry.DateLayout)
	f[qsoentry.TimeOn] = fmt.Sprintf("%02d%02d", next.hour, next.minute)
	f[qsoentry.Call] = call
	f[qsoentry.Mode] = next.mode
	f[qsoentry.Submode] = next.submode
	f[qsoentry.Freq] = next.freq
	f[qsoentry.Band] = next.band

	rest := toks[2:]
	var reports []string
	for len(reports) < 2 && len(rest) > 0 && report.MatchString(rest[0]) {
		reports = append(reports, rest[0])
		rest = rest[1:]
	}
	reports = append(reports, "", "")
	def := qsoentry.DefaultRST(next.mode, next.submode)
	f[qsoentry.RSTSent] = expandReport(reports[0], def)
	f[qsoentry.RSTRcvd] = expandReport(reports[1], def)

	if err := segments(&f, rest); err != nil {
		return s, logbook.QSO{}, err
	}
	q, err := qsoentry.Build(f, opts.Now)
	if err != nil {
		return s, logbook.QSO{}, errors.New(strings.TrimPrefix(err.Error(), "qsoentry: "))
	}
	return next, q, nil
}

// clock sets the time of the QSO from tok: HHMM; two digits, a minute of
// the hour of the QSO before; or one digit, the last digit of that QSO's
// minute. A time lower than that QSO's moves on to the next hour, or the
// next ten minutes for one digit, and past midnight to the next day.
func (s *state) clock(tok string) error {
	switch len(tok) {
	case 4:
		h, _ := strconv.Atoi(tok[:2])
		m, _ := strconv.Atoi(tok[2:])
		if h > 23 || m > 59 {
			return fmt.Errorf("time %q is not a time of day", tok)
		}
		s.hour, s.minute, s.haveTime = h, m, true
	case 1, 2:
		m, _ := strconv.Atoi(tok)
		switch {
		case m > 59:
			return fmt.Errorf("minute %q is out of range", tok)
		case !s.haveTime:
			return fmt.Errorf("minute %q has no hour yet; give the first QSO a full time, HHMM", tok)
		}
		step := 60
		if len(tok) == 1 {
			// One digit is the last digit of the minute, keeping the tens.
			m += s.minute / 10 * 10
			step = 10
		}
		if m < s.minute {
			m += step
		}
		if m >= 60 {
			m -= 60
			s.hour++
		}
		if s.hour == 24 {
			s.hour = 0
			s.date = s.date.Add(oneDay)
		}
		s.minute = m
	default:
		return fmt.Errorf("time %q is not HHMM or a minute", tok)
	}
	return nil
}

// segments reads what follows a QSO's reports into f: a grid, references,
// name, comment and QSL message.
func segments(f *qsoentry.Form, toks []string) error {
	for _, tok := range toks {
		up := strings.ToUpper(tok)
		switch {
		case strings.HasPrefix(tok, "<"):
			f[qsoentry.Comment] = tok[1 : len(tok)-1]
		case strings.HasPrefix(tok, "["):
			f[qsoentry.QSLMsg] = tok[1 : len(tok)-1]
		case strings.HasPrefix(tok, "@") && len(tok) > 1:
			f[qsoentry.Name] = tok[1:]
		case grid.MatchString(strings.TrimPrefix(up, "#")):
			f[qsoentry.Gridsquare] = strings.TrimPrefix(up, "#")
		case wwffRef.MatchString(up):
			f[qsoentry.WWFFRef] = up
		case sotaRef.MatchString(up):
			f[qsoentry.SOTARef] = up
		case iotaRef.MatchString(up):
			f[qsoentry.IOTA] = up
		case isPOTAList(up):
			if f[qsoentry.POTARef] != "" {
				up = f[qsoentry.POTARef] + "," + up
			}
			f[qsoentry.POTARef] = up
		default:
			return fmt.Errorf("%q is not a report, grid, reference, name, comment or QSL message", tok)
		}
	}
	return nil
}

// expandReport returns the report r, defaulting to def when it is empty.
// A single unsigned digit is the signal part of the report, filled out to
// the length of def: 5 is 55 when def is 59 and 559 when def is 599.
func expandReport(r, def string) string {
	switch {
	case r == "":
		return def
	case len(r) == 1 && allDigits(r) && allDigits(def) && len(def) == 2:
		return "5" + r
	case len(r) == 1 && allDigits(r) && allDigits(def) && len(def) == 3:
		return "5" + r + "9"
	}
	return r
}

// isCallsign reports whether s, upper-cased, is a callsign: letters,
// digits and slashes, with at least one letter and one digit.
func isCallsign(s string) bool {
	return callsign.MatchString(s) &&
		strings.ContainsAny(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") &&
		strings.ContainsAny(s, "0123456789")
}

// isPOTAList reports whether s, upper-cased, is one or more POTA park
// references separated by commas.
func isPOTAList(s string) bool {
	for p := range strings.SplitSeq(s, ",") {
		if !potaRef.MatchString(p) {
			return false
		}
	}
	return true
}

// allDigits reports whether s is one or more ASCII digits.
func allDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
