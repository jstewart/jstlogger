package adif

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		header  *Header
		records []Record
	}{
		{
			name:  "no header",
			input: "<CALL:5>K1ABC<BAND:3>20m<EOR>\n<CALL:4>W1AW<EOR>\n",
			records: []Record{
				{{"CALL", "K1ABC"}, {"BAND", "20m"}},
				{{"CALL", "W1AW"}},
			},
		},
		{
			name:  "header with preamble",
			input: "  Exported by test\nsecond line \n<ADIF_VER:5>3.1.4\n<PROGRAMID:4>test\n<EOH>\n<CALL:5>K1ABC<EOR>\n",
			header: &Header{
				Preamble: "Exported by test\nsecond line",
				Fields:   Record{{"ADIF_VER", "3.1.4"}, {"PROGRAMID", "test"}},
			},
			records: []Record{{{"CALL", "K1ABC"}}},
		},
		{
			name:    "header with no fields",
			input:   "preamble only<eoh>",
			header:  &Header{Preamble: "preamble only"},
			records: nil,
		},
		{
			name:    "CRLF line endings",
			input:   "hdr\r\n<EOH>\r\n<CALL:5>K1ABC\r\n<BAND:3>20m\r\n<EOR>\r\n",
			header:  &Header{Preamble: "hdr"},
			records: []Record{{{"CALL", "K1ABC"}, {"BAND", "20m"}}},
		},
		{
			name:  "mixed line endings",
			input: "<CALL:5>K1ABC\r\n<BAND:3>20m\n<EOR>\r\n<CALL:4>W1AW\n<EOR>\n",
			records: []Record{
				{{"CALL", "K1ABC"}, {"BAND", "20m"}},
				{{"CALL", "W1AW"}},
			},
		},
		{
			name:    "lower-case field names",
			input:   "<call:5>K1ABC<Band:3>20m<eor>",
			records: []Record{{{"CALL", "K1ABC"}, {"BAND", "20m"}}},
		},
		{
			name:    "type indicators",
			input:   "<FREQ:6:N>14.074<QSO_DATE:8:D>20260101<EOR>",
			records: []Record{{{"FREQ", "14.074"}, {"QSO_DATE", "20260101"}}},
		},
		{
			name:    "multi-line value",
			input:   "<NOTES:12>line1\r\nline2<EOR>",
			records: []Record{{{"NOTES", "line1\r\nline2"}}},
		},
		{
			name:    "value containing angle brackets",
			input:   "<COMMENT:9><EOR> a>b<EOR>",
			records: []Record{{{"COMMENT", "<EOR> a>b"}}},
		},
		{
			name:    "UTF-8 multibyte value uses byte length",
			input:   "<NAME_INTL:6>José!<EOR>",
			records: []Record{{{"NAME_INTL", "José!"}}},
		},
		{
			name:    "zero-length field",
			input:   "<CALL:5>K1ABC<NAME:0><EOR>",
			records: []Record{{{"CALL", "K1ABC"}, {"NAME", ""}}},
		},
		{
			name:    "case-insensitive EOR and EOH",
			input:   "x<eOh><CALL:5>K1ABC<eor><CALL:4>W1AW<Eor>",
			header:  &Header{Preamble: "x"},
			records: []Record{{{"CALL", "K1ABC"}}, {{"CALL", "W1AW"}}},
		},
		{
			name:    "text between specifiers is ignored",
			input:   "<CALL:5>K1ABC some comment <BAND:3>20m  junk\n<EOR> trailing\n",
			records: []Record{{{"CALL", "K1ABC"}, {"BAND", "20m"}}},
		},
		{
			name:    "empty file",
			input:   "",
			records: nil,
		},
		{
			name:    "byte order mark before records",
			input:   "\xef\xbb\xbf<CALL:5>K1ABC<EOR>",
			records: []Record{{{"CALL", "K1ABC"}}},
		},
		{
			name:    "byte order mark before header",
			input:   "\xef\xbb\xbfexport\n<ADIF_VER:5>3.1.4<EOH><CALL:5>K1ABC<EOR>",
			header:  &Header{Preamble: "export", Fields: Record{{"ADIF_VER", "3.1.4"}}},
			records: []Record{{{"CALL", "K1ABC"}}},
		},
		{
			name:    "leading blank lines without a header",
			input:   "\r\n\n<CALL:5>K1ABC<EOR>",
			records: []Record{{{"CALL", "K1ABC"}}},
		},
		{
			name:    "blank preamble with a header",
			input:   "\n<ADIF_VER:5>3.1.4<eoh><CALL:5>K1ABC<EOR>",
			header:  &Header{Fields: Record{{"ADIF_VER", "3.1.4"}}},
			records: []Record{{{"CALL", "K1ABC"}}},
		},
		{
			name:    "trailing whitespace after last EOR",
			input:   "<CALL:5>K1ABC<EOR>\n\n  \r\n",
			records: []Record{{{"CALL", "K1ABC"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.header, got.Header)
			assert.Equal(t, tt.records, got.Records)
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		record int
		field  string
	}{
		{name: "header without EOH", input: "preamble <ADIF_VER:5>3.1.4\n", record: 0, field: ""},
		{name: "non-numeric length", input: "<CALL:5>K1ABC<EOR><BAND:x>20m<EOR>", record: 2, field: "BAND"},
		{name: "negative length", input: "<CALL:-1>K<EOR>", record: 1, field: "CALL"},
		{name: "length past end of input", input: "<CALL:5>K1ABC<EOR><CALL:10>W1AW<EOR>", record: 2, field: "CALL"},
		{name: "unterminated tag", input: "<CALL:5>K1ABC<BAND:3", record: 1, field: ""},
		{name: "tag without length", input: "<CALL>K1ABC<EOR>", record: 1, field: "CALL"},
		{name: "duplicate field", input: "<CALL:5>K1ABC<call:4>W1AW<EOR>", record: 1, field: "CALL"},
		{name: "truncated record", input: "<CALL:5>K1ABC<EOR><CALL:4>W1AW\n", record: 2, field: ""},
		{name: "EOH in data", input: "<CALL:5>K1ABC<EOR><EOH>", record: 2, field: ""},
		{name: "whitespace inside tag", input: "<CALL: 5>K1ABC<EOR>", record: 1, field: "CALL"},
		{name: "empty field name", input: "<:5>K1ABC<EOR>", record: 1, field: ""},
		{name: "error in header field", input: "hdr<ADIF_VER:x>3<EOH>", record: 0, field: "ADIF_VER"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.input))
			require.Error(t, err)
			var pe *ParseError
			require.True(t, errors.As(err, &pe), "error %v is not a *ParseError", err)
			assert.Equal(t, tt.record, pe.Record)
			assert.Equal(t, tt.field, pe.Field)
			assert.NotNil(t, pe.Unwrap())
		})
	}
}

