package logbook

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidFilter is returned (possibly wrapped) when a Filter cannot be
// applied.
var ErrInvalidFilter = errors.New("logbook: invalid filter")

const (
	// DefaultPageSize is the page size List uses when PageRequest.Limit is
	// zero or negative.
	DefaultPageSize = 50
	// MaxPageSize is the largest page List returns; larger limits are
	// capped to it.
	MaxPageSize = 500
)

// List returns one page of QSOs matching f, newest first (StartUTC
// descending, then ID descending), with their fields, confirmations and
// tags. A limit of zero or less means DefaultPageSize, and limits above
// MaxPageSize are capped. When nothing matches it returns the zero Page,
// whose QSOs are nil. It returns an error wrapping ErrInvalidFilter if f is
// invalid.
func (s *SQLiteStore) List(ctx context.Context, f Filter, page PageRequest) (Page, error) {
	p, err := s.list(ctx, f, page)
	if err != nil {
		return Page{}, fmt.Errorf("logbook: list qsos: %w", err)
	}
	return p, nil
}

func (s *SQLiteStore) list(ctx context.Context, f Filter, page PageRequest) (p Page, err error) {
	clause, args, err := filterSQL(f)
	if err != nil {
		return Page{}, err
	}
	var conds []string
	if clause != "" {
		conds = append(conds, clause)
	}
	if !page.After.IsZero() {
		after := formatTime(page.After.StartUTC)
		conds = append(conds, "(start_utc < ? OR (start_utc = ? AND id < ?))")
		args = append(args, after, after, page.After.ID)
	}
	limit := page.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limit = min(limit, MaxPageSize)
	// One extra row tells us whether another page follows.
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx,
		"SELECT "+qsoColumns+" FROM qso"+where(conds)+" ORDER BY start_utc DESC, id DESC LIMIT ?",
		args...)
	if err != nil {
		return Page{}, fmt.Errorf("query qsos: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		q, err := scanQSO(rows)
		if err != nil {
			return Page{}, err
		}
		p.QSOs = append(p.QSOs, q)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("query qsos: %w", err)
	}

	if len(p.QSOs) > limit {
		p.QSOs = p.QSOs[:limit]
		p.HasMore = true
		last := p.QSOs[limit-1]
		p.Next = Cursor{StartUTC: last.StartUTC, ID: last.ID}
	}
	if err := s.loadChildren(ctx, p.QSOs); err != nil {
		return Page{}, err
	}
	return p, nil
}

// Count returns the number of QSOs matching e, or an error wrapping
// ErrInvalidFilter if a Filter in e is invalid. A nil e matches every QSO.
func (s *SQLiteStore) Count(ctx context.Context, e Expr) (int, error) {
	conds, args, err := exprConds(e)
	if err != nil {
		return 0, fmt.Errorf("logbook: count qsos: %w", err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM qso"+where(conds), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("logbook: count qsos: %w", err)
	}
	return n, nil
}

// IDs returns the IDs of every QSO matching e, newest first (StartUTC
// descending, then ID descending, as Index orders them), or nil when none
// match. It returns an error wrapping ErrInvalidFilter if a Filter in e is
// invalid. A nil e matches every QSO.
func (s *SQLiteStore) IDs(ctx context.Context, e Expr) ([]int64, error) {
	out, err := s.ids(ctx, e)
	if err != nil {
		return nil, fmt.Errorf("logbook: list qso ids: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) ids(ctx context.Context, e Expr) (out []int64, err error) {
	conds, args, err := exprConds(e)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id FROM qso"+where(conds)+" ORDER BY start_utc DESC, id DESC", args...)
	if err != nil {
		return nil, fmt.Errorf("query qso ids: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan qso id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query qso ids: %w", err)
	}
	return out, nil
}

// where returns " WHERE " and conds AND'd together, or "" if there are none.
func where(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

// filterSQL translates f into a boolean SQL expression over the qso table
// and its arguments, or "" when f places no constraint. Values only ever
// travel as arguments; the expression holds column names and placeholders.
func filterSQL(f Filter) (string, []any, error) {
	if err := validateFilter(f); err != nil {
		return "", nil, err
	}
	var (
		conds []string
		args  []any
	)
	add := func(cond string, a ...any) {
		conds = append(conds, cond)
		args = append(args, a...)
	}
	in := func(expr string, vals []any) {
		if len(vals) > 0 {
			add(expr+" IN ("+placeholders(len(vals))+")", vals...)
		}
	}

	if !f.From.IsZero() {
		add("start_utc >= ?", formatTime(f.From))
	}
	if !f.To.IsZero() {
		add("start_utc < ?", formatTime(f.To))
	}
	in("dxcc", toArgs(f.DXCC, func(v int) any { return v }))
	in("itu_region", toArgs(f.ITURegions, func(v int) any { return v }))
	// Mode, band and continent are normalized on write, so normalize the
	// filter values the same way and keep the column indexes usable. Country
	// and state are stored as given, so compare ignoring case.
	in("mode", toArgs(f.Modes, func(v string) any { return upper(v) }))
	in("band", toArgs(f.Bands, func(v string) any { return strings.ToLower(strings.TrimSpace(v)) }))
	in("country COLLATE NOCASE", toArgs(f.Countries, func(v string) any { return strings.TrimSpace(v) }))
	in("cont", toArgs(f.Continents, func(v string) any { return upper(v) }))
	in("state COLLATE NOCASE", toArgs(f.States, func(v string) any { return strings.TrimSpace(v) }))
	for _, tag := range normalizeTags(f.Tags) {
		// tag.name is COLLATE NOCASE, so this matches ignoring case.
		add(`EXISTS (SELECT 1 FROM qso_tag qt JOIN tag t ON t.id = qt.tag_id
			WHERE qt.qso_id = qso.id AND t.name = ?)`, tag)
	}
	// A comparison with NULL is never true, so either bound excludes QSOs
	// without a distance.
	if f.MinDistanceKm.Valid {
		add("distance_km >= ?", f.MinDistanceKm.V)
	}
	if f.MaxDistanceKm.Valid {
		add("distance_km <= ?", f.MaxDistanceKm.V)
	}
	if f.Confirmed != ConfirmedAny {
		sub := "EXISTS (SELECT 1 FROM qso_confirmation c WHERE c.qso_id = qso.id AND c.rcvd = 'Y'"
		var subArgs []any
		if f.ConfirmedService != "" {
			sub += " AND c.service = ?"
			subArgs = append(subArgs, normalizeService(f.ConfirmedService))
		}
		sub += ")"
		if f.Confirmed == ConfirmedNo {
			sub = "NOT " + sub
		}
		add(sub, subArgs...)
	}
	return strings.Join(conds, " AND "), args, nil
}

// validateFilter returns an error wrapping ErrInvalidFilter if f has an
// unknown Confirmed value or a ConfirmedService without a Confirmed status.
func validateFilter(f Filter) error {
	switch f.Confirmed {
	case ConfirmedAny:
		if f.ConfirmedService != "" {
			return fmt.Errorf("confirmed service %q needs a confirmed status: %w", f.ConfirmedService, ErrInvalidFilter)
		}
	case ConfirmedYes, ConfirmedNo:
	default:
		return fmt.Errorf("unknown confirmed status %d: %w", f.Confirmed, ErrInvalidFilter)
	}
	return nil
}

// toArgs converts vals to query arguments with conv.
func toArgs[T any](vals []T, conv func(T) any) []any {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = conv(v)
	}
	return out
}
