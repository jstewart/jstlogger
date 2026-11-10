// Package qsoentry turns the values typed into the New QSO form into a
// logbook.QSO. It parses and validates each field, fills in default signal
// reports and works out what carries over to the next QSO. It knows nothing
// about the terminal UI, which only collects the values and shows the
// errors.
package qsoentry

import (
	"database/sql"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
)

// Field is one field of the New QSO form.
type Field int

// The form's fields, in the order the form shows them.
const (
	Date Field = iota
	TimeOn
	DateOff
	TimeOff
	Call
	Mode
	Submode
	Freq
	Band
	FreqRX
	BandRX
	RSTSent
	RSTRcvd
	Name
	QTH
	Gridsquare
	Comment
	Tags
	// FieldCount is the number of fields.
	FieldCount
)

// fieldNames are the ADIF names of the fields. Tags are not an ADIF field.
var fieldNames = [FieldCount]string{
	"QSO_DATE", "TIME_ON", "QSO_DATE_OFF", "TIME_OFF", "CALL", "MODE",
	"SUBMODE", "FREQ", "BAND", "FREQ_RX", "BAND_RX", "RST_SENT", "RST_RCVD",
	"NAME", "QTH", "GRIDSQUARE", "COMMENT", "TAGS",
}

// String returns the field's upper-case ADIF name, or TAGS for the tags.
func (f Field) String() string {
	if f < 0 || f >= FieldCount {
		return "Field(" + strconv.Itoa(int(f)) + ")"
	}
	return fieldNames[f]
}

// Form holds the text of each field of the New QSO form, as typed.
//
// Dates are YYYY-MM-DD or YYYYMMDD and times HH:MM, HH:MM:SS, HHMM or
// HHMMSS, all UTC. A freq is in MHz or kHz, as ParseFreq reads it. Tags are
// separated by commas.
type Form [FieldCount]string

// Carry returns the values that stay in the form after a QSO is saved, so
// that a batch of QSOs from a paper log is quick to enter: the date, band,
// mode, submode, freq and signal reports.
func (f Form) Carry() Form {
	var c Form
	for _, k := range []Field{Date, Mode, Submode, Freq, Band, RSTSent, RSTRcvd} {
		c[k] = f[k]
	}
	return c
}

// Station is our side of the QSO, which the form does not ask for.
type Station struct {
	// Callsign is our callsign (ADIF STATION_CALLSIGN).
	Callsign string
	// Operator is the operator's callsign (ADIF OPERATOR).
	Operator string
	// Grid is our Maidenhead locator (ADIF MY_GRIDSQUARE).
	Grid string
}

// Errors holds the validation error of each field that has one. Build
// returns it as its error; use errors.As to get it.
type Errors map[Field]error

// Fields returns the fields with errors, in form order.
func (e Errors) Fields() []Field {
	var out []Field
	for f := range FieldCount {
		if e[f] != nil {
			out = append(out, f)
		}
	}
	return out
}

// Error joins the field errors in form order.
func (e Errors) Error() string {
	msgs := make([]string, 0, len(e))
	for _, f := range e.Fields() {
		msgs = append(msgs, e[f].Error())
	}
	return "qsoentry: " + strings.Join(msgs, "; ")
}

// Build validates f and returns the QSO it describes, logged from station.
// now is the current time; a QSO can't start after it. When any field is
// invalid, Build returns an Errors holding every field's error.
func Build(f Form, station Station, now time.Time) (logbook.QSO, error) {
	b := builder{f: f, errs: Errors{}}
	q := logbook.QSO{
		Call:            b.call(),
		Mode:            strings.ToUpper(strings.TrimSpace(f[Mode])),
		Submode:         strings.ToUpper(strings.TrimSpace(f[Submode])),
		RSTSent:         strings.TrimSpace(f[RSTSent]),
		RSTRcvd:         strings.TrimSpace(f[RSTRcvd]),
		Name:            strings.TrimSpace(f[Name]),
		QTH:             strings.TrimSpace(f[QTH]),
		Gridsquare:      b.grid(),
		Comment:         strings.TrimSpace(f[Comment]),
		Tags:            splitTags(f[Tags]),
		StationCallsign: station.Callsign,
		Operator:        station.Operator,
		MyGridsquare:    strings.ToUpper(station.Grid),
	}
	q.StartUTC, q.EndUTC = b.times(now)
	b.mode(q.Mode, q.Submode)
	q.FreqHz, q.Band = b.freqBand(Freq, Band, "freq", "band")
	var bandRX string
	q.FreqRxHz, bandRX = b.freqBand(FreqRX, BandRX, "freq RX", "band RX")
	if bandRX != "" {
		q.Fields = map[string]string{BandRX.String(): bandRX}
	}
	if len(b.errs) > 0 {
		return logbook.QSO{}, b.errs
	}
	return q, nil
}

