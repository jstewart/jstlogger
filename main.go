// Command jstlogger is a terminal amateur radio logger.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/pflag"

	"jstlogger/internal/config"
	"jstlogger/internal/logbook"
	"jstlogger/internal/rig"
	"jstlogger/internal/tui"
)

// version is the program version, overridden at build time with
// -ldflags "-X main.version=...".
var version = "dev"

// runTUI starts the terminal UI on store, and the rig when the session
// enables one, and blocks until the UI exits. Rig events are logged to logger
// and sent to the UI. Tests replace it so the TUI never starts.
var runTUI = func(ctx context.Context, session config.Session, store logbook.Store, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	opts, ok := rigOptions(session)
	if !ok {
		_, err := tea.NewProgram(tui.New(tui.WithLogbook(store)), tea.WithContext(ctx)).Run()
		return err
	}

	r := rig.New(opts)
	p := tea.NewProgram(tui.New(tui.WithLogbook(store), tui.WithRig(session.RigProfileName, r)), tea.WithContext(ctx))
	emit := func(event any) {
		logEvent(logger, event)
		p.Send(event)
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := r.Run(ctx, emit); err != nil {
			// Rig control failed to start; the logger keeps running without it.
			emit(rig.ConnectionChanged{Status: rig.Disconnected, Err: err})
		}
	})
	_, err := p.Run()
	cancel()
	wg.Wait()
	return err
}

// rigOptions returns the rig options for session, and false when the
// session has rig control disabled.
func rigOptions(session config.Session) (rig.Options, bool) {
	if !session.RigEnabled {
		return rig.Options{}, false
	}
	p := session.Config.Rig.Profiles[session.RigProfileName]
	opts := rig.Options{
		Addr:         net.JoinHostPort(p.Host, strconv.Itoa(p.Port)),
		PollInterval: session.Config.Rig.PollInterval,
	}
	if p.Spawn != nil {
		opts.Spawn = &rig.Spawn{Model: p.Spawn.Model, Device: p.Spawn.Device, Baud: p.Spawn.Baud}
	}
	return opts, true
}

// Exit codes returned by run.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// options holds the parsed command-line flags.
type options struct {
	configPath         string
	overrides          config.Overrides
	checkConfig        bool
	printConfig        bool
	printDefaultConfig bool
	version            bool
	debug              bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run is the testable body of main: it parses args, loads and validates the
// config, then either handles a mode flag or starts the TUI. It returns the
// process exit code.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	opts, code, ok := parseFlags(args, stderr)
	if !ok {
		return code
	}

	switch {
	case opts.version:
		fmt.Fprintf(stdout, "jstlogger %s\n", version)
		return exitOK
	case opts.printDefaultConfig:
		return report(stderr, config.WriteFullExample(stdout))
	}

	path, session, ok := loadSession(opts, getenv, stderr)
	if !ok {
		return exitError
	}

	switch {
	case opts.checkConfig:
		fmt.Fprintf(stdout, "config OK: %s\n", path)
		return exitOK
	case opts.printConfig:
		return report(stderr, config.WriteTOML(stdout, session.Config))
	}

	logger, closeLog := slog.New(slog.DiscardHandler), func() error { return nil }
	if opts.debug {
		var err error
		if logger, closeLog, err = openDebugLog(getenv); err != nil {
			return report(stderr, err)
		}
	}
	ctx := context.Background()
	store, err := openLogbook(ctx, session)
	if err == nil {
		err = runTUI(ctx, session, store, logger)
		if cerr := store.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("closing logbook: %w", cerr))
		}
	}
	if cerr := closeLog(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("closing debug log: %w", cerr))
	}
	return report(stderr, err)
}

