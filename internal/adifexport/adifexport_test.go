package adifexport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
)

// fixedNow is the creation time used by tests that check the header.
func fixedNow() time.Time {
	return time.Date(2026, 10, 31, 13, 34, 5, 0, time.FixedZone("X", 3600))
}

// newStore opens an empty logbook in a temp directory.
func newStore(t *testing.T) *logbook.SQLiteStore {
	t.Helper()
	ctx := context.Background()
	s, err := logbook.Open(ctx, filepath.Join(t.TempDir(), "logbook.db"), logbook.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, s.Close()) })
	return s
}

// add stores a QSO with call at start and applies tags to it.
func add(t *testing.T, s logbook.Store, call string, start time.Time, tags ...string) {
	t.Helper()
	ctx := context.Background()
	q, err := s.Add(ctx, logbook.QSO{StartUTC: start, Call: call})
	require.NoError(t, err)
	if len(tags) > 0 {
		require.NoError(t, s.Tag(ctx, []int64{q.ID}, tags...))
	}
}

// calls returns each record's CALL.
func calls(f adif.File) []string {
	var out []string
	for _, r := range f.Records {
		c, _ := r.Get("CALL")
		out = append(out, c)
	}
	return out
}

// parse parses an ADI document.
func parse(t *testing.T, data []byte) adif.File {
	t.Helper()
	f, err := adif.Parse(bytes.NewReader(data))
	require.NoError(t, err)
	return f
}

func TestWriteHeader(t *testing.T) {
	s := newStore(t)
	var buf bytes.Buffer
	n, err := Write(context.Background(), s, &buf, Options{Version: "1.2.3", Now: fixedNow})
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	f := parse(t, buf.Bytes())
	require.NotNil(t, f.Header)
	assert.Equal(t, "jstlogger ADIF export", f.Header.Preamble)
	assert.Equal(t, adif.Record{
		{Name: "ADIF_VER", Value: "3.1.6"},
		{Name: "PROGRAMID", Value: "jstlogger"},
		{Name: "PROGRAMVERSION", Value: "1.2.3"},
		{Name: "CREATED_TIMESTAMP", Value: "20261031 123405"},
	}, f.Header.Fields)
	assert.Empty(t, f.Records)
}

func TestWriteDefaultNow(t *testing.T) {
	s := newStore(t)
	var buf bytes.Buffer
	before := time.Now().UTC().Truncate(time.Second)
	_, err := Write(context.Background(), s, &buf, Options{})
	require.NoError(t, err)
	after := time.Now().UTC()

	f := parse(t, buf.Bytes())
	require.NotNil(t, f.Header)
	ts, ok := f.Header.Fields.Get("CREATED_TIMESTAMP")
	require.True(t, ok)
	got, err := time.Parse("20060102 150405", ts)
	require.NoError(t, err)
	assert.False(t, got.Before(before) || got.After(after), "a nil Now uses the current time")
}

func TestWriteRecordsOldestFirst(t *testing.T) {
	s := newStore(t)
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	add(t, s, "K1MID", t0.Add(time.Hour))
	add(t, s, "K1NEW", t0.Add(2*time.Hour))
	add(t, s, "K1OLD", t0)

	var buf bytes.Buffer
	n, err := Write(context.Background(), s, &buf, Options{Now: fixedNow})
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	f := parse(t, buf.Bytes())
	assert.Equal(t, []string{"K1OLD", "K1MID", "K1NEW"}, calls(f))
	for _, r := range f.Records {
		uuid, ok := r.Get("APP_JSTLOGGER_UUID")
		assert.True(t, ok && uuid != "", "every record carries its UUID")
	}
}

func TestWriteFilter(t *testing.T) {
	s := newStore(t)
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	add(t, s, "K1A", t0, "a")
	add(t, s, "K1AB", t0.Add(time.Hour), "a", "b")
	add(t, s, "K1N", t0.Add(2*time.Hour))

	tests := []struct {
		name string
		tags []string
		want []string
	}{
		{"no tags writes all", nil, []string{"K1A", "K1AB", "K1N"}},
		{"one tag", []string{"a"}, []string{"K1A", "K1AB"}},
		{"all tags must match", []string{"a", "b"}, []string{"K1AB"}},
		{"unknown tag", []string{"nope"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := Write(context.Background(), s, &buf, Options{Filter: logbook.Filter{Tags: tt.tags}, Now: fixedNow})
			require.NoError(t, err)
			assert.Equal(t, len(tt.want), n)
			assert.Equal(t, tt.want, calls(parse(t, buf.Bytes())))
		})
	}
}

// failingSource is a Source whose Each fails.
type failingSource struct{ err error }

func (f failingSource) Each(context.Context, logbook.Filter, func(logbook.QSO) error) error {
	return f.err
}

func TestWriteSourceError(t *testing.T) {
	boom := errors.New("boom")
	_, err := Write(context.Background(), failingSource{boom}, io.Discard, Options{})
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "write QSOs")
}

func TestWriteFileNew(t *testing.T) {
	s := newStore(t)
	add(t, s, "K1AB", time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	dir := t.TempDir()
	path := filepath.Join(dir, "out.adi")

	n, err := WriteFile(context.Background(), s, path, Options{Version: "v", Now: fixedNow})
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"K1AB"}, calls(parse(t, data)))

	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm(), "a new export has the usual file mode")
	assertOnlyFile(t, dir, "out.adi")
}

func TestWriteFileReplacesKeepingMode(t *testing.T) {
	s := newStore(t)
	add(t, s, "K1AB", time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	dir := t.TempDir()
	path := filepath.Join(dir, "out.adi")
	require.NoError(t, os.WriteFile(path, []byte("old contents that are much longer than the new ones would be"), 0o600))
	require.NoError(t, os.Chmod(path, 0o600))

	_, err := WriteFile(context.Background(), s, path, Options{Now: fixedNow})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"K1AB"}, calls(parse(t, data)))
	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "a replaced export keeps its mode")
	assertOnlyFile(t, dir, "out.adi")
}

func TestWriteFileFailureLeavesNoFile(t *testing.T) {
	boom := errors.New("boom")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.adi")

	_, err := WriteFile(context.Background(), failingSource{boom}, path, Options{})
	require.ErrorIs(t, err, boom)
	assert.NoFileExists(t, path)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no temporary file is left behind")
}

func TestWriteFileFailureKeepsExistingFile(t *testing.T) {
	boom := errors.New("boom")
	dir := t.TempDir()
	path := filepath.Join(dir, "out.adi")
	require.NoError(t, os.WriteFile(path, []byte("old"), 0o644))

	_, err := WriteFile(context.Background(), failingSource{boom}, path, Options{})
	require.ErrorIs(t, err, boom)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "old", string(data), "a failed export leaves the old file alone")
	assertOnlyFile(t, dir, "out.adi")
}

func TestWriteFileMissingDirectory(t *testing.T) {
	s := newStore(t)
	path := filepath.Join(t.TempDir(), "missing", "out.adi")
	_, err := WriteFile(context.Background(), s, path, Options{})
	require.Error(t, err)
	assert.NoFileExists(t, path)
}

// assertOnlyFile checks that dir holds exactly the file name.
func assertOnlyFile(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file is left behind")
	assert.Equal(t, name, entries[0].Name())
}
