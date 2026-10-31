package adif

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	require.NoError(t, w.WriteHeader(Header{
		Preamble: "jstlogger export",
		Fields:   Record{{"adif_ver", "3.1.4"}},
	}))
	require.NoError(t, w.Write(Record{{"call", "K1ABC"}, {"NAME", ""}, {"BAND", "20m"}}))
	require.NoError(t, w.Flush())

	want := "jstlogger export\n<ADIF_VER:5>3.1.4\n<EOH>\n\n<CALL:5>K1ABC\n<BAND:3>20m\n<EOR>\n\n"
	assert.Equal(t, want, buf.String())
}

func TestWriterNoPreamble(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	require.NoError(t, w.WriteHeader(Header{Fields: Record{{"PROGRAMID", "x"}}}))
	require.NoError(t, w.Flush())
	// A header is recognized only when the file does not start with "<",
	// so an empty preamble is written as a blank line.
	got, err := Parse(&buf)
	require.NoError(t, err)
	require.NotNil(t, got.Header)
	assert.Equal(t, Record{{"PROGRAMID", "x"}}, got.Header.Fields)
}

func TestWriterRoundTrip(t *testing.T) {
	header := Header{Preamble: "Generated\nby test", Fields: Record{{"ADIF_VER", "3.1.4"}, {"PROGRAMID", "jstlogger"}}}
	records := []Record{
		{{"CALL", "K1ABC"}, {"QSO_DATE", "20260101"}, {"TIME_ON", "1200"}},
		{{"CALL", "W1AW"}, {"NOTES", "multi\r\nline <with> brackets\n"}, {"NAME_INTL", "José Ø"}},
		{{"COMMENT", "<EOR>"}},
	}

	var buf bytes.Buffer
	w := NewWriter(&buf)
	require.NoError(t, w.WriteHeader(header))
	for _, r := range records {
		require.NoError(t, w.Write(r))
	}
	require.NoError(t, w.Flush())

	got, err := Parse(&buf)
	require.NoError(t, err)
	require.NotNil(t, got.Header)
	assert.Equal(t, header, *got.Header)
	assert.Equal(t, records, got.Records)
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, assert.AnError }

func TestWriterPropagatesErrors(t *testing.T) {
	w := NewWriter(failWriter{})
	require.NoError(t, w.Write(Record{{"CALL", "K1ABC"}}))
	err := w.Flush()
	assert.ErrorIs(t, err, assert.AnError)
}
