package qsoentry

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// testNow is the current time in the tests.
var testNow = time.Date(2026, 11, 10, 18, 30, 0, 0, time.UTC)

// testStation is a station profile with every field set.
var testStation = Station{
	Name:     "home",
	Callsign: "W1AW",
	Operator: "K1XX",
	Grid:     "FN31pr",
	State:    "CT",
	City:     "Newington",
	POTA:     []string{"US-0001", "US-0002"},
	SOTA:     "W1/HA-001",
	IOTA:     "NA-001",
	WWFF:     "KFF-1234",
}

// validForm returns a form that builds without errors.
func validForm() Form {
	var f Form
	f[Date] = "2026-11-09"
	f[TimeOn] = "14:05"
	f[Call] = "k1abc"
	f[Mode] = "MFSK"
	f[Submode] = "FT4"
	f[Freq] = "14.080"
	f[RSTSent] = "-10"
	f[RSTRcvd] = "-12"
	return f
}

// fieldErrors returns the per-field errors in err, failing the test when
// err is not an Errors.
func fieldErrors(t *testing.T, err error) Errors {
	t.Helper()
	var errs Errors
	require.ErrorAs(t, err, &errs)
	return errs
}

func TestBuild(t *testing.T) {
	f := validForm()
	f[DateOff] = "2026-11-09"
	f[TimeOff] = "14:07:30"
	f[FreqRX] = "14081"
	f[Name] = " Ann "
	f[QTH] = "Boston"
	f[Gridsquare] = "fn42"
	f[Comment] = "  nice signal "
	f[Tags] = "pota, contest ,, pota"
	testStation.Fill(&f)

	q, err := Build(f, testNow)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 11, 9, 14, 5, 0, 0, time.UTC), q.StartUTC)
	assert.Equal(t, sql.Null[time.Time]{V: time.Date(2026, 11, 9, 14, 7, 30, 0, time.UTC), Valid: true}, q.EndUTC)
	assert.Equal(t, "K1ABC", q.Call)
	assert.Equal(t, "MFSK", q.Mode)
	assert.Equal(t, "FT4", q.Submode)
	assert.Equal(t, sql.Null[int64]{V: 14_080_000, Valid: true}, q.FreqHz)
	assert.Equal(t, "20m", q.Band, "the band comes from the freq")
	assert.Equal(t, sql.Null[int64]{V: 14_081_000, Valid: true}, q.FreqRxHz)
	assert.Equal(t, "20m", q.Fields["BAND_RX"], "BAND_RX has no column of its own")
	assert.Equal(t, "-10", q.RSTSent)
	assert.Equal(t, "-12", q.RSTRcvd)
	assert.Equal(t, "Ann", q.Name)
	assert.Equal(t, "Boston", q.QTH)
	assert.Equal(t, "FN42", q.Gridsquare)
	assert.Equal(t, "nice signal", q.Comment)
	assert.Equal(t, []string{"pota", "contest", "pota"}, q.Tags, "the store drops duplicate tags")
	assert.Equal(t, "W1AW", q.StationCallsign)
	assert.Equal(t, "K1XX", q.Operator)
	assert.Equal(t, "FN31PR", q.MyGridsquare)
	assert.Equal(t, "home", q.Fields[logbook.StationField], "the station name is stored")
}

func TestBuildStationNameTrimmed(t *testing.T) {
	f := validForm()
	f[StationName] = "  portable  "
	q, err := Build(f, testNow)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{logbook.StationField: "portable"}, q.Fields)

	f[StationName] = "   "
	q, err = Build(f, testNow)
	require.NoError(t, err)
	assert.Nil(t, q.Fields, "a blank station name is not stored")
}

func TestBuildMinimal(t *testing.T) {
	var f Form
	f[Date] = "20261109"
	f[TimeOn] = "0005"
	f[Call] = "K1ABC"

	q, err := Build(f, testNow)
	require.NoError(t, err)

	assert.Equal(t, time.Date(2026, 11, 9, 0, 5, 0, 0, time.UTC), q.StartUTC)
	assert.False(t, q.EndUTC.Valid)
	assert.False(t, q.FreqHz.Valid)
	assert.False(t, q.FreqRxHz.Valid)
	assert.Empty(t, q.Band)
	assert.Empty(t, q.Mode)
	assert.Empty(t, q.Fields)
	assert.Empty(t, q.Tags)
}

