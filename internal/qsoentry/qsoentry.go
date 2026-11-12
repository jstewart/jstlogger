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

	// The Station tab, our side of the QSO, which Station.Fill fills in
	// from a station profile.

	StationName
	StationCallsign
	Operator
	MyRig
	TxPwr
	MyGridsquare
	MyPOTARef
	MySOTARef
	MyIOTA
	MyWWFFRef
	MyState
	MyCity

	// The General tab, more about the contacted station.

	DXCC
	Cont
	Region
	CQZ
	ITUZ
	State
	Cnty
	IOTA
	SOTARef
	WWFFRef
	POTARef
	Sig
	SigInfo
	DarcDok
	Email
	PropMode
	AntPath

	// The Satellite tab.

	SatName
	SatMode

	// The Notes tab.

	Notes

	// The QSL tab.

	QSLSent
	QSLSentVia
	QSLVia
	QSLMsg
	LoTWSent
	EQSLSent
	ClubLogSent

	// FieldCount is the number of fields.
	FieldCount
)

// fieldNames are the ADIF names of the fields. Tags and the Club Log sent
// status are not ADIF fields, and the station name is jstlogger's own
// application-defined field.
var fieldNames = [FieldCount]string{
	"QSO_DATE", "TIME_ON", "QSO_DATE_OFF", "TIME_OFF", "CALL", "MODE",
	"SUBMODE", "FREQ", "BAND", "FREQ_RX", "BAND_RX", "RST_SENT", "RST_RCVD",
	"NAME", "QTH", "GRIDSQUARE", "COMMENT", "TAGS",
	// Station.
	logbook.StationField, "STATION_CALLSIGN", "OPERATOR", "MY_RIG", "TX_PWR",
	"MY_GRIDSQUARE", "MY_POTA_REF", "MY_SOTA_REF", "MY_IOTA", "MY_WWFF_REF",
	"MY_STATE", "MY_CITY",
	// General.
	"DXCC", "CONT", "REGION", "CQZ", "ITUZ", "STATE", "CNTY", "IOTA",
	"SOTA_REF", "WWFF_REF", "POTA_REF", "SIG", "SIG_INFO", "DARC_DOK",
	"EMAIL", "PROP_MODE", "ANT_PATH",
	// Satellite.
	"SAT_NAME", "SAT_MODE",
	// Notes.
	"NOTES",
	// QSL.
	"QSL_SENT", "QSL_SENT_VIA", "QSL_VIA", "QSLMSG", "LOTW_QSL_SENT",
	"EQSL_QSL_SENT", "CLUBLOG_QSL_SENT",
}

// String returns the field's upper-case ADIF name, or the name it goes by
// when it is not an ADIF field: TAGS, CLUBLOG_QSL_SENT or
// logbook.StationField.
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

// carried are the fields whose values stay in the form after a QSO is
// saved.
var carried = []Field{
	Date, Mode, Submode, Freq, Band, RSTSent, RSTRcvd,
	StationName, StationCallsign, Operator, MyRig, TxPwr, MyGridsquare,
	MyPOTARef, MySOTARef, MyIOTA, MyWWFFRef, MyState, MyCity,
	PropMode, AntPath, SatName, SatMode,
	QSLSent, QSLSentVia, QSLVia, QSLMsg, LoTWSent, EQSLSent, ClubLogSent,
}

// Carry returns the values that stay in the form after a QSO is saved, so
// that a batch of QSOs from a paper log is quick to enter: the date, band,
// mode, submode, freq and signal reports, the whole Station tab, the
// propagation mode, antenna path and satellite, and the whole QSL tab. What
// is particular to the contacted station, such as its zones, references
// and email, and the notes, does not carry.
func (f Form) Carry() Form {
	var c Form
	for _, k := range carried {
		c[k] = f[k]
	}
	return c
}

// Station is a station profile: our side of the QSO as configured, which
// Fill copies into the form's Station tab.
type Station struct {
	// Name is the configured station's name, stored on each QSO in
	// logbook.StationField since ADIF has no field for it.
	Name string
	// Callsign is our callsign (ADIF STATION_CALLSIGN).
	Callsign string
	// Operator is the operator's callsign (ADIF OPERATOR).
	Operator string
	// Grid is our Maidenhead locator (ADIF MY_GRIDSQUARE).
	Grid string
	// State is our state or province (ADIF MY_STATE).
	State string
	// City is our city (ADIF MY_CITY).
	City string
	// POTA are the parks we operate from (ADIF MY_POTA_REF).
	POTA []string
	// SOTA is the summit we operate from (ADIF MY_SOTA_REF).
	SOTA string
	// IOTA is the island we operate from (ADIF MY_IOTA).
	IOTA string
	// WWFF is the WWFF area we operate from (ADIF MY_WWFF_REF).
	WWFF string
}

