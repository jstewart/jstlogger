// Package browser opens URLs in the user's web browser by handing them to
// the platform's opener command (open on macOS, xdg-open on Linux and the
// BSDs). It knows nothing about the terminal UI.
package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
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

// outputDelay bounds how long run waits for the opener's output to close
// after it exits. xdg-open can start a browser that inherits its output and
// keeps it open for as long as the browser runs.
const outputDelay = time.Second

// run runs name with url as its only argument and waits for it to exit.
// The URL is passed as a single argument, never through a shell. When the
// command fails, any output it produced is included in the error.
//
// A command still running when ctx is done is left running, and run
// returns nil: xdg-open with no desktop environment runs the browser in
// the foreground, so it exits only when the browser does.
func run(ctx context.Context, name, url string) error {
	cmd := exec.Command(name, url)
	cmd.WaitDelay = outputDelay
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", url, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		return nil
	}
	if err == nil || errors.Is(err, exec.ErrWaitDelay) {
		// ErrWaitDelay means the opener succeeded but something it started
		// still holds its output.
		return nil
	}
	if msg := strings.TrimSpace(out.String()); msg != "" {
		return fmt.Errorf("open %s: %w: %s", url, err, msg)
	}
	return fmt.Errorf("open %s: %w", url, err)
}