func TestBuildTimeOff(t *testing.T) {
	tests := []struct {
		name             string
		date, on         string
		dateOff, timeOff string
		want             time.Time
	}{
		{
			name: "same day", date: "2026-11-09", on: "14:05", timeOff: "14:10",
			want: time.Date(2026, 11, 9, 14, 10, 0, 0, time.UTC),
		},
		{
			name: "seconds", date: "2026-11-09", on: "000000", timeOff: "235959",
			want: time.Date(2026, 11, 9, 23, 59, 59, 0, time.UTC),
		},
		{
			name: "past midnight", date: "2026-11-09", on: "23:58", timeOff: "00:03",
			want: time.Date(2026, 11, 10, 0, 3, 0, 0, time.UTC),
		},
		{
			name: "explicit date off", date: "2026-11-08", on: "23:58", dateOff: "2026-11-09", timeOff: "0003",
			want: time.Date(2026, 11, 9, 0, 3, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f Form
			f[Date], f[TimeOn], f[DateOff], f[TimeOff], f[Call] = tt.date, tt.on, tt.dateOff, tt.timeOff, "K1ABC"

			q, err := Build(f, testNow)
			require.NoError(t, err)
			assert.Equal(t, sql.Null[time.Time]{V: tt.want, Valid: true}, q.EndUTC)
		})
	}
}

func TestBuildBandOverride(t *testing.T) {
	f := validForm()
	f[Freq] = ""
	f[Band] = "40M"

	q, err := Build(f, testNow)
	require.NoError(t, err)
	assert.Equal(t, "40m", q.Band, "a band without a freq is kept")

	f[Freq] = "7074"
	q, err = Build(f, testNow)
	require.NoError(t, err)
	assert.Equal(t, "40m", q.Band)
}

func TestBuildValidation(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(f *Form)
		field Field
		want  string
	}{
		{"no call", func(f *Form) { f[Call] = " " }, Call, "call is required"},
		{"call with a space", func(f *Form) { f[Call] = "K1 ABC" }, Call, "call can't contain spaces"},
		{"no date", func(f *Form) { f[Date] = "" }, Date, "date is required"},
		{"bad date", func(f *Form) { f[Date] = "2026-13-01" }, Date, "date must be YYYY-MM-DD"},
		{"future date", func(f *Form) { f[Date] = "2026-11-11" }, Date, "date can't be in the future"},
		{"no time on", func(f *Form) { f[TimeOn] = "" }, TimeOn, "time on is required"},
		{"bad time on", func(f *Form) { f[TimeOn] = "25:00" }, TimeOn, "time on must be HH:MM or HH:MM:SS"},
		{"future time today", func(f *Form) { f[Date], f[TimeOn] = "2026-11-10", "18:31" }, TimeOn, "time on can't be in the future"},
		{"bad date off", func(f *Form) { f[DateOff], f[TimeOff] = "11/09", "14:10" }, DateOff, "date off must be YYYY-MM-DD"},
		{"date off without time off", func(f *Form) { f[DateOff] = "2026-11-09" }, TimeOff, "time off is required with a date off"},
		{"bad time off", func(f *Form) { f[TimeOff] = "1x:00" }, TimeOff, "time off must be HH:MM or HH:MM:SS"},
		{"time off before time on", func(f *Form) { f[DateOff], f[TimeOff] = "2026-11-09", "14:00" }, TimeOff, "time off can't be before time on"},
		{"unknown mode", func(f *Form) { f[Mode], f[Submode] = "NOSUCH", "" }, Mode, `"NOSUCH" is not an ADIF mode`},
		{"submode of another mode", func(f *Form) { f[Submode] = "USB" }, Submode, "USB is not a submode of MFSK"},
		{"bad freq", func(f *Form) { f[Freq] = "fourteen" }, Freq, `invalid freq "fourteen"`},
		{"unknown band", func(f *Form) { f[Freq], f[Band] = "", "21m" }, Band, `"21m" is not a band`},
		{"band doesn't match freq", func(f *Form) { f[Band] = "40m" }, Band, "band 40m doesn't match 14.08 MHz (20m)"},
		{"freq outside every band", func(f *Form) { f[Freq], f[Band] = "15000", "20m" }, Band, "band 20m doesn't match 15 MHz"},
		{"band rx doesn't match freq rx", func(f *Form) { f[FreqRX], f[BandRX] = "7.074", "20m" }, BandRX, "band RX 20m doesn't match 7.074 MHz (40m)"},
		{"bad freq rx", func(f *Form) { f[FreqRX] = "-1" }, FreqRX, `freq RX "-1" must be positive`},
		{"bad grid", func(f *Form) { f[Gridsquare] = "FN4" }, Gridsquare, `"FN4" is not a Maidenhead locator`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validForm()
			tt.edit(&f)

			_, err := Build(f, testNow)

			errs := fieldErrors(t, err)
			require.Contains(t, errs, tt.field, "errors: %v", errs)
			assert.EqualError(t, errs[tt.field], tt.want)
			assert.Len(t, errs, 1, "errors: %v", errs)
		})
	}
}

