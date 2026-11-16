package qsoentry

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"jstlogger/internal/logbook"
)

// fieldsBacked are the fields Build stores in QSO.Fields under their own
// names, which Load reads back from there. The satellite is one of them
// only when the QSO went via a satellite; see satFields.
var fieldsBacked = []Field{
	BandRX,
	StationName, MyRig, MyPOTARef, MySOTARef, MyIOTA, MyWWFFRef, MyState, MyCity,
	Region, IOTA, SOTARef, WWFFRef, POTARef, Sig, SigInfo, DarcDok, Email,
	PropMode, AntPath,
	Notes,
	QSLSentVia, QSLVia, QSLMsg,
}

// satFields are the Satellite tab's fields, which Build stores in
// QSO.Fields only when PROP_MODE is SAT.
var satFields = []Field{SatName, SatMode}

// Load returns the form that shows q, for editing it: the reverse of Build.
// Dates are YYYY-MM-DD and times HH:MM:SS, in UTC; the date and time off are
// filled in only when q has an end. Freqs are in MHz, with an MHz suffix
// from 1000 MHz up so that ParseFreq does not read them as kHz. Tags are
// joined with ", ". Each QSL sent status is the Sent of the confirmation for
// its service. What q lacks is left empty.
func Load(q logbook.QSO) Form {
	var f Form
	if !q.StartUTC.IsZero() {
		start := q.StartUTC.UTC()
		f[Date], f[TimeOn] = start.Format(time.DateOnly), start.Format(time.TimeOnly)
	}
	if q.EndUTC.Valid {
		end := q.EndUTC.V.UTC()
		f[DateOff], f[TimeOff] = end.Format(time.DateOnly), end.Format(time.TimeOnly)
	}
	f[Call] = q.Call
	f[Mode], f[Submode] = q.Mode, q.Submode
	if q.FreqHz.Valid {
		f[Freq] = formatFreq(q.FreqHz.V)
	}
	f[Band] = q.Band
	if q.FreqRxHz.Valid {
		f[FreqRX] = formatFreq(q.FreqRxHz.V)
	}
	f[RSTSent], f[RSTRcvd] = q.RSTSent, q.RSTRcvd
	f[Name], f[QTH], f[Gridsquare], f[Comment] = q.Name, q.QTH, q.Gridsquare, q.Comment
	f[Tags] = strings.Join(q.Tags, ", ")

	f[StationCallsign], f[Operator], f[MyGridsquare] = q.StationCallsign, q.Operator, q.MyGridsquare
	if q.TxPwr.Valid {
		f[TxPwr] = strconv.FormatFloat(q.TxPwr.V, 'f', -1, 64)
	}

	f[DXCC] = formatInt(q.DXCC.V, q.DXCC.Valid)
	f[Cont] = q.Cont
	f[CQZ] = formatInt(q.CQZ.V, q.CQZ.Valid)
	f[ITUZ] = formatInt(q.ITUZ.V, q.ITUZ.Valid)
	f[State], f[Cnty] = q.State, q.Cnty

	for _, k := range slices.Concat(fieldsBacked, satFields) {
		f[k] = q.Fields[k.String()]
	}
	for _, s := range qslServices {
		if i := confIndex(q.Confirmations, s.service); i >= 0 {
			f[s.field] = q.Confirmations[i].Sent
		}
	}
	return f
}

// formatFreq returns hz in MHz as ParseFreq reads it back: a bare number
// below 1000 MHz and one with an MHz suffix from there up, which ParseFreq
// would otherwise read as kHz.
func formatFreq(hz int64) string {
	s := logbook.FormatFreqMHz(hz)
	if hz >= 1_000_000_000 {
		s += " MHz"
	}
	return s
}

// formatInt returns n in decimal when valid is set, and "" otherwise.
func formatInt(n int, valid bool) string {
	if !valid {
		return ""
	}
	return strconv.Itoa(n)
}

// Edit validates f, the form showing orig after the operator's changes, and
// returns orig with those changes applied. now is the current time, as for
// Build. When any field is invalid, Edit returns Build's Errors as they are.
//
// Everything the form does not show is kept from orig: its ID, UUID,
// country, ITU region, distance and timestamps, its fields other than the
// form's, and its confirmations from other services. The Satellite tab is
// the form's only when PROP_MODE is SAT, as Build drops it otherwise, so a
// stored satellite is kept when the propagation mode is anything else. Each
// QSL sent status sets the Sent of its service's confirmation, keeping the
// received status and the dates; a confirmation left with nothing in it is
// dropped. Edit never changes orig, and the QSO it returns shares no maps
// or slices with it.
func Edit(f Form, orig logbook.QSO, now time.Time) (logbook.QSO, error) {
	q, err := Build(f, now)
	if err != nil {
		return logbook.QSO{}, err
	}
	q.ID, q.UUID = orig.ID, orig.UUID
	q.Country = orig.Country
	q.ITURegion = orig.ITURegion
	q.DistanceKm = orig.DistanceKm
	q.CreatedAt, q.UpdatedAt = orig.CreatedAt, orig.UpdatedAt
	q.Fields = mergeFields(orig.Fields, q.Fields)
	q.Confirmations = mergeConfirmations(orig.Confirmations, q.Confirmations)
	return q, nil
}

// mergeFields returns orig's fields with the form's replaced by built's,
// Build's fields for the edited form, or nil when none are left. The
// satellite is the form's only when built goes via a satellite.
func mergeFields(orig, built map[string]string) map[string]string {
	out := maps.Clone(orig)
	if out == nil {
		out = map[string]string{}
	}
	owned := fieldsBacked
	if built[PropMode.String()] == "SAT" {
		owned = slices.Concat(fieldsBacked, satFields)
	}
	for _, k := range owned {
		delete(out, k.String())
	}
	maps.Copy(out, built)
	if len(out) == 0 {
		return nil
	}
	return out
}

// mergeConfirmations returns orig's confirmations with the Sent of each
// service the form shows taken from built, Build's confirmations for the
// edited form. orig's order is kept and services new to it follow in
// built's order. A confirmation of the form's left with nothing in it is
// dropped; other services' confirmations are kept as they are. It returns
// nil when none are left.
func mergeConfirmations(orig, built []logbook.Confirmation) []logbook.Confirmation {
	var out []logbook.Confirmation
	for _, c := range orig {
		if isFormService(c.Service) {
			c.Sent = ""
			if i := confIndex(built, c.Service); i >= 0 {
				c.Sent = built[i].Sent
			}
			if c == (logbook.Confirmation{Service: c.Service}) {
				continue
			}
		}
		out = append(out, c)
	}
	for _, c := range built {
		if confIndex(orig, c.Service) < 0 {
			out = append(out, c)
		}
	}
	return out
}

// isFormService reports whether the QSL tab has a sent status for service.
func isFormService(service string) bool {
	for _, s := range qslServices {
		if s.service == service {
			return true
		}
	}
	return false
}

// confIndex returns the index of the confirmation for service in confs, or
// -1 when there is none.
func confIndex(confs []logbook.Confirmation, service string) int {
	return slices.IndexFunc(confs, func(c logbook.Confirmation) bool { return c.Service == service })
}
