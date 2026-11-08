package lotw

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		want        []User
		wantSkipped int
	}{
		{
			name: "valid rows",
			in:   "W1AW,2026-11-01,12:34:56\nVE3ABC,2025-01-02,00:00:01\n",
			want: []User{
				{Call: "W1AW", LastUpload: utc(2026, 11, 1, 12, 34, 56)},
				{Call: "VE3ABC", LastUpload: utc(2025, 1, 2, 0, 0, 1)},
			},
		},
		{
			name: "lower case and whitespace",
			in:   "  w1aw , 2026-11-01 , 12:34:56 \r\n",
			want: []User{{Call: "W1AW", LastUpload: utc(2026, 11, 1, 12, 34, 56)}},
		},
		{
			name: "malformed rows are skipped and counted",
			in: strings.Join([]string{
				"W1AW,2026-11-01",                // too few fields
				"W1AW,2026-11-01,12:00:00,extra", // too many fields
				"W1AW,2026-13-01,12:00:00",       // bad date
				"W1AW,2026-11-01,25:00:00",       // bad time
				",2026-11-01,12:00:00",           // empty call
				"K1ABC,2026-11-02,01:02:03",
			}, "\n"),
			want:        []User{{Call: "K1ABC", LastUpload: utc(2026, 11, 2, 1, 2, 3)}},
			wantSkipped: 5,
		},
		{
			name: "duplicates keep the latest upload",
			in: "W1AW,2026-10-01,00:00:00\n" +
				"K1ABC,2026-01-01,00:00:00\n" +
				"w1aw,2026-11-01,00:00:00\n" +
				"W1AW,2026-09-01,00:00:00\n",
			want: []User{
				{Call: "W1AW", LastUpload: utc(2026, 11, 1, 0, 0, 0)},
				{Call: "K1ABC", LastUpload: utc(2026, 1, 1, 0, 0, 0)},
			},
		},
		{
			name:        "header row is skipped",
			in:          "callsign,date,time\nW1AW,2026-11-01,12:34:56\n",
			want:        []User{{Call: "W1AW", LastUpload: utc(2026, 11, 1, 12, 34, 56)}},
			wantSkipped: 1,
		},
		{
			name: "empty input",
			in:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, skipped, err := Parse(strings.NewReader(tt.in))

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantSkipped, skipped)
		})
	}
}

func TestParseReturnsCSVErrors(t *testing.T) {
	_, _, err := Parse(strings.NewReader("W1AW,\"2026-11-01,12:00:00\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "lotw: parse")
}
