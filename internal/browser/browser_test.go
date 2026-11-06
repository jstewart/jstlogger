package browser

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommand(t *testing.T) {
	tests := []struct {
		goos        string
		want        string
		unsupported bool
	}{
		{goos: "darwin", want: "open"},
		{goos: "linux", want: "xdg-open"},
		{goos: "freebsd", want: "xdg-open"},
		{goos: "netbsd", want: "xdg-open"},
		{goos: "openbsd", want: "xdg-open"},
		{goos: "windows", unsupported: true},
		{goos: "plan9", unsupported: true},
		{goos: "", unsupported: true},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			got, err := command(tt.goos)
			if tt.unsupported {
				require.Error(t, err)
				assert.ErrorIs(t, err, errors.ErrUnsupported)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenOnUnsupportedGOOS(t *testing.T) {
	err := openOn(context.Background(), "windows", "https://example.com")
	require.Error(t, err)
	assert.ErrorIs(t, err, errors.ErrUnsupported)
	assert.Contains(t, err.Error(), "https://example.com")
}

func TestRunMissingCommand(t *testing.T) {
	name := filepath.Join(t.TempDir(), "no-such-browser")
	err := run(context.Background(), name, "https://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open https://example.com")
	var execErr *exec.Error
	var pathErr *os.PathError
	assert.True(t, errors.As(err, &execErr) || errors.As(err, &pathErr),
		"want an exec or path error, got %T: %v", err, err)
}

func TestRunFailingCommandIncludesOutput(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-open")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho \"cannot open $1\" >&2\nexit 3\n"), 0o700))

	err := run(context.Background(), script, "https://example.com")
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 3, exitErr.ExitCode())
	assert.Contains(t, err.Error(), "open https://example.com")
	assert.Contains(t, err.Error(), "cannot open https://example.com")
}

func TestRunSucceeds(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-open")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700))

	assert.NoError(t, run(context.Background(), script, "https://example.com"))
}

// TestRunDoesNotWaitForBrowser covers an opener such as xdg-open that
// exits after starting a browser which keeps the opener's output open.
func TestRunDoesNotWaitForBrowser(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-open")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nsleep 30 &\nexit 0\n"), 0o700))

	start := time.Now()
	assert.NoError(t, run(context.Background(), script, "https://example.com"))
	assert.Less(t, time.Since(start), 10*time.Second)
}

// TestRunForegroundBrowser covers an opener such as xdg-open with no
// desktop environment, which runs the browser in the foreground and so
// outlives ctx.
func TestRunForegroundBrowser(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-open")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\nexit 0\n"), 0o700))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	assert.NoError(t, run(ctx, script, "https://example.com"))
	assert.Less(t, time.Since(start), 10*time.Second)
}
