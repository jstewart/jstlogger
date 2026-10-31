package logbook

import (
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"jstlogger/internal/adif"
)

// ErrInvalidRecord is returned (wrapped) by QSOFromRecord when a required
// ADIF field is missing or a field's value cannot be parsed. The message
// names the field, and the value when there is one.
var ErrInvalidRecord = errors.New("logbook: invalid ADIF record")

// ADIF date and time layouts.
const (
	adifDate      = "20060102"
	adifTime      = "150405"
	adifShortTime = "1504"
)

// uuidField is jstlogger's application-defined ADIF field carrying QSO.UUID.
const uuidField = "APP_JSTLOGGER_UUID"

// confSpec names the ADIF fields that hold one confirmation service.
type confSpec struct {
	service, sent, rcvd, sentDate, rcvdDate string
}

// confSpecs lists the confirmation services that map to ADIF, in the order
// they are exported. Other services are not exported.
var confSpecs = []confSpec{
	{ServiceCard, "QSL_SENT", "QSL_RCVD", "QSLSDATE", "QSLRDATE"},
	{ServiceLoTW, "LOTW_QSL_SENT", "LOTW_QSL_RCVD", "LOTW_QSLSDATE", "LOTW_QSLRDATE"},
	{ServiceEQSL, "EQSL_QSL_SENT", "EQSL_QSL_RCVD", "EQSL_QSLSDATE", "EQSL_QSLRDATE"},
}

// textFields returns the promoted ADIF fields that map one to one onto
// string fields of q, keyed by ADIF name.
func textFields(q *QSO) map[string]*string {
	return map[string]*string{
		"CALL":             &q.Call,
		"STATION_CALLSIGN": &q.StationCallsign,
		"OPERATOR":         &q.Operator,
		"BAND":             &q.Band,
		"MODE":             &q.Mode,
		"SUBMODE":          &q.Submode,
		"RST_SENT":         &q.RSTSent,
		"RST_RCVD":         &q.RSTRcvd,
		"NAME":             &q.Name,
		"QTH":              &q.QTH,
		"GRIDSQUARE":       &q.Gridsquare,
		"MY_GRIDSQUARE":    &q.MyGridsquare,
		"COUNTRY":          &q.Country,
		"CONT":             &q.Cont,
		"STATE":            &q.State,
		"CNTY":             &q.Cnty,
		"COMMENT":          &q.Comment,
	}
}

// QSOFromRecord converts an ADIF record to a QSO.
//
// Values are trimmed, and fields whose value is empty are treated as
// absent. CALL, QSO_DATE and TIME_ON are required. QSO_DATE_OFF and TIME_OFF
// give EndUTC; a TIME_OFF without QSO_DATE_OFF falls on the start date, or
// the next day when it is earlier than TIME_ON, and a QSO_DATE_OFF without
// TIME_OFF is ignored. FREQ and FREQ_RX are read exactly as decimal MHz and
// rounded to the nearest Hz. The QSL_*, LOTW_* and EQSL_* fields become
// Confirmations, APP_JSTLOGGER_UUID becomes UUID, and every other field goes
// into Fields. Errors wrap ErrInvalidRecord.
//
// The QSO is not normalized; the store does that when it is written.
func QSOFromRecord(r adif.Record) (QSO, error) {
	var q QSO
	text := textFields(&q)
	times := make(map[string]string, 4) // QSO_DATE, TIME_ON, QSO_DATE_OFF, TIME_OFF
	confs := make(map[string]string)

	for _, f := range r {
		name := strings.ToUpper(strings.TrimSpace(f.Name))
		v := strings.TrimSpace(f.Value)
		if v == "" {
			continue
		}
		if p, ok := text[name]; ok {
			*p = v
			continue
		}
		if isConfField(name) {
			confs[name] = v
			continue
		}

		var err error
		switch name {
		case "QSO_DATE", "TIME_ON", "QSO_DATE_OFF", "TIME_OFF":
			times[name] = v
		case "FREQ":
			q.FreqHz, err = nullFreq(v)
		case "FREQ_RX":
			q.FreqRxHz, err = nullFreq(v)
		case "TX_PWR":
			q.TxPwr, err = nullFloat(v)
		case "DISTANCE":
			q.DistanceKm, err = nullFloat(v)
		case "DXCC":
			q.DXCC, err = nullInt(v)
		case "CQZ":
			q.CQZ, err = nullInt(v)
		case "ITUZ":
			q.ITUZ, err = nullInt(v)
		case uuidField:
			q.UUID = v
		default:
			if q.Fields == nil {
				q.Fields = make(map[string]string)
			}
			q.Fields[name] = v
		}
		if err != nil {
			return QSO{}, invalidValue(name, v)
		}
	}

	if q.Call == "" {
		return QSO{}, missingField("CALL")
	}
	if err := setTimes(&q, times); err != nil {
		return QSO{}, err
	}
	var err error
	if q.Confirmations, err = confirmationsFrom(confs); err != nil {
		return QSO{}, err
	}
	return q, nil
}

