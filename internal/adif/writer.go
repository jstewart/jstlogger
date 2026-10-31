package adif

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Writer writes ADI files. Output is buffered; call Flush when done.
type Writer struct {
	w *bufio.Writer
}

// NewWriter returns a Writer that writes to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: bufio.NewWriter(w)}
}

// WriteHeader writes the file header: the preamble on its own line, the
// header fields, then <EOH> and a blank line. It must be called before any
// Write. An empty preamble is written as a blank line, because a header is
// recognized only when the file does not start with '<'.
func (w *Writer) WriteHeader(h Header) error {
	w.w.WriteString(h.Preamble)
	w.w.WriteString("\n")
	w.fields(h.Fields)
	if _, err := w.w.WriteString("<EOH>\n\n"); err != nil {
		return fmt.Errorf("adif: write header: %w", err)
	}
	return nil
}

// Write writes one record, one field per line followed by <EOR> and a blank
// line. Fields with an empty value are skipped.
func (w *Writer) Write(r Record) error {
	w.fields(r)
	if _, err := w.w.WriteString("<EOR>\n\n"); err != nil {
		return fmt.Errorf("adif: write record: %w", err)
	}
	return nil
}

// Flush writes any buffered data to the underlying io.Writer.
func (w *Writer) Flush() error {
	if err := w.w.Flush(); err != nil {
		return fmt.Errorf("adif: flush: %w", err)
	}
	return nil
}

// fields writes each non-empty field as <NAME:len>value on its own line.
// bufio.Writer errors are sticky, so callers check the last write only.
func (w *Writer) fields(r Record) {
	for _, f := range r {
		if f.Value == "" {
			continue
		}
		fmt.Fprintf(w.w, "<%s:%d>%s\n", strings.ToUpper(f.Name), len(f.Value), f.Value)
	}
}
