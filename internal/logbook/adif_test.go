package logbook

import (
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/adif"
)

// rec builds an adif.Record from name/value pairs.
func rec(t *testing.T, pairs ...string) adif.Record {
	t.Helper()
	require.Zero(t, len(pairs)%2, "rec needs name/value pairs")
	r := make(adif.Record, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		r = append(r, adif.Field{Name: pairs[i], Value: pairs[i+1]})
	}
	return r
}

// minimal returns the required fields followed by extra pairs.
func minimal(t *testing.T, extra ...string) adif.Record {
	t.Helper()
	return rec(t, append([]string{"CALL", "W1AW", "QSO_DATE", "20261028", "TIME_ON", "144217"}, extra...)...)
}

func TestQSOFromRecordRequiredFields(t *testing.T) {
	q, err := QSOFromRecord(minimal(t))
	require.NoError(t, err)
	assert.Equal(t, "W1AW", q.Call)
	assert.Equal(t, time.Date(2026, 10, 28, 14, 42, 17, 0, time.UTC), q.StartUTC)
	assert.False(t, q.EndUTC.Valid)
	assert.Nil(t, q.Fields)
	assert.Nil(t, q.Confirmations)
}

func TestQSOFromRecordFourDigitTimeOn(t *testing.T) {
	q, err := QSOFromRecord(rec(t, "CALL", "W1AW", "QSO_DATE", "20261028", "TIME_ON", "1442"))
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 10, 28, 14, 42, 0, 0, time.UTC), q.StartUTC)
}

