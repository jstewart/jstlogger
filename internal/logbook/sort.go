package logbook

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidSort is returned (possibly wrapped) when a sort name or Sort is
// not one the store knows.
var ErrInvalidSort = errors.New("logbook: invalid sort")

// SortField is a QSO field the index can be sorted by.
type SortField int

// The fields the index can be sorted by. SortDate, the zero value, sorts by
// start time.
const (
	SortDate SortField = iota
	SortCall
	SortBand
	SortMode
	SortCountry
	SortState
	SortContinent
)

// sortFieldNames holds each SortField's name, indexed by the field.
var sortFieldNames = []string{"date", "call", "band", "mode", "country", "state", "continent"}

// SortFields returns every SortField, in the order they are documented.
func SortFields() []SortField {
	out := make([]SortField, len(sortFieldNames))
	for i := range out {
		out[i] = SortField(i)
	}
	return out
}

// String returns the field's name as ParseSort accepts it, e.g. "band".
func (f SortField) String() string {
	if f < 0 || int(f) >= len(sortFieldNames) {
		return "SortField(" + strconv.Itoa(int(f)) + ")"
	}
	return sortFieldNames[f]
}

// Sort is an ordering of the QSO index. Like every field, date sorts
// ascending, oldest first, so the zero Sort is oldest first; the index
// shows DefaultSort, newest first, unless told otherwise.
//
// Sorting by date orders by start time, oldest first, then by ID, lowest
// first; Reverse turns it exactly around. Every other field sorts by its
// value, A to Z (bands from low frequency to high, modes by the submode
// when there is one), or Z to A when Reverse is set. Empty values come
// last either way, and QSOs with the same value are always newest first,
// then highest ID first.
type Sort struct {
	Field   SortField
	Reverse bool
}

// DefaultSort is the index's usual order: by date, newest first.
var DefaultSort = Sort{Field: SortDate, Reverse: true}

// reversePrefix marks a reversed sort in a sort name, as in mutt.
const reversePrefix = "reverse-"

// ParseSort parses a sort name: a field name such as "band", optionally
// prefixed with "reverse-", ignoring case and surrounding space. It returns
// an error wrapping ErrInvalidSort for an unknown name.
func ParseSort(s string) (Sort, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	var out Sort
	if rest, ok := strings.CutPrefix(name, reversePrefix); ok {
		name, out.Reverse = rest, true
	}
	for i, n := range sortFieldNames {
		if n == name {
			out.Field = SortField(i)
			return out, nil
		}
	}
	return Sort{}, fmt.Errorf("%w: %q", ErrInvalidSort, s)
}

// String returns the sort's name as ParseSort accepts it, e.g. "band" or
// "reverse-band".
func (s Sort) String() string {
	if s.Reverse {
		return reversePrefix + s.Field.String()
	}
	return s.Field.String()
}

// sortTerm is one term of an ORDER BY: an SQL expression over the qso table
// and whether it sorts descending.
type sortTerm struct {
	expr string
	desc bool
}

// modeName is the SQL expression for the name a QSO's mode is shown by: its
// submode when it has one.
const modeName = "CASE WHEN submode <> '' THEN submode ELSE mode END"

// terms returns the ORDER BY terms of s. Every ordering ends in id, so it
// is total and a row's term values place it exactly. It returns an error
// wrapping ErrInvalidSort for an unknown field.
//
// The terms match the indexes of migration 0006 (and qso_by_start for
// date), so SQLite reads rows in index order instead of sorting them.
func (s Sort) terms() ([]sortTerm, error) {
	if s.Field == SortDate {
		// Both directions match the qso_by_start index.
		return []sortTerm{{"start_utc", s.Reverse}, {"id", s.Reverse}}, nil
	}
	// value is the field's value, which is empty when the field is; key is
	// what it sorts by.
	var value, key string
	switch s.Field {
	case SortCall:
		value, key = "call", "call"
	case SortBand:
		value, key = "band", bandOrder()
	case SortMode:
		value, key = modeName, modeName
	case SortCountry:
		value, key = "country", "country COLLATE NOCASE"
	case SortState:
		value, key = "state", "state COLLATE NOCASE"
	case SortContinent:
		value, key = "cont", "cont"
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidSort, s.Field)
	}
	return []sortTerm{
		{"(" + value + " = '')", false}, // Empty values last.
		{key, s.Reverse},
		{"start_utc", true},
		{"id", true},
	}, nil
}

// bandOrder returns an SQL expression giving a band's position in bands, so
// that bands sort by frequency; any other band, empty included, follows them
// all. Bands are stored in lower case, as bands names them.
func bandOrder() string {
	var b strings.Builder
	b.WriteString("CASE band")
	for i, bd := range bands {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", strings.ReplaceAll(bd.name, "'", "''"), i)
	}
	fmt.Fprintf(&b, " ELSE %d END", len(bands))
	return b.String()
}

// orderBy returns the ORDER BY clause, without the keywords, for terms, or
// for terms each turned around when reverse is true.
func orderBy(terms []sortTerm, reverse bool) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		dir := " ASC"
		if t.desc != reverse {
			dir = " DESC"
		}
		parts[i] = t.expr + dir
	}
	return strings.Join(parts, ", ")
}

// selectTerms returns the select list of terms' expressions, for reading a
// row's term values.
func selectTerms(terms []sortTerm) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = t.expr
	}
	return strings.Join(parts, ", ")
}

// keysetLevels returns the conditions that place a row strictly before
// (before is true) or after the row whose term values are vals, in the order
// of terms, one for each term, with their arguments. Condition i holds when
// the row ties on the terms before term i and is on the wanted side of it:
// t1 = v1 AND ... AND ti < vi, with > for a descending term when before, and
// the other way around when after. The conditions are disjoint, and a row
// is on the wanted side exactly when one of them holds.
//
// Each condition is an equality prefix and a range on an index that
// matches terms, so SQLite can count its rows within the index.
func keysetLevels(terms []sortTerm, vals []any, before bool) ([]string, [][]any) {
	conds := make([]string, len(terms))
	args := make([][]any, len(terms))
	for i, t := range terms {
		var parts []string
		for j := range i {
			parts = append(parts, terms[j].expr+" = ?")
			args[i] = append(args[i], vals[j])
		}
		op := " > "
		if t.desc != before {
			op = " < "
		}
		parts = append(parts, t.expr+op+"?")
		args[i] = append(args[i], vals[i])
		conds[i] = strings.Join(parts, " AND ")
	}
	return conds, args
}

// keysetCond returns a condition that holds for rows strictly before
// (before is true) or after the row whose term values are vals, in the
// order of terms, and its arguments: the disjunction of keysetLevels.
func keysetCond(terms []sortTerm, vals []any, before bool) (string, []any) {
	conds, levelArgs := keysetLevels(terms, vals, before)
	var args []any
	for i := range conds {
		conds[i] = "(" + conds[i] + ")"
		args = append(args, levelArgs[i]...)
	}
	return "(" + strings.Join(conds, " OR ") + ")", args
}
