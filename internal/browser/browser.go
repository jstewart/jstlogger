// Package browser opens URLs in the user's web browser by handing them to
// the platform's opener command (open on macOS, xdg-open on Linux and the
// BSDs). It knows nothing about the terminal UI.
package browser

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Open opens url in the user's browser. It returns an error wrapping
// errors.ErrUnsupported when the platform has no known opener command.
func Open(ctx context.Context, url string) error {
	return openOn(ctx, runtime.GOOS, url)
}

// openOn opens url with the opener command for goos.
func openOn(ctx context.Context, goos, url string) error {
	name, err := command(goos)
	if err != nil {
		return fmt.Errorf("open %s: %w", url, err)
	}
	return run(ctx, name, url)
}

// command returns the name of the command that opens a URL on goos.
func command(goos string) (string, error) {
	switch goos {
	case "darwin":
		return "open", nil
	case "linux", "freebsd", "netbsd", "openbsd":
		return "xdg-open", nil
	default:
		return "", fmt.Errorf("open a browser on %s: %w", goos, errors.ErrUnsupported)
	}
}

// run runs name with url as its only argument and waits for it to exit.
// The URL is passed as a single argument, never through a shell. When the
// command fails, any output it produced is included in the error.
func run(ctx context.Context, name, url string) error {
	out, err := exec.CommandContext(ctx, name, url).CombinedOutput()
	if err == nil {
		return nil
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("open %s: %w: %s", url, err, msg)
	}
	return fmt.Errorf("open %s: %w", url, err)
}
