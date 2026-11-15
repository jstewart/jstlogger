package callbook

import (
	"context"
	"errors"
	"fmt"
)

// Chain is a Looker that tries each of its Lookers in order and returns
// the first success.
//
// When every Looker fails, Lookup returns an error that wraps ErrNotFound
// if they all reported ErrNotFound, and otherwise the errors.Join of all
// their errors, so errors.Is and errors.As match each one. An empty Chain
// returns ErrNotFound. If ctx is done before a Looker is tried, Lookup
// returns ctx.Err(), wrapped.
type Chain []Looker

// Lookup implements Looker.
func (c Chain) Lookup(ctx context.Context, call string) (Result, error) {
	errs := make([]error, 0, len(c))
	allNotFound := true
	for _, l := range c {
		if err := ctx.Err(); err != nil {
			return Result{}, fmt.Errorf("callbook: lookup %s: %w", call, err)
		}
		r, err := l.Lookup(ctx, call)
		if err == nil {
			return r, nil
		}
		errs = append(errs, err)
		if !errors.Is(err, ErrNotFound) {
			allNotFound = false
		}
	}
	if allNotFound {
		return Result{}, ErrNotFound
	}
	return Result{}, errors.Join(errs...)
}