// newFlagSet returns the jstlogger flag set, writing parsed values to opts
// and usage and parse errors to stderr.
func newFlagSet(opts *options, stderr io.Writer) *pflag.FlagSet {
	fs := pflag.NewFlagSet("jstlogger", pflag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.SortFlags = false
	o := &opts.overrides

	fs.StringVarP(&opts.configPath, "config", "c", "", "path to the config file")
	fs.StringVarP(&o.Station, "station", "s", "", "select a station from [stations.*], overriding default_station")
	fs.StringVar(&o.Grid, "grid", "", "gridsquare for this session")
	fs.StringVar(&o.Operator, "operator", "", "operator callsign for this session (defaults to the station's callsign)")
	fs.StringSliceVar(&o.POTA, "pota", nil, "POTA reference(s) for this session; repeatable or comma-separated")
	fs.StringVar(&o.SOTA, "sota", "", "SOTA reference for this session")
	fs.StringVar(&o.IOTA, "iota", "", "IOTA reference for this session")
	fs.StringVar(&o.WWFF, "wwff", "", "WWFF reference for this session")
	fs.StringVar(&o.Rig, "rig", "", "select a rig profile from [rig.profiles.*]")
	fs.BoolVar(&o.NoRig, "no-rig", false, "disable rig control for this session")
	fs.BoolVar(&opts.checkConfig, "check-config", false, "validate the config, report any problems, then exit")
	fs.BoolVar(&opts.printConfig, "print-config", false, "print the effective config as TOML and exit")
	fs.BoolVar(&opts.printDefaultConfig, "print-default-config", false, "print the full commented default config and exit")
	fs.BoolVar(&opts.version, "version", false, "print the version and exit")
	fs.BoolVar(&opts.debug, "debug", false, "write a debug log to $XDG_STATE_HOME/jstlogger/debug.log")
	return fs
}

// parseFlags parses args. When ok is false the caller should exit with code:
// 0 after --help, exitUsage after a parse or flag-combination error.
func parseFlags(args []string, stderr io.Writer) (opts options, code int, ok bool) {
	fs := newFlagSet(&opts, stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return opts, exitOK, false
		}
		fmt.Fprintf(stderr, "jstlogger: %v\nRun 'jstlogger --help' for usage.\n", err)
		return opts, exitUsage, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "jstlogger: unexpected argument %q\nRun 'jstlogger --help' for usage.\n", fs.Arg(0))
		return opts, exitUsage, false
	}
	opts.overrides.OperatorSet = fs.Changed("operator")

	var modes []string
	for _, name := range []string{"check-config", "print-config", "print-default-config", "version"} {
		if fs.Changed(name) {
			modes = append(modes, "--"+name)
		}
	}
	if len(modes) > 1 {
		fmt.Fprintf(stderr, "jstlogger: %s are mutually exclusive\n", strings.Join(modes, ", "))
		return opts, exitUsage, false
	}
	return opts, exitOK, true
}

// loadSession finds, loads and validates the config, applies the session
// overrides and fills in the default logbook path, reporting warnings and
// errors to stderr. It returns the config path and the resolved session; ok
// is false on any error.
func loadSession(opts options, getenv func(string) string, stderr io.Writer) (string, config.Session, bool) {
	path, explicit := opts.configPath, opts.configPath != ""
	if !explicit {
		p, err := config.DefaultPath(getenv)
		if err != nil {
			report(stderr, fmt.Errorf("locating config file: %w", err))
			return "", config.Session{}, false
		}
		path = p
	}

	cfg, res, err := config.Load(path, explicit)
	if err != nil {
		report(stderr, fmt.Errorf("loading config: %w", err))
		return path, config.Session{}, false
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "jstlogger: warning: %s: %s\n", path, w)
	}
	if res.Created {
		fmt.Fprintf(stderr, "jstlogger: created %s with the default config\n", path)
	}

	if err := cfg.Validate(opts.overrides.Station); err != nil {
		printErrors(stderr, "invalid config file "+path, err)
		if res.Created {
			fmt.Fprintf(stderr, "jstlogger: edit %s to set callsign and gridsquare for your station\n", path)
		}
		return path, config.Session{}, false
	}

	session, err := config.Resolve(cfg, opts.overrides)
	if err != nil {
		printErrors(stderr, "invalid flags", err)
		return path, config.Session{}, false
	}

	if session.Config.Logbook.Path == "" {
		p, err := config.LogbookPath(getenv)
		if err != nil {
			report(stderr, fmt.Errorf("locating logbook: %w", err))
			return path, config.Session{}, false
		}
		session.Config.Logbook.Path = p
	}
	return path, session, true
}

// printErrors writes a "jstlogger: <header>:" line to stderr, followed by
// each error joined in err on its own indented line.
func printErrors(stderr io.Writer, header string, err error) {
	errs := []error{err}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		errs = joined.Unwrap()
	}
	fmt.Fprintf(stderr, "jstlogger: %s:\n", header)
	for _, e := range errs {
		fmt.Fprintf(stderr, "  %v\n", e)
	}
}

// report prints a non-nil err to stderr and returns the matching exit code.
func report(stderr io.Writer, err error) int {
	if err == nil {
		return exitOK
	}
	fmt.Fprintf(stderr, "jstlogger: %v\n", err)
	return exitError
}