func TestParseErrorMessage(t *testing.T) {
	inner := errors.New("bad length")
	tests := []struct {
		name string
		err  *ParseError
		want string
	}{
		{"record and field", &ParseError{Record: 12, Field: "FREQ", Err: inner}, "adif: record 12, field FREQ: bad length"},
		{"record only", &ParseError{Record: 3, Err: inner}, "adif: record 3: bad length"},
		{"header", &ParseError{Record: 0, Err: inner}, "adif: header: bad length"},
		{"header field", &ParseError{Record: 0, Field: "ADIF_VER", Err: inner}, "adif: header, field ADIF_VER: bad length"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
			assert.ErrorIs(t, tt.err, inner)
		})
	}
}

func TestParseTruncatedIsSentinel(t *testing.T) {
	_, err := Parse(strings.NewReader("<CALL:4>W1AW"))
	assert.ErrorIs(t, err, ErrTruncated)
}

func TestRecordGet(t *testing.T) {
	r := Record{{"CALL", "K1ABC"}, {"NAME", ""}}
	v, ok := r.Get("call")
	assert.True(t, ok)
	assert.Equal(t, "K1ABC", v)
	v, ok = r.Get("NAME")
	assert.True(t, ok)
	assert.Equal(t, "", v)
	_, ok = r.Get("BAND")
	assert.False(t, ok)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseReadError(t *testing.T) {
	_, err := Parse(errReader{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}
