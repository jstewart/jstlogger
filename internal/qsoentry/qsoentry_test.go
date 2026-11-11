package qsoentry

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
)

// testNow is the current time in the tests.
var testNow = time.Date(2026, 11, 10, 18, 30, 0, 0, time.UTC)

var testStation = Station{Callsign: "W1AW", Operator: "K1XX", Grid: "FN31pr"}

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

	q, err := Build(f, testStation, testNow)
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
}

func TestBuildMinimal(t *testing.T) {
	var f Form
	f[Date] = "20261109"
	f[TimeOn] = "0005"
	f[Call] = "K1ABC"

	q, err := Build(f, Station{}, testNow)
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

			q, err := Build(f, Station{}, testNow)
			require.NoError(t, err)
			assert.Equal(t, sql.Null[time.Time]{V: tt.want, Valid: true}, q.EndUTC)
		})
	}
}

func TestBuildBandOverride(t *testing.T) {
	f := validForm()
	f[Freq] = ""
	f[Band] = "40M"

	q, err := Build(f, Station{}, testNow)
	require.NoError(t, err)
	assert.Equal(t, "40m", q.Band, "a band without a freq is kept")

	f[Freq] = "7074"
	q, err = Build(f, Station{}, testNow)
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

			_, err := Build(f, Station{}, testNow)

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

	_, err := Build(f, Station{}, testNow)

	errs := fieldErrors(t, err)
	assert.ElementsMatch(t, []Field{Call, Date, TimeOn, Gridsquare}, errs.Fields())
	assert.Equal(t, []Field{Date, TimeOn, Call, Gridsquare}, errs.Fields(), "fields come in form order")
	assert.Contains(t, err.Error(), "call is required")
}

func TestErrorsMatchErrorsAs(t *testing.T) {
	_, err := Build(Form{}, Station{}, testNow)
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
	f := validForm()
	f[Band] = "20m"
	f[DateOff], f[TimeOff] = "2026-11-09", "14:10"
	f[FreqRX], f[BandRX] = "14.081", "20m"
	f[Name], f[QTH], f[Gridsquare], f[Comment], f[Tags] = "Ann", "Boston", "FN42", "hi", "pota"

	got := f.Carry()

	var want Form
	want[Date] = "2026-11-09"
	want[Mode], want[Submode] = "MFSK", "FT4"
	want[Freq], want[Band] = "14.080", "20m"
	want[RSTSent], want[RSTRcvd] = "-10", "-12"
	assert.Equal(t, want, got)
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
}
