package logbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

// ImportResult reports the outcome of Store.Import.
type ImportResult struct {
	// Added is the number of QSOs stored.
	Added int
	// Skipped is the number of duplicate QSOs that were not stored.
	Skipped int
}

// eachBatchSize is how many QSOs Each reads per query; tests lower it.
var eachBatchSize = 500

// naturalKey identifies a QSO for duplicate detection when UUIDs differ.
// Its values are normalized by prepare, and start is formatTime text, so it
// compares to the second.
type naturalKey struct {
	call, start, band, mode, stationCallsign string
}

// keyOf returns the natural key of q, which prepare has normalized.
func keyOf(q QSO) naturalKey {
	return naturalKey{
		call:            q.Call,
		start:           formatTime(q.StartUTC),
		band:            q.Band,
		mode:            q.Mode,
		stationCallsign: q.StationCallsign,
	}
}

// Import stores qsos in a single transaction, skipping duplicates, and
// applies tags to each QSO it stores in addition to the QSO's own Tags.
// Duplicates are not updated and are not tagged. If any QSO fails, nothing
// is stored.
//
// A QSO is a duplicate when its UUID is set and matches a stored QSO or an
// earlier QSO in qsos, or when its call, start time (to the second), band,
// mode and station callsign, after normalization, match a stored QSO or an
// earlier QSO in qsos.
func (s *SQLiteStore) Import(ctx context.Context, qsos []QSO, tags []string) (ImportResult, error) {
	prepared := make([]QSO, len(qsos))
	for i, q := range qsos {
		// Clone so that appending never writes into the caller's array.
		q.Tags = append(slices.Clone(q.Tags), tags...)
		p, err := prepare(q, s.opts.StationGrid)
		if err != nil {
			return ImportResult{}, fmt.Errorf("logbook: import qso %d: %w", i+1, err)
		}
		prepared[i] = p
	}

	var res ImportResult
	now := s.now()
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		seenUUIDs := make(map[string]bool)
		seenKeys := make(map[naturalKey]bool)
		for i, q := range prepared {
			key := keyOf(q)
			dup := seenKeys[key] || (q.UUID != "" && seenUUIDs[q.UUID])
			if !dup {
				var err error
				if dup, err = storedDuplicate(ctx, tx, q.UUID, key); err != nil {
					return fmt.Errorf("qso %d: %w", i+1, err)
				}
			}
			seenKeys[key] = true
			if q.UUID != "" {
				seenUUIDs[q.UUID] = true
			}
			if dup {
				res.Skipped++
				continue
			}

			if q.UUID == "" {
				q.UUID = newUUID()
			}
			q.CreatedAt, q.UpdatedAt = now, now
			if _, err := insertQSO(ctx, tx, q); err != nil {
				return fmt.Errorf("qso %d: %w", i+1, err)
			}
			res.Added++
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, fmt.Errorf("logbook: import: %w", err)
	}
	return res, nil
}

// storedDuplicate reports whether a stored QSO has the given UUID, when it
// is not empty, or the given natural key.
func storedDuplicate(ctx context.Context, tx *sql.Tx, uuid string, key naturalKey) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM qso WHERE (? <> '' AND uuid = ?)
		OR (call = ? AND start_utc = ? AND band = ? AND mode = ? AND station_callsign = ?)
		LIMIT 1`,
		uuid, uuid, key.call, key.start, key.band, key.mode, key.stationCallsign).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("look up duplicate: %w", err)
	}
	return true, nil
}

// Each calls fn for every QSO matching f, oldest first (StartUTC ascending,
// then ID ascending), with fields, confirmations and tags loaded. It reads
// the QSOs in batches and holds no query open while fn runs, so fn may use
// the store. It stops and returns fn's error if fn fails, and returns an
// error wrapping ErrInvalidFilter if f is invalid.
func (s *SQLiteStore) Each(ctx context.Context, f Filter, fn func(QSO) error) error {
	clause, args, err := filterSQL(f)
	if err != nil {
		return fmt.Errorf("logbook: each qso: %w", err)
	}
	var after Cursor // the zero Cursor starts from the oldest QSO
	for {
		batch, err := s.eachBatch(ctx, clause, args, after)
		if err != nil {
			return fmt.Errorf("logbook: each qso: %w", err)
		}
		for _, q := range batch {
			if err := fn(q); err != nil {
				return err
			}
		}
		if len(batch) < eachBatchSize {
			return nil
		}
		last := batch[len(batch)-1]
		after = Cursor{StartUTC: last.StartUTC, ID: last.ID}
	}
}

// eachBatch returns up to eachBatchSize QSOs matching clause and args, oldest
// first, that follow after (or from the oldest when after is zero), with
// their children loaded. after is a position in the oldest-first ordering.
func (s *SQLiteStore) eachBatch(ctx context.Context, clause string, args []any, after Cursor) (qs []QSO, err error) {
	var conds []string
	if clause != "" {
		conds = append(conds, clause)
	}
	// Copy args so appending never shares the caller's array across batches.
	args = slices.Clone(args)
	if !after.IsZero() {
		start := formatTime(after.StartUTC)
		conds = append(conds, "(start_utc > ? OR (start_utc = ? AND id > ?))")
		args = append(args, start, start, after.ID)
	}
	args = append(args, eachBatchSize)

	rows, err := s.db.QueryContext(ctx,
		"SELECT "+qsoColumns+" FROM qso"+where(conds)+" ORDER BY start_utc, id LIMIT ?",
		args...)
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
	if err := s.loadChildren(ctx, qs); err != nil {
		return nil, err
	}
	return qs, nil
}
