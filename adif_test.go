package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"jstlogger/internal/adif"
)

// roundTripFixture is a synthetic ADI file whose values are already in
// jstlogger's canonical form, so it survives import and export unchanged.
const roundTripFixture = "testdata/roundtrip.adi"

// logbookConfig writes a valid config whose logbook lives in a fresh temp
// directory and returns the config path.
func logbookConfig(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "logbook.db")
	return writeConfig(t, validConfig+"\n[logbook]\npath = \""+db+"\"\n")
}

// runMode invokes jstlogger with the config at cfg and args, and checks
// that the TUI did not start.
func runMode(t *testing.T, cfg string, args ...string) result {
	t.Helper()
	r := invoke(t, nil, nil, append([]string{"-c", cfg}, args...)...)
	assert.False(t, r.tuiCalled, "the TUI must not start in ADIF modes")
	return r
}

// parseADIF parses the ADI file at path.
func parseADIF(t *testing.T, path string) adif.File {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	f, err := adif.Parse(bytes.NewReader(data))
	require.NoError(t, err)
	return f
}

// writeADIFFile writes content to a temp .adi file and returns its path.
func writeADIFFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.adi")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// roundTrip imports the ADI file at path into an empty logbook, exports the
// logbook and returns the differences between the two record sets,
// ignoring APP_JSTLOGGER_UUID.
func roundTrip(t *testing.T, path string) []string {
	t.Helper()
	cfg := logbookConfig(t)
	r := runMode(t, cfg, "--import-adif", path)
	require.Equal(t, 0, r.code, r.stderr)

	out := filepath.Join(t.TempDir(), "out.adi")
	r = runMode(t, cfg, "--export-adif", out)
	require.Equal(t, 0, r.code, r.stderr)

	orig, exported := parseADIF(t, path), parseADIF(t, out)
	return adif.Diff(orig.Records, exported.Records, "APP_JSTLOGGER_UUID")
}

func TestImportExportRoundTrip(t *testing.T) {
	assert.Empty(t, roundTrip(t, roundTripFixture))
}

func TestRoundTripExternalFile(t *testing.T) {
	path := os.Getenv("JSTLOGGER_ROUNDTRIP_ADIF")
	if path == "" {
		t.Skip("set JSTLOGGER_ROUNDTRIP_ADIF to an ADI file to check its round trip")
	}
	if diffs := roundTrip(t, path); len(diffs) > 0 {
		t.Errorf("%d differences after a round trip:\n%s", len(diffs), strings.Join(diffs, "\n"))
	}
}

func TestImportSummary(t *testing.T) {
	cfg := logbookConfig(t)
	r := runMode(t, cfg, "--import-adif", roundTripFixture)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "jstlogger: imported 4 QSOs, skipped 0 duplicates\n", r.stderr)
	assert.Empty(t, r.stdout)
}

func TestExportFile(t *testing.T) {
	orig := exportNow
	exportNow = func() time.Time { return time.Date(2026, 10, 31, 13, 34, 5, 0, time.FixedZone("X", 3600)) }
	t.Cleanup(func() { exportNow = orig })

	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture).code)

	out := filepath.Join(t.TempDir(), "out.adi")
	r := runMode(t, cfg, "--export-adif", out)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "jstlogger: exported 4 QSOs to "+out+"\n", r.stderr)
	assert.Empty(t, r.stdout)

	f := parseADIF(t, out)
	require.NotNil(t, f.Header)
	assert.Equal(t, "jstlogger ADIF export", f.Header.Preamble)
	assert.Equal(t, adif.Record{
		{Name: "ADIF_VER", Value: "3.1.6"},
		{Name: "PROGRAMID", Value: "jstlogger"},
		{Name: "PROGRAMVERSION", Value: "dev"},
		{Name: "CREATED_TIMESTAMP", Value: "20261031 123405"},
	}, f.Header.Fields)

	require.Len(t, f.Records, 4)
	var calls []string
	for _, rec := range f.Records {
		uuid, ok := rec.Get("APP_JSTLOGGER_UUID")
		assert.True(t, ok && uuid != "", "every record carries its UUID")
		call, _ := rec.Get("CALL")
		calls = append(calls, call)
	}
	assert.Equal(t, []string{"W1AW/P", "KJ1R", "AE6CH", "EA8ABC"}, calls, "oldest first")
}

func TestExportOverwritesExistingFile(t *testing.T) {
	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture).code)

	dir := t.TempDir()
	out := filepath.Join(dir, "out.adi")
	require.NoError(t, os.WriteFile(out, []byte("old contents that are much longer than nothing"), 0o644))

	r := runMode(t, cfg, "--export-adif", out)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Len(t, parseADIF(t, out).Records, 4)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary file is left behind")
	assert.Equal(t, "out.adi", entries[0].Name())
}

func TestExportFileMode(t *testing.T) {
	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture).code)
	dir := t.TempDir()

	fresh := filepath.Join(dir, "fresh.adi")
	require.Equal(t, 0, runMode(t, cfg, "--export-adif", fresh).code)
	fi, err := os.Stat(fresh)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm(), "a new export has the usual file mode")

	private := filepath.Join(dir, "private.adi")
	require.NoError(t, os.WriteFile(private, nil, 0o600))
	require.NoError(t, os.Chmod(private, 0o600))
	require.Equal(t, 0, runMode(t, cfg, "--export-adif", private).code)
	fi, err = os.Stat(private)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "an overwritten export keeps its mode")
}

