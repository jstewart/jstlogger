package callinfo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"jstlogger/internal/callbook"
	"jstlogger/internal/logbook"
)

// PendingTag is the tag given to QSOs saved while the callbook was
// unreachable, so they can be found and synced with it later.
const PendingTag = "callbook-pending"

// SyncOutcome is what syncing a QSO with the callbook did.
type SyncOutcome int

const (
	// SyncUpdated means the callbook filled in empty fields and the QSO
	// was saved.
	SyncUpdated SyncOutcome = iota
	// SyncUnchanged means the callbook found the call but had nothing to
	// add, so the QSO was not saved.
	SyncUnchanged
	// SyncNotFound means the callbook does not list the call, or the call
	// has no base callsign to look up, and the QSO was left untouched.
	SyncNotFound
	// SyncFailed means the QSO could not be loaded, looked up or saved; the
	// error says why.
	SyncFailed
)

// Sync fills in the empty fields of the stored QSO with ID id from the
// callbook, as Fill does, and saves it when that changes it. Values the QSO
// already has are kept. Only the callbook is asked; the logbook is never a
// fallback, so a QSO is not filled from earlier QSOs with the call. A QSO
// whose call the callbook does not find is left untouched, even when its
// DXCC entity is known. A lookup that fails leaves the QSO untouched and
// returns SyncFailed with the error, which StopsSync classifies. A QSO the
// callbook finds, updated or not, loses PendingTag, as it no longer waits
// for the callbook.
func (l Looker) Sync(ctx context.Context, s logbook.Store, id int64) (SyncOutcome, error) {
	l.Logbook = nil
	var q logbook.QSO
	err := l.inLogbook(ctx, "get", id, func(ctx context.Context) (err error) {
		q, err = s.Get(ctx, id)
		return err
	})
	if err != nil {
		return SyncFailed, err
	}
	filled, d, err := l.fill(ctx, q)
	if err != nil {
		return SyncFailed, err
	}
	if d.Source == "" {
		return SyncNotFound, nil
	}
	if !filledChanged(q, filled) {
		if slices.Contains(q.Tags, PendingTag) {
			err := l.inLogbook(ctx, "untag", id, func(ctx context.Context) error {
				return s.Untag(ctx, []int64{id}, PendingTag)
			})
			if err != nil {
				return SyncFailed, err
			}
		}
		return SyncUnchanged, nil
	}
	filled.Tags = slices.DeleteFunc(slices.Clone(filled.Tags), func(t string) bool { return t == PendingTag })
	err = l.inLogbook(ctx, "update", id, func(ctx context.Context) error {
		return s.Update(ctx, filled)
	})
	if err != nil {
		return SyncFailed, err
	}
	return SyncUpdated, nil
}

// LogbookError is a failure to read or write the QSO with ID ID while
// syncing it with the callbook. It is no callbook failure, so Unreachable
// reports false for it even when it ran out of time.
type LogbookError struct {
	Op  string // "get", "untag" or "update"
	ID  int64
	Err error
}

// Error implements error.
func (e *LogbookError) Error() string {
	return fmt.Sprintf("%s QSO %d: %v", e.Op, e.ID, e.Err)
}

// Unwrap returns the logbook's error.
func (e *LogbookError) Unwrap() error { return e.Err }

// inLogbook runs op on the logbook, bounded by LogbookTimeout, and reports
// its failure as a *LogbookError.
func (l Looker) inLogbook(ctx context.Context, name string, id int64, op func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(l.LogbookTimeout, DefaultLogbookTimeout))
	defer cancel()
	if err := op(ctx); err != nil {
		return &LogbookError{Op: name, ID: id, Err: err}
	}
	return nil
}

// filledChanged reports whether Fill changed any field of before to give
// after.
func filledChanged(before, after logbook.QSO) bool {
	return before.DXCC != after.DXCC || before.Cont != after.Cont ||
		before.CQZ != after.CQZ || before.ITUZ != after.ITUZ ||
		before.Country != after.Country || before.Name != after.Name ||
		before.QTH != after.QTH || before.Gridsquare != after.Gridsquare ||
		before.State != after.State || before.Cnty != after.Cnty ||
		!maps.Equal(before.Fields, after.Fields)
}

// Unreachable reports whether err, or any error it wraps or joins, says the
// callbook could not be reached: a *callbook.NetworkError or a lookup that
// ran out of time. A *LogbookError is never one.
func Unreachable(err error) bool {
	if _, ok := errors.AsType[*LogbookError](err); ok {
		return false
	}
	_, network := errors.AsType[*callbook.NetworkError](err)
	return network || errors.Is(err, context.DeadlineExceeded)
}

// StopsSync reports whether err should stop a sync of several QSOs rather
// than move on to the next: the callbook is unreachable, rejected the
// login or has run out of lookups, or the sync was canceled. Each would
// fail every remaining QSO the same way.
func StopsSync(err error) bool {
	return Unreachable(err) ||
		errors.Is(err, callbook.ErrRateLimit) ||
		errors.Is(err, callbook.ErrAuth) ||
		errors.Is(err, context.Canceled)
}
