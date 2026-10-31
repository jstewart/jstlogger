package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"jstlogger/internal/adif"
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
	fmt.Fprintf(stderr, "jstlogger: imported %d QSOs, skipped %d duplicates\n", res.Added, res.Skipped)
	return nil
}

// exportADIF writes the QSOs in store that carry all of tags (every QSO
// when tags is empty), oldest first, as an ADI file to path, or to stdout
// when path is "-", and prints a summary to stderr. A file is written to a
// temporary file in the same directory and renamed into place, so a failed
// export never leaves a partial file; an existing file is replaced.
func exportADIF(ctx context.Context, store logbook.Store, path string, tags []string, stdout, stderr io.Writer) error {
	if path == "-" {
		n, err := writeADIF(ctx, store, tags, stdout)
		if err != nil {
			return fmt.Errorf("exporting ADIF: %w", err)
		}
		fmt.Fprintf(stderr, "jstlogger: exported %d QSOs to stdout\n", n)
		return nil
	}

	n, err := writeFileAtomic(path, func(w io.Writer) (int, error) {
		return writeADIF(ctx, store, tags, w)
	})
	if err != nil {
		return fmt.Errorf("exporting to %s: %w", path, err)
	}
	fmt.Fprintf(stderr, "jstlogger: exported %d QSOs to %s\n", n, path)
	return nil
}

// writeFileAtomic calls write with a temporary file in path's directory,
// then renames the file to path. The temporary file is removed on any
// error. It returns write's count.
func writeFileAtomic(path string, write func(io.Writer) (int, error)) (n int, err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			// The file may already be closed, so a second Close error says
			// nothing; a failed removal is reported alongside the cause.
			_ = tmp.Close()
			if rerr := os.Remove(tmp.Name()); rerr != nil {
				err = errors.Join(err, fmt.Errorf("remove temporary file: %w", rerr))
			}
		}
	}()

	if n, err = write(tmp); err != nil {
		return 0, err
	}
	// CreateTemp makes the file private; give it the mode of the file it
	// replaces, or the usual mode of a new file.
	mode := os.FileMode(0o644)
	if fi, serr := os.Stat(path); serr == nil {
		mode = fi.Mode().Perm()
	}
	if err = tmp.Chmod(mode); err != nil {
		return 0, fmt.Errorf("set temporary file mode: %w", err)
	}
	// Sync before the rename so a crash cannot leave a short file in place.
	if err = tmp.Sync(); err != nil {
		return 0, fmt.Errorf("sync temporary file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return 0, fmt.Errorf("close temporary file: %w", err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return 0, fmt.Errorf("rename temporary file: %w", err)
	}
	return n, nil
}

// writeADIF writes a header and the QSOs in store carrying all of tags to
// w, oldest first, and returns how many QSOs it wrote.
func writeADIF(ctx context.Context, store logbook.Store, tags []string, w io.Writer) (int, error) {
	aw := adif.NewWriter(w)
	err := aw.WriteHeader(adif.Header{
		Preamble: "jstlogger ADIF export",
		Fields: adif.Record{
			{Name: "ADIF_VER", Value: "3.1.6"},
			{Name: "PROGRAMID", Value: "jstlogger"},
			{Name: "PROGRAMVERSION", Value: version},
			{Name: "CREATED_TIMESTAMP", Value: exportNow().UTC().Format("20060102 150405")},
		},
	})
	if err != nil {
		return 0, err
	}

	n := 0
	err = store.Each(ctx, logbook.Filter{Tags: tags}, func(q logbook.QSO) error {
		n++
		return aw.Write(logbook.RecordFromQSO(q))
	})
	if err != nil {
		return 0, fmt.Errorf("write QSOs: %w", err)
	}
	if err := aw.Flush(); err != nil {
		return 0, err
	}
	return n, nil
}
