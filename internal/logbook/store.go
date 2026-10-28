package logbook

import "context"

// Store persists QSOs, their confirmations and tags.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock/store.go -package=mock jstlogger/internal/logbook Store
type Store interface {
	// Add stores a new QSO and returns it with its ID, UUID and derived
	// fields filled in.
	Add(ctx context.Context, q QSO) (QSO, error)
	// Get returns the QSO with the given ID, or ErrNotFound.
	Get(ctx context.Context, id int64) (QSO, error)
	// Update replaces the stored QSO with q.ID, or returns ErrNotFound.
	Update(ctx context.Context, q QSO) error
	// Delete removes the QSO with the given ID, or returns ErrNotFound.
	Delete(ctx context.Context, id int64) error
	// List returns one page of QSOs matching f, newest first.
	List(ctx context.Context, f Filter, page PageRequest) (Page, error)
	// Count returns the number of QSOs matching f.
	Count(ctx context.Context, f Filter) (int, error)
	// Tag applies tags to each of the given QSOs, creating tags as needed.
	Tag(ctx context.Context, qsoIDs []int64, tags ...string) error
	// Untag removes tags from each of the given QSOs.
	Untag(ctx context.Context, qsoIDs []int64, tags ...string) error
	// Tags returns every tag.
	Tags(ctx context.Context) ([]Tag, error)
}
