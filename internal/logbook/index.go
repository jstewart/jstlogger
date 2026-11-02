package logbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Index returns up to limit QSOs matching e, newest first (StartUTC
// descending, then ID descending), starting at row offset, where 0 is the
// newest. Only the qso row is loaded: Fields, Confirmations and Tags are
// nil. A limit of zero or less returns nil, and limits above MaxPageSize are
// capped. It returns an error wrapping ErrInvalidFilter if a Filter in e is
// invalid. A nil e matches every QSO.
//
// Unlike List, Index can start anywhere in the ordering, so a caller can
// show any range of a large logbook without reading the rows before it.
func (s *SQLiteStore) Index(ctx context.Context, e Expr, offset, limit int) ([]QSO, error) {
	qs, err := s.index(ctx, e, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("logbook: index qsos: %w", err)
	}
	return qs, nil
}

func (s *SQLiteStore) index(ctx context.Context, e Expr, offset, limit int) (qs []QSO, err error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative offset %d", offset)
	}
	if limit <= 0 {
		return nil, nil
	}
	query, args, err := indexQuery(e, offset, min(limit, MaxPageSize))
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
// matches the qso_by_start index, so SQLite skips offset rows of the index
// instead of sorting the table.
func indexQuery(e Expr, offset, limit int) (string, []any, error) {
	conds, args, err := exprConds(e)
	if err != nil {
		return "", nil, err
	}
	query := "SELECT " + qsoColumns + " FROM qso" + where(conds) +
		" ORDER BY start_utc DESC, id DESC LIMIT ? OFFSET ?"
	return query, append(args, limit, offset), nil
}

// IndexOf returns the row number of the QSO with the given ID among the QSOs
// matching e, in the order Index uses, so that Index(ctx, e, n, 1) returns
// it. It returns ErrNotFound if the QSO does not exist or does not match e,
// and an error wrapping ErrInvalidFilter if a Filter in e is invalid. A nil e
// matches every QSO.
func (s *SQLiteStore) IndexOf(ctx context.Context, e Expr, id int64) (int, error) {
	n, err := s.indexOf(ctx, e, id)
	if errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("logbook: index of qso %d: %w", id, err)
	}
	return n, nil
}

func (s *SQLiteStore) indexOf(ctx context.Context, e Expr, id int64) (int, error) {
	match, args, err := exprConds(e)
	if err != nil {
		return 0, err
	}
	conds := append([]string{"id = ?"}, match...)
	var start string
	err = s.db.QueryRowContext(ctx, "SELECT start_utc FROM qso"+where(conds), append([]any{id}, args...)...).Scan(&start)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query qso: %w", err)
	}

	return s.rowOf(ctx, match, args, start, id)
}

// Conditions that place a QSO strictly before or after the QSO with a given
// start_utc and id in newest-first order (StartUTC descending, then ID
// descending). Each takes the start time twice, then the ID, as arguments.
const (
	beforeCond = "(start_utc > ? OR (start_utc = ? AND id > ?))"
	afterCond  = "(start_utc < ? OR (start_utc = ? AND id < ?))"
)

// rowOf returns the row number, in newest-first order, of the QSO with the
// given start time and ID among the QSOs matching conds and args, by
// counting the matching QSOs that come before it.
func (s *SQLiteStore) rowOf(ctx context.Context, conds []string, args []any, start string, id int64) (int, error) {
	conds = append([]string{beforeCond}, conds...)
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM qso"+where(conds), append([]any{start, start, id}, args...)...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count qsos: %w", err)
	}
	return n, nil
}

// Search returns the row number, in Index's ordering of the QSOs matching
// within, of the first QSO after row from that also matches match, or of the
// last one before row from when backward is true. From may lie outside the
// rows: -1 searches forward from the first row and a from at or past the
// last row searches backward from the last. It returns ErrNotFound when no
// such QSO exists, and an error wrapping ErrInvalidFilter if a Filter in
// within or match is invalid. A nil within or match matches every QSO.
//
// Only the anchor row, the matching QSO and a count are read, so a caller
// can search a large logbook without loading every ID.
func (s *SQLiteStore) Search(ctx context.Context, within, match Expr, from int, backward bool) (int, error) {
	n, err := s.search(ctx, within, match, from, backward)
	if errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("logbook: search qsos: %w", err)
	}
	return n, nil
}

func (s *SQLiteStore) search(ctx context.Context, within, match Expr, from int, backward bool) (int, error) {
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
		var start string
		var id int64
		err := s.db.QueryRowContext(ctx,
			"SELECT start_utc, id FROM qso"+where(withinConds)+" ORDER BY start_utc DESC, id DESC LIMIT 1 OFFSET ?",
			append(append([]any(nil), withinArgs...), from)...).Scan(&start, &id)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if !backward {
				return 0, ErrNotFound // Nothing comes after the last row.
			}
			// Past the end: search backward from the last row.
		case err != nil:
			return 0, fmt.Errorf("query anchor qso: %w", err)
		default:
			cond := afterCond
			if backward {
				cond = beforeCond
			}
			conds = append(conds, cond)
			args = append(args, start, start, id)
		}
	}

	// Find the nearest QSO in the search direction.
	order := " ORDER BY start_utc DESC, id DESC LIMIT 1"
	if backward {
		order = " ORDER BY start_utc ASC, id ASC LIMIT 1"
	}
	var start string
	var id int64
	err = s.db.QueryRowContext(ctx, "SELECT start_utc, id FROM qso"+where(conds)+order, args...).Scan(&start, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query qso: %w", err)
	}
	return s.rowOf(ctx, withinConds, withinArgs, start, id)
}