func TestBuildReportsEveryError(t *testing.T) {
	var f Form
	f[Gridsquare] = "ZZ"

	_, err := Build(f, testNow)

	errs := fieldErrors(t, err)
	assert.ElementsMatch(t, []Field{Call, Date, TimeOn, Gridsquare}, errs.Fields())
	assert.Equal(t, []Field{Date, TimeOn, Call, Gridsquare}, errs.Fields(), "fields come in form order")
	assert.Contains(t, err.Error(), "call is required")
}

func TestErrorsMatchErrorsAs(t *testing.T) {
	_, err := Build(Form{}, testNow)
	wrapped := errors.Join(errors.New("save"), err)

	var errs Errors
	assert.ErrorAs(t, wrapped, &errs)
}

func TestParseFreq(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"14.074", 14_074_000},
		{"14074", 14_074_000},
		{"14074.0", 14_074_000},
		{" 7.0745 ", 7_074_500},
		{"144.174", 144_174_000},
		{"999.999", 999_999_000},
		{"1000", 1_000_000},
		{"1296 MHz", 1_296_000_000},
		{"1296mhz", 1_296_000_000},
		{"7074 kHz", 7_074_000},
		{"0.1378", 137_800},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseFreq(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseFreqInvalid(t *testing.T) {
	for _, in := range []string{"", "  ", "abc", "14.074.000", "0", "-14", "NaN", "Inf", "1e30", "14 GHz"} {
		t.Run(in, func(t *testing.T) {
			_, err := ParseFreq(in)
			assert.Error(t, err)
		})
	}
}

func TestDefaultRST(t *testing.T) {
	tests := []struct {
		mode, submode string
		want          string
	}{
		{"SSB", "", "59"},
		{"SSB", "USB", "59"},
		{"AM", "", "59"},
		{"FM", "", "59"},
		{"DIGITALVOICE", "DSTAR", "59"},
		{"CW", "", "599"},
		{"RTTY", "", "599"},
		{"FT8", "", "-10"},
		{"MFSK", "FT4", "-10"},
		{"mfsk", "ft4", "-10"},
		{"MFSK", "JS8", ""},
		{"PSK", "PSK31", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.mode+"/"+tt.submode, func(t *testing.T) {
			assert.Equal(t, tt.want, DefaultRST(tt.mode, tt.submode))
		})
	}
}

func TestCarry(t *testing.T) {
	f := fullForm()

	got := f.Carry()

	carried := []Field{
		Date, Mode, Submode, Freq, Band, RSTSent, RSTRcvd,
		StationName, StationCallsign, Operator, MyRig, TxPwr, MyGridsquare,
		MyPOTARef, MySOTARef, MyIOTA, MyWWFFRef, MyState, MyCity,
		PropMode, AntPath, SatName, SatMode,
		QSLSent, QSLSentVia, QSLVia, QSLMsg, LoTWSent, EQSLSent, ClubLogSent,
	}
	for k := range FieldCount {
		if slices.Contains(carried, k) {
			assert.Equal(t, f[k], got[k], "%s carries to the next QSO", k)
			assert.NotEmpty(t, got[k], "%s is filled in by fullForm", k)
		} else {
			assert.Empty(t, got[k], "%s is per contact and doesn't carry", k)
		}
	}
}

func TestPath(t *testing.T) {
	km, bearing, err := Path("FN42", "IO91")
	require.NoError(t, err)
	assert.InDelta(t, 5194, km, 1)
	assert.InDelta(t, 53.5, bearing, 0.5)

	_, _, err = Path("", "IO91")
	require.ErrorIs(t, err, logbook.ErrInvalidGrid)
	_, _, err = Path("FN42", "IO9")
	assert.ErrorIs(t, err, logbook.ErrInvalidGrid)
}

func TestFieldString(t *testing.T) {
	assert.Equal(t, "QSO_DATE", Date.String())
	assert.Equal(t, "GRIDSQUARE", Gridsquare.String())
	assert.Equal(t, "TAGS", Tags.String())
	assert.Equal(t, logbook.StationField, StationName.String(), "the station profile name has jstlogger's own field")
	assert.Equal(t, "MY_GRIDSQUARE", MyGridsquare.String())
	assert.Equal(t, "QSLMSG", QSLMsg.String())
	assert.Equal(t, "CLUBLOG_QSL_SENT", ClubLogSent.String())
	assert.Equal(t, "Field(-1)", Field(-1).String())
}

// fullForm returns a valid form with every field filled in.
func fullForm() Form {
	f := validForm()
	f[DateOff], f[TimeOff] = "2026-11-09", "14:10"
	f[Band] = "20m"
	f[FreqRX], f[BandRX] = "14.081", "20m"
	f[Name], f[QTH], f[Gridsquare], f[Comment], f[Tags] = "Ann", "Boston", "FN42", "hi", "pota"
	testStation.Fill(&f)
	f[MyRig], f[TxPwr] = "IC-7300", "100"
	f[DXCC], f[Cont], f[Region], f[CQZ], f[ITUZ] = "291", "na", "none", "5", "8"
	f[State], f[Cnty] = "ma", "MA,Suffolk"
	f[IOTA], f[SOTARef], f[WWFFRef], f[POTARef] = "na-046", "w1/ma-001", "kff-0001", "us-1234, us-5678"
	f[Sig], f[SigInfo], f[DarcDok], f[Email] = "WAB", "TQ38", "b36", "ann@example.com"
	f[PropMode], f[AntPath] = "sat", "s"
	f[SatName], f[SatMode] = "ao-91", "u/v"
	f[Notes] = "  first line\nsecond line  \n"
	f[QSLSent], f[QSLSentVia], f[QSLVia], f[QSLMsg] = "q", "b", "via K1MGR", "TNX QSO"
	f[LoTWSent], f[EQSLSent], f[ClubLogSent] = "y", "r", "n"
	return f
}

func TestStationFill(t *testing.T) {
	var f Form
	f[MyRig], f[TxPwr] = "IC-7300", "100"
	testStation.Fill(&f)

	assert.Equal(t, "home", f[StationName])
	assert.Equal(t, "W1AW", f[StationCallsign])
	assert.Equal(t, "K1XX", f[Operator])
	assert.Equal(t, "FN31pr", f[MyGridsquare])
	assert.Equal(t, "US-0001,US-0002", f[MyPOTARef], "the POTA refs are joined with commas")
	assert.Equal(t, "W1/HA-001", f[MySOTARef])
	assert.Equal(t, "NA-001", f[MyIOTA])
	assert.Equal(t, "KFF-1234", f[MyWWFFRef])
	assert.Equal(t, "CT", f[MyState])
	assert.Equal(t, "Newington", f[MyCity])

	Station{Name: "portable", Callsign: "W1AW/P", Grid: "FN32"}.Fill(&f)

	assert.Equal(t, "portable", f[StationName], "switching profile refills the station")
	assert.Equal(t, "W1AW/P", f[StationCallsign])
	assert.Equal(t, "FN32", f[MyGridsquare])
	for _, k := range []Field{Operator, MyPOTARef, MySOTARef, MyIOTA, MyWWFFRef, MyState, MyCity} {
		assert.Empty(t, f[k], "%s is cleared when the new profile has none", k)
	}
	assert.Equal(t, "IC-7300", f[MyRig], "the rig is not part of a profile")
	assert.Equal(t, "100", f[TxPwr], "the power is not part of a profile")
}

func TestBuildStationTab(t *testing.T) {
	f := validForm()
	testStation.Fill(&f)
	f[StationCallsign], f[Operator] = " w1aw/p ", "k1xx"
	f[MyRig], f[TxPwr] = " IC-7300 ", "5.5"
	f[MyPOTARef] = "us-0001 , us-0002@us-ct"
	f[MySOTARef], f[MyIOTA], f[MyWWFFRef], f[MyState] = "w1/ha-001", "na-001", "kff-1234", "ct"

	q, err := Build(f, testNow)
	require.NoError(t, err)

	assert.Equal(t, "W1AW/P", q.StationCallsign)
	assert.Equal(t, "K1XX", q.Operator)
	assert.Equal(t, "FN31PR", q.MyGridsquare)
	assert.Equal(t, sql.Null[float64]{V: 5.5, Valid: true}, q.TxPwr)
	assert.Equal(t, map[string]string{
		logbook.StationField: "home",
		"MY_RIG":             "IC-7300",
		"MY_POTA_REF":        "US-0001,US-0002@US-CT",
		"MY_SOTA_REF":        "W1/HA-001",
		"MY_IOTA":            "NA-001",
		"MY_WWFF_REF":        "KFF-1234",
		"MY_STATE":           "CT",
		"MY_CITY":            "Newington",
	}, q.Fields, "station fields without a column of their own go in Fields, upper-cased where they are codes")
}

func TestBuildGeneralTab(t *testing.T) {
	f := validForm()
	f[DXCC], f[Cont], f[CQZ], f[ITUZ] = " 291 ", "na", "5", "8"
	f[Region], f[State], f[Cnty] = "none", "ma", "MA,Suffolk"
	f[IOTA], f[SOTARef], f[WWFFRef], f[POTARef] = "na-046", "w1/ma-001", "kff-0001", "us-1234, us-5678"
	f[Sig], f[SigInfo], f[DarcDok], f[Email] = "WAB", "TQ38", "b36", " ann@example.com "
	f[PropMode], f[AntPath] = "es", "l"

	q, err := Build(f, testNow)
	require.NoError(t, err)

	assert.Equal(t, sql.Null[int]{V: 291, Valid: true}, q.DXCC)
	assert.Equal(t, "NA", q.Cont)
	assert.Equal(t, sql.Null[int]{V: 5, Valid: true}, q.CQZ)
	assert.Equal(t, sql.Null[int]{V: 8, Valid: true}, q.ITUZ)
	assert.Equal(t, "MA", q.State)
	assert.Equal(t, "MA,Suffolk", q.Cnty)
	assert.Equal(t, map[string]string{
		"REGION":    "NONE",
		"IOTA":      "NA-046",
		"SOTA_REF":  "W1/MA-001",
		"WWFF_REF":  "KFF-0001",
		"POTA_REF":  "US-1234,US-5678",
		"SIG":       "WAB",
		"SIG_INFO":  "TQ38",
		"DARC_DOK":  "B36",
		"EMAIL":     "ann@example.com",
		"PROP_MODE": "ES",
		"ANT_PATH":  "L",
	}, q.Fields)
}

func TestBuildDXCCZero(t *testing.T) {
	f := validForm()
	f[DXCC] = "0"

	q, err := Build(f, testNow)
	require.NoError(t, err)
	assert.Equal(t, sql.Null[int]{V: 0, Valid: true}, q.DXCC, "DXCC 0 means no entity and is stored")
}

func TestBuildSatellite(t *testing.T) {
	tests := []struct {
		name     string
		propMode string
		want     map[string]string
	}{
		{"via satellite", "sat", map[string]string{"PROP_MODE": "SAT", "SAT_NAME": "AO-91", "SAT_MODE": "U/V"}},
		{"other propagation", "ES", map[string]string{"PROP_MODE": "ES"}},
		{"no propagation", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validForm()
			f[PropMode], f[SatName], f[SatMode] = tt.propMode, " ao-91 ", "u/v"

			q, err := Build(f, testNow)
			require.NoError(t, err)
			assert.Equal(t, tt.want, q.Fields, "the satellite is stored only when PROP_MODE is SAT")
		})
	}
}