// setTimes sets q.StartUTC and q.EndUTC from the QSO_DATE, TIME_ON,
// QSO_DATE_OFF and TIME_OFF values in times.
func setTimes(q *QSO, times map[string]string) error {
	for _, name := range []string{"QSO_DATE", "TIME_ON"} {
		if times[name] == "" {
			return missingField(name)
		}
	}
	date, err := parseDate("QSO_DATE", times["QSO_DATE"])
	if err != nil {
		return err
	}
	on, err := parseClock("TIME_ON", times["TIME_ON"])
	if err != nil {
		return err
	}
	q.StartUTC = date.Add(on)

	offText, ok := times["TIME_OFF"]
	if !ok {
		return nil
	}
	off, err := parseClock("TIME_OFF", offText)
	if err != nil {
		return err
	}
	endDate := date
	if dateOff, ok := times["QSO_DATE_OFF"]; ok {
		if endDate, err = parseDate("QSO_DATE_OFF", dateOff); err != nil {
			return err
		}
	} else if off < on {
		endDate = date.AddDate(0, 0, 1)
	}
	q.EndUTC = sql.Null[time.Time]{V: endDate.Add(off), Valid: true}
	return nil
}

// isConfField reports whether name is one of the confirmation fields.
func isConfField(name string) bool {
	return slices.ContainsFunc(confSpecs, func(s confSpec) bool {
		return name == s.sent || name == s.rcvd || name == s.sentDate || name == s.rcvdDate
	})
}

