package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"jstlogger/internal/adif"
	"jstlogger/internal/adifexport"
	"jstlogger/internal/config"
	"jstlogger/internal/logbook"
)

// exportNow returns the time stamped in an export's header. Tests replace
// it.
var exportNow = time.Now

// runADIF opens the session's logbook, runs the --import-adif or
// --export-adif mode selected in opts on it, and closes it.
func runADIF(ctx context.Context, session config.Session, opts options, stdout, stderr io.Writer) error {
	store, err := openLogbook(ctx, session)
	if err != nil {
		return err
	}
	if opts.importADIF != "" {
		err = importADIF(ctx, store, opts.importADIF, opts.tags, stderr)
	} else {
		err = exportADIF(ctx, store, opts.exportADIF, opts.tags, stdout, stderr)
	}
	if cerr := store.Close(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("closing logbook: %w", cerr))
	}
	return err
}

// importADIF imports every record in the ADI file at path into store,
// applying tags to the QSOs it adds, and prints a summary to stderr. If any
// record fails to parse or convert, nothing is imported.
func importADIF(ctx context.Context, store logbook.Store, path string, tags []string, stderr io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("importing ADIF: %w", err)
	}
	file, err := adif.Parse(f)
	if cerr := f.Close(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("close: %w", cerr))
	}
	if err != nil {
		return fmt.Errorf("importing %s: %w", path, err)
	}

	qsos := make([]logbook.QSO, len(file.Records))
	for i, r := range file.Records {
		if qsos[i], err = logbook.QSOFromRecord(r); err != nil {
			return fmt.Errorf("importing %s: record %d: %w", path, i+1, err)
		}
	}
	res, err := store.Import(ctx, qsos, tags)
	if err != nil {
		return fmt.Errorf("importing %s: %w", path, err)
	}
	printStderr(stderr, "jstlogger: imported %d QSOs, skipped %d duplicates\n", res.Added, res.Skipped)
	return nil
}

// exportADIF writes the QSOs in store that carry all of tags (every QSO
// when tags is empty), oldest first, as an ADI file to path, or to stdout
// when path is "-", and prints a summary to stderr. A file is written to a
// temporary file in the same directory and renamed into place, so a failed
// export never leaves a partial file; an existing file is replaced.
func exportADIF(ctx context.Context, store logbook.Store, path string, tags []string, stdout, stderr io.Writer) error {
	opts := adifexport.Options{Filter: logbook.Filter{Tags: tags}, Version: version, Now: exportNow}
	if path == "-" {
		n, err := adifexport.Write(ctx, store, stdout, opts)
		if err != nil {
			return fmt.Errorf("exporting ADIF: %w", err)
		}
		printStderr(stderr, "jstlogger: exported %d QSOs to stdout\n", n)
		return nil
	}

	n, err := adifexport.WriteFile(ctx, store, path, opts)
	if err != nil {
		return fmt.Errorf("exporting to %s: %w", path, err)
	}
	printStderr(stderr, "jstlogger: exported %d QSOs to %s\n", n, path)
	return nil
}
