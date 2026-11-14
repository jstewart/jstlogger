package editor

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{name: "single word", in: "vi", want: []string{"vi"}},
		{name: "words", in: "code --wait", want: []string{"code", "--wait"}},
		{name: "extra whitespace", in: "  nvim \t -u\nNONE  ", want: []string{"nvim", "-u", "NONE"}},
		{name: "single quotes literal", in: `'my editor' '\"$x'`, want: []string{"my editor", `\"$x`}},
		{name: "double quotes", in: `"my editor" -c "set ft=txt"`, want: []string{"my editor", "-c", "set ft=txt"}},
		{name: "double quote escapes", in: `"a\"b\\c\$d` + "\\`e" + `"`, want: []string{"a\"b\\c$d`e"}},
		{name: "double quote keeps other backslashes", in: `"a\nb"`, want: []string{`a\nb`}},
		{name: "backslash outside quotes", in: `my\ editor \'x`, want: []string{"my editor", "'x"}},
		{name: "adjacent quoted parts join", in: `a'b c'"d e"f`, want: []string{"ab cd ef"}},
		{name: "empty quotes make a word", in: `vi ''`, want: []string{"vi", ""}},
		{name: "unterminated single quote", in: `vi 'oops`, wantErr: true},
		{name: "unterminated double quote", in: `vi "oops`, wantErr: true},
		{name: "trailing backslash", in: `vi \`, wantErr: true},
		{name: "empty", in: "", wantErr: true},
		{name: "only whitespace", in: " \t\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Split(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCommand(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    []string
		wantErr bool
	}{
		{name: "visual wins", env: map[string]string{"VISUAL": "code --wait", "EDITOR": "nano"}, want: []string{"code", "--wait"}},
		{name: "editor when visual unset", env: map[string]string{"EDITOR": "nano"}, want: []string{"nano"}},
		{name: "blank visual is unset", env: map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, want: []string{"nano"}},
		{name: "vi fallback", env: map[string]string{}, want: []string{"vi"}},
		{name: "blank both falls back to vi", env: map[string]string{"VISUAL": "\t", "EDITOR": " "}, want: []string{"vi"}},
		{name: "unsplittable", env: map[string]string{"EDITOR": `vim "oops`}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			got, err := Command(getenv)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "EDITOR")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSingleLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "nice signal", want: "nice signal"},
		{name: "trailing newline", in: "nice signal\n", want: "nice signal"},
		{name: "newlines become spaces", in: "line one\nline two\r\nline three", want: "line one line two line three"},
		{name: "runs collapse", in: "  a \t\t b   c  ", want: "a b c"},
		{name: "empty", in: "\n\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SingleLine(tt.in))
		})
	}
}

func TestMultiLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "hello", want: "hello"},
		{name: "trailing newline trimmed", in: "hello\n", want: "hello"},
		{name: "several trailing newlines trimmed", in: "hello\n\n\n", want: "hello"},
		{name: "crlf normalized", in: "a\r\nb\r\n", want: "a\nb"},
		{name: "lone cr normalized", in: "a\rb", want: "a\nb"},
		{name: "inner blank lines and indentation kept", in: "  a\n\n  b  \n", want: "  a\n\n  b  "},
		{name: "empty", in: "\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MultiLine(tt.in))
		})
	}
}

func TestPrepareReadBack(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	value := "first line\nsecond line"
	cmd, path, err := Prepare([]string{"code", "--wait"}, value)
	require.NoError(t, err)
	require.NotNil(t, cmd)

	assert.Equal(t, dir, filepath.Dir(path))
	assert.Equal(t, []string{"code", "--wait", path}, cmd.Args)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, value, string(written))

	// Simulate the user editing the file.
	require.NoError(t, os.WriteFile(path, []byte("edited\r\n"), 0o600))

	got, err := ReadBack(path)
	require.NoError(t, err)
	assert.Equal(t, "edited\r\n", got)

	_, err = os.Stat(path)
	assert.True(t, errors.Is(err, fs.ErrNotExist), "temp file should be removed, got %v", err)
}

func TestPrepareNoCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)

	cmd, path, err := Prepare(nil, "x")
	require.Error(t, err)
	assert.Nil(t, cmd)
	assert.Empty(t, path)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no temp file should be left behind")
}

func TestPrepareTempDirMissing(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))

	cmd, path, err := Prepare([]string{"vi"}, "x")
	require.Error(t, err)
	assert.Nil(t, cmd)
	assert.Empty(t, path)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestReadBackMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone.txt")
	got, err := ReadBack(path)
	require.Error(t, err)
	assert.Empty(t, got)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.Contains(t, err.Error(), path)
}