// builder parses a Form's fields, collecting their errors.
type builder struct {
	f    Form
	errs Errors
}

// fail records err as the error of field k.
func (b *builder) fail(k Field, format string, args ...any) {
	b.errs[k] = fmt.Errorf(format, args...)
}

func (b *builder) call() string {
	call := strings.ToUpper(strings.TrimSpace(b.f[Call]))
	switch {
	case call == "":
		b.fail(Call, "call is required")
	case strings.ContainsFunc(call, unicode.IsSpace):
		b.fail(Call, "call can't contain spaces")
	}
	return call
}

func (b *builder) grid() string {
	grid := strings.ToUpper(strings.TrimSpace(b.f[Gridsquare]))
	if grid == "" {
		return ""
	}
	if _, _, err := logbook.GridCenter(grid); err != nil {
		b.fail(Gridsquare, "%q is not a Maidenhead locator", grid)
	}
	return grid
}

// times returns the start and end of the QSO. The start can't be after now.
// A time off without a date off is on the date of the QSO, or the next day
// when it is earlier than the time on.
func (b *builder) times(now time.Time) (time.Time, sql.Null[time.Time]) {
	var end sql.Null[time.Time]
	date, dateOK := b.date(Date, "date", true)
	on, onOK := b.clock(TimeOn, "time on", true)
	dateOff, dateOffOK := b.date(DateOff, "date off", false)
	off, offOK := b.clock(TimeOff, "time off", false)
	if strings.TrimSpace(b.f[DateOff]) != "" && strings.TrimSpace(b.f[TimeOff]) == "" {
		b.fail(TimeOff, "time off is required with a date off")
	}
	if !dateOK || !onOK {
		return time.Time{}, end
	}

	start := date.Add(on)
	today := now.UTC().Truncate(24 * time.Hour)
	switch {
	case date.After(today):
		b.fail(Date, "date can't be in the future")
	case start.After(now):
		b.fail(TimeOn, "time on can't be in the future")
	}

	if !offOK || strings.TrimSpace(b.f[TimeOff]) == "" {
		return start, end
	}
	switch {
	case strings.TrimSpace(b.f[DateOff]) == "":
		end.V = date.Add(off)
		if end.V.Before(start) {
			end.V = end.V.Add(24 * time.Hour)
		}
	case dateOffOK:
		end.V = dateOff.Add(off)
		if end.V.Before(start) {
			b.fail(TimeOff, "time off can't be before time on")
		}
	default:
		return start, end
	}
	end.Valid = true
	return start, end
}

// dateLayouts are the layouts a date can be typed in.
var dateLayouts = []string{"2006-01-02", "20060102"}

// date parses the date in field k, named name in errors, reporting false
// when it is invalid, or empty and required.
func (b *builder) date(k Field, name string, required bool) (time.Time, bool) {
	s := strings.TrimSpace(b.f[k])
	if s == "" {
		if required {
			b.fail(k, "%s is required", name)
		}
		return time.Time{}, !required
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	b.fail(k, "%s must be YYYY-MM-DD", name)
	return time.Time{}, false
}

// clockLayouts are the layouts a time can be typed in.
var clockLayouts = []string{"15:04", "15:04:05", "1504", "150405"}

// clock parses the time of day in field k, named name in errors, as the
// time since midnight, reporting false when it is invalid, or empty and
// required.
func (b *builder) clock(k Field, name string, required bool) (time.Duration, bool) {
	s := strings.TrimSpace(b.f[k])
	if s == "" {
		if required {
			b.fail(k, "%s is required", name)
		}
		return 0, !required
	}
	for _, layout := range clockLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Sub(t.Truncate(24 * time.Hour)), true
		}
	}
	b.fail(k, "%s must be HH:MM or HH:MM:SS", name)
	return 0, false
}

// mode checks that mode is an ADIF mode and submode one of its submodes.
func (b *builder) mode(mode, submode string) {
	if mode != "" && !slices.Contains(adif.Modes(), mode) {
		b.fail(Mode, "%q is not an ADIF mode", mode)
		return
	}
	if submode != "" && !slices.Contains(adif.Submodes(mode), submode) {
		if mode == "" {
			b.fail(Submode, "submode %s needs a mode", submode)
			return
		}
		b.fail(Submode, "%s is not a submode of %s", submode, mode)
	}
}

