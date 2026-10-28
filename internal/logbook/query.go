package logbook

import (
	"context"
	"errors"
)

// errNotImplemented is returned by the query methods until they are
// implemented.
var errNotImplemented = errors.New("logbook: not implemented")

// List returns one page of QSOs matching f, newest first.
func (s *SQLiteStore) List(ctx context.Context, f Filter, page PageRequest) (Page, error) {
	return Page{}, errNotImplemented
}

// Count returns the number of QSOs matching f.
func (s *SQLiteStore) Count(ctx context.Context, f Filter) (int, error) {
	return 0, errNotImplemented
}

// Tag applies tags to each of the given QSOs, creating tags as needed.
func (s *SQLiteStore) Tag(ctx context.Context, qsoIDs []int64, tags ...string) error {
	return errNotImplemented
}

// Untag removes tags from each of the given QSOs.
func (s *SQLiteStore) Untag(ctx context.Context, qsoIDs []int64, tags ...string) error {
	return errNotImplemented
}

// Tags returns every tag.
func (s *SQLiteStore) Tags(ctx context.Context) ([]Tag, error) {
	return nil, errNotImplemented
}