func TestBuildNotes(t *testing.T) {
	f := validForm()
	f[Notes] = "\n  first line\n\n  second line \n "

	q, err := Build(f, testNow)
	require.NoError(t, err)
	assert.Equal(t, "first line\n\n  second line", q.Fields["NOTES"], "notes keep their inner lines")
}

func TestBuildQSL(t *testing.T) {
	f := validForm()
	f[QSLSent], f[QSLSentVia], f[QSLVia], f[QSLMsg] = "q", "b", " K1MGR ", " TNX QSO "
	f[EQSLSent], f[ClubLogSent] = "R", "y"

	q, err := Build(f, testNow)
	require.NoError(t, err)

	assert.Equal(t, []logbook.Confirmation{
		{Service: logbook.ServiceCard, Sent: "Q"},
		{Service: logbook.ServiceEQSL, Sent: "R"},
		{Service: logbook.ServiceClubLog, Sent: "Y"},
	}, q.Confirmations, "one confirmation per set status, LoTW left out")
	assert.Equal(t, map[string]string{
		"QSL_SENT_VIA": "B",
		"QSL_VIA":      "K1MGR",
		"QSLMSG":       "TNX QSO",
	}, q.Fields, "the statuses are confirmations, not Fields")
}

func TestBuildNoConfirmations(t *testing.T) {
	q, err := Build(validForm(), testNow)
	require.NoError(t, err)
	assert.Nil(t, q.Confirmations)
	assert.Nil(t, q.Fields)
}