// confirmationsFrom builds a Confirmation for each service in confSpecs with
// at least one value in vals, which is keyed by ADIF field name.
func confirmationsFrom(vals map[string]string) ([]Confirmation, error) {
	var out []Confirmation
	for _, s := range confSpecs {
		sentDate, sdOK := vals[s.sentDate]
		rcvdDate, rdOK := vals[s.rcvdDate]
		c := Confirmation{Service: s.service, Sent: vals[s.sent], Rcvd: vals[s.rcvd]}
		if c.Sent == "" && c.Rcvd == "" && !sdOK && !rdOK {
			continue
		}
		var err error
		if c.SentDate, err = nullDate(s.sentDate, sentDate, sdOK); err != nil {
			return nil, err
		}
		if c.RcvdDate, err = nullDate(s.rcvdDate, rcvdDate, rdOK); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// RecordFromQSO converts q to an ADIF record.
//
// Promoted fields come first in a fixed order, then the card, LoTW and eQSL
// confirmations, then Fields sorted by name, then APP_JSTLOGGER_UUID. Times
// are converted to UTC and written as HHMMSS; QSO_DATE_OFF is written
// whenever there is an end time. Empty values are never written. ID,
// ITURegion, Tags, CreatedAt, UpdatedAt and confirmation services other than
// those three are not exported, nor are Fields entries that name a promoted
// field.
func RecordFromQSO(q QSO) adif.Record {
	var r adif.Record
	add := func(name, v string) {
		if v != "" {
			r = append(r, adif.Field{Name: name, Value: v})
		}
	}

	add("CALL", q.Call)
	if !q.StartUTC.IsZero() {
		start := q.StartUTC.UTC()
		add("QSO_DATE", start.Format(adifDate))
		add("TIME_ON", start.Format(adifTime))
	}
	if q.EndUTC.Valid {
		end := q.EndUTC.V.UTC()
		add("QSO_DATE_OFF", end.Format(adifDate))
		add("TIME_OFF", end.Format(adifTime))
	}
	add("BAND", q.Band)
	add("FREQ", formatNull(q.FreqHz, formatFreqMHz))
	add("FREQ_RX", formatNull(q.FreqRxHz, formatFreqMHz))
	add("MODE", q.Mode)
	add("SUBMODE", q.Submode)
	add("RST_SENT", q.RSTSent)
	add("RST_RCVD", q.RSTRcvd)
	add("TX_PWR", formatNull(q.TxPwr, formatFloat))
	add("STATION_CALLSIGN", q.StationCallsign)
	add("OPERATOR", q.Operator)
	add("NAME", q.Name)
	add("QTH", q.QTH)
	add("GRIDSQUARE", q.Gridsquare)
	add("MY_GRIDSQUARE", q.MyGridsquare)
	add("DXCC", formatNull(q.DXCC, strconv.Itoa))
	add("COUNTRY", q.Country)
	add("CONT", q.Cont)
	add("CQZ", formatNull(q.CQZ, strconv.Itoa))
	add("ITUZ", formatNull(q.ITUZ, strconv.Itoa))
	add("STATE", q.State)
	add("CNTY", q.Cnty)
	add("DISTANCE", formatNull(q.DistanceKm, formatFloat))
	add("COMMENT", q.Comment)

	for _, s := range confSpecs {
		i := slices.IndexFunc(q.Confirmations, func(c Confirmation) bool { return c.Service == s.service })
		if i < 0 {
			continue
		}
		c := q.Confirmations[i]
		add(s.sent, c.Sent)
		add(s.rcvd, c.Rcvd)
		add(s.sentDate, formatNull(c.SentDate, formatDate))
		add(s.rcvdDate, formatNull(c.RcvdDate, formatDate))
	}

	for _, name := range slices.Sorted(maps.Keys(q.Fields)) {
		if slices.Contains(promotedFields, strings.ToUpper(name)) {
			continue
		}
		add(strings.ToUpper(name), q.Fields[name])
	}

	add(uuidField, q.UUID)
	return r
}

// missingField returns an error for a required field that is absent.
func missingField(name string) error {
	return fmt.Errorf("%w: field %s is required", ErrInvalidRecord, name)
}

// invalidValue returns an error for a field whose value cannot be parsed.
func invalidValue(name, v string) error {
	return fmt.Errorf("%w: field %s: invalid value %q", ErrInvalidRecord, name, v)
}

// parseDate parses an ADIF date (YYYYMMDD) as midnight UTC.
func parseDate(name, v string) (time.Time, error) {
	if len(v) != len(adifDate) || !allDigits(v) {
		return time.Time{}, invalidValue(name, v)
	}
	t, err := time.Parse(adifDate, v)
	if err != nil {
		return time.Time{}, invalidValue(name, v)
	}
	return t, nil
}

// nullDate parses an optional ADIF date; ok reports whether it is present.
func nullDate(name, v string, ok bool) (sql.Null[time.Time], error) {
	if !ok {
		return sql.Null[time.Time]{}, nil
	}
	t, err := parseDate(name, v)
	if err != nil {
		return sql.Null[time.Time]{}, err
	}
	return sql.Null[time.Time]{V: t, Valid: true}, nil
}

// parseClock parses an ADIF time (HHMM or HHMMSS) as the time since
// midnight.
func parseClock(name, v string) (time.Duration, error) {
	layout := adifTime
	if len(v) == len(adifShortTime) {
		layout = adifShortTime
	}
	if len(v) != len(layout) || !allDigits(v) {
		return 0, invalidValue(name, v)
	}
	t, err := time.Parse(layout, v)
	if err != nil {
		return 0, invalidValue(name, v)
	}
	h, m, s := t.Clock()
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second, nil
}

// maxWholeMHz is the largest whole number of MHz whose value in Hz, plus a
// rounded fraction, still fits in an int64.
const maxWholeMHz = math.MaxInt64/1_000_000 - 1

// parseFreqHz converts a decimal MHz value to Hz without going through
// floating point. Digits beyond the sixth decimal place round half up to
// the nearest Hz. Signs, exponents and anything but digits and one '.' are
// rejected.
func parseFreqHz(v string) (int64, error) {
	v = strings.TrimSpace(v)
	whole, frac, _ := strings.Cut(v, ".")
	if whole == "" && frac == "" || !allDigits(whole) || !allDigits(frac) {
		return 0, fmt.Errorf("invalid frequency %q", v)
	}
	var mhz int64
	if whole != "" {
		var err error
		if mhz, err = strconv.ParseInt(whole, 10, 64); err != nil || mhz > maxWholeMHz {
			return 0, fmt.Errorf("invalid frequency %q: out of range", v)
		}
	}
	roundUp := len(frac) > 6 && frac[6] >= '5'
	frac = (frac + "000000")[:6]
	hz, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid frequency %q: %w", v, err)
	}
	hz += mhz * 1_000_000
	if roundUp {
		hz++
	}
	return hz, nil
}

// formatFreqMHz formats hz as decimal MHz with trailing zeros trimmed, e.g.
// 14075219 as "14.075219" and 7000000 as "7".
func formatFreqMHz(hz int64) string {
	whole := strconv.FormatInt(hz/1_000_000, 10)
	frac := strings.TrimRight(fmt.Sprintf("%06d", hz%1_000_000), "0")
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

// allDigits reports whether s holds only ASCII digits; "" is all digits.
func allDigits(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
}

// nullFreq parses an ADIF frequency in MHz as Hz.
func nullFreq(v string) (sql.Null[int64], error) {
	hz, err := parseFreqHz(v)
	if err != nil {
		return sql.Null[int64]{}, err
	}
	return sql.Null[int64]{V: hz, Valid: true}, nil
}

// nullFloat parses a finite decimal number.
func nullFloat(v string) (sql.Null[float64], error) {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return sql.Null[float64]{}, fmt.Errorf("parse number: %w", err)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return sql.Null[float64]{}, fmt.Errorf("parse number %q: not finite", v)
	}
	return sql.Null[float64]{V: f, Valid: true}, nil
}

// nullInt parses a decimal integer.
func nullInt(v string) (sql.Null[int], error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return sql.Null[int]{}, fmt.Errorf("parse integer: %w", err)
	}
	return sql.Null[int]{V: n, Valid: true}, nil
}

// formatNull formats a valid value with format, and returns "" for NULL.
func formatNull[T any](n sql.Null[T], format func(T) string) string {
	if !n.Valid {
		return ""
	}
	return format(n.V)
}

// formatFloat formats f in the shortest decimal form, e.g. "10" or "7.5".
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatDate formats t's UTC date as an ADIF date.
func formatDate(t time.Time) string {
	return t.UTC().Format(adifDate)
}
