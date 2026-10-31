package logbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Index returns up to limit QSOs matching f, newest first (StartUTC
// descending, then ID descending), starting at row offset, where 0 is the
// newest. Only the qso row is loaded: Fields, Confirmations and Tags are
// nil. A limit of zero or less returns nil, and limits above MaxPageSize are
// capped. It returns an error wrapping ErrInvalidFilter if f is invalid.
//
// Unlike List, Index can start anywhere in the ordering, so a caller can
// show any range of a large logbook without reading the rows before it.
func (s *SQLiteStore) Index(ctx context.Context, f Filter, offset, limit int) ([]QSO, error) {
	qs, err := s.index(ctx, f, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("logbook: index qsos: %w", err)
	}
	return qs, nil
}

func (s *SQLiteStore) index(ctx context.Context, f Filter, offset, limit int) (qs []QSO, err error) {
	if offset < 0 {
		return nil, fmt.Errorf("negative offset %d", offset)
	}
	if limit <= 0 {
		return nil, nil
	}
	query, args, err := indexQuery(f, offset, min(limit, MaxPageSize))
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
func indexQuery(f Filter, offset, limit int) (string, []any, error) {
	clause, args, err := filterSQL(f)
	if err != nil {
		return "", nil, err
	}
	var conds []string
	if clause != "" {
		conds = append(conds, clause)
	}
	query := "SELECT " + qsoColumns + " FROM qso" + where(conds) +
		" ORDER BY start_utc DESC, id DESC LIMIT ? OFFSET ?"
	return query, append(args, limit, offset), nil
}

// IndexOf returns the row number of the QSO with the given ID among the QSOs
// matching f, in the order Index uses, so that Index(ctx, f, n, 1) returns
// it. It returns ErrNotFound if the QSO does not exist or does not match f,
// and an error wrapping ErrInvalidFilter if f is invalid.
func (s *SQLiteStore) IndexOf(ctx context.Context, f Filter, id int64) (int, error) {
	n, err := s.indexOf(ctx, f, id)
	if errors.Is(err, ErrNotFound) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("logbook: index of qso %d: %w", id, err)
	}
	return n, nil
}

func (s *SQLiteStore) indexOf(ctx context.Context, f Filter, id int64) (int, error) {
	clause, args, err := filterSQL(f)
	if err != nil {
		return 0, err
	}
	conds := []string{"id = ?"}
	if clause != "" {
		conds = append(conds, clause)
	}
	var start string
	err = s.db.QueryRowContext(ctx, "SELECT start_utc FROM qso"+where(conds), append([]any{id}, args...)...).Scan(&start)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query qso: %w", err)
	}

	// Count the matching QSOs that come before it in newest-first order.
	conds = []string{"(start_utc > ? OR (start_utc = ? AND id > ?))"}
	if clause != "" {
		conds = append(conds, clause)
	}
	var n int
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM qso"+where(conds), append([]any{start, start, id}, args...)...).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count qsos: %w", err)
	}
	return n, nil
}
