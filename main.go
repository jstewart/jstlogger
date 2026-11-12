// Command jstlogger is a terminal amateur radio logger.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/pflag"

	"jstlogger/internal/browser"
	"jstlogger/internal/config"
	"jstlogger/internal/eqsl"
	"jstlogger/internal/indexfmt"
	"jstlogger/internal/logbook"
	"jstlogger/internal/lotw"
	"jstlogger/internal/qsoentry"
	"jstlogger/internal/rig"
	"jstlogger/internal/tui"
)

// version is the program version, overridden at build time with
// -ldflags "-X main.version=...".
var version = "dev"

// runTUI starts the terminal UI on store, the rig when the session enables
// one, and background syncs of the LoTW activity feed and the eQSL AG member
// list when store is a SQLite logbook, and blocks until the UI exits. Rig
// events and the syncs' outcomes are logged to logger and sent to the UI.
// Tests replace it so the TUI never starts.
var runTUI = func(ctx context.Context, session config.Session, store logbook.Store, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	format, err := indexfmt.Parse(session.Config.Index.Format)
	if err != nil {
		return fmt.Errorf("index format: %w", err)
	}
	tuiOpts := []tui.Option{
		tui.WithLogbook(store),
		tui.WithVersion(version),
		tui.WithIndexFormat(format, indexfmt.Unit(session.Config.Logbook.DistanceUnit)),
		tui.WithPagerIndexLines(session.Config.Index.PagerIndexLines),
		tui.WithStationName(func(callsign, grid string) string {
			return config.StationName(session.Config.Stations, callsign, grid)
		}),
		tui.WithURLOpener(browser.Open),
		tui.WithStations(stationProfiles(session)),
	}

	var syncer *lotw.Syncer
	var eqslSyncer *eqsl.Syncer
	if db, ok := store.(*logbook.SQLiteStore); ok {
		users := lotw.NewStore(db.DB())
		syncer = newLoTWSyncer(users)
		members := eqsl.NewStore(db.DB())
		eqslSyncer = newEQSLSyncer(members)
		tuiOpts = append(tuiOpts, tui.WithLoTW(users), tui.WithEQSL(members))
	}
	var r *rig.Rig
	if opts, ok := rigOptions(session); ok {
		r = rig.New(opts)
		tuiOpts = append(tuiOpts, tui.WithRig(session.RigProfileName, r))
	}

	p := tea.NewProgram(tui.New(tuiOpts...), tea.WithContext(ctx))
	// The background work stops, and is waited for, before the caller
	// closes the store.
	var wg sync.WaitGroup
	if r != nil {
		emit := func(event any) {
			logEvent(logger, event)
			p.Send(event)
		}
		wg.Go(func() {
			if err := r.Run(ctx, emit); err != nil {
				// Rig control failed to start; the logger keeps running without it.
				emit(rig.ConnectionChanged{Status: rig.Disconnected, Err: err})
			}
		})
	}
	if syncer != nil {
		wg.Go(func() { syncLoTWInBackground(ctx, syncer, logger, p.Send) })
	}
	if eqslSyncer != nil {
		wg.Go(func() { syncEQSLInBackground(ctx, eqslSyncer, logger, p.Send) })
	}
	_, err = p.Run()
	cancel()
	wg.Wait()
	return err
}