func TestQSOFromRecordErrors(t *testing.T) {
	tests := []struct {
		name    string
		r       adif.Record
		wantMsg []string
	}{
		{"missing call", rec(t, "QSO_DATE", "20261028", "TIME_ON", "1442"), []string{"CALL"}},
		{"empty call", rec(t, "CALL", " ", "QSO_DATE", "20261028", "TIME_ON", "1442"), []string{"CALL"}},
		{"missing date", rec(t, "CALL", "W1AW", "TIME_ON", "1442"), []string{"QSO_DATE"}},
		{"missing time", rec(t, "CALL", "W1AW", "QSO_DATE", "20261028"), []string{"TIME_ON"}},
		{"bad date", rec(t, "CALL", "W1AW", "QSO_DATE", "20261341", "TIME_ON", "1442"), []string{"QSO_DATE", `"20261341"`}},
		{"short date", rec(t, "CALL", "W1AW", "QSO_DATE", "2026102", "TIME_ON", "1442"), []string{"QSO_DATE", `"2026102"`}},
		{"bad time", rec(t, "CALL", "W1AW", "QSO_DATE", "20261028", "TIME_ON", "2460"), []string{"TIME_ON", `"2460"`}},
		{"five digit time", rec(t, "CALL", "W1AW", "QSO_DATE", "20261028", "TIME_ON", "14421"), []string{"TIME_ON", `"14421"`}},
		{"signed time", rec(t, "CALL", "W1AW", "QSO_DATE", "20261028", "TIME_ON", "+14421"), []string{"TIME_ON"}},
		{"bad time off", minimal(t, "TIME_OFF", "99"), []string{"TIME_OFF", `"99"`}},
		{"bad date off", minimal(t, "QSO_DATE_OFF", "x", "TIME_OFF", "1500"), []string{"QSO_DATE_OFF", `"x"`}},
		{"bad freq", minimal(t, "FREQ", "abc"), []string{"FREQ", `"abc"`}},
		{"bad freq rx", minimal(t, "FREQ_RX", "1.2.3"), []string{"FREQ_RX", `"1.2.3"`}},
		{"bad tx pwr", minimal(t, "TX_PWR", "lots"), []string{"TX_PWR", `"lots"`}},
		{"bad distance", minimal(t, "DISTANCE", "far"), []string{"DISTANCE", `"far"`}},
		{"bad dxcc", minimal(t, "DXCC", "2.5"), []string{"DXCC", `"2.5"`}},
		{"bad cqz", minimal(t, "CQZ", "five"), []string{"CQZ"}},
		{"bad ituz", minimal(t, "ITUZ", "x"), []string{"ITUZ"}},
		{"bad qsl date", minimal(t, "QSLSDATE", "2026"), []string{"QSLSDATE", `"2026"`}},
		{"bad lotw date", minimal(t, "LOTW_QSLRDATE", "20260230"), []string{"LOTW_QSLRDATE"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := QSOFromRecord(tt.r)
			require.ErrorIs(t, err, ErrInvalidRecord)
			for _, want := range tt.wantMsg {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestQSOFromRecordErrorKeepsCause(t *testing.T) {
	_, err := QSOFromRecord(minimal(t, "CQZ", "99999999999999999999"))
	require.ErrorIs(t, err, ErrInvalidRecord)
	assert.ErrorIs(t, err, strconv.ErrRange, "the parse error is wrapped, not dropped")
}

func TestQSOFromRecordEndTime(t *testing.T) {
	tests := []struct {
		name  string
		extra []string
		want  sql.Null[time.Time]
	}{
		{"none", nil, sql.Null[time.Time]{}},
		{"date and time", []string{"QSO_DATE_OFF", "20261029", "TIME_OFF", "0105"},
			sql.Null[time.Time]{V: time.Date(2026, 10, 29, 1, 5, 0, 0, time.UTC), Valid: true}},
		{"time off later same day", []string{"TIME_OFF", "150000"},
			sql.Null[time.Time]{V: time.Date(2026, 10, 28, 15, 0, 0, 0, time.UTC), Valid: true}},
		{"time off equal to time on", []string{"TIME_OFF", "144217"},
			sql.Null[time.Time]{V: time.Date(2026, 10, 28, 14, 42, 17, 0, time.UTC), Valid: true}},
		{"time off past midnight", []string{"TIME_OFF", "0001"},
			sql.Null[time.Time]{V: time.Date(2026, 10, 29, 0, 1, 0, 0, time.UTC), Valid: true}},
		{"date off without time off is ignored", []string{"QSO_DATE_OFF", "20261029"}, sql.Null[time.Time]{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := QSOFromRecord(minimal(t, tt.extra...))
			require.NoError(t, err)
			assert.Equal(t, tt.want, q.EndUTC)
		})
	}
}

func TestQSOFromRecordMidnightRolloverAtMonthEnd(t *testing.T) {
	q, err := QSOFromRecord(rec(t, "CALL", "W1AW", "QSO_DATE", "20261231", "TIME_ON", "235930", "TIME_OFF", "000415"))
	require.NoError(t, err)
	assert.Equal(t, time.Date(2027, 1, 1, 0, 4, 15, 0, time.UTC), q.EndUTC.V)
}

func TestParseFreqHz(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"14.075219", 14075219},
		{"14.074", 14074000},
		{"7", 7000000},
		{"7.", 7000000},
		{".5", 500000},
		{"146.520000", 146520000},
		{" 14.074 ", 14074000},
		{"14.0752195", 14075220},
		{"14.0752194999", 14075219},
		{"0.9999995", 1000000},
		{"0", 0},
		{"10368.1", 10368100000},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseFreqHz(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseFreqHzErrors(t *testing.T) {
	for _, in := range []string{"", ".", "abc", "-14.074", "+14", "1.2.3", "14,074", "1e3", "14. 074", "99999999999999999999"} {
		t.Run(in, func(t *testing.T) {
			_, err := parseFreqHz(in)
			assert.Error(t, err)
		})
	}
}

func TestFormatFreqMHz(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{14075219, "14.075219"},
		{7047500, "7.0475"},
		{146520000, "146.52"},
		{7000000, "7"},
		{500000, "0.5"},
		{1, "0.000001"},
		{0, "0"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, formatFreqMHz(tt.in))
		})
	}
}

func TestQSOFromRecordMapsFields(t *testing.T) {
	r := minimal(t,
		"STATION_CALLSIGN", "N0CALL", "OPERATOR", "N0OP",
		"BAND", "20m", "FREQ", "14.074", "FREQ_RX", "14.0755",
		"MODE", "MFSK", "SUBMODE", "FT4",
		"RST_SENT", "-08", "RST_RCVD", "+04", "TX_PWR", "7.5",
		"NAME", " José ", "QTH", "Boston", "GRIDSQUARE", "FN42", "MY_GRIDSQUARE", "CM87",
		"DXCC", " 291 ", "COUNTRY", "United States", "CONT", "NA", "CQZ", "05", "ITUZ", "8",
		"DISTANCE", "0", "STATE", "MA", "CNTY", "MA,SUFFOLK", "COMMENT", "hi\nthere",
		"POTA_REF", "US-0001", "MY_CITY", "", "APP_JSTLOGGER_UUID", "abc-123",
	)
	q, err := QSOFromRecord(r)
	require.NoError(t, err)
	want := QSO{
		UUID:            "abc-123",
		StartUTC:        time.Date(2026, 10, 28, 14, 42, 17, 0, time.UTC),
		Call:            "W1AW",
		StationCallsign: "N0CALL",
		Operator:        "N0OP",
		FreqHz:          sql.Null[int64]{V: 14074000, Valid: true},
		FreqRxHz:        sql.Null[int64]{V: 14075500, Valid: true},
		Band:            "20m",
		Mode:            "MFSK",
		Submode:         "FT4",
		RSTSent:         "-08",
		RSTRcvd:         "+04",
		TxPwr:           sql.Null[float64]{V: 7.5, Valid: true},
		Name:            "José",
		QTH:             "Boston",
		Gridsquare:      "FN42",
		MyGridsquare:    "CM87",
		DXCC:            sql.Null[int]{V: 291, Valid: true},
		Country:         "United States",
		Cont:            "NA",
		CQZ:             sql.Null[int]{V: 5, Valid: true},
		ITUZ:            sql.Null[int]{V: 8, Valid: true},
		DistanceKm:      sql.Null[float64]{V: 0, Valid: true},
		State:           "MA",
		Cnty:            "MA,SUFFOLK",
		Comment:         "hi\nthere",
		Fields:          map[string]string{"POTA_REF": "US-0001"},
	}
	assert.Equal(t, want, q)
}

func TestQSOFromRecordConfirmations(t *testing.T) {
	day := func(d int) sql.Null[time.Time] {
		return sql.Null[time.Time]{V: time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC), Valid: true}
	}
	tests := []struct {
		name  string
		extra []string
		want  []Confirmation
	}{
		{"none", nil, nil},
		{"empty values make no confirmation", []string{"QSL_SENT", "", "LOTW_QSL_RCVD", " "}, nil},
		{"card", []string{"QSL_SENT", "Y", "QSL_RCVD", "N", "QSLSDATE", "20261001", "QSLRDATE", "20261002"},
			[]Confirmation{{Service: ServiceCard, Sent: "Y", SentDate: day(1), Rcvd: "N", RcvdDate: day(2)}}},
		{"lotw", []string{"LOTW_QSL_SENT", "Y", "LOTW_QSL_RCVD", "Y", "LOTW_QSLSDATE", "20261003", "LOTW_QSLRDATE", "20261004"},
			[]Confirmation{{Service: ServiceLoTW, Sent: "Y", SentDate: day(3), Rcvd: "Y", RcvdDate: day(4)}}},
		{"eqsl rcvd date only", []string{"EQSL_QSLRDATE", "20261005"},
			[]Confirmation{{Service: ServiceEQSL, RcvdDate: day(5)}}},
		{"all three in stable order", []string{"EQSL_QSL_SENT", "R", "LOTW_QSL_SENT", "N", "QSL_RCVD", "I"},
			[]Confirmation{{Service: ServiceCard, Rcvd: "I"}, {Service: ServiceLoTW, Sent: "N"}, {Service: ServiceEQSL, Sent: "R"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := QSOFromRecord(minimal(t, tt.extra...))
			require.NoError(t, err)
			assert.Equal(t, tt.want, q.Confirmations)
		})
	}
}

func TestRecordFromQSO(t *testing.T) {
	q := QSO{
		ID:              7,
		UUID:            "abc-123",
		StartUTC:        time.Date(2026, 10, 28, 14, 42, 0, 0, time.UTC),
		EndUTC:          sql.Null[time.Time]{V: time.Date(2026, 10, 29, 0, 1, 2, 0, time.UTC), Valid: true},
		Call:            "W1AW",
		FreqHz:          sql.Null[int64]{V: 7047500, Valid: true},
		Band:            "40m",
		Mode:            "FT8",
		TxPwr:           sql.Null[float64]{V: 10, Valid: true},
		DXCC:            sql.Null[int]{V: 0, Valid: true},
		ITURegion:       sql.Null[int]{V: 2, Valid: true},
		DistanceKm:      sql.Null[float64]{V: 3111.2, Valid: true},
		Fields:          map[string]string{"SOTA_REF": "W1/HA-001", "ANT_AZ": "090", "EMPTY": ""},
		Confirmations:   []Confirmation{{Service: ServiceEQSL, Rcvd: "Y"}, {Service: "clublog", Sent: "Y"}, {Service: ServiceCard, Sent: "N", SentDate: sql.Null[time.Time]{V: time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), Valid: true}}},
		Tags:            []string{"pota"},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		StationCallsign: "",
	}
	want := rec(t,
		"CALL", "W1AW",
		"QSO_DATE", "20261028", "TIME_ON", "144200",
		"QSO_DATE_OFF", "20261029", "TIME_OFF", "000102",
		"BAND", "40m", "FREQ", "7.0475", "MODE", "FT8",
		"TX_PWR", "10", "DXCC", "0", "DISTANCE", "3111.2",
		"QSL_SENT", "N", "QSLSDATE", "20261030",
		"EQSL_QSL_RCVD", "Y",
		"ANT_AZ", "090", "SOTA_REF", "W1/HA-001",
		"APP_JSTLOGGER_UUID", "abc-123",
	)
	assert.Equal(t, want, RecordFromQSO(q))
}

func TestRecordFromQSOOmitsEmptyUUID(t *testing.T) {
	r := RecordFromQSO(QSO{Call: "W1AW", StartUTC: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	assert.Equal(t, rec(t, "CALL", "W1AW", "QSO_DATE", "20260102", "TIME_ON", "030405"), r)
}

func TestRecordFromQSOConvertsToUTC(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	r := RecordFromQSO(QSO{Call: "W1AW", StartUTC: time.Date(2026, 1, 1, 21, 0, 0, 0, loc)})
	assert.Equal(t, rec(t, "CALL", "W1AW", "QSO_DATE", "20260102", "TIME_ON", "020000"), r)
}

func TestADIFRecordRoundTrip(t *testing.T) {
	r := rec(t,
		"CALL", "EA8ABC", "QSO_DATE", "20261030", "TIME_ON", "235930",
		"QSO_DATE_OFF", "20261031", "TIME_OFF", "000415",
		"BAND", "40m", "FREQ", "7.0475", "FREQ_RX", "7.049", "MODE", "MFSK", "SUBMODE", "FT4",
		"RST_SENT", "-08", "RST_RCVD", "+04", "TX_PWR", "7.5",
		"STATION_CALLSIGN", "N0CALL", "OPERATOR", "N0OP",
		"NAME", "José", "QTH", "Las Palmas", "GRIDSQUARE", "IL18", "MY_GRIDSQUARE", "CM87",
		"DXCC", "29", "COUNTRY", "Canary Islands", "CONT", "AF", "CQZ", "33", "ITUZ", "36",
		"DISTANCE", "8000.1", "STATE", "GC", "CNTY", "X,Y", "COMMENT", "line one\nline two <x>",
		"QSL_SENT", "Y", "QSL_RCVD", "N", "QSLSDATE", "20261101", "QSLRDATE", "20261102",
		"LOTW_QSL_SENT", "Y", "LOTW_QSL_RCVD", "Y", "LOTW_QSLSDATE", "20261103", "LOTW_QSLRDATE", "20261104",
		"EQSL_QSL_SENT", "Y", "EQSL_QSL_RCVD", "R", "EQSL_QSLSDATE", "20261105", "EQSL_QSLRDATE", "20261106",
		"SOTA_REF", "EA8/GC-001", "APP_OTHERLOG_ID", "abc-123", "ANT_AZ", "090",
		"APP_JSTLOGGER_UUID", "0b6c3a9e-1111-4222-8333-444455556666",
	)
	q, err := QSOFromRecord(r)
	require.NoError(t, err)
	assert.Empty(t, adif.Diff([]adif.Record{r}, []adif.Record{RecordFromQSO(q)}))
}

func TestADIFRoundTripThroughPrepare(t *testing.T) {
	// prepare is what the store applies; a record already in canonical form
	// must survive it unchanged.
	r := minimal(t, "BAND", "20m", "FREQ", "14.074", "MODE", "FT8", "GRIDSQUARE", "FN42", "DISTANCE", "4000.25")
	q, err := QSOFromRecord(r)
	require.NoError(t, err)
	p, err := prepare(q, "CM87")
	require.NoError(t, err)
	assert.Empty(t, adif.Diff([]adif.Record{r}, []adif.Record{RecordFromQSO(p)}))
}
