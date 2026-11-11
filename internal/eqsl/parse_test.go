package eqsl

import (
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		want        []string
		wantSkipped int
	}{
		{
			name: "header and CRLF",
			in:   "List of AG members as of 31-Oct-2026 08:19 UTC\r\nW1AW\r\nVE3ABC\r\n",
			want: []string{"W1AW", "VE3ABC"},
		},
		{
			name: "LF only and no final newline",
			in:   "W1AW\nK1ABC",
			want: []string{"W1AW", "K1ABC"},
		},
		{
			name: "lower case and whitespace",
			in:   "  w1aw \r\n\tk1abc\t\r\n",
			want: []string{"W1AW", "K1ABC"},
		},
		{
			name: "slashed calls kept verbatim",
			in:   "KH6/K1ABC\r\nw1aw/p\r\n",
			want: []string{"KH6/K1ABC", "W1AW/P"},
		},
		{
			name: "blank lines are not counted as skipped",
			in:   "\r\nW1AW\r\n\r\n   \r\nK1ABC\r\n\r\n",
			want: []string{"W1AW", "K1ABC"},
		},
		{
			name: "duplicates kept once in first-seen order",
			in:   "W1AW\r\nK1ABC\r\nw1aw\r\nW1AW\r\n",
			want: []string{"W1AW", "K1ABC"},
		},
		{
			name: "odd entries are skipped and counted",
			in: strings.Join([]string{
				"2E0CRV-2", // hyphen
				"W1 AW",    // inner space
				"W1AW\xe9", // non-UTF-8 byte
				"W1ÄW",     // non-ASCII rune
				"<html>",   // markup
				"K1ABC",
			}, "\r\n"),
			want:        []string{"K1ABC"},
			wantSkipped: 5,
		},
		{
			name: "empty input",
			in:   "",
		},
		{
			name: "header only",
			in:   "List of AG members as of 31-Oct-2026 08:19 UTC\r\n",
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

func TestParseLongLine(t *testing.T) {
	// Well past bufio.Scanner's default 64 KiB token limit.
	in := "W1AW\r\n" + strings.Repeat("A", 200<<10) + "\r\nK1ABC\r\n"

	got, skipped, err := Parse(strings.NewReader(in))

	require.NoError(t, err)
	assert.Equal(t, []string{"W1AW", strings.Repeat("A", 200<<10), "K1ABC"}, got)
	assert.Zero(t, skipped)
}

func TestParseReadError(t *testing.T) {
	errBroken := errors.New("broken pipe")

	_, _, err := Parse(iotest.ErrReader(errBroken))

	require.ErrorIs(t, err, errBroken)
	assert.Contains(t, err.Error(), "eqsl: parse:")
}