func TestExportStdout(t *testing.T) {
	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture).code)

	r := runMode(t, cfg, "--export-adif", "-")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "jstlogger: exported 4 QSOs to stdout\n", r.stderr)
	assert.True(t, strings.HasPrefix(r.stdout, "jstlogger ADIF export\n"), "stdout carries only ADIF")
	f, err := adif.Parse(strings.NewReader(r.stdout))
	require.NoError(t, err)
	assert.Len(t, f.Records, 4)
	assert.NoFileExists(t, "-")
}

func TestExportErrorLeavesNoFile(t *testing.T) {
	cfg := logbookConfig(t)
	out := filepath.Join(t.TempDir(), "missing", "out.adi")
	r := runMode(t, cfg, "--export-adif", out)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, "jstlogger: ")
	assert.Contains(t, r.stderr, "export")
	assert.NoFileExists(t, out)
}

func TestReimportSkipsAndDoesNotTagDuplicates(t *testing.T) {
	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture).code)

	r := runMode(t, cfg, "--import-adif", roundTripFixture, "--tag", "again")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "jstlogger: imported 0 QSOs, skipped 4 duplicates\n", r.stderr)

	r = runMode(t, cfg, "--export-adif", "-", "--tag", "again")
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "jstlogger: exported 0 QSOs to stdout\n", r.stderr)
	f, err := adif.Parse(strings.NewReader(r.stdout))
	require.NoError(t, err)
	assert.Empty(t, f.Records)
}

func TestImportReimportsExport(t *testing.T) {
	// An exported file carries UUIDs, so importing it into the same logbook
	// finds every QSO a duplicate.
	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture).code)
	out := filepath.Join(t.TempDir(), "out.adi")
	require.Equal(t, 0, runMode(t, cfg, "--export-adif", out).code)

	r := runMode(t, cfg, "--import-adif", out)
	require.Equal(t, 0, r.code, r.stderr)
	assert.Equal(t, "jstlogger: imported 0 QSOs, skipped 4 duplicates\n", r.stderr)
}

func TestExportTagFilter(t *testing.T) {
	cfg := logbookConfig(t)
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", roundTripFixture, "--tag", "a").code)
	extra := writeADIFFile(t, "<CALL:4>K1AB<QSO_DATE:8>20260601<TIME_ON:4>1200<BAND:3>20m<MODE:2>CW<EOR>\n")
	require.Equal(t, 0, runMode(t, cfg, "--import-adif", extra, "--tag", "a", "--tag", "b").code)

	tests := []struct {
		name string
		tags []string
		want int
	}{
		{"no tags exports all", nil, 5},
		{"one tag", []string{"--tag", "a"}, 5},
		{"comma-separated tags must all match", []string{"--tag", "a,b"}, 1},
		{"repeated tags must all match", []string{"--tag", "b", "--tag", "a"}, 1},
		{"unknown tag", []string{"--tag", "nope"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runMode(t, cfg, append([]string{"--export-adif", "-"}, tt.tags...)...)
			require.Equal(t, 0, r.code, r.stderr)
			f, err := adif.Parse(strings.NewReader(r.stdout))
			require.NoError(t, err)
			assert.Len(t, f.Records, tt.want)
		})
	}
}

func TestImportErrorsImportNothing(t *testing.T) {
	good := "<CALL:4>K1AB<QSO_DATE:8>20260601<TIME_ON:4>1200<EOR>\n"
	tests := []struct {
		name       string
		content    string
		wantStderr []string
	}{
		{"missing call", good + "<QSO_DATE:8>20260601<TIME_ON:4>1300<EOR>\n", []string{"record 2", "CALL"}},
		{"bad frequency", good + "<CALL:4>K1CD<QSO_DATE:8>20260601<TIME_ON:4>1300<FREQ:3>abc<EOR>\n", []string{"record 2", "FREQ", `"abc"`}},
		{"malformed tag", good + "<CALL:x>K1CD<EOR>\n", []string{"record 2"}},
		{"truncated record", good + "<CALL:4>K1CD", []string{"record 2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := logbookConfig(t)
			r := runMode(t, cfg, "--import-adif", writeADIFFile(t, tt.content))
			assert.Equal(t, 1, r.code)
			assert.Contains(t, r.stderr, "jstlogger: ")
			for _, want := range tt.wantStderr {
				assert.Contains(t, r.stderr, want)
			}
			assert.NotContains(t, r.stderr, "imported")
			assert.Empty(t, r.stdout)

			r = runMode(t, cfg, "--export-adif", "-")
			require.Equal(t, 0, r.code, r.stderr)
			assert.Equal(t, "jstlogger: exported 0 QSOs to stdout\n", r.stderr, "nothing was imported")
		})
	}
}

func TestImportMissingFile(t *testing.T) {
	cfg := logbookConfig(t)
	missing := filepath.Join(t.TempDir(), "missing.adi")
	r := runMode(t, cfg, "--import-adif", missing)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, missing)
}

func TestADIFModeLogbookOpenError(t *testing.T) {
	p := writeConfig(t, validConfig)
	data := t.TempDir()
	// A file where the jstlogger directory should be makes the logbook unopenable.
	require.NoError(t, os.WriteFile(filepath.Join(data, "jstlogger"), nil, 0o644))
	vars := map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_DATA_HOME": data}

	r := invoke(t, vars, nil, "-c", p, "--export-adif", "-")
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.stderr, "opening logbook")
	assert.Empty(t, r.stdout)
	assert.False(t, r.tuiCalled)
}