// freqBand parses the freq in field fk and the band in field bk, named
// freqName and bandName in errors. The band comes from the freq unless one
// is typed, which must then match the freq.
func (b *builder) freqBand(fk, bk Field, freqName, bandName string) (sql.Null[int64], string) {
	var freq sql.Null[int64]
	band := strings.ToLower(strings.TrimSpace(b.f[bk]))
	if band != "" && !logbook.IsBand(band) {
		b.fail(bk, "%q is not a band", band)
		band = ""
	}
	if strings.TrimSpace(b.f[fk]) == "" {
		return freq, band
	}
	hz, err := parseFreq(freqName, b.f[fk])
	if err != nil {
		b.errs[fk] = err
		return freq, band
	}
	freq = sql.Null[int64]{V: hz, Valid: true}
	derived, ok := logbook.BandForFreq(hz)
	switch {
	case band == "":
		band = derived
	case !ok && b.errs[bk] == nil:
		b.fail(bk, "%s %s doesn't match %s MHz", bandName, band, logbook.FormatFreqMHz(hz))
	case derived != band && b.errs[bk] == nil:
		b.fail(bk, "%s %s doesn't match %s MHz (%s)", bandName, band, logbook.FormatFreqMHz(hz), derived)
	}
	return freq, band
}

// splitTags splits comma-separated tags, dropping empty ones.
func splitTags(s string) []string {
	var out []string
	for t := range strings.SplitSeq(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// freqUnits maps unit suffixes to multipliers. Longer suffixes come first so
// "mhz" is matched whole.
var freqUnits = []struct {
	suffix string
	mult   float64
}{
	{"mhz", 1e6},
	{"khz", 1e3},
}

// ParseFreq parses a freq typed into the form and returns it in Hz. An
// optional MHz or kHz suffix sets the unit; without one, values below 1000
// are MHz and larger ones kHz, so 14.074, 14074 and 14074.0 all mean
// 14.074 MHz.
func ParseFreq(s string) (int64, error) {
	return parseFreq("freq", s)
}

// parseFreq is ParseFreq with the field named name in errors.
func parseFreq(name, s string) (int64, error) {
	num := strings.ToLower(strings.TrimSpace(s))
	if num == "" {
		return 0, fmt.Errorf("%s is empty", name)
	}
	mult := 0.0
	for _, u := range freqUnits {
		if rest, ok := strings.CutSuffix(num, u.suffix); ok {
			num, mult = strings.TrimSpace(rest), u.mult
			break
		}
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid %s %q", name, strings.TrimSpace(s))
	}
	if mult == 0 {
		mult = 1e6
		if v >= 1000 {
			mult = 1e3
		}
	}
	hz := math.Round(v * mult)
	switch {
	case hz <= 0:
		return 0, fmt.Errorf("%s %q must be positive", name, strings.TrimSpace(s))
	case hz >= math.MaxInt64:
		return 0, fmt.Errorf("%s %q is too large", name, strings.TrimSpace(s))
	}
	return int64(hz), nil
}

// DefaultRST returns the usual signal report for a QSO in mode and submode,
// ignoring case: 59 for phone, 599 for CW and RTTY and -10 for FT8 and
// FT4. It returns "" for other modes, which have no usual report.
func DefaultRST(mode, submode string) string {
	mode, submode = strings.ToUpper(mode), strings.ToUpper(submode)
	switch {
	case mode == "FT8", mode == "MFSK" && submode == "FT4":
		return "-10"
	case slices.Contains([]string{"SSB", "AM", "FM", "DIGITALVOICE"}, mode):
		return "59"
	case mode == "CW", mode == "RTTY":
		return "599"
	}
	return ""
}

// Path returns the great-circle distance in km and the bearing in degrees
// from our grid, myGrid, to the contacted station's grid. An error wraps
// logbook.ErrInvalidGrid when either grid is missing or invalid.
func Path(myGrid, grid string) (km, bearing float64, err error) {
	km, err = logbook.GridDistanceKm(myGrid, grid)
	if err != nil {
		return 0, 0, fmt.Errorf("qsoentry: path: %w", err)
	}
	bearing, err = logbook.GridBearing(myGrid, grid)
	if err != nil {
		return 0, 0, fmt.Errorf("qsoentry: path: %w", err)
	}
	return km, bearing, nil
}