func TestBuildFullFormIsStored(t *testing.T) {
	ctx := t.Context()
	store, err := logbook.Open(ctx, filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	q, err := Build(fullForm(), testNow)
	require.NoError(t, err)
	stored, err := store.Add(ctx, q)
	require.NoError(t, err, "no field Build puts in Fields is one the logbook promotes")

	assert.Equal(t, q.Fields, stored.Fields)
	assert.ElementsMatch(t, q.Confirmations, stored.Confirmations)
	assert.Equal(t, q.TxPwr, stored.TxPwr)
	assert.Equal(t, q.DXCC, stored.DXCC)
	assert.Equal(t, q.State, stored.State)
	assert.Equal(t, q.Cnty, stored.Cnty)
}

func TestBuildTabValidation(t *testing.T) {
	tests := []struct {
		name  string
		field Field
		value string
		want  string
	}{
		{"station callsign with a space", StationCallsign, "W1 AW", "station callsign can't contain spaces"},
		{"operator with a space", Operator, "K1 XX", "operator can't contain spaces"},
		{"bad my grid", MyGridsquare, "FN3", `"FN3" is not a Maidenhead locator`},
		{"bad power", TxPwr, "100W", `"100W" is not a power in watts`},
		{"zero power", TxPwr, "0", `"0" is not a power in watts`},
		{"bad my POTA", MyPOTARef, "K-1234, nope", `"NOPE" is not a POTA reference`},
		{"bad my SOTA", MySOTARef, "w1ha-001", `"W1HA-001" is not a SOTA reference`},
		{"bad my IOTA", MyIOTA, "NA-1", `"NA-1" is not an IOTA reference`},
		{"bad my WWFF", MyWWFFRef, "KFF-1", `"KFF-1" is not a WWFF reference`},
		{"bad DXCC", DXCC, "1000", `"1000" is not a DXCC entity code`},
		{"bad continent", Cont, "am", `"AM" is not a continent`},
		{"bad CQ zone", CQZ, "41", `"41" is not a CQ zone`},
		{"bad ITU zone", ITUZ, "0", `"0" is not an ITU zone`},
		{"bad IOTA", IOTA, "XX-001", `"XX-001" is not an IOTA reference`},
		{"bad SOTA", SOTARef, "W1/HA-1", `"W1/HA-1" is not a SOTA reference`},
		{"bad WWFF", WWFFRef, "FF-0001", `"FF-0001" is not a WWFF reference`},
		{"bad POTA", POTARef, "K-12", `"K-12" is not a POTA reference`},
		{"bad email", Email, "ann at example.com", `"ann at example.com" is not an email address`},
		{"bad propagation mode", PropMode, "warp", `"WARP" is not an ADIF propagation mode`},
		{"bad antenna path", AntPath, "x", `"X" is not an ADIF antenna path`},
		{"bad QSL sent", QSLSent, "v", `"V" is not a QSL sent status`},
		{"bad QSL sent via", QSLSentVia, "x", `"X" is not a QSL sent via`},
		{"bad LoTW sent", LoTWSent, "maybe", `"MAYBE" is not a QSL sent status`},
		{"bad eQSL sent", EQSLSent, "V", `"V" is not a QSL sent status`},
		{"bad Club Log sent", ClubLogSent, "x", `"X" is not a QSL sent status`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := validForm()
			f[tt.field] = tt.value

			_, err := Build(f, testNow)

			errs := fieldErrors(t, err)
			require.Contains(t, errs, tt.field, "errors: %v", errs)
			assert.EqualError(t, errs[tt.field], tt.want)
			assert.Len(t, errs, 1, "errors: %v", errs)
		})
	}
}
