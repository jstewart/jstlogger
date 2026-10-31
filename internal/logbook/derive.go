package logbook

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

var (
	// ErrMissingStart is returned (wrapped) when a QSO has no StartUTC.
	ErrMissingStart = errors.New("logbook: qso start time is required")
	// ErrPromotedField is returned (wrapped) when QSO.Fields holds an ADIF
	// field that has its own QSO field or confirmation.
	ErrPromotedField = errors.New("logbook: promoted ADIF field in Fields")
	// ErrInvalidField is returned (wrapped) when QSO.Fields holds an empty
	// field name, or two names that are equal after normalization.
	ErrInvalidField = errors.New("logbook: invalid ADIF field name")
	// ErrInvalidConfirmation is returned (wrapped) when QSO.Confirmations
	// holds an empty service or the same service more than once.
	ErrInvalidConfirmation = errors.New("logbook: invalid confirmation")
)

// promotedFields lists the ADIF field names stored in QSO fields or
// Confirmations rather than in QSO.Fields. A field lives in exactly one place.
var promotedFields = []string{
	"QSO_DATE", "TIME_ON", "QSO_DATE_OFF", "TIME_OFF",
	"CALL", "STATION_CALLSIGN", "OPERATOR",
	"FREQ", "FREQ_RX", "BAND", "MODE", "SUBMODE",
	"RST_SENT", "RST_RCVD", "TX_PWR",
	"NAME", "QTH", "GRIDSQUARE", "MY_GRIDSQUARE",
	"DXCC", "COUNTRY", "CONT", "CQZ", "ITUZ", "DISTANCE",
	"STATE", "CNTY", "COMMENT",
	// Confirmations.
	"QSL_SENT", "QSL_RCVD", "QSLSDATE", "QSLRDATE",
	"LOTW_QSL_SENT", "LOTW_QSL_RCVD", "LOTW_QSLSDATE", "LOTW_QSLRDATE",
	"EQSL_QSL_SENT", "EQSL_QSL_RCVD", "EQSL_QSLSDATE", "EQSL_QSLRDATE",
}

// prepare normalizes q and derives values before it is written, returning a
// copy; the caller's maps and slices are not modified. stationGrid is the
// configured station locator, used for the distance when q.MyGridsquare is
// empty. It is never stored in the QSO.
func prepare(q QSO, stationGrid string) (QSO, error) {
	if q.StartUTC.IsZero() {
		return QSO{}, fmt.Errorf("logbook: prepare qso: %w", ErrMissingStart)
	}

	q.StartUTC = q.StartUTC.UTC()
	q.EndUTC = utcNull(q.EndUTC)

	q.Call = upper(q.Call)
	q.StationCallsign = upper(q.StationCallsign)
	q.Gridsquare = upper(q.Gridsquare)
	q.MyGridsquare = upper(q.MyGridsquare)
	q.Cont = upper(q.Cont)
	q.Mode = upper(q.Mode)
	q.Submode = upper(q.Submode)

	if strings.TrimSpace(q.Band) == "" && q.FreqHz.Valid {
		q.Band, _ = BandForFreq(q.FreqHz.V)
	}
	q.Band = strings.ToLower(strings.TrimSpace(q.Band))

	// ITU region is not an ADIF field, so it is always derived; a stale value
	// from an earlier read is never kept.
	q.ITURegion = deriveITURegion(q)

	if km, ok := gridDistance(q.Gridsquare, q.MyGridsquare, stationGrid); ok {
		q.DistanceKm = sql.Null[float64]{V: km, Valid: true}
	}

	fields, err := normalizeFields(q.Fields)
	if err != nil {
		return QSO{}, fmt.Errorf("logbook: prepare qso: %w", err)
	}
	q.Fields = fields

	if q.Confirmations != nil {
		confs := make([]Confirmation, len(q.Confirmations))
		for i, c := range q.Confirmations {
			c.Service = normalizeService(c.Service)
			if c.Service == "" {
				return QSO{}, fmt.Errorf("logbook: prepare qso: empty service: %w", ErrInvalidConfirmation)
			}
			if slices.ContainsFunc(confs[:i], func(o Confirmation) bool { return o.Service == c.Service }) {
				return QSO{}, fmt.Errorf("logbook: prepare qso: service %q given more than once: %w", c.Service, ErrInvalidConfirmation)
			}
			c.SentDate = utcNull(c.SentDate)
			c.RcvdDate = utcNull(c.RcvdDate)
			confs[i] = c
		}
		q.Confirmations = confs
	}

	q.Tags = normalizeTags(q.Tags)
	return q, nil
}

// gridDistance returns the distance between grid and myGrid, using
// stationGrid when myGrid is empty, and false when either grid is missing or
// invalid.
func gridDistance(grid, myGrid, stationGrid string) (float64, bool) {
	if upper(myGrid) == "" {
		myGrid = stationGrid
	}
	km, err := GridDistanceKm(upper(grid), upper(myGrid))
	return km, err == nil
}

// deriveITURegion returns the ITU region from q's ITU zone if that zone lies
// in one region, else from the centre of q's grid, else NULL.
func deriveITURegion(q QSO) sql.Null[int] {
	if q.ITUZ.Valid {
		if r, ok := ITURegionForZone(q.ITUZ.V); ok {
			return sql.Null[int]{V: r, Valid: true}
		}
	}
	if lat, lon, err := GridCenter(q.Gridsquare); err == nil {
		return sql.Null[int]{V: ITURegionForLocation(lat, lon), Valid: true}
	}
	return sql.Null[int]{}
}

// normalizeFields returns a copy of fields with trimmed, upper-case names and
// trimmed values. It rejects empty names, names that collide after
// normalization and promoted ADIF fields.
func normalizeFields(fields map[string]string) (map[string]string, error) {
	if fields == nil {
		return nil, nil
	}
	out := make(map[string]string, len(fields))
	for k, v := range fields {
		name := upper(k)
		switch {
		case name == "":
			return nil, fmt.Errorf("field %q: %w", k, ErrInvalidField)
		case slices.Contains(promotedFields, name):
			return nil, fmt.Errorf("field %q: %w", name, ErrPromotedField)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("field %q given more than once: %w", name, ErrInvalidField)
		}
		out[name] = strings.TrimSpace(v)
	}
	return out, nil
}

// normalizeTags returns trimmed, non-empty tags with case-insensitive
// duplicates removed, keeping the first spelling of each. It returns nil when
// no tags remain.
func normalizeTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if slices.ContainsFunc(out, func(s string) bool { return strings.EqualFold(s, t) }) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// normalizeService trims a confirmation service name and lower-cases it, to
// match the Service constants.
func normalizeService(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// upper trims s and upper-cases it.
func upper(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// utcNull converts a valid time to UTC.
func utcNull(t sql.Null[time.Time]) sql.Null[time.Time] {
	if t.Valid {
		t.V = t.V.UTC()
	}
	return t
}
