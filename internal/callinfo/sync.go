package callinfo

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"jstlogger/internal/callbook"
	"jstlogger/internal/logbook"
)

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
// returns SyncFailed with the error, which StopsSync classifies.
func (l Looker) Sync(ctx context.Context, s logbook.Store, id int64) (SyncOutcome, error) {
	l.Logbook = nil
	q, err := s.Get(ctx, id)
	if err != nil {
		return SyncFailed, fmt.Errorf("get QSO %d: %w", id, err)
	}
	filled, d, err := l.fill(ctx, q)
	if err != nil {
		return SyncFailed, err
	}
	if d.Source == "" {
		return SyncNotFound, nil
	}
	if !filledChanged(q, filled) {
		return SyncUnchanged, nil
	}
	if err := s.Update(ctx, filled); err != nil {
		return SyncFailed, fmt.Errorf("update QSO %d: %w", id, err)
	}
	return SyncUpdated, nil
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
// ran out of time.
func Unreachable(err error) bool {
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
