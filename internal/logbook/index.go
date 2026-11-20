package logbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Index returns up to limit QSOs matching e, in sort's order, starting at
// row offset, where 0 is the first. DefaultSort is newest first (StartUTC
// descending, then ID descending). Only the qso row is loaded: Fields,
// Confirmations and Tags are nil. A limit of zero or less returns nil, and
// limits above MaxPageSize are capped. It returns an error wrapping
// ErrInvalidFilter if a Filter in e is invalid, or ErrInvalidSort if sort
// is. A nil e matches every QSO.
//
// Unlike List, Index can start anywhere in the ordering, so a caller can
// show any range of a large logbook without reading the rows before it.
func (s *SQLiteStore) Index(ctx context.Context, e Expr, sort Sort, offset, limit int) ([]QSO, error) {
	qs, err := s.index(ctx, e, sort, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("logbook: index qsos: %w", err)
	}
	return qs, nil
}

func (s *SQLiteStore) index(ctx context.Context, e Expr, sort Sort, offset, limit int) (qs []QSO, err error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative offset %d", offset)
	}
	if limit <= 0 {
		return nil, nil
	}
	query, args, err := indexQuery(e, sort, offset, min(limit, MaxPageSize))
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query qsos: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		q, err := scanQSO(rows)
		if err != nil {
			return nil, err
		}
		qs = append(qs, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query qsos: %w", err)
	}
	return qs, nil
}

// indexQuery returns the query Index runs and its arguments. The ORDER BY
// matches an index (qso_by_start for date), so SQLite skips offset rows of
// the index instead of sorting the table.
func indexQuery(e Expr, sort Sort, offset, limit int) (string, []any, error) {
	terms, err := sort.terms()
	if err != nil {
		return "", nil, err
	}
	conds, args, err := exprConds(e)
	if err != nil {
		return "", nil, err
	}
	query := "SELECT " + qsoColumns + " FROM qso" + where(conds) +
		" ORDER BY " + orderBy(terms, false) + " LIMIT ? OFFSET ?"
	return query, append(args, limit, offset), nil
}

// IndexOf returns the row number of the QSO with the given ID among the QSOs
// matching e, in sort's order as Index uses it, so that
// Index(ctx, e, sort, n, 1) returns it. It returns ErrNotFound if the QSO
// does not exist or does not match e, an error wrapping ErrInvalidFilter if
// a Filter in e is invalid, and one wrapping ErrInvalidSort if sort is. A
// nil e matches every QSO.
func (s *SQLiteStore) IndexOf(ctx context.Context, e Expr, sort Sort, id int64) (int, error) {
	n, err := s.indexOf(ctx, e, sort, id)
	if errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("logbook: index of qso %d: %w", id, err)
	}
	return n, nil
}

func (s *SQLiteStore) indexOf(ctx context.Context, e Expr, sort Sort, id int64) (int, error) {
	terms, err := sort.terms()
	if err != nil {
		return 0, err
	}
	match, args, err := exprConds(e)
	if err != nil {
		return 0, err
	}
	conds := append([]string{"id = ?"}, match...)
	vals, err := s.termValues(ctx, terms, "SELECT "+selectTerms(terms)+" FROM qso"+where(conds), append([]any{id}, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query qso: %w", err)
	}

	return s.rowOf(ctx, match, args, terms, vals)
}

// termValues runs query, which selects terms' expressions, and returns the
// values of its first row, or an error wrapping sql.ErrNoRows when there is
// none. Text comes back as a string, so that binding it again compares it as
// text.
func (s *SQLiteStore) termValues(ctx context.Context, terms []sortTerm, query string, args []any) ([]any, error) {
	vals := make([]any, len(terms))
	ptrs := make([]any, len(terms))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(ptrs...); err != nil {
		return nil, err
	}
	for i, v := range vals {
		if b, ok := v.([]byte); ok {
			vals[i] = string(b)
		}
	}
	return vals, nil
}

// rowOf returns the row number, in the order of terms, of the QSO whose
// term values are vals among the QSOs matching conds and args, by counting
// the matching QSOs that come before it. It counts each of keysetLevels
// separately, since each count is a range of an index, and adds them up.
func (s *SQLiteStore) rowOf(ctx context.Context, conds []string, args []any, terms []sortTerm, vals []any) (int, error) {
	levels, levelArgs := keysetLevels(terms, vals, true)
	counts := make([]string, len(levels))
	var all []any
	for i, level := range levels {
		counts[i] = "(SELECT COUNT(*) FROM qso" + where(append([]string{level}, conds...)) + ")"
		all = append(append(all, levelArgs[i]...), args...)
	}
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT "+strings.Join(counts, " + "), all...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count qsos: %w", err)
	}
	return n, nil
}

// Search returns the row number, in Index's ordering by sort of the QSOs
// matching within, of the first QSO after row from that also matches match,
// or of the last one before row from when backward is true. From may lie
// outside the rows: -1 searches forward from the first row and a from at or
// past the last row searches backward from the last. It returns ErrNotFound
// when no such QSO exists, an error wrapping ErrInvalidFilter if a Filter in
// within or match is invalid, and one wrapping ErrInvalidSort if sort is. A
// nil within or match matches every QSO.
//
// Only the anchor row, the matching QSO and a count are read, so a caller
// can search a large logbook without loading every ID.
func (s *SQLiteStore) Search(ctx context.Context, within, match Expr, sort Sort, from int, backward bool) (int, error) {
	n, err := s.search(ctx, within, match, sort, from, backward)
	if errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("logbook: search qsos: %w", err)
	}
	return n, nil
}

func (s *SQLiteStore) search(ctx context.Context, within, match Expr, sort Sort, from int, backward bool) (int, error) {
	terms, err := sort.terms()
	if err != nil {
		return 0, err
	}
	withinConds, withinArgs, err := exprConds(within)
	if err != nil {
		return 0, err
	}
	matchConds, matchArgs, err := exprConds(match)
	if err != nil {
		return 0, err
	}
	if backward && from <= 0 {
		return 0, ErrNotFound // Nothing comes before the first row.
	}

	// Resolve the anchor: the QSO at row from under within, if any.
	conds := append(append([]string(nil), withinConds...), matchConds...)
	args := append(append([]any(nil), withinArgs...), matchArgs...)
	if from >= 0 {
		anchor, err := s.termValues(ctx, terms,
			"SELECT "+selectTerms(terms)+" FROM qso"+where(withinConds)+" ORDER BY "+orderBy(terms, false)+" LIMIT 1 OFFSET ?",
			append(append([]any(nil), withinArgs...), from))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if !backward {
				return 0, ErrNotFound // Nothing comes after the last row.
			}
			// Past the end: search backward from the last row.
		case err != nil:
			return 0, fmt.Errorf("query anchor qso: %w", err)
		default:
			cond, condArgs := keysetCond(terms, anchor, backward)
			conds = append(conds, cond)
			args = append(args, condArgs...)
		}
	}

	// Find the nearest QSO in the search direction.
	found, err := s.termValues(ctx, terms,
		"SELECT "+selectTerms(terms)+" FROM qso"+where(conds)+" ORDER BY "+orderBy(terms, backward)+" LIMIT 1",
		args)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query qso: %w", err)
	}
	return s.rowOf(ctx, withinConds, withinArgs, terms, found)
}