// stationProfiles returns the station profiles the New QSO screen picks
// from: the session's station, with its overrides, then the other
// configured stations in key order. A guest --operator operates every
// profile; otherwise each is operated by its own callsign.
func stationProfiles(session config.Session) []qsoentry.Station {
	guest := !strings.EqualFold(session.Operator, session.Station.Callsign)
	profile := func(st config.Station) qsoentry.Station {
		operator := st.Callsign
		if guest {
			operator = session.Operator
		}
		return qsoentry.Station{
			Name:     st.Name,
			Callsign: st.Callsign,
			Operator: operator,
			Grid:     st.Gridsquare,
			State:    st.State,
			City:     st.City,
			POTA:     slices.Clone(st.POTAReferences),
			SOTA:     st.SOTAReference,
			IOTA:     st.IOTAReference,
			WWFF:     st.WWFFReference,
		}
	}
	profiles := []qsoentry.Station{profile(session.Station)}
	for _, key := range slices.Sorted(maps.Keys(session.Config.Stations)) {
		if key != session.StationName {
			profiles = append(profiles, profile(session.Config.Stations[key]))
		}
	}
	return profiles
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
	importADIF         string
	exportADIF         string
	tags               []string
	syncLoTW           bool
	syncEQSL           bool
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
		_, err := fmt.Fprintf(stdout, "jstlogger %s\n", version)
		return report(stderr, err)
	case opts.printDefaultConfig:
		return report(stderr, config.WriteFullExample(stdout))
	}

	path, session, ok := loadSession(opts, getenv, stderr)
	if !ok {
		return exitError
	}

	switch {
	case opts.checkConfig:
		_, err := fmt.Fprintf(stdout, "config OK: %s\n", path)
		return report(stderr, err)
	case opts.printConfig:
		return report(stderr, config.WriteTOML(stdout, session.Config))
	}

	ctx := context.Background()
	switch {
	case opts.importADIF != "" || opts.exportADIF != "":
		return report(stderr, runADIF(ctx, session, opts, stdout, stderr))
	case opts.syncLoTW:
		return report(stderr, runLoTWSync(ctx, session, stdout))
	case opts.syncEQSL:
		return report(stderr, runEQSLSync(ctx, session, stdout))
	}

	logger, closeLog := slog.New(slog.DiscardHandler), func() error { return nil }
	if opts.debug {
		var err error
		if logger, closeLog, err = openDebugLog(getenv); err != nil {
			return report(stderr, err)
		}
	}
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
	fs.StringVar(&opts.importADIF, "import-adif", "", "import the QSOs in the ADIF (.adi) file at `path` into the logbook, then exit")
	fs.StringVar(&opts.exportADIF, "export-adif", "", "export the logbook as ADIF to `path`, or to stdout for -, then exit")
	fs.StringSliceVar(&opts.tags, "tag", nil, "`tags` for --import-adif or --export-adif; repeatable or comma-separated. "+
		"On import, tags newly imported QSOs (duplicates are skipped and not tagged); "+
		"on export, exports only QSOs carrying all of these tags")
	fs.BoolVar(&opts.syncLoTW, "sync-lotw", false, "download the LoTW user activity feed into the logbook now, then exit "+
		"(the TUI refreshes it in the background when it is over a week old)")
	fs.BoolVar(&opts.syncEQSL, "sync-eqsl", false, "download the eQSL Authenticity Guaranteed member list into the logbook now, then exit "+
		"(the TUI refreshes it in the background when it is over a week old)")
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
		printStderr(stderr, "jstlogger: %v\nRun 'jstlogger --help' for usage.\n", err)
		return opts, exitUsage, false
	}
	if fs.NArg() > 0 {
		printStderr(stderr, "jstlogger: unexpected argument %q\nRun 'jstlogger --help' for usage.\n", fs.Arg(0))
		return opts, exitUsage, false
	}
	opts.overrides.OperatorSet = fs.Changed("operator")

	var modes []string
	for _, name := range []string{"check-config", "print-config", "print-default-config", "version", "import-adif", "export-adif", "sync-lotw", "sync-eqsl"} {
		if fs.Changed(name) {
			modes = append(modes, "--"+name)
		}
	}
	if len(modes) > 1 {
		printStderr(stderr, "jstlogger: %s are mutually exclusive\n", strings.Join(modes, ", "))
		return opts, exitUsage, false
	}
	for _, name := range []string{"import-adif", "export-adif"} {
		if fs.Changed(name) && fs.Lookup(name).Value.String() == "" {
			printStderr(stderr, "jstlogger: --%s requires a path\nRun 'jstlogger --help' for usage.\n", name)
			return opts, exitUsage, false
		}
	}
	if fs.Changed("tag") && !fs.Changed("import-adif") && !fs.Changed("export-adif") {
		printStderr(stderr, "jstlogger: --tag requires --import-adif or --export-adif\nRun 'jstlogger --help' for usage.\n")
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
		printStderr(stderr, "jstlogger: warning: %s: %s\n", path, w)
	}
	if res.Created {
		printStderr(stderr, "jstlogger: created %s with the default config\n", path)
	}

	if err := cfg.Validate(opts.overrides.Station); err != nil {
		printErrors(stderr, "invalid config file "+path, err)
		if res.Created {
			printStderr(stderr, "jstlogger: edit %s to set callsign and gridsquare for your station\n", path)
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
	printStderr(stderr, "jstlogger: %s:\n", header)
	for _, e := range errs {
		printStderr(stderr, "  %v\n", e)
	}
}

// printStderr writes a message to stderr. A failed write is ignored: stderr
// is where errors are reported, so there is nowhere left to report it.
func printStderr(stderr io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(stderr, format, args...)
}

// report prints a non-nil err to stderr and returns the matching exit code.
func report(stderr io.Writer, err error) int {
	if err == nil {
		return exitOK
	}
	printStderr(stderr, "jstlogger: %v\n", err)
	return exitError
}
