package logbook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Tag applies tags to each of the given QSOs in one transaction, creating
// tags as needed. Tag names are trimmed and match existing tags ignoring
// case. If any ID is unknown it returns an error wrapping ErrNotFound and
// changes nothing. No IDs or no tags is a no-op.
func (s *SQLiteStore) Tag(ctx context.Context, qsoIDs []int64, tags ...string) error {
	tags = normalizeTags(tags)
	if len(qsoIDs) == 0 || len(tags) == 0 {
		return nil
	}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		for _, id := range qsoIDs {
			var one int
			err := tx.QueryRowContext(ctx, "SELECT 1 FROM qso WHERE id = ?", id).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("qso %d: %w", id, ErrNotFound)
			}
			if err != nil {
				return fmt.Errorf("look up qso %d: %w", id, err)
			}
			if err := attachTags(ctx, tx, id, tags); err != nil {
				return fmt.Errorf("qso %d: %w", id, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("logbook: tag qsos: %w", err)
	}
	return nil
}

// Untag removes tags, matched ignoring case, from each of the given QSOs in
// one transaction, then deletes tags that no longer label any QSO. Unknown
// IDs and tags are ignored.
func (s *SQLiteStore) Untag(ctx context.Context, qsoIDs []int64, tags ...string) error {
	tags = normalizeTags(tags)
	if len(qsoIDs) == 0 || len(tags) == 0 {
		return nil
	}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		for _, name := range tags {
			for _, id := range qsoIDs {
				// tag.name is COLLATE NOCASE, so this matches ignoring case.
				if _, err := tx.ExecContext(ctx,
					"DELETE FROM qso_tag WHERE qso_id = ? AND tag_id IN (SELECT id FROM tag WHERE name = ?)",
					id, name); err != nil {
					return fmt.Errorf("detach tag %q from qso %d: %w", name, id, err)
				}
			}
		}
		return deleteUnusedTags(ctx, tx)
	})
	if err != nil {
		return fmt.Errorf("logbook: untag qsos: %w", err)
	}
	return nil
}

// Tags returns every tag ordered by name ignoring case, or nil if there are
// none.
func (s *SQLiteStore) Tags(ctx context.Context) (tags []Tag, err error) {
	// tag.name is COLLATE NOCASE, so ORDER BY name ignores case.
	rows, err := s.db.QueryContext(ctx, "SELECT id, name FROM tag ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("logbook: list tags: %w", err)
	}
	defer func() { err = errors.Join(err, closeRows(rows)) }()
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, fmt.Errorf("logbook: list tags: scan tag: %w", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("logbook: list tags: %w", err)
	}
	return tags, nil
}

// deleteUnusedTags deletes every tag that no longer labels a QSO, so Tags
// lists only tags in use.
func deleteUnusedTags(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM tag WHERE NOT EXISTS (SELECT 1 FROM qso_tag WHERE tag_id = tag.id)"); err != nil {
		return fmt.Errorf("delete unused tags: %w", err)
	}
	return nil
}
