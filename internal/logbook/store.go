package logbook

import "context"

// Store persists QSOs, their confirmations and tags.
//
//go:generate go tool mockgen -source=store.go -destination=mock/store.go -package=mock
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
	// Index returns up to limit QSOs matching f, newest first, starting at
	// row offset (0 is the newest), without their fields, confirmations or
	// tags.
	Index(ctx context.Context, f Filter, offset, limit int) ([]QSO, error)
	// IndexOf returns the row number of the QSO with the given ID in Index's
	// ordering of the QSOs matching f, or ErrNotFound.
	IndexOf(ctx context.Context, f Filter, id int64) (int, error)
	// Tag applies tags to each of the given QSOs, creating tags as needed.
	Tag(ctx context.Context, qsoIDs []int64, tags ...string) error
	// Untag removes tags from each of the given QSOs.
	Untag(ctx context.Context, qsoIDs []int64, tags ...string) error
	// Tags returns every tag.
	Tags(ctx context.Context) ([]Tag, error)
	// Import stores qsos in a single transaction, skipping duplicates, and
	// applies tags to each QSO it stores (in addition to the QSO's own
	// Tags). Duplicates are not updated and are not tagged. If any QSO
	// fails, nothing is stored.
	Import(ctx context.Context, qsos []QSO, tags []string) (ImportResult, error)
	// Each calls fn for every QSO matching f, oldest first (StartUTC
	// ascending, then ID ascending), with fields, confirmations and tags
	// loaded. It stops and returns fn's error if fn fails.
	Each(ctx context.Context, f Filter, fn func(QSO) error) error
}
