package fle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/logbook"
	"jstlogger/internal/qsoentry"
)

// testNow is the current time in the tests.
var testNow = time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)

// brief is the part of a parsed QSO the tests check.
type brief struct {
	Line    int
	Call    string
	Start   string
	Band    string
	FreqHz  int64
	Mode    string
	Submode string
	Sent    string
	Rcvd    string
	Grid    string
	Name    string
	Comment string
	Fields  map[string]string
}

// briefs reduces the QSOs of r to what the tests check.
func briefs(r Result) []brief {
	var out []brief
	for _, q := range r.QSOs {
		out = append(out, brief{
			Line:    q.Line,
			Call:    q.QSO.Call,
			Start:   q.QSO.StartUTC.Format("2006-01-02 15:04"),
			Band:    q.QSO.Band,
			FreqHz:  q.QSO.FreqHz.V,
			Mode:    q.QSO.Mode,
			Submode: q.QSO.Submode,
			Sent:    q.QSO.RSTSent,
			Rcvd:    q.QSO.RSTRcvd,
			Grid:    q.QSO.Gridsquare,
			Name:    q.QSO.Name,
			Comment: q.QSO.Comment,
			Fields:  q.QSO.Fields,
		})
	}
	return out
}

func TestParseQSOs(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []brief
	}{
		{
			name: "issue example",
			text: "date 2026-11-27\n20m ssb\n1734 4W7EST\n5 HB9HIL 57 55\ncw\n1800 DJ7NT #JO31\n13 dn5ce @Anne <fun qso> k-0003\n",
			want: []brief{
				{Line: 3, Call: "4W7EST", Start: "2026-11-27 17:34", Band: "20m", Mode: "SSB", Sent: "59", Rcvd: "59"},
				{Line: 4, Call: "HB9HIL", Start: "2026-11-27 17:35", Band: "20m", Mode: "SSB", Sent: "57", Rcvd: "55"},
				{Line: 6, Call: "DJ7NT", Start: "2026-11-27 18:00", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599", Grid: "JO31"},
				{
					Line: 7, Call: "DN5CE", Start: "2026-11-27 18:13", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599",
					Name: "Anne", Comment: "fun qso", Fields: map[string]string{"POTA_REF": "K-0003"},
				},
			},
		},
		{
			name: "default date is today",
			text: "40m cw\n0900 K1ABC",
			want: []brief{{Line: 2, Call: "K1ABC", Start: "2026-11-28 09:00", Band: "40m", Mode: "CW", Sent: "599", Rcvd: "599"}},
		},
		{
			name: "single digit reports expand in phone",
			text: "date 2026-11-27\n20m ssb\n1000 DL1ABC 7 3",
			want: []brief{{Line: 3, Call: "DL1ABC", Start: "2026-11-27 10:00", Band: "20m", Mode: "SSB", Sent: "57", Rcvd: "53"}},
		},
		{
			name: "single digit reports expand in cw",
			text: "date 2026-11-27\n20m cw\n1000 DL1ABC 5 4",
			want: []brief{{Line: 3, Call: "DL1ABC", Start: "2026-11-27 10:00", Band: "20m", Mode: "CW", Sent: "559", Rcvd: "549"}},
		},
		{
			name: "signed reports in ft8",
			text: "date 2026-11-27\n20m ft8\n1000 K1ABC\n42 DF2ET -4 -12\n43 DF3ET +3",
			want: []brief{
				{Line: 3, Call: "K1ABC", Start: "2026-11-27 10:00", Band: "20m", Mode: "FT8", Sent: "-10", Rcvd: "-10"},
				{Line: 4, Call: "DF2ET", Start: "2026-11-27 10:42", Band: "20m", Mode: "FT8", Sent: "-4", Rcvd: "-12"},
				{Line: 5, Call: "DF3ET", Start: "2026-11-27 10:43", Band: "20m", Mode: "FT8", Sent: "+3", Rcvd: "-10"},
			},
		},
		{
			name: "one report is the one sent",
			text: "date 2026-11-27\n20m ssb\n1423 LA8AJA/P 43",
			want: []brief{{Line: 3, Call: "LA8AJA/P", Start: "2026-11-27 14:23", Band: "20m", Mode: "SSB", Sent: "43", Rcvd: "59"}},
		},
		{
			name: "submode sets its mode",
			text: "date 2026-11-27\n20m ft4\n1000 K1ABC\nUSB\n1001 K1ABD",
			want: []brief{
				{Line: 3, Call: "K1ABC", Start: "2026-11-27 10:00", Band: "20m", Mode: "MFSK", Submode: "FT4", Sent: "-10", Rcvd: "-10"},
				{Line: 5, Call: "K1ABD", Start: "2026-11-27 10:01", Band: "20m", Mode: "SSB", Submode: "USB", Sent: "59", Rcvd: "59"},
			},
		},
		{
			name: "frequency sets the band, a band clears the frequency",
			text: "date 2026-11-27\n10.112 CW\n1000 K1ABC\n70CM fm\n1001 K1ABD\n7.145 ssb\n1002 K1ABE",
			want: []brief{
				{Line: 3, Call: "K1ABC", Start: "2026-11-27 10:00", Band: "30m", FreqHz: 10_112_000, Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 5, Call: "K1ABD", Start: "2026-11-27 10:01", Band: "70cm", Mode: "FM", Sent: "59", Rcvd: "59"},
				{Line: 7, Call: "K1ABE", Start: "2026-11-27 10:02", Band: "40m", FreqHz: 7_145_000, Mode: "SSB", Sent: "59", Rcvd: "59"},
			},
		},
		{
			name: "minutes roll into the next hour and past midnight",
			text: "date 2026-11-26\n20m cw\n2358 K1ABC\n59 K1ABD\n2 K1ABE\n5 K1ABF\n0010 K1ABG",
			want: []brief{
				{Line: 3, Call: "K1ABC", Start: "2026-11-26 23:58", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 4, Call: "K1ABD", Start: "2026-11-26 23:59", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 5, Call: "K1ABE", Start: "2026-11-27 00:02", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 6, Call: "K1ABF", Start: "2026-11-27 00:05", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 7, Call: "K1ABG", Start: "2026-11-27 00:10", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
			},
		},
		{
			name: "minutes roll into the next hour",
			text: "date 2026-11-27\n20m cw\n1055 K1ABC\n3 K1ABD",
			want: []brief{
				{Line: 3, Call: "K1ABC", Start: "2026-11-27 10:55", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 4, Call: "K1ABD", Start: "2026-11-27 11:03", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
			},
		},
		{
			name: "one digit is the last digit of the minute",
			text: "date 2026-11-27\n20m cw\n1734 K1ABC\n8 K1ABD\n2 K1ABE\n1758 K1ABF\n3 K1ABG",
			want: []brief{
				{Line: 3, Call: "K1ABC", Start: "2026-11-27 17:34", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 4, Call: "K1ABD", Start: "2026-11-27 17:38", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 5, Call: "K1ABE", Start: "2026-11-27 17:42", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 6, Call: "K1ABF", Start: "2026-11-27 17:58", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 7, Call: "K1ABG", Start: "2026-11-27 18:03", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
			},
		},
		{
			name: "day moves the date forward",
			text: "date 2026-11-25\n20m cw\nday +\n1000 K1ABC\nday ++\n1000 K1ABD",
			want: []brief{
				{Line: 4, Call: "K1ABC", Start: "2026-11-26 10:00", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
				{Line: 6, Call: "K1ABD", Start: "2026-11-28 10:00", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"},
			},
		},
		{
			name: "references",
			text: "date 2026-11-27\n20m ssb\n" +
				"1000 HB0/F4ANS ch-0067,ch-0068\n" +
				"1001 dl/4w7est/m dlff-0001\n" +
				"1002 K1ABC w1/ha-001\n" +
				"1003 K1ABD eu-005\n" +
				"1004 K1ABE k-1234@us-ma\n",
			want: []brief{
				{Line: 3, Call: "HB0/F4ANS", Start: "2026-11-27 10:00", Band: "20m", Mode: "SSB", Sent: "59", Rcvd: "59", Fields: map[string]string{"POTA_REF": "CH-0067,CH-0068"}},
				{Line: 4, Call: "DL/4W7EST/M", Start: "2026-11-27 10:01", Band: "20m", Mode: "SSB", Sent: "59", Rcvd: "59", Fields: map[string]string{"WWFF_REF": "DLFF-0001"}},
				{Line: 5, Call: "K1ABC", Start: "2026-11-27 10:02", Band: "20m", Mode: "SSB", Sent: "59", Rcvd: "59", Fields: map[string]string{"SOTA_REF": "W1/HA-001"}},
				{Line: 6, Call: "K1ABD", Start: "2026-11-27 10:03", Band: "20m", Mode: "SSB", Sent: "59", Rcvd: "59", Fields: map[string]string{"IOTA": "EU-005"}},
				{Line: 7, Call: "K1ABE", Start: "2026-11-27 10:04", Band: "20m", Mode: "SSB", Sent: "59", Rcvd: "59", Fields: map[string]string{"POTA_REF": "K-1234@US-MA"}},
			},
		},
		{
			name: "grid, name, comment and QSL message in any order",
			text: "date 2026-11-27\n20m ssb\n1000 K1ABC 55 [tnx qso] fn31pr <nice  sig> @Bob",
			want: []brief{{
				Line: 3, Call: "K1ABC", Start: "2026-11-27 10:00", Band: "20m", Mode: "SSB", Sent: "55", Rcvd: "59",
				Grid: "FN31PR", Name: "Bob", Comment: "nice  sig", Fields: map[string]string{"QSLMSG": "tnx qso"},
			}},
		},
		{
			name: "blank lines are skipped and CRLF is fine",
			text: "\r\ndate 2026-11-27\r\n   \r\n20m CW\r\n\t\r\n1000 k1abc\r\n",
			want: []brief{{Line: 6, Call: "K1ABC", Start: "2026-11-27 10:00", Band: "20m", Mode: "CW", Sent: "599", Rcvd: "599"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.text, Options{Now: testNow})
			assert.Empty(t, r.Ignored)
			assert.Equal(t, tt.want, briefs(r))
		})
	}
}

func TestParseIgnored(t *testing.T) {
	tests := []struct {
		name string
		text string
		// want are the ignored lines; a reason is checked to contain the
		// wanted one.
		want []Ignored
		// calls are the calls of the QSOs parsed.
		calls []string
	}{
		{
			name:  "unknown operator changes nothing",
			text:  "date 2026-11-27\n20m cw\n40m foo\n1000 K1ABC",
			want:  []Ignored{{Line: 3, Text: "40m foo", Reason: `"foo"`}},
			calls: []string{"K1ABC"},
		},
		{
			name:  "invalid date",
			text:  "date 2026-11-27\n20m cw\ndate 2026-13-01\ndate\n1000 K1ABC",
			want:  []Ignored{{Line: 3, Text: "date 2026-13-01", Reason: "date"}, {Line: 4, Text: "date", Reason: "date"}},
			calls: []string{"K1ABC"},
		},
		{
			name:  "frequency outside every band",
			text:  "date 2026-11-27\n20m cw\n0.001\n1000 K1ABC",
			want:  []Ignored{{Line: 3, Text: "0.001", Reason: "band"}},
			calls: []string{"K1ABC"},
		},
		{
			name:  "minute with no hour",
			text:  "date 2026-11-27\n20m cw\n5 K1ABC\n1000 K1ABD",
			want:  []Ignored{{Line: 3, Text: "5 K1ABC", Reason: "hour"}},
			calls: []string{"K1ABD"},
		},
		{
			name: "bad times",
			text: "date 2026-11-27\n20m cw\n123 K1ABC\n2460 K1ABD\n2400 K1ABE\n75 K1ABF\n12345 K1ABG\n1000 K1ABH",
			want: []Ignored{
				{Line: 3, Text: "123 K1ABC", Reason: "123"},
				{Line: 4, Text: "2460 K1ABD", Reason: "2460"},
				{Line: 5, Text: "2400 K1ABE", Reason: "2400"},
				{Line: 6, Text: "75 K1ABF", Reason: "75"},
				{Line: 7, Text: "12345 K1ABG", Reason: "12345"},
			},
			calls: []string{"K1ABH"},
		},
		{
			name: "bad or missing callsign",
			text: "date 2026-11-27\n20m cw\n1000 cw\n1001\n1002 1234\n1003 K1A-BC\n1004 K1ABC",
			want: []Ignored{
				{Line: 3, Text: "1000 cw", Reason: "callsign"},
				{Line: 4, Text: "1001", Reason: "callsign"},
				{Line: 5, Text: "1002 1234", Reason: "callsign"},
				{Line: 6, Text: "1003 K1A-BC", Reason: "callsign"},
			},
			calls: []string{"K1ABC"},
		},
		{
			name: "unknown segment names the token",
			text: "date 2026-11-27\n20m cw\n1000 K1ABC foo\n1001 K1ABD 599 599 599\n1002 K1ABE",
			want: []Ignored{
				{Line: 3, Text: "1000 K1ABC foo", Reason: `"foo" is not a report, grid, reference, name, comment or QSL message`},
				{Line: 4, Text: "1001 K1ABD 599 599 599", Reason: `"599"`},
			},
			calls: []string{"K1ABE"},
		},
		{
			name: "unterminated brackets",
			text: "date 2026-11-27\n20m cw\n1000 K1ABC <oops\n1001 K1ABD [oops\n1002 K1ABE",
			want: []Ignored{
				{Line: 3, Text: "1000 K1ABC <oops", Reason: "<"},
				{Line: 4, Text: "1001 K1ABD [oops", Reason: "["},
			},
			calls: []string{"K1ABE"},
		},
		{
			name:  "build failure keeps the reason without the package prefix",
			text:  "date 2026-11-28\n20m cw\n1300 K1ABC\n1100 K1ABD",
			want:  []Ignored{{Line: 3, Text: "1300 K1ABC", Reason: "time on can't be in the future"}},
			calls: []string{"K1ABD"},
		},
		{
			name: "an ignored QSO line leaves the previous time alone",
			text: "date 2026-11-27\n20m cw\n1050 K1ABC\n1055 K1ABD foo\n52 K1ABE",
			want: []Ignored{{Line: 4, Text: "1055 K1ABD foo", Reason: `"foo"`}},
			// 52 is after 50, so it stays in hour 10; see TestParsePreviousTime.
			calls: []string{"K1ABC", "K1ABE"},
		},
		{
			name:  "trailing space and CR trimmed from the text",
			text:  "date 2026-11-27\n20m cw\nbogus  \r\n1000 K1ABC",
			want:  []Ignored{{Line: 3, Text: "bogus", Reason: `"bogus"`}},
			calls: []string{"K1ABC"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Parse(tt.text, Options{Now: testNow})
			require.Len(t, r.Ignored, len(tt.want), "ignored: %+v", r.Ignored)
			for i, w := range tt.want {
				assert.Equal(t, w.Line, r.Ignored[i].Line)
				assert.Equal(t, w.Text, r.Ignored[i].Text)
				assert.Contains(t, r.Ignored[i].Reason, w.Reason)
				assert.NotContains(t, r.Ignored[i].Reason, "qsoentry:")
			}
			var calls []string
			for _, q := range r.QSOs {
				calls = append(calls, q.QSO.Call)
			}
			assert.Equal(t, tt.calls, calls)
		})
	}
}

func TestParsePreviousTime(t *testing.T) {
	r := Parse("date 2026-11-27\n20m cw\n1050 K1ABC\n1055 K1ABD foo\n52 K1ABE", Options{Now: testNow})
	require.Len(t, r.QSOs, 2)
	assert.Equal(t, "2026-11-27 10:52", r.QSOs[1].QSO.StartUTC.Format("2006-01-02 15:04"))
}

func TestParseRolledDateCarries(t *testing.T) {
	r := Parse("date 2026-11-26\n20m cw\n2359 K1ABC\n1 K1ABD\n1000 K1ABE", Options{Now: testNow})
	require.Empty(t, r.Ignored)
	require.Len(t, r.QSOs, 3)
	assert.Equal(t, "2026-11-27 10:00", r.QSOs[2].QSO.StartUTC.Format("2006-01-02 15:04"))
}

func TestParseFullTimeKeepsDate(t *testing.T) {
	r := Parse("date 2026-11-26\n20m cw\n2350 K1ABC\n0010 K1ABD", Options{Now: testNow})
	require.Empty(t, r.Ignored)
	require.Len(t, r.QSOs, 2)
	assert.Equal(t, "2026-11-26 00:10", r.QSOs[1].QSO.StartUTC.Format("2006-01-02 15:04"))
}

func TestParseStation(t *testing.T) {
	st := qsoentry.Station{
		Name:     "home",
		Callsign: "W1AW",
		Operator: "K1XX",
		Grid:     "FN31pr",
		POTA:     []string{"US-0001"},
	}
	r := Parse("date 2026-11-27\n20m cw\n1000 K1ABC", Options{Now: testNow, Station: st})
	require.Empty(t, r.Ignored)
	require.Len(t, r.QSOs, 1)
	q := r.QSOs[0].QSO
	assert.Equal(t, "W1AW", q.StationCallsign)
	assert.Equal(t, "K1XX", q.Operator)
	assert.Equal(t, "FN31PR", q.MyGridsquare)
	assert.Equal(t, "US-0001", q.Fields["MY_POTA_REF"])
	assert.Equal(t, "home", q.Fields[logbook.StationField])
}

func TestParseEmpty(t *testing.T) {
	r := Parse("", Options{Now: testNow})
	assert.Empty(t, r.QSOs)
	assert.Empty(t, r.Ignored)
}
