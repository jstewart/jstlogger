package adif

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	// ErrTruncated is returned (wrapped in a *ParseError) when the input ends
	// inside a record, before its closing <EOR>.
	ErrTruncated = errors.New("truncated record: missing <EOR>")
	// ErrMissingEOH is returned (wrapped in a *ParseError) when a file has a
	// header but no <EOH> tag ending it.
	ErrMissingEOH = errors.New("header has no <EOH>")
)

// Field is one ADIF data specifier.
type Field struct {
	Name  string // upper-case ADIF field name
	Value string // raw value, exactly as read (no trimming)
}

// Record is an ordered list of fields: one QSO, or the header's fields.
type Record []Field

// Get returns the value of the named field and whether it is present. The
// name is matched case-insensitively. A zero-length field is present with an
// empty value.
func (r Record) Get(name string) (string, bool) {
	for _, f := range r {
		if strings.EqualFold(f.Name, name) {
			return f.Value, true
		}
	}
	return "", false
}

// Header is an ADIF file header.
type Header struct {
	Preamble string // free text before the first field ("" if none)
	Fields   Record
}

// File is a parsed ADI file.
type File struct {
	Header  *Header // nil when the file has no header
	Records []Record
}

// ParseError reports a malformed header or record. Use errors.As to inspect
// it; Err holds the underlying reason.
type ParseError struct {
	Record int    // 1-based record number; 0 for the header
	Field  string // field name if known, else ""
	Err    error
}

// Error formats the error as "adif: record 12, field FREQ: <reason>", leaving
// out the parts that are unknown.
func (e *ParseError) Error() string {
	var b strings.Builder
	b.WriteString("adif: ")
	if e.Record == 0 {
		b.WriteString("header")
	} else {
		b.WriteString("record ")
		b.WriteString(strconv.Itoa(e.Record))
	}
	if e.Field != "" {
		fmt.Fprintf(&b, ", field %s", e.Field)
	}
	fmt.Fprintf(&b, ": %v", e.Err)
	return b.String()
}

// Unwrap returns the underlying reason, for errors.Is and errors.As.
func (e *ParseError) Unwrap() error { return e.Err }
