// Package adifexport writes logbook QSOs as ADI files.
package adifexport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"jstlogger/internal/adif"
	"jstlogger/internal/logbook"
)

// Source supplies the QSOs to export. logbook.Store satisfies it.
type Source interface {
	// Each calls fn for every QSO matching f, oldest first.
	Each(ctx context.Context, f logbook.Filter, fn func(logbook.QSO) error) error
}

// Options configures an export.
type Options struct {
	// Filter selects the QSOs to export.
	Filter logbook.Filter
	// Version is the program version written to the header.
	Version string
	// Now returns the header's creation time; nil means time.Now.
	Now func() time.Time
}

// Write writes an ADI header and the QSOs in src matching opts.Filter,
// oldest first, to w, and returns how many QSOs it wrote.
func Write(ctx context.Context, src Source, w io.Writer, opts Options) (int, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	aw := adif.NewWriter(w)
	err := aw.WriteHeader(adif.Header{
		Preamble: "jstlogger ADIF export",
		Fields: adif.Record{
			{Name: "ADIF_VER", Value: "3.1.6"},
			{Name: "PROGRAMID", Value: "jstlogger"},
			{Name: "PROGRAMVERSION", Value: opts.Version},
			{Name: "CREATED_TIMESTAMP", Value: now().UTC().Format("20060102 150405")},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("write header: %w", err)
	}

	n := 0
	err = src.Each(ctx, opts.Filter, func(q logbook.QSO) error {
		n++
		return aw.Write(logbook.RecordFromQSO(q))
	})
	if err != nil {
		return 0, fmt.Errorf("write QSOs: %w", err)
	}
	if err := aw.Flush(); err != nil {
		return 0, fmt.Errorf("flush: %w", err)
	}
	return n, nil
}

// WriteFile exports like Write to the file at path, through a temporary
// file in the same directory renamed into place, so a failed export never
// leaves a partial file. An existing file is replaced, keeping its mode; a
// new file gets mode 0644.
func WriteFile(ctx context.Context, src Source, path string, opts Options) (int, error) {
	return writeFileAtomic(path, func(w io.Writer) (int, error) {
		return Write(ctx, src, w, opts)
	})
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