// Fill writes the profile into f's Station tab, replacing what is there, so
// switching profiles leaves nothing of the old one behind. The rig and
// power are not part of a profile and are left as they are.
func (s Station) Fill(f *Form) {
	f[StationName] = s.Name
	f[StationCallsign] = s.Callsign
	f[Operator] = s.Operator
	f[MyGridsquare] = s.Grid
	f[MyPOTARef] = strings.Join(s.POTA, ",")
	f[MySOTARef] = s.SOTA
	f[MyIOTA] = s.IOTA
	f[MyWWFFRef] = s.WWFF
	f[MyState] = s.State
	f[MyCity] = s.City
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

// Build validates f and returns the QSO it describes. now is the current
// time; a QSO can't start after it. Our side of the QSO comes from the
// Station tab, which Station.Fill fills in. When any field is invalid, Build
// returns an Errors holding every field's error.
func Build(f Form, now time.Time) (logbook.QSO, error) {
	b := builder{f: f, errs: Errors{}, fields: map[string]string{}}
	q := logbook.QSO{
		Call:       b.callsign(Call, "call", true),
		Mode:       b.upper(Mode),
		Submode:    b.upper(Submode),
		RSTSent:    b.text(RSTSent),
		RSTRcvd:    b.text(RSTRcvd),
		Name:       b.text(Name),
		QTH:        b.text(QTH),
		Gridsquare: b.grid(Gridsquare),
		Comment:    b.text(Comment),
		Tags:       splitTags(f[Tags]),
	}
	q.StartUTC, q.EndUTC = b.times(now)
	b.mode(q.Mode, q.Submode)
	q.FreqHz, q.Band = b.freqBand(Freq, Band, "freq", "band")
	var bandRX string
	q.FreqRxHz, bandRX = b.freqBand(FreqRX, BandRX, "freq RX", "band RX")
	b.put(BandRX, bandRX)
	b.station(&q)
	b.general(&q)
	b.satellite()
	b.put(Notes, b.text(Notes))
	q.Confirmations = b.qsl()

	if len(b.errs) > 0 {
		return logbook.QSO{}, b.errs
	}
	if len(b.fields) > 0 {
		q.Fields = b.fields
	}
	return q, nil
}

// builder parses a Form's fields, collecting their errors and the fields
// with no column of their own.
type builder struct {
	f    Form
	errs Errors
	// fields holds what has no column of its own, for QSO.Fields.
	fields map[string]string
}

// fail records err as the error of field k.
func (b *builder) fail(k Field, format string, args ...any) {
	b.errs[k] = fmt.Errorf(format, args...)
}

// text returns field k, trimmed.
func (b *builder) text(k Field) string {
	return strings.TrimSpace(b.f[k])
}

// upper returns field k, trimmed and upper-cased.
func (b *builder) upper(k Field) string {
	return strings.ToUpper(b.text(k))
}

// put stores v in the QSO's fields under field k's name, unless it is
// empty.
func (b *builder) put(k Field, v string) {
	if v != "" {
		b.fields[k.String()] = v
	}
}

// station reads the Station tab into q.
func (b *builder) station(q *logbook.QSO) {
	b.put(StationName, b.text(StationName))
	q.StationCallsign = b.callsign(StationCallsign, "station callsign", false)
	q.Operator = b.callsign(Operator, "operator", false)
	q.MyGridsquare = b.grid(MyGridsquare)
	b.put(MyRig, b.text(MyRig))
	if s := b.text(TxPwr); s != "" {
		if w, ok := parsePower(s); ok {
			q.TxPwr = sql.Null[float64]{V: w, Valid: true}
		} else {
			b.fail(TxPwr, "%q is not a power in watts", s)
		}
	}
	b.pota(MyPOTARef)
	b.put(MySOTARef, b.ref(MySOTARef, isSOTA, "a SOTA reference"))
	b.put(MyIOTA, b.ref(MyIOTA, isIOTA, "an IOTA reference"))
	b.put(MyWWFFRef, b.ref(MyWWFFRef, isWWFF, "a WWFF reference"))
	b.put(MyState, b.upper(MyState))
	b.put(MyCity, b.text(MyCity))
}

// general reads the General tab into q.
func (b *builder) general(q *logbook.QSO) {
	q.DXCC = b.number(DXCC, parseDXCC, "a DXCC entity code")
	q.Cont = b.ref(Cont, isContinent, "a continent")
	b.put(Region, b.upper(Region))
	q.CQZ = b.number(CQZ, parseCQZone, "a CQ zone")
	q.ITUZ = b.number(ITUZ, parseITUZone, "an ITU zone")
	q.State = b.upper(State)
	q.Cnty = b.text(Cnty)
	b.put(IOTA, b.ref(IOTA, isIOTA, "an IOTA reference"))
	b.put(SOTARef, b.ref(SOTARef, isSOTA, "a SOTA reference"))
	b.put(WWFFRef, b.ref(WWFFRef, isWWFF, "a WWFF reference"))
	b.pota(POTARef)
	b.put(Sig, b.text(Sig))
	b.put(SigInfo, b.text(SigInfo))
	b.put(DarcDok, b.upper(DarcDok))
	if email := b.text(Email); email != "" {
		if !isEmail(email) {
			b.fail(Email, "%q is not an email address", email)
		}
		b.put(Email, email)
	}
	b.put(PropMode, b.enum(PropMode, adif.PropModes(), "an ADIF propagation mode"))
	b.put(AntPath, b.enum(AntPath, adif.AntPaths(), "an ADIF antenna path"))
}

// satellite reads the Satellite tab, which only means something when the
// QSO went via a satellite; otherwise it is dropped.
func (b *builder) satellite() {
	if b.upper(PropMode) != "SAT" {
		return
	}
	b.put(SatName, b.upper(SatName))
	b.put(SatMode, b.upper(SatMode))
}

// qslServices are the confirmation services of the QSL tab's sent statuses,
// in the order their confirmations are built.
var qslServices = []struct {
	field   Field
	service string
}{
	{QSLSent, logbook.ServiceCard},
	{LoTWSent, logbook.ServiceLoTW},
	{EQSLSent, logbook.ServiceEQSL},
	{ClubLogSent, logbook.ServiceClubLog},
}

// qsl reads the QSL tab, returning a confirmation for each service with a
// sent status, or nil when there is none. The statuses are confirmations,
// never fields.
func (b *builder) qsl() []logbook.Confirmation {
	var confs []logbook.Confirmation
	statuses := adif.QSLSentStatuses()
	for _, s := range qslServices {
		sent := b.upper(s.field)
		if sent == "" {
			continue
		}
		if !slices.Contains(statuses, sent) {
			b.fail(s.field, "%q is not a QSL sent status", sent)
			continue
		}
		confs = append(confs, logbook.Confirmation{Service: s.service, Sent: sent})
	}
	b.put(QSLSentVia, b.enum(QSLSentVia, adif.QSLSentVias(), "a QSL sent via"))
	b.put(QSLVia, b.text(QSLVia))
	b.put(QSLMsg, b.text(QSLMsg))
	return confs
}

// ref returns field k upper-cased, checking it with valid when it is not
// empty and failing with "is not what".
func (b *builder) ref(k Field, valid func(string) bool, what string) string {
	v := b.upper(k)
	if v != "" && !valid(v) {
		b.fail(k, "%q is not %s", v, what)
	}
	return v
}

// enum returns field k upper-cased, checking that it is one of values when
// it is not empty and failing with "is not what".
func (b *builder) enum(k Field, values []string, what string) string {
	return b.ref(k, func(v string) bool { return slices.Contains(values, v) }, what)
}

// pota normalizes the comma-separated POTA references in field k and stores
// them in the QSO's fields.
func (b *builder) pota(k Field) {
	refs, bad := normalizePOTA(b.f[k])
	if bad != "" {
		b.fail(k, "%q is not a POTA reference", bad)
		return
	}
	b.put(k, refs)
}

// number parses field k with parse when it is not empty, failing with "is
// not what".
func (b *builder) number(k Field, parse func(string) (int, bool), what string) sql.Null[int] {
	s := b.text(k)
	if s == "" {
		return sql.Null[int]{}
	}
	n, ok := parse(s)
	if !ok {
		b.fail(k, "%q is not %s", s, what)
		return sql.Null[int]{}
	}
	return sql.Null[int]{V: n, Valid: true}
}

// callsign returns the callsign in field k, named name in errors,
// upper-cased. It can't contain spaces, and is required if required is set.
func (b *builder) callsign(k Field, name string, required bool) string {
	call := b.upper(k)
	switch {
	case call == "" && required:
		b.fail(k, "%s is required", name)
	case strings.ContainsFunc(call, unicode.IsSpace):
		b.fail(k, "%s can't contain spaces", name)
	}
	return call
}

// grid returns the Maidenhead locator in field k, upper-cased.
func (b *builder) grid(k Field) string {
	grid := b.upper(k)
	if grid == "" {
		return ""
	}
	if _, _, err := logbook.GridCenter(grid); err != nil {
		b.fail(k, "%q is not a Maidenhead locator", grid)
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
			h, m, sec := t.Clock()
			return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, true
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
